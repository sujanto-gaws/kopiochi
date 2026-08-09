package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/sujanto-gaws/kopiochi/internal/db"
	domain "github.com/sujanto-gaws/kopiochi/modules/notification/domain"
	"github.com/sujanto-gaws/kopiochi/modules/notification/infrastructure/persistence/models"
)

// PreferenceRepo stores the sparse per-user delivery preferences in the
// notification_preferences table.
type PreferenceRepo struct {
	db bun.IDB
}

// NewPreferenceRepo returns a repository over idb.
func NewPreferenceRepo(idb bun.IDB) *PreferenceRepo {
	return &PreferenceRepo{db: idb}
}

// ListForUser returns every stored preference for userID.
//
// A user who has never changed anything yields an empty slice and a nil error,
// not an error and not nil: absence is the default, and the domain's Allowed
// reads an empty slice as "everything enabled". The slice is allocated even when
// empty so a caller ranging over it cannot tell the two apart by accident.
//
// Ordered by (channel, category) so a caller — or a test — gets a stable
// sequence; the table has at most one row per pair and nine pairs exist today,
// so the sort is free.
func (r *PreferenceRepo) ListForUser(ctx context.Context, userID uuid.UUID) ([]domain.Preference, error) {
	var rows []models.NotificationPreferenceRow
	if err := r.db.NewSelect().
		Model(&rows).
		Where("user_id = ?", userID).
		OrderExpr("channel, category").
		Scan(ctx); err != nil {
		return nil, db.Translate(err)
	}

	prefs := make([]domain.Preference, 0, len(rows))
	for _, row := range rows {
		prefs = append(prefs, domain.Preference{
			UserID:    row.UserID,
			Channel:   domain.Channel(row.Channel),
			Category:  domain.Category(row.Category),
			Enabled:   row.Enabled,
			UpdatedAt: row.UpdatedAt,
		})
	}
	return prefs, nil
}

// Upsert writes prefs, replacing any existing row for the same
// (user, channel, category).
//
// One multi-row INSERT ... ON CONFLICT DO UPDATE, so the whole slice lands or
// none of it does: a user who flips four toggles in one request must not end up
// with two applied. That atomicity is the statement's, not a transaction's —
// which is what lets this work unchanged when the caller hands in a bun.Tx.
//
// The conflict target is the primary key, so a pair that already exists is
// updated rather than rejected. enabled and updated_at are taken from EXCLUDED
// — the row the caller supplied — rather than recomputed here, so the timestamp
// is the application's clock and not the database's, matching every other time
// in this module.
//
// The same pair twice in one call is a Postgres error ("ON CONFLICT DO UPDATE
// command cannot affect row a second time") and is left as one: it has no
// single correct outcome, and the application layer already refuses it before
// it gets here.
//
// An empty slice is a no-op and not a round trip. bun cannot build an INSERT
// with no rows, and "write nothing" has an obvious answer.
func (r *PreferenceRepo) Upsert(ctx context.Context, prefs []domain.Preference) error {
	if len(prefs) == 0 {
		return nil
	}

	rows := make([]models.NotificationPreferenceRow, 0, len(prefs))
	for _, p := range prefs {
		rows = append(rows, models.NotificationPreferenceRow{
			UserID:    p.UserID,
			Channel:   string(p.Channel),
			Category:  string(p.Category),
			Enabled:   p.Enabled,
			UpdatedAt: p.UpdatedAt,
		})
	}

	_, err := r.db.NewInsert().
		Model(&rows).
		On("CONFLICT (user_id, channel, category) DO UPDATE").
		Set("enabled = EXCLUDED.enabled").
		Set("updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	return db.Translate(err)
}

// Compile-time proof the repository still satisfies its port.
var _ domain.PreferenceRepository = (*PreferenceRepo)(nil)
