// Package repository implements the notification module's persistence ports
// over bun.
//
// It is the only package that sees both the domain entities and the row structs
// in ../models, and the translation between them lives here: the domain speaks
// in strings and zero values, the table speaks in nullable columns, and neither
// has to know about the other.
//
// Errors leave this package as either a domain sentinel — ErrNotFound,
// ErrDuplicateIdempotencyKey — or an internal/db one, via db.Translate. A raw
// *pgconn.PgError never escapes, so no layer above has to know Postgres error
// codes.
package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/sujanto-gaws/kopiochi/internal/db"
	domain "github.com/sujanto-gaws/kopiochi/modules/notification/domain"
	"github.com/sujanto-gaws/kopiochi/modules/notification/infrastructure/persistence/models"
)

// NotificationRepo is the outbox, stored in the notifications table.
type NotificationRepo struct {
	db bun.IDB
}

// NewNotificationRepo returns a repository over idb.
//
// idb is a bun.IDB and not a *bun.DB so a caller can hand it a transaction:
// an enqueue belongs in the business transaction that caused it, which is the
// whole point of an outbox.
func NewNotificationRepo(idb bun.IDB) *NotificationRepo {
	return &NotificationRepo{db: idb}
}

// Enqueue inserts n, and reports a duplicate idempotency key as
// domain.ErrDuplicateIdempotencyKey without inserting anything.
//
// The conflict is resolved by the insert itself rather than by a preceding
// SELECT, which would race with the concurrent enqueue of the same business
// event that the key exists to handle.
//
// The conflict target restates the index predicate:
//
//	ON CONFLICT (idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING
//
// idx_notifications_idem is a *partial* unique index, and a bare
// "ON CONFLICT (idempotency_key)" does not compile against one — Postgres
// answers "there is no unique or exclusion constraint matching the ON CONFLICT
// specification", because arbiter inference only considers an index whose
// predicate the statement restates. The WHERE goes before DO, not after it.
//
// The predicate is also what makes an unkeyed enqueue work: a row with a NULL
// key is not in the index, so it is never a candidate for arbitration and is
// always inserted. That is why the domain carries IdempotencyKey as a string
// with "" meaning "no key" — a *string in the domain would admit a pointer to
// "", which the index would treat as a real key, and the second unkeyed enqueue
// would then collide with the first.
func (r *NotificationRepo) Enqueue(ctx context.Context, n *domain.Notification) error {
	if n == nil {
		return errors.New("notification: cannot enqueue nil")
	}

	res, err := r.db.NewInsert().
		Model(toNotificationRow(n)).
		On("CONFLICT (idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING").
		Exec(ctx)
	if err != nil {
		return db.Translate(err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if affected == 0 {
		return domain.ErrDuplicateIdempotencyKey
	}
	return nil
}

// ClaimBatch takes up to n pending rows whose NextAttemptAt has passed, marks
// them sending, and returns them — in one statement:
//
//	UPDATE notifications AS n SET status = 'sending'
//	 WHERE id IN (SELECT id FROM notifications
//	               WHERE status = 'pending' AND next_attempt_at <= $1
//	               ORDER BY next_attempt_at
//	               LIMIT $2
//	               FOR UPDATE SKIP LOCKED)
//	RETURNING *
//
// One statement is the whole design. Selecting the ids and then updating them
// leaves a window in which a second dispatcher selects the same ids, and no
// amount of care in the second statement closes it; here the row lock taken by
// the subquery is held by the same statement that writes the new status, so a
// claimed row is already sending before any other session can look at it.
//
// SKIP LOCKED, not NOWAIT and not plain FOR UPDATE: a second dispatcher must
// step over rows the first is holding and take the next ones, rather than block
// behind them or fail. The LIMIT sits *above* the lock in the plan, so rows
// skipped that way do not consume it — a second dispatcher still gets a full
// batch instead of n minus whatever the first holds.
//
// now is bound as a parameter rather than written as SQL now(). The application
// layer injects a Clock and its tests fix it; emitting the database's clock here
// would quietly bypass that and make the dispatch cycle untestable at exact
// values. Binding a parameter does not change the plan — idx_notifications_claim
// is still an index scan on next_attempt_at with no sort.
//
// The ORDER BY decides *which* rows the statement takes — the oldest due ones,
// so a row that has been waiting does not starve behind a stream of fresh
// arrivals. It does not order the result: an UPDATE ... RETURNING result set has
// no defined order in Postgres, and the port promises none either. It would not
// be a useful promise anyway, since the ORDER BY has no tiebreaker and rows
// sharing a next_attempt_at are arbitrarily ordered by design.
//
// A batch size of zero or less claims nothing, without a round trip: LIMIT 0
// would be a query that can only return nothing, and a negative LIMIT is an
// error.
func (r *NotificationRepo) ClaimBatch(ctx context.Context, n int, now time.Time) ([]*domain.Notification, error) {
	if n <= 0 {
		return nil, nil
	}

	// Unaliased on purpose. The outer UPDATE carries the model's "n" alias, and
	// giving the subquery the same one would shadow it; leaving this one bare
	// means every column below resolves to this FROM and nothing is ambiguous.
	claimable := r.db.NewSelect().
		ColumnExpr("id").
		TableExpr("notifications").
		Where("status = ?", string(domain.StatusPending)).
		Where("next_attempt_at <= ?", now).
		OrderExpr("next_attempt_at").
		Limit(n).
		For("UPDATE SKIP LOCKED")

	var rows []models.NotificationRow
	if _, err := r.db.NewUpdate().
		Model((*models.NotificationRow)(nil)).
		Set("status = ?", string(domain.StatusSending)).
		Set("claimed_at = ?", now).
		Where("id IN (?)", claimable).
		Returning("*").
		Exec(ctx, &rows); err != nil {
		return nil, db.Translate(err)
	}

	claimed := make([]*domain.Notification, 0, len(rows))
	for i := range rows {
		claimed = append(claimed, fromNotificationRow(&rows[i]))
	}
	return claimed, nil
}

// ClaimStalled takes ownership of rows that were claimed for delivery and never
// settled, so the caller can run each one back through the domain's
// RecoverStalled and Save it.
//
// It deliberately does NOT change status, and that is the whole point. E9c ruled
// that recovering with one set-based UPDATE would be a second, untested copy of
// the state machine; the transition stays in the domain, and this decides only
// WHO may apply it.
//
// Exclusion matters more here than for an ordinary claim. RecoverStalled
// increments Attempts, so two dispatchers recovering the same row burn two
// attempts and can dead-letter a row that was merely slow. FOR UPDATE SKIP
// LOCKED hands the rows to exactly one sweeper.
//
// Re-stamping claimed_at is what makes a crashed sweeper safe: a row whose
// recovery never lands is deferred by another full stall window rather than
// being immediately eligible again. It costs the original claim timestamp,
// which nothing reads after the sweep has decided.
//
// stalledBefore is the policy — "claimed before this instant counts as
// stalled" — and stays with the caller, so this method has no opinion about how
// long a send may take.
func (r *NotificationRepo) ClaimStalled(
	ctx context.Context, n int, stalledBefore, now time.Time,
) ([]*domain.Notification, error) {
	if n <= 0 {
		return nil, nil
	}

	// Unaliased for the same reason as ClaimBatch's subquery above.
	stalled := r.db.NewSelect().
		ColumnExpr("id").
		TableExpr("notifications").
		Where("status = ?", string(domain.StatusSending)).
		Where("claimed_at IS NOT NULL").
		Where("claimed_at < ?", stalledBefore).
		OrderExpr("claimed_at").
		Limit(n).
		For("UPDATE SKIP LOCKED")

	var rows []models.NotificationRow
	if _, err := r.db.NewUpdate().
		Model((*models.NotificationRow)(nil)).
		Set("claimed_at = ?", now).
		Where("id IN (?)", stalled).
		Returning("*").
		Exec(ctx, &rows); err != nil {
		return nil, db.Translate(err)
	}

	out := make([]*domain.Notification, 0, len(rows))
	for i := range rows {
		out = append(out, fromNotificationRow(&rows[i]))
	}
	return out, nil
}

// Save writes back the settled state of a claimed row and nothing else.
//
// The five columns are exactly the ones the delivery outcome owns. Updating the
// whole row instead would let a stale copy of payload, recipient or channel —
// read minutes earlier, before a retry — overwrite the current one, and would
// make Save able to rewrite a row's identity, which is not what settling means.
//
// A row that no longer exists is domain.ErrNotFound rather than a silent
// success: Save does not create rows, so zero rows affected means the row the
// dispatcher claimed is gone, and the caller records that against the batch.
func (r *NotificationRepo) Save(ctx context.Context, n *domain.Notification) error {
	if n == nil {
		return errors.New("notification: cannot save nil")
	}

	res, err := r.db.NewUpdate().
		Model((*models.NotificationRow)(nil)).
		Set("status = ?", string(n.Status)).
		Set("attempts = ?", n.Attempts).
		Set("next_attempt_at = ?", n.NextAttemptAt).
		Set("last_error = ?", nullString(n.LastError)).
		Set("sent_at = ?", n.SentAt).
		Where("id = ?", n.ID).
		Exec(ctx)
	if err != nil {
		return db.Translate(err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if affected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// ListForRecipient returns one page of recipientID's in-app mailbox, newest
// first.
//
// The recipient predicate is in the query, not applied to the result. An
// ownership check a caller can forget is an ownership check that will be
// forgotten, and this is the read behind a user-facing endpoint.
//
// channel = 'inapp' is both the contract — the other channels leave the system
// and have no mailbox — and what makes idx_notifications_recipient
// ((recipient_id, created_at DESC) WHERE channel = 'inapp') usable: a partial
// index is only chosen when the query restates its predicate.
//
// Pagination is a keyset seek written as a row-value comparison:
//
//	WHERE (created_at, id) < ($1, $2)
//
// and deliberately not the OR-expansion created_at < $1 OR (created_at = $1 AND
// id < $2). The two are logically identical and only the row-value form is a
// range predicate the planner can seek with. It is not an OFFSET either: the
// list is ordered newest first and grows at the head, so an offset shifts under
// a reader between pages and makes rows repeat or vanish.
//
// f.Normalize() is applied here rather than trusted from the caller, so the
// bound on how much of the table one request can read is not something a caller
// can omit.
func (r *NotificationRepo) ListForRecipient(
	ctx context.Context, recipientID uuid.UUID, f domain.ListFilter,
) ([]*domain.Notification, error) {
	f = f.Normalize()

	var rows []models.NotificationRow
	q := r.db.NewSelect().
		Model(&rows).
		Where("recipient_id = ?", recipientID).
		Where("channel = ?", string(domain.ChannelInApp)).
		OrderExpr("created_at DESC, id DESC").
		Limit(f.Limit)

	if f.UnreadOnly {
		q = q.Where("read_at IS NULL")
	}
	if f.Before != nil {
		q = q.Where("(created_at, id) < (?, ?)", f.Before.CreatedAt, f.Before.ID)
	}

	if err := q.Scan(ctx); err != nil {
		return nil, db.Translate(err)
	}

	out := make([]*domain.Notification, 0, len(rows))
	for i := range rows {
		out = append(out, fromNotificationRow(&rows[i]))
	}
	return out, nil
}

// MarkRead stamps ReadAt on one of recipientID's notifications.
//
// It returns domain.ErrNotFound when the id does not exist or belongs to
// somebody else. The two are indistinguishable on purpose: telling a caller
// that a notification exists but is not theirs turns the endpoint into a probe
// for ids.
//
// The recipient predicate is in the UPDATE, so there is no window between
// checking ownership and writing, and no caller can skip the check.
//
// read_at = coalesce(read_at, $3) rather than a plain assignment under a
// "read_at IS NULL" filter, because the two clauses of the contract pull in
// opposite directions and this is the single statement that satisfies both.
// Marking an already-read notification read again is a no-op and not an error —
// a client that retries the request must not get a 404 — while a wrong or
// foreign id must be one. Filtering on read_at IS NULL would collapse the first
// case into the second; coalesce keeps the original timestamp (a second read
// does not move it) and leaves "no rows matched" meaning exactly "not yours or
// not there".
//
// Scoped by channel as well, for the same reason as ListForRecipient and
// MarkAllRead: read_at is a mailbox concept, and the mailbox is in-app. Without
// this predicate the three queries disagreed about which rows they own — an
// email or webhook row could be marked read individually here, while never
// being returned by ListForRecipient and never touched by MarkAllRead (E30).
//
// This narrows what the endpoint accepts, and does so in the safe direction: a
// non-in-app id now returns ErrNotFound, which is the answer a foreign id and a
// missing id already get, so the three stay indistinguishable and the route
// cannot be used to probe for ids of any channel.
func (r *NotificationRepo) MarkRead(ctx context.Context, recipientID, id uuid.UUID, now time.Time) error {
	res, err := r.db.NewUpdate().
		Model((*models.NotificationRow)(nil)).
		Set("read_at = coalesce(read_at, ?)", now).
		Where("id = ?", id).
		Where("recipient_id = ?", recipientID).
		Where("channel = ?", string(domain.ChannelInApp)).
		Exec(ctx)
	if err != nil {
		return db.Translate(err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if affected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// MarkAllRead stamps ReadAt on every unread in-app notification of recipientID
// and reports how many rows it changed.
//
// read_at IS NULL is in the predicate here, unlike MarkRead: the count is the
// answer, and including rows that were already read would report work that did
// not happen. Scoped by recipient and channel in the query, for the same reasons
// as ListForRecipient.
func (r *NotificationRepo) MarkAllRead(ctx context.Context, recipientID uuid.UUID, now time.Time) (int, error) {
	res, err := r.db.NewUpdate().
		Model((*models.NotificationRow)(nil)).
		Set("read_at = ?", now).
		Where("recipient_id = ?", recipientID).
		Where("channel = ?", string(domain.ChannelInApp)).
		Where("read_at IS NULL").
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

// toNotificationRow maps the entity onto the storage shape.
func toNotificationRow(n *domain.Notification) *models.NotificationRow {
	payload := n.Payload
	if payload == nil {
		// The column is NOT NULL. The domain constructor already substitutes an
		// empty map, but a hand-built entity does not go through it.
		payload = map[string]any{}
	}

	return &models.NotificationRow{
		ID:             n.ID,
		RecipientID:    n.RecipientID,
		Channel:        string(n.Channel),
		Category:       string(n.Category),
		TemplateKey:    n.TemplateKey,
		Payload:        payload,
		Status:         string(n.Status),
		Attempts:       n.Attempts,
		NextAttemptAt:  n.NextAttemptAt,
		IdempotencyKey: nullString(n.IdempotencyKey),
		ReadAt:         n.ReadAt,
		LastError:      nullString(n.LastError),
		CreatedAt:      n.CreatedAt,
		SentAt:         n.SentAt,
	}
}

// fromNotificationRow maps a stored row back onto the entity.
func fromNotificationRow(row *models.NotificationRow) *domain.Notification {
	payload := row.Payload
	if payload == nil {
		payload = map[string]any{}
	}

	return &domain.Notification{
		ID:             row.ID,
		RecipientID:    row.RecipientID,
		Channel:        domain.Channel(row.Channel),
		Category:       domain.Category(row.Category),
		TemplateKey:    row.TemplateKey,
		Payload:        payload,
		Status:         domain.Status(row.Status),
		Attempts:       row.Attempts,
		NextAttemptAt:  row.NextAttemptAt,
		IdempotencyKey: stringValue(row.IdempotencyKey),
		ReadAt:         row.ReadAt,
		LastError:      stringValue(row.LastError),
		CreatedAt:      row.CreatedAt,
		SentAt:         row.SentAt,
	}
}

// nullString maps the domain's "" to SQL NULL.
//
// Both nullable text columns this module writes — idempotency_key and
// last_error — are typed as string in the domain, where "" is the single
// representation of "absent". For idempotency_key that is load-bearing:
// idx_notifications_idem indexes only non-NULL keys, so storing "" instead
// would make every unkeyed enqueue collide with the previous one.
func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// stringValue is nullString's inverse: SQL NULL reads back as "".
func stringValue(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// Compile-time proof the repository still satisfies the port it exists for.
// Without it a signature drift surfaces at the composition root, far from the
// code that caused it.
var _ domain.NotificationRepository = (*NotificationRepo)(nil)
