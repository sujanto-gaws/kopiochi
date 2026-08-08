package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ListFilter narrows a recipient's in-app mailbox.
//
// Pagination is by cursor rather than offset: the list is ordered by CreatedAt
// descending and grows at the head, so an offset shifts under the reader
// between pages and duplicates rows. Before is that cursor — the CreatedAt of
// the last row the caller has seen — and nil means the first page.
type ListFilter struct {
	UnreadOnly bool
	Limit      int
	Before     *time.Time
}

// NotificationRepository is the outbox. Implemented in infrastructure; declared
// here because the dispatch use case is defined in terms of it.
type NotificationRepository interface {
	// Enqueue inserts n.
	//
	// When n carries an idempotency key that is already present it inserts
	// nothing and returns ErrDuplicateIdempotencyKey. The conflict is resolved
	// in the insert itself rather than by a preceding SELECT, which would race
	// with a concurrent enqueue of the same business event — the case the key
	// exists to handle.
	Enqueue(ctx context.Context, n *Notification) error

	// ClaimBatch atomically takes up to n rows that are pending with
	// NextAttemptAt at or before now, marks them sending, and returns them.
	//
	// Atomic and exclusive: two dispatchers, or two workers in one dispatcher,
	// must never receive the same row. Returning the rows already in sending is
	// what makes that true — a claim that only selected would leave a window in
	// which a second caller selects the same ids.
	//
	// now is a parameter rather than the database's clock so the caller's
	// injected clock governs the whole dispatch cycle, including the tests.
	ClaimBatch(ctx context.Context, n int, now time.Time) ([]*Notification, error)

	// Save persists the settled state of a claimed row: status, attempts,
	// next attempt, last error and sent-at. It does not create rows.
	Save(ctx context.Context, n *Notification) error

	// ListForRecipient returns recipientID's in-app notifications, newest
	// first, narrowed by f.
	//
	// Scoped by recipient in the query rather than filtered afterwards: an
	// ownership check a caller can forget is an ownership check that will be
	// forgotten.
	ListForRecipient(ctx context.Context, recipientID uuid.UUID, f ListFilter) ([]*Notification, error)

	// MarkRead stamps ReadAt on one of recipientID's notifications, and is a
	// no-op on one already read. It returns ErrNotFound when the id does not
	// exist or belongs to someone else — the two are deliberately
	// indistinguishable, so the endpoint cannot be used to probe for ids.
	MarkRead(ctx context.Context, recipientID, id uuid.UUID, now time.Time) error

	// MarkAllRead stamps ReadAt on every unread in-app notification of
	// recipientID and reports how many rows it changed.
	MarkAllRead(ctx context.Context, recipientID uuid.UUID, now time.Time) (int, error)
}

// PreferenceRepository stores the sparse preference rows. Absent rows are not
// an error: see Allowed for what absence means.
type PreferenceRepository interface {
	// ListForUser returns every stored preference for userID. A user who has
	// never changed anything yields an empty slice and a nil error.
	ListForUser(ctx context.Context, userID uuid.UUID) ([]Preference, error)

	// Upsert writes prefs, replacing any existing row for the same
	// (user, channel, category). The whole slice is written together so that a
	// user's update of several toggles cannot land half-applied.
	Upsert(ctx context.Context, prefs []Preference) error
}
