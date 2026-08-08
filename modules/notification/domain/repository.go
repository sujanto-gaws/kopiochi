package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Cursor is a position in a recipient's mailbox: the sort key of the last row a
// caller has already seen.
//
// It is a pair and not just a timestamp because CreatedAt is not unique. A
// fan-out writes several notifications in one transaction, and Postgres stamps
// them from the same now() — identical to the microsecond. Against a single
// column there is no correct predicate for "the rows after this one": created_at
// < $1 drops every sibling that shares the boundary timestamp, and <= repeats
// them on the next page. ID breaks the tie, so the pair is total.
//
// ID carries no ordering meaning of its own. It is a v4 uuid and its order is
// arbitrary; it is here only to make the composite key unique, and it is only
// ever compared between rows whose CreatedAt is equal.
type Cursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

// ListFilter narrows a recipient's in-app mailbox.
//
// Pagination is by keyset and not by offset: the list is ordered newest first
// and grows at the head, so OFFSET shifts under a reader between pages and
// makes rows repeat or vanish depending on what was inserted meanwhile. It also
// costs the database a scan of everything skipped, which gets worse the deeper
// a caller pages.
type ListFilter struct {
	UnreadOnly bool

	// Limit is the maximum number of rows to return. Zero or negative means
	// DefaultListLimit and anything above MaxListLimit is capped to it; see
	// Normalize, which ListForRecipient applies.
	Limit int

	// Before is the keyset seek position: only rows that sort strictly after it
	// are returned. nil means the first page.
	//
	// The ordering is (created_at DESC, id DESC), so the predicate an
	// implementation must produce is the row-value comparison
	//
	//	WHERE (created_at, id) < ($1, $2)
	//	ORDER BY created_at DESC, id DESC
	//	LIMIT $3
	//
	// with $1 = Before.CreatedAt and $2 = Before.ID. It must be the row-value
	// form and not created_at < $1 OR (created_at = $1 AND id < $2): the two are
	// logically equal, but only the row-value comparison is a range predicate
	// the planner can seek with, which is the entire point of the shape.
	//
	// Descending matches idx_notifications_recipient — (recipient_id,
	// created_at DESC) WHERE channel = 'inapp' — so the seek starts inside the
	// index rather than sorting the recipient's whole mailbox. id is the leaf
	// tiebreak and is not in that index; it only ever discriminates within one
	// timestamp, so the handful of rows sharing it are the only ones fetched to
	// resolve it.
	//
	// Not an offset. An implementation that translates this into OFFSET has
	// reintroduced exactly the bug the pair exists to prevent.
	Before *Cursor
}

// The bounds on ListFilter.Limit.
//
// They are here rather than in transport because the interface contract is what
// D4 implements against: a repository that trusts an unbounded Limit will
// eventually be handed one. Twenty fills a notification panel without a caller
// having to page immediately. The ceiling of a hundred bounds the response,
// which matters more here than in most lists because every row carries a Payload
// of arbitrary jsonb — the row count is the only thing anyone can actually
// bound.
const (
	DefaultListLimit = 20
	MaxListLimit     = 100
)

// Normalize returns f with Limit forced into [1, MaxListLimit]: a non-positive
// value becomes DefaultListLimit, and anything larger than MaxListLimit is
// capped to it. Every other field is copied through untouched.
//
// A non-positive Limit means "unspecified" and not "no rows". Reading zero as an
// empty page would turn the zero value of ListFilter — which is what a caller
// that only wants defaults constructs — into a list endpoint that returns
// nothing.
//
// It takes a value receiver and returns a copy, so it cannot surprise a caller
// by rewriting the filter it passed in, and calling it twice gives the same
// answer as calling it once.
func (f ListFilter) Normalize() ListFilter {
	if f.Limit <= 0 {
		f.Limit = DefaultListLimit
	}
	if f.Limit > MaxListLimit {
		f.Limit = MaxListLimit
	}
	return f
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

	// ListForRecipient returns recipientID's in-app notifications, ordered
	// (created_at DESC, id DESC), narrowed by f.
	//
	// Scoped by recipient in the query rather than filtered afterwards: an
	// ownership check a caller can forget is an ownership check that will be
	// forgotten.
	//
	// Implementations apply f.Normalize() rather than trusting f.Limit, for the
	// same reason: the bound on how much of the table one request can read is
	// not something a caller should be able to omit.
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
