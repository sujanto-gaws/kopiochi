package repository

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"

	"github.com/sujanto-gaws/kopiochi/internal/testsupport"
	domain "github.com/sujanto-gaws/kopiochi/modules/notification/domain"
)

// Integration tests against a real Postgres. Not sqlmock: every claim in this
// package is about what the *server* does — partial-index arbitration for
// ON CONFLICT, row-value comparison for the keyset seek, and FOR UPDATE SKIP
// LOCKED — and a mocked driver would only assert that bun produced the string
// the test expected (docs/architectures/06-quality/testing-strategy.md).
//
// They skip cleanly when no database is reachable. See
// testsupport.ScratchPostgres for the precedence: TEST_DATABASE_URL, then a
// disposable docker container, then skip. That means a green `go test ./...` on
// a laptop with no Postgres proves nothing here; CI, which sets
// TEST_DATABASE_URL, is the arbiter.

// newRepo returns a notification repository over a migrated, empty database,
// plus the handle, for the tests that need to reach past the repository.
func newRepo(t *testing.T) (*NotificationRepo, *bun.DB) {
	t.Helper()

	bunDB := testsupport.MigratedDB(t)
	testsupport.TruncateAll(t, bunDB)
	return NewNotificationRepo(bunDB), bunDB
}

// newNotification builds a valid pending entity. Everything a test wants to
// vary it varies afterwards, on the returned value.
func newNotification(t *testing.T, recipient uuid.UUID, now time.Time) *domain.Notification {
	t.Helper()

	n, err := domain.NewNotification(domain.NewNotificationParams{
		ID:          uuid.New(),
		RecipientID: recipient,
		Channel:     domain.ChannelInApp,
		Category:    domain.CategoryAccount,
		TemplateKey: "account.welcome",
		Payload:     map[string]any{"name": "Alice"},
	}, now)
	require.NoError(t, err)
	return n
}

// pgTime rounds t to what a timestamptz column can actually store. Postgres
// keeps microseconds and Go keeps nanoseconds, so a round-trip equality
// assertion on an untruncated value fails on the last three digits.
func pgTime(t time.Time) time.Time { return t.UTC().Truncate(time.Microsecond) }

func TestNotificationRepo_EnqueueRoundTrip(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()

	recipient := uuid.New()
	now := pgTime(time.Now())
	n := newNotification(t, recipient, now)
	n.IdempotencyKey = "welcome:" + recipient.String()

	require.NoError(t, repo.Enqueue(ctx, n))

	got, err := repo.ListForRecipient(ctx, recipient, domain.ListFilter{})
	require.NoError(t, err)
	require.Len(t, got, 1)

	require.Equal(t, n.ID, got[0].ID)
	require.Equal(t, recipient, got[0].RecipientID)
	require.Equal(t, domain.ChannelInApp, got[0].Channel)
	require.Equal(t, domain.CategoryAccount, got[0].Category)
	require.Equal(t, "account.welcome", got[0].TemplateKey)
	require.Equal(t, map[string]any{"name": "Alice"}, got[0].Payload)
	require.Equal(t, domain.StatusPending, got[0].Status)
	require.Equal(t, 0, got[0].Attempts)
	require.Equal(t, now, pgTime(got[0].NextAttemptAt))
	require.Equal(t, now, pgTime(got[0].CreatedAt))
	require.Equal(t, "welcome:"+recipient.String(), got[0].IdempotencyKey)
	require.Nil(t, got[0].ReadAt)
	require.Equal(t, "", got[0].LastError, "a NULL last_error must read back as the empty string")
	require.Nil(t, got[0].SentAt)
}

// TestNotificationRepo_DuplicateIdempotencyKeyIsASQLNoOp is the idempotency
// half of D4's acceptance. The second enqueue must insert nothing — decided by
// the INSERT itself, not by a preceding SELECT that would race with the
// concurrent duplicate the key exists to absorb — and must report
// ErrDuplicateIdempotencyKey, which the application layer swallows as success.
//
// The row is left exactly as the first enqueue wrote it: DO NOTHING, not DO
// UPDATE. A replayed message must not be able to rewrite a notification that is
// already queued, or already sent.
func TestNotificationRepo_DuplicateIdempotencyKeyIsASQLNoOp(t *testing.T) {
	repo, bunDB := newRepo(t)
	ctx := context.Background()

	recipient := uuid.New()
	now := pgTime(time.Now())

	first := newNotification(t, recipient, now)
	first.IdempotencyKey = "pwchange:1"
	require.NoError(t, repo.Enqueue(ctx, first))

	// A different row in every respect except the key.
	second := newNotification(t, recipient, now)
	second.IdempotencyKey = "pwchange:1"
	second.TemplateKey = "security.password_changed"
	second.Category = domain.CategorySecurity

	err := repo.Enqueue(ctx, second)
	require.ErrorIs(t, err, domain.ErrDuplicateIdempotencyKey)

	count, err := bunDB.NewSelect().TableExpr("notifications").Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count, "the duplicate enqueue inserted a row")

	stored, err := repo.ListForRecipient(ctx, recipient, domain.ListFilter{})
	require.NoError(t, err)
	require.Len(t, stored, 1)
	require.Equal(t, first.ID, stored[0].ID, "DO NOTHING must leave the first row in place")
	require.Equal(t, "account.welcome", stored[0].TemplateKey, "the duplicate overwrote the stored row")
}

// TestNotificationRepo_UnkeyedEnqueuesDoNotCollide is the other half of the
// partial-index contract, and the reason the domain carries IdempotencyKey as a
// string rather than a *string.
//
// idx_notifications_idem indexes only rows WHERE idempotency_key IS NOT NULL. A
// repository that stored "" instead of NULL would put every unkeyed row into
// that unique index, and the second notification with no key at all would be
// rejected as a duplicate of the first — silently dropping mail nobody asked to
// deduplicate.
//
// Both directions are asserted: both rows persist, and both read back as "" and
// not as some rendering of a nil pointer.
func TestNotificationRepo_UnkeyedEnqueuesDoNotCollide(t *testing.T) {
	repo, bunDB := newRepo(t)
	ctx := context.Background()

	recipient := uuid.New()
	now := pgTime(time.Now())

	first := newNotification(t, recipient, now)
	second := newNotification(t, recipient, now.Add(time.Millisecond))
	require.Equal(t, "", first.IdempotencyKey)
	require.Equal(t, "", second.IdempotencyKey)

	require.NoError(t, repo.Enqueue(ctx, first))
	require.NoError(t, repo.Enqueue(ctx, second), "the second unkeyed enqueue was rejected as a duplicate")

	// Stored as SQL NULL, which is what keeps them out of the partial index.
	var nulls int
	require.NoError(t, bunDB.NewSelect().
		TableExpr("notifications").
		ColumnExpr("count(*)").
		Where("idempotency_key IS NULL").
		Scan(ctx, &nulls))
	require.Equal(t, 2, nulls, `an empty idempotency key was stored as text instead of NULL`)

	got, err := repo.ListForRecipient(ctx, recipient, domain.ListFilter{})
	require.NoError(t, err)
	require.Len(t, got, 2)
	for _, n := range got {
		require.Equal(t, "", n.IdempotencyKey, "NULL must read back as the empty string")
	}
}

// TestNotificationRepo_ClaimBatchTakesTheOldestDueRows covers the ordinary
// path: only pending rows whose next_attempt_at has passed are claimed, the
// batch limit picks the oldest of them, and every returned row is already
// marked sending.
//
// The assertion is on the set that was claimed, not on the sequence it came
// back in. ORDER BY next_attempt_at decides *which* rows the statement takes;
// the order of an UPDATE ... RETURNING result set is unspecified in Postgres,
// and the port promises none. (Nor would ordering be assertable in general —
// the ORDER BY has no tiebreaker, so rows sharing a timestamp are ordered
// arbitrarily by design. The timestamps here are distinct so that "the two
// oldest" is a single well-defined answer.)
func TestNotificationRepo_ClaimBatchTakesTheOldestDueRows(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()

	recipient := uuid.New()
	now := pgTime(time.Now())

	oldest := newNotification(t, recipient, now.Add(-3*time.Minute))
	middle := newNotification(t, recipient, now.Add(-2*time.Minute))
	newest := newNotification(t, recipient, now.Add(-1*time.Minute))
	future := newNotification(t, recipient, now.Add(time.Hour))
	for _, n := range []*domain.Notification{newest, oldest, future, middle} {
		require.NoError(t, repo.Enqueue(ctx, n))
	}

	claimed, err := repo.ClaimBatch(ctx, 2, now)
	require.NoError(t, err)
	require.Len(t, claimed, 2, "the limit was not applied")
	require.ElementsMatch(t, []uuid.UUID{oldest.ID, middle.ID},
		[]uuid.UUID{claimed[0].ID, claimed[1].ID},
		"the batch limit did not take the two oldest due rows")
	for _, n := range claimed {
		require.Equal(t, domain.StatusSending, n.Status, "a claimed row was returned still pending")
	}

	// The rest of the batch, then nothing — the future row is not due.
	claimed, err = repo.ClaimBatch(ctx, 10, now)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.Equal(t, newest.ID, claimed[0].ID)

	claimed, err = repo.ClaimBatch(ctx, 10, now)
	require.NoError(t, err)
	require.Empty(t, claimed, "a row scheduled in the future was claimed")
}

// TestNotificationRepo_ClaimBatchUsesTheCallersClock is the test that fails if
// the claim predicate is written as SQL now() instead of a bound parameter.
//
// The application layer injects a Clock and its tests fix it; a repository that
// reads the database's clock silently bypasses that, and the fixed-clock tests
// upstream then describe behaviour this code does not have.
//
// The first half is the one that matters: a row that is due by the wall clock
// must NOT be claimed when the caller's now is earlier than its next_attempt_at.
// Only a bound parameter can produce that answer.
func TestNotificationRepo_ClaimBatchUsesTheCallersClock(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()

	wallClock := pgTime(time.Now())
	n := newNotification(t, uuid.New(), wallClock.Add(-time.Minute))
	require.NoError(t, repo.Enqueue(ctx, n))

	claimed, err := repo.ClaimBatch(ctx, 10, wallClock.Add(-time.Hour))
	require.NoError(t, err)
	require.Empty(t, claimed, "the claim predicate ignored the caller's clock and used the database's")

	claimed, err = repo.ClaimBatch(ctx, 10, wallClock)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.Equal(t, n.ID, claimed[0].ID)
}

func TestNotificationRepo_ClaimBatchOfNonPositiveSizeClaimsNothing(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()

	now := pgTime(time.Now())
	require.NoError(t, repo.Enqueue(ctx, newNotification(t, uuid.New(), now)))

	for _, size := range []int{0, -1} {
		claimed, err := repo.ClaimBatch(ctx, size, now)
		require.NoError(t, err, "batch size %d", size)
		require.Empty(t, claimed, "batch size %d claimed rows", size)
	}
}

// TestNotificationRepo_ClaimBatchIsExclusiveUnderConcurrency is D4's mandatory
// acceptance test: two goroutines claiming from one seeded set must never
// receive the same row.
//
// What makes it mean anything:
//
//   - Separate connections. The pool is widened and the widening asserted; with
//     MaxOpenConns(1) the two goroutines would serialise behind one connection
//     and the test would pass for a reason that has nothing to do with SKIP
//     LOCKED.
//   - Many small claims rather than one big one, so the two sessions actually
//     interleave inside each other's statements.
//   - No ordering assertion. ORDER BY next_attempt_at has no tiebreaker and
//     SKIP LOCKED reorders what each session sees; the guarantee under test is
//     exclusivity, which is what is asserted.
//
// A goroutine can legitimately see an empty batch while rows remain — every row
// still due may be locked by the other session at that instant — so the drain
// is finished sequentially afterwards and the accounting covers all three sets.
//
// Run under -race, per the plan.
func TestNotificationRepo_ClaimBatchIsExclusiveUnderConcurrency(t *testing.T) {
	repo, bunDB := newRepo(t)
	ctx := context.Background()

	// Two claimers need two connections. Asserted rather than assumed: a pool
	// of one turns this into a sequential test that cannot fail.
	bunDB.DB.SetMaxOpenConns(4)
	bunDB.DB.SetMaxIdleConns(4)
	require.GreaterOrEqual(t, bunDB.DB.Stats().MaxOpenConnections, 2,
		"the pool cannot hand out two connections; the claimers would serialise and prove nothing")

	const seeded = 400
	now := pgTime(time.Now())
	recipient := uuid.New()
	for i := 0; i < seeded; i++ {
		n := newNotification(t, recipient, now.Add(-time.Duration(i)*time.Millisecond))
		require.NoError(t, repo.Enqueue(ctx, n))
	}

	claim := func(claimed *[]uuid.UUID, errs *[]error) {
		for {
			batch, err := repo.ClaimBatch(ctx, 25, now)
			if err != nil {
				*errs = append(*errs, err)
				return
			}
			if len(batch) == 0 {
				return
			}
			for _, n := range batch {
				*claimed = append(*claimed, n.ID)
			}
		}
	}

	var (
		wg           sync.WaitGroup
		start        = make(chan struct{})
		claimedA     []uuid.UUID
		claimedB     []uuid.UUID
		errsA, errsB []error
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		claim(&claimedA, &errsA)
	}()
	go func() {
		defer wg.Done()
		<-start
		claim(&claimedB, &errsB)
	}()
	close(start)
	wg.Wait()

	require.Empty(t, errsA)
	require.Empty(t, errsB)
	require.NotEmpty(t, claimedA, "one claimer took nothing at all; the two never ran concurrently")
	require.NotEmpty(t, claimedB, "one claimer took nothing at all; the two never ran concurrently")

	// The acceptance criterion: zero overlap between the two claimers.
	inA := make(map[uuid.UUID]struct{}, len(claimedA))
	for _, id := range claimedA {
		inA[id] = struct{}{}
	}
	for _, id := range claimedB {
		_, dup := inA[id]
		require.False(t, dup, "notification %s was claimed by both goroutines", id)
	}

	// And no row claimed twice anywhere, including within one claimer.
	seen := make(map[uuid.UUID]struct{}, seeded)
	for _, id := range append(append([]uuid.UUID{}, claimedA...), claimedB...) {
		_, dup := seen[id]
		require.False(t, dup, "notification %s was claimed twice", id)
		seen[id] = struct{}{}
	}

	// Finish the drain sequentially; SKIP LOCKED may have left rows behind at
	// the moment a claimer saw an empty batch.
	for {
		batch, err := repo.ClaimBatch(ctx, 100, now)
		require.NoError(t, err)
		if len(batch) == 0 {
			break
		}
		for _, n := range batch {
			_, dup := seen[n.ID]
			require.False(t, dup, "notification %s was claimed twice", n.ID)
			seen[n.ID] = struct{}{}
		}
	}

	require.Len(t, seen, seeded, "the seeded set was not fully claimed")

	var sending int
	require.NoError(t, bunDB.NewSelect().
		TableExpr("notifications").
		ColumnExpr("count(*)").
		Where("status = ?", string(domain.StatusSending)).
		Scan(ctx, &sending))
	require.Equal(t, seeded, sending, "claiming and the status transition are not atomic")
}

func TestNotificationRepo_SaveSettlesTheClaimedRow(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()

	recipient := uuid.New()
	now := pgTime(time.Now())
	require.NoError(t, repo.Enqueue(ctx, newNotification(t, recipient, now)))

	claimed, err := repo.ClaimBatch(ctx, 1, now)
	require.NoError(t, err)
	require.Len(t, claimed, 1)

	n := claimed[0]
	retryAt := pgTime(now.Add(30 * time.Second))
	require.NoError(t, n.RecordFailure(retryAt.Add(-30*time.Second), 30*time.Second, 5, "smtp 421 try again later"))
	require.NoError(t, repo.Save(ctx, n))

	// Re-claim at the retry time to read the settled row back through SQL.
	again, err := repo.ClaimBatch(ctx, 1, retryAt)
	require.NoError(t, err)
	require.Len(t, again, 1)
	require.Equal(t, n.ID, again[0].ID)
	require.Equal(t, 1, again[0].Attempts)
	require.Equal(t, retryAt, pgTime(again[0].NextAttemptAt))
	require.Equal(t, "smtp 421 try again later", again[0].LastError)
	require.Nil(t, again[0].SentAt)

	// A success clears the error and stamps sent_at.
	sentAt := pgTime(now.Add(time.Minute))
	require.NoError(t, again[0].MarkSent(sentAt))
	require.NoError(t, repo.Save(ctx, again[0]))

	var (
		status    string
		lastError *string
		stored    time.Time
	)
	require.NoError(t, repo.db.NewSelect().
		TableExpr("notifications").
		ColumnExpr("status, last_error, sent_at").
		Where("id = ?", n.ID).
		Scan(ctx, &status, &lastError, &stored))
	require.Equal(t, string(domain.StatusSent), status)
	require.Nil(t, lastError, `MarkSent cleared LastError, so the column must be NULL and not ""`)
	require.Equal(t, sentAt, pgTime(stored))
}

// TestNotificationRepo_SaveMissingRowIsNotFound pins Save's "it does not create
// rows" clause. A claimed row that has vanished is a real condition — a purge
// running against the outbox — and the dispatcher records it per row rather
// than losing the batch.
func TestNotificationRepo_SaveMissingRowIsNotFound(t *testing.T) {
	repo, _ := newRepo(t)

	n := newNotification(t, uuid.New(), pgTime(time.Now()))
	n.Status = domain.StatusSent

	require.ErrorIs(t, repo.Save(context.Background(), n), domain.ErrNotFound)
}

func TestNotificationRepo_ListForRecipientIsScopedAndOrdered(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()

	mine, theirs := uuid.New(), uuid.New()
	base := pgTime(time.Now())

	older := newNotification(t, mine, base.Add(-2*time.Minute))
	newer := newNotification(t, mine, base.Add(-time.Minute))
	email := newNotification(t, mine, base)
	email.Channel = domain.ChannelEmail
	other := newNotification(t, theirs, base)

	for _, n := range []*domain.Notification{older, newer, email, other} {
		require.NoError(t, repo.Enqueue(ctx, n))
	}

	got, err := repo.ListForRecipient(ctx, mine, domain.ListFilter{})
	require.NoError(t, err)
	require.Len(t, got, 2, "the list is not scoped to the recipient's in-app mailbox")
	require.Equal(t, []uuid.UUID{newer.ID, older.ID}, []uuid.UUID{got[0].ID, got[1].ID},
		"the mailbox is not ordered newest first")
}

func TestNotificationRepo_ListForRecipientUnreadOnly(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()

	recipient := uuid.New()
	base := pgTime(time.Now())
	read := newNotification(t, recipient, base.Add(-time.Minute))
	unread := newNotification(t, recipient, base)
	require.NoError(t, repo.Enqueue(ctx, read))
	require.NoError(t, repo.Enqueue(ctx, unread))
	require.NoError(t, repo.MarkRead(ctx, recipient, read.ID, base))

	got, err := repo.ListForRecipient(ctx, recipient, domain.ListFilter{UnreadOnly: true})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, unread.ID, got[0].ID)
}

// TestNotificationRepo_ListForRecipientNormalizesTheLimit checks that the
// repository applies ListFilter.Normalize rather than trusting f.Limit. A
// caller that omits the limit — the zero value of ListFilter — must get a page,
// not everything and not nothing, and a caller that asks for a thousand must
// get the ceiling.
func TestNotificationRepo_ListForRecipientNormalizesTheLimit(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()

	recipient := uuid.New()
	base := pgTime(time.Now())
	for i := 0; i < domain.DefaultListLimit+5; i++ {
		require.NoError(t, repo.Enqueue(ctx, newNotification(t, recipient, base.Add(-time.Duration(i)*time.Second))))
	}

	got, err := repo.ListForRecipient(ctx, recipient, domain.ListFilter{})
	require.NoError(t, err)
	require.Len(t, got, domain.DefaultListLimit, "a zero Limit must mean the default page, not every row")

	got, err = repo.ListForRecipient(ctx, recipient, domain.ListFilter{Limit: 10_000})
	require.NoError(t, err)
	require.LessOrEqual(t, len(got), domain.MaxListLimit)
}

// TestNotificationRepo_ListForRecipientKeysetPagesThroughATie is why the cursor
// is a pair and why the predicate is a row-value comparison.
//
// Every row here shares one created_at, which is what a fan-out inside a single
// transaction produces. Against created_at alone there is no correct predicate:
// `<` drops every sibling on the boundary and `<=` repeats them forever. The
// pair (created_at, id) is total, so paging with a page size of one walks the
// whole set exactly once.
func TestNotificationRepo_ListForRecipientKeysetPagesThroughATie(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()

	recipient := uuid.New()
	tie := pgTime(time.Now())

	const total = 5
	for i := 0; i < total; i++ {
		require.NoError(t, repo.Enqueue(ctx, newNotification(t, recipient, tie)))
	}

	seen := make(map[uuid.UUID]struct{}, total)
	var cursor *domain.Cursor
	for page := 0; page < total+1; page++ {
		got, err := repo.ListForRecipient(ctx, recipient, domain.ListFilter{Limit: 1, Before: cursor})
		require.NoError(t, err)
		if len(got) == 0 {
			break
		}
		require.Len(t, got, 1)
		_, dup := seen[got[0].ID]
		require.False(t, dup, "notification %s was returned on two pages", got[0].ID)
		seen[got[0].ID] = struct{}{}
		cursor = &domain.Cursor{CreatedAt: got[0].CreatedAt, ID: got[0].ID}
	}

	require.Len(t, seen, total, "paging through a created_at tie skipped or repeated rows")
}

// TestNotificationRepo_MarkReadRefusesAnotherRecipientsRow is the negative case
// the cross-user 404 rests on. The ownership predicate is in the UPDATE, so
// there is no window between the check and the write and no caller can skip it;
// a foreign id is reported as ErrNotFound — indistinguishable from a missing
// one, so the endpoint cannot be used to probe for ids — and the row is left
// untouched.
func TestNotificationRepo_MarkReadRefusesAnotherRecipientsRow(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()

	owner, intruder := uuid.New(), uuid.New()
	now := pgTime(time.Now())
	n := newNotification(t, owner, now)
	require.NoError(t, repo.Enqueue(ctx, n))

	err := repo.MarkRead(ctx, intruder, n.ID, now)
	require.ErrorIs(t, err, domain.ErrNotFound)

	var readAt *time.Time
	require.NoError(t, repo.db.NewSelect().
		TableExpr("notifications").
		ColumnExpr("read_at").
		Where("id = ?", n.ID).
		Scan(ctx, &readAt))
	require.Nil(t, readAt, "another recipient's MarkRead stamped the row")

	// An id that exists for nobody is the same answer.
	require.ErrorIs(t, repo.MarkRead(ctx, owner, uuid.New(), now), domain.ErrNotFound)
}

// TestNotificationRepo_MarkReadIsIdempotent pins the other clause of the
// contract: marking an already-read notification read is a no-op and not an
// error — a client that retries must not get a 404 — and it does not move the
// original timestamp.
func TestNotificationRepo_MarkReadIsIdempotent(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()

	recipient := uuid.New()
	first := pgTime(time.Now())
	n := newNotification(t, recipient, first)
	require.NoError(t, repo.Enqueue(ctx, n))

	require.NoError(t, repo.MarkRead(ctx, recipient, n.ID, first))
	require.NoError(t, repo.MarkRead(ctx, recipient, n.ID, first.Add(time.Hour)))

	got, err := repo.ListForRecipient(ctx, recipient, domain.ListFilter{})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.NotNil(t, got[0].ReadAt)
	require.Equal(t, first, pgTime(*got[0].ReadAt), "the second MarkRead moved the read timestamp")
}

func TestNotificationRepo_MarkAllRead(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()

	mine, theirs := uuid.New(), uuid.New()
	now := pgTime(time.Now())

	first := newNotification(t, mine, now)
	second := newNotification(t, mine, now)
	email := newNotification(t, mine, now)
	email.Channel = domain.ChannelEmail
	other := newNotification(t, theirs, now)
	for _, n := range []*domain.Notification{first, second, email, other} {
		require.NoError(t, repo.Enqueue(ctx, n))
	}

	changed, err := repo.MarkAllRead(ctx, mine, now)
	require.NoError(t, err)
	require.Equal(t, 2, changed, "MarkAllRead crossed a recipient or a channel boundary")

	// Already read: nothing left to change.
	changed, err = repo.MarkAllRead(ctx, mine, now.Add(time.Minute))
	require.NoError(t, err)
	require.Equal(t, 0, changed)

	unread, err := repo.ListForRecipient(ctx, mine, domain.ListFilter{UnreadOnly: true})
	require.NoError(t, err)
	require.Empty(t, unread)

	stillUnread, err := repo.ListForRecipient(ctx, theirs, domain.ListFilter{UnreadOnly: true})
	require.NoError(t, err)
	require.Len(t, stillUnread, 1, "another recipient's mailbox was marked read")
}
