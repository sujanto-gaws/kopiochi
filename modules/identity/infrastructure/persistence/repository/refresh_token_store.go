package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/sujanto-gaws/kopiochi/internal/db"
	domain "github.com/sujanto-gaws/kopiochi/modules/identity/domain"
	"github.com/sujanto-gaws/kopiochi/modules/identity/infrastructure/persistence/models"
)

type RefreshTokenStore struct {
	db bun.IDB
}

func NewRefreshTokenStore(idb bun.IDB) *RefreshTokenStore {
	return &RefreshTokenStore{db: idb}
}

func (s *RefreshTokenStore) Store(ctx context.Context, token domain.RefreshToken) error {
	row, err := toRefreshRow(token)
	if err != nil {
		return err
	}
	_, err = s.db.NewInsert().Model(row).Exec(ctx)
	return db.Translate(err)
}

// FindValid returns a token only if it is usable right now.
//
// The filters live in SQL rather than in Go so `now()` is the *database's*
// clock. Comparing against the application's clock would make expiry depend on
// how far each replica's time had drifted.
func (s *RefreshTokenStore) FindValid(ctx context.Context, tokenHash string) (*domain.RefreshToken, error) {
	row := new(models.RefreshTokenRow)
	err := s.db.NewSelect().
		Model(row).
		Where("token_hash = ?", tokenHash).
		Where("revoked = false").
		Where("used_at IS NULL").
		Where("expires_at > now()").
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("invalid token")
		}
		return nil, db.Translate(err)
	}
	return fromRefreshRow(row), nil
}

// FindAny returns the token whatever its state, so the caller can distinguish
// "no such token" from "a real token that has already been spent".
//
// FindValid cannot make that distinction — it filters both into the same
// error — and the difference is exactly what reuse detection turns on: one is
// a credential that never existed, the other is evidence that a real one was
// captured.
func (s *RefreshTokenStore) FindAny(ctx context.Context, tokenHash string) (*domain.RefreshToken, error) {
	row := new(models.RefreshTokenRow)
	err := s.db.NewSelect().
		Model(row).
		Where("token_hash = ?", tokenHash).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, db.ErrNotFound
		}
		return nil, db.Translate(err)
	}
	return fromRefreshRow(row), nil
}

// Rotate retires oldHash and issues next inside one transaction.
//
// The UPDATE repeats the predicates FindValid used, and the rowsAffected check
// is what makes this correct under concurrency: two simultaneous refreshes
// presenting the same token both pass the earlier read, but only one UPDATE
// matches a still-unused row. The loser gets ErrRefreshTokenAlreadyUsed
// instead of both being issued children of one parent.
func (s *RefreshTokenStore) Rotate(ctx context.Context, oldHash string, next domain.RefreshToken) error {
	nextRow, err := toRefreshRow(next)
	if err != nil {
		return err
	}

	return db.InTx(ctx, s.db, func(ctx context.Context, tx bun.Tx) error {
		res, err := tx.NewUpdate().
			Model((*models.RefreshTokenRow)(nil)).
			Set("used_at = now()").
			Set("revoked = true").
			Where("token_hash = ?", oldHash).
			Where("revoked = false").
			Where("used_at IS NULL").
			Exec(ctx)
		if err != nil {
			return db.Translate(err)
		}

		affected, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("rows affected: %w", err)
		}
		if affected == 0 {
			return domain.ErrRefreshTokenAlreadyUsed
		}

		if _, err := tx.NewInsert().Model(nextRow).Exec(ctx); err != nil {
			return db.Translate(err)
		}
		return nil
	})
}

// RevokeFamily invalidates every still-live token descending from one login
// and records that it was a reuse detection rather than a logout.
//
// The returned count is how many were still live, which is the number worth
// putting in the audit event: "revoked 4 tokens" and "revoked 0 tokens" mean
// very different things about how long the theft went unnoticed.
func (s *RefreshTokenStore) RevokeFamily(ctx context.Context, familyID string) (int, error) {
	fid, err := uuid.Parse(familyID)
	if err != nil {
		return 0, fmt.Errorf("parse family id: %w", err)
	}

	res, err := s.db.NewUpdate().
		Model((*models.RefreshTokenRow)(nil)).
		Set("revoked = true").
		Set("reuse_detected_at = now()").
		Where("family_id = ?", fid).
		Where("revoked = false").
		Exec(ctx)
	if err != nil {
		return 0, db.Translate(err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected: %w", err)
	}
	return int(affected), nil
}

func (s *RefreshTokenStore) RevokeAllForUser(ctx context.Context, userID string) error {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return fmt.Errorf("parse user id: %w", err)
	}
	_, err = s.db.NewUpdate().
		Model((*models.RefreshTokenRow)(nil)).
		Set("revoked = true").
		Where("user_id = ?", uid).
		Exec(ctx)
	return db.Translate(err)
}

func toRefreshRow(t domain.RefreshToken) (*models.RefreshTokenRow, error) {
	uid, err := uuid.Parse(t.UserID)
	if err != nil {
		return nil, fmt.Errorf("parse user id: %w", err)
	}

	// An empty FamilyID means "this token starts a new family" — a fresh
	// login. It is generated here rather than left to the column default so
	// the value is knowable to the caller that just stored it.
	familyID := uuid.New()
	if t.FamilyID != "" {
		familyID, err = uuid.Parse(t.FamilyID)
		if err != nil {
			return nil, fmt.Errorf("parse family id: %w", err)
		}
	}

	return &models.RefreshTokenRow{
		UserID:    uid,
		TokenHash: t.TokenHash,
		ExpiresAt: t.ExpiresAt,
		FamilyID:  familyID,
	}, nil
}

func fromRefreshRow(row *models.RefreshTokenRow) *domain.RefreshToken {
	return &domain.RefreshToken{
		UserID:    row.UserID.String(),
		TokenHash: row.TokenHash,
		ExpiresAt: row.ExpiresAt,
		FamilyID:  row.FamilyID.String(),
		Used:      row.UsedAt != nil,
		Revoked:   row.Revoked,
	}
}

// Compile-time proof the store still satisfies the domain port. Without it a
// signature drift surfaces as a confusing error at the composition root rather
// than here, next to the code that caused it.
var _ domain.RefreshTokenStore = (*RefreshTokenStore)(nil)
