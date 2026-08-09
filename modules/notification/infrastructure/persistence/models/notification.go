// Package models holds the notification module's bun row structs.
//
// These are the storage shape, not the domain shape: repositories translate
// between the two so the domain never depends on the ORM.
//
// The columns here mirror migrations/20260808170025_create_notifications.sql
// exactly — tools/schemacheck is what keeps that claim honest. Nullable
// columns are modelled as pointers so "absent" and "zero" stay distinguishable;
// mapping a nullable column to a plain value would silently turn a NULL into
// an empty string or a zero time.
package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// NotificationRow maps to the "notifications" table — the outbox row that is
// written inside the business transaction and claimed later by the dispatcher.
type NotificationRow struct {
	bun.BaseModel `bun:"table:notifications,alias:n"`

	ID          uuid.UUID `bun:"id,pk,type:uuid,default:gen_random_uuid()"`
	RecipientID uuid.UUID `bun:"recipient_id,notnull,type:uuid"`
	Channel     string    `bun:"channel,notnull"`
	Category    string    `bun:"category,notnull"`
	TemplateKey string    `bun:"template_key,notnull"`

	// Payload is template data only, never rendered content. jsonb, so the
	// column can be queried without a round trip through the application.
	Payload map[string]any `bun:"payload,notnull,type:jsonb,nullzero"`

	// Status is constrained by notifications_status_chk to
	// pending|sending|sent|failed|dead; the domain owns the transitions.
	Status string `bun:"status,notnull,default:'pending'"`

	Attempts int `bun:"attempts,notnull,default:0"`

	// NextAttemptAt is half of the dispatcher's claim predicate and is
	// covered by the partial index idx_notifications_claim.
	NextAttemptAt time.Time `bun:"next_attempt_at,notnull,default:now()"`

	// IdempotencyKey is nullable and unique when present
	// (idx_notifications_idem is a partial unique index), which is what makes
	// a duplicate enqueue an ON CONFLICT DO NOTHING no-op.
	IdempotencyKey *string `bun:"idempotency_key"`

	// ReadAt is in-app only and stays NULL until the recipient reads the row.
	ReadAt *time.Time `bun:"read_at"`

	// LastError is truncated operator detail from the most recent failed
	// send; NULL means no attempt has failed yet.
	LastError *string `bun:"last_error"`

	CreatedAt time.Time `bun:"created_at,notnull,default:now()"`

	// SentAt stays NULL until the row reaches the sent status.
	SentAt *time.Time `bun:"sent_at"`
}

// NotificationPreferenceRow maps to the "notification_preferences" table. Its
// primary key is the (user_id, channel, category) triple; an absent row means
// the default, so only explicit choices are stored.
type NotificationPreferenceRow struct {
	bun.BaseModel `bun:"table:notification_preferences,alias:np"`

	UserID   uuid.UUID `bun:"user_id,pk,type:uuid"`
	Channel  string    `bun:"channel,pk"`
	Category string    `bun:"category,pk"`

	// Enabled deliberately carries no `default:` tag, even though the column
	// has DEFAULT true.
	//
	// bun substitutes the SQL DEFAULT keyword for any zero-valued field that
	// carries a default tag (InsertQuery.marshalsToDefault). On a bool that
	// makes false unrepresentable: an insert of Enabled=false emits DEFAULT,
	// Postgres resolves it to true, and every attempt to switch a notification
	// off silently stores it as on. Without the tag bun writes the value, and
	// the column default still applies to any statement that omits the column.
	Enabled bool `bun:"enabled,notnull"`

	UpdatedAt time.Time `bun:"updated_at,notnull,default:now()"`
}
