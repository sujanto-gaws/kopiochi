package application

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	domain "github.com/sujanto-gaws/kopiochi/modules/notification/domain"
)

// Hand-written fakes rather than a mocking library, and no database or network
// anywhere in this package's tests. The ports have one to five methods each,
// and what these tests assert is the state the use cases leave behind — the
// status a row settles in, the exact second its next attempt is scheduled for —
// which a generated mock's call log cannot express.
//
// The outbox fake is a small in-memory implementation rather than a canned
// response, because the dispatch cycle only makes sense against one: ClaimBatch
// honours the same predicate the real repository does (pending, and due), and
// hands back a copy so that a settled row is only visible once Save has written
// it. A use case that forgot to save would leave the row claimed and stuck, and
// the next claim in the test would come back empty.

var (
	testNow       = time.Date(2026, 3, 14, 15, 9, 26, 0, time.UTC)
	testUser      = uuid.MustParse("11111111-1111-4111-8111-111111111111")
	testOtherUser = uuid.MustParse("22222222-2222-4222-8222-222222222222")
)

var testDispatchConfig = DispatchConfig{
	BatchSize:   10,
	MaxAttempts: 4,
	BackoffBase: 30 * time.Second,
	BackoffCap:  time.Hour,
}

// fixedClock is the injected Clock. It never moves on its own; a test that
// wants time to pass says so.
type fixedClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFixedClock(t time.Time) *fixedClock { return &fixedClock{now: t} }

func (c *fixedClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fixedClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// fixedJitter is a domain.JitterSource that always returns the same draw.
type fixedJitter struct{ f float64 }

func (j fixedJitter) Float64() float64 { return j.f }

type markReadCall struct {
	recipientID uuid.UUID
	id          uuid.UUID
	now         time.Time
}

type claimCall struct {
	n   int
	now time.Time
}

// fakeNotificationRepo is an in-memory domain.NotificationRepository.
type fakeNotificationRepo struct {
	mu sync.Mutex

	// rows is the outbox, in insertion order.
	rows []*domain.Notification

	// savedIDs records every Save in order, including the ones that failed.
	savedIDs []uuid.UUID

	// listRows is what ListForRecipient returns; the outbox above is the
	// dispatcher's view and this is the reader's, kept apart so a list test
	// does not have to construct a claimable row.
	listRows []*domain.Notification

	// claimedAt mirrors the notifications.claimed_at column, which the domain
	// entity deliberately does not carry: ClaimStalled filters on it and Save
	// never touches it, so the fake keeps it beside the rows rather than on
	// them (E9b).
	claimedAt map[uuid.UUID]time.Time

	enqueueErr      error
	claimErr        error
	claimStalledErr error
	listErr         error
	markReadErr     error
	markAllErr      error
	markAllN        int

	// saveErrs and savePanics fail Save for one specific row.
	saveErrs   map[uuid.UUID]error
	savePanics map[uuid.UUID]string

	// injectRows is prepended to every claim, unfiltered. It is how a test
	// hands back what no real repository should — a nil, or a row that is not
	// in sending — none of which may cost the rest of the batch.
	injectRows []*domain.Notification

	claims        []claimCall
	markReadCalls []markReadCall
	markAllNow    time.Time
	listedFor     uuid.UUID
	listedWith    domain.ListFilter
}

func newFakeNotificationRepo() *fakeNotificationRepo {
	return &fakeNotificationRepo{
		saveErrs:   map[uuid.UUID]error{},
		savePanics: map[uuid.UUID]string{},
	}
}

func (r *fakeNotificationRepo) Enqueue(_ context.Context, n *domain.Notification) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.enqueueErr != nil {
		return r.enqueueErr
	}
	if n.IdempotencyKey != "" {
		for _, existing := range r.rows {
			if existing.IdempotencyKey == n.IdempotencyKey {
				return domain.ErrDuplicateIdempotencyKey
			}
		}
	}

	stored := *n
	r.rows = append(r.rows, &stored)
	return nil
}

// stampClaim records claimed_at. Caller holds the lock.
func (r *fakeNotificationRepo) stampClaim(id uuid.UUID, now time.Time) {
	if r.claimedAt == nil {
		r.claimedAt = map[uuid.UUID]time.Time{}
	}
	r.claimedAt[id] = now
}

// ClaimStalled mirrors the repository: it hands back sending rows claimed
// before stalledBefore WITHOUT applying any transition, and re-stamps
// claimed_at so a sweeper that dies before saving defers the row instead of
// leaving it immediately eligible again.
func (r *fakeNotificationRepo) ClaimStalled(
	_ context.Context, n int, stalledBefore, now time.Time,
) ([]*domain.Notification, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.claimStalledErr != nil {
		return nil, r.claimStalledErr
	}

	var stalled []*domain.Notification
	for _, row := range r.rows {
		if len(stalled) >= n {
			break
		}
		if row.Status != domain.StatusSending {
			continue
		}
		at, ok := r.claimedAt[row.ID]
		if !ok || !at.Before(stalledBefore) {
			continue
		}
		r.stampClaim(row.ID, now)

		handed := *row
		stalled = append(stalled, &handed)
	}
	return stalled, nil
}

func (r *fakeNotificationRepo) ClaimBatch(_ context.Context, n int, now time.Time) ([]*domain.Notification, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.claims = append(r.claims, claimCall{n: n, now: now})
	if r.claimErr != nil {
		return nil, r.claimErr
	}

	claimed := append([]*domain.Notification(nil), r.injectRows...)
	for _, row := range r.rows {
		if len(claimed) >= n {
			break
		}
		if row.Status != domain.StatusPending || row.NextAttemptAt.After(now) {
			continue
		}
		row.Status = domain.StatusSending
		r.stampClaim(row.ID, now)

		handed := *row
		claimed = append(claimed, &handed)
	}
	return claimed, nil
}

func (r *fakeNotificationRepo) Save(_ context.Context, n *domain.Notification) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.savedIDs = append(r.savedIDs, n.ID)

	if msg, ok := r.savePanics[n.ID]; ok {
		panic(msg)
	}
	if err, ok := r.saveErrs[n.ID]; ok {
		return err
	}

	for i, row := range r.rows {
		if row.ID == n.ID {
			stored := *n
			r.rows[i] = &stored
			return nil
		}
	}
	return domain.ErrNotFound
}

func (r *fakeNotificationRepo) ListForRecipient(_ context.Context, recipientID uuid.UUID, f domain.ListFilter) ([]*domain.Notification, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.listedFor = recipientID
	r.listedWith = f
	if r.listErr != nil {
		return nil, r.listErr
	}
	return r.listRows, nil
}

func (r *fakeNotificationRepo) MarkRead(_ context.Context, recipientID, id uuid.UUID, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.markReadCalls = append(r.markReadCalls, markReadCall{recipientID: recipientID, id: id, now: now})
	return r.markReadErr
}

func (r *fakeNotificationRepo) MarkAllRead(_ context.Context, _ uuid.UUID, now time.Time) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.markAllNow = now
	if r.markAllErr != nil {
		return 0, r.markAllErr
	}
	return r.markAllN, nil
}

// find returns the stored row with the given id, or nil.
func (r *fakeNotificationRepo) find(id uuid.UUID) *domain.Notification {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, row := range r.rows {
		if row.ID == id {
			return row
		}
	}
	return nil
}

// only returns the single stored row, failing the test when there is not
// exactly one.
func (r *fakeNotificationRepo) only(t *testing.T) *domain.Notification {
	t.Helper()

	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.rows) != 1 {
		t.Fatalf("outbox holds %d rows, want exactly 1", len(r.rows))
	}
	return r.rows[0]
}

func (r *fakeNotificationRepo) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.rows)
}

// fakePreferenceRepo is an in-memory domain.PreferenceRepository.
type fakePreferenceRepo struct {
	mu sync.Mutex

	prefs     []domain.Preference
	listErr   error
	upsertErr error
	upserts   [][]domain.Preference
	listCalls int
}

func (r *fakePreferenceRepo) ListForUser(_ context.Context, userID uuid.UUID) ([]domain.Preference, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.listCalls++
	if r.listErr != nil {
		return nil, r.listErr
	}

	var out []domain.Preference
	for _, p := range r.prefs {
		if p.UserID == userID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (r *fakePreferenceRepo) Upsert(_ context.Context, prefs []domain.Preference) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.upserts = append(r.upserts, prefs)
	if r.upsertErr != nil {
		return r.upsertErr
	}

	for _, p := range prefs {
		replaced := false
		for i, existing := range r.prefs {
			if existing.UserID == p.UserID && existing.Channel == p.Channel && existing.Category == p.Category {
				r.prefs[i] = p
				replaced = true
				break
			}
		}
		if !replaced {
			r.prefs = append(r.prefs, p)
		}
	}
	return nil
}

type renderCall struct {
	key     string
	channel domain.Channel
	payload map[string]any
}

// fakeRenderer produces a message derived from its inputs, so a test can assert
// that the right template was rendered for the right channel.
type fakeRenderer struct {
	mu sync.Mutex

	calls []renderCall

	// err fails every render; errFor fails one template key.
	err    error
	errFor map[string]error

	panicWith string
}

func (r *fakeRenderer) Render(key string, channel domain.Channel, payload map[string]any) (domain.RenderedMessage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, renderCall{key: key, channel: channel, payload: payload})

	if r.panicWith != "" {
		panic(r.panicWith)
	}
	if err, ok := r.errFor[key]; ok {
		return domain.RenderedMessage{}, err
	}
	if r.err != nil {
		return domain.RenderedMessage{}, r.err
	}

	return domain.RenderedMessage{
		// Deliberately filled in: the routing fields are the dispatcher's to
		// stamp, and a renderer that guesses them must be overruled.
		NotificationID: uuid.MustParse("99999999-9999-4999-8999-999999999999"),
		RecipientID:    uuid.MustParse("99999999-9999-4999-8999-999999999999"),
		Channel:        "carrier-pigeon",

		Subject: "subject:" + key,
		Body:    fmt.Sprintf("body:%s:%s", key, channel),
	}, nil
}

// fakeSender is a ChannelSender for one channel.
type fakeSender struct {
	mu sync.Mutex

	ch   domain.Channel
	sent []domain.RenderedMessage

	// err is returned by every Send. errs, when non-empty, is consumed one
	// entry per call and takes precedence, which is how a test makes the first
	// attempt fail and the second succeed.
	err  error
	errs []error

	panicWith string
}

func (s *fakeSender) Channel() domain.Channel { return s.ch }

func (s *fakeSender) Send(_ context.Context, msg domain.RenderedMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sent = append(s.sent, msg)

	if s.panicWith != "" {
		panic(s.panicWith)
	}
	if len(s.errs) > 0 {
		err := s.errs[0]
		s.errs = s.errs[1:]
		return err
	}
	return s.err
}

// harness is a Service over fresh fakes, with the fakes still reachable.
type harness struct {
	notifications *fakeNotificationRepo
	preferences   *fakePreferenceRepo
	renderer      *fakeRenderer
	senders       map[domain.Channel]*fakeSender
	clock         *fixedClock
	svc           *Service
}

// newHarness wires a service with a sender for every known channel and no
// jitter, so the retry schedule is the bare Backoff ladder.
func newHarness(t *testing.T) *harness {
	t.Helper()
	return newHarnessWith(t, domain.Channels(), nil, testDispatchConfig)
}

// newHarnessWith is newHarness with control over which channels have a sender,
// the jitter source, and the dispatch config.
func newHarnessWith(t *testing.T, channels []domain.Channel, jitter domain.JitterSource, cfg DispatchConfig) *harness {
	t.Helper()

	h := &harness{
		notifications: newFakeNotificationRepo(),
		preferences:   &fakePreferenceRepo{},
		renderer:      &fakeRenderer{},
		senders:       map[domain.Channel]*fakeSender{},
		clock:         newFixedClock(testNow),
	}

	senders := make([]ChannelSender, 0, len(channels))
	for _, ch := range channels {
		s := &fakeSender{ch: ch}
		h.senders[ch] = s
		senders = append(senders, s)
	}

	svc, err := NewService(h.notifications, h.preferences, h.renderer, senders, h.clock, jitter, cfg)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	h.svc = svc
	return h
}

// enqueue puts one pending, immediately claimable row in the outbox and returns
// it. It goes through the service so that a row under test is built the same
// way a real one is.
// seedRow puts a pending row straight into the outbox, bypassing Service.Enqueue.
//
// It exists for the one case Enqueue now refuses: a row whose channel has no
// registered sender. Since E13 that is rejected at enqueue, but it remains
// reachable in production — a row queued while a sender was wired, and drained
// after it was removed — and the dispatcher's fail-closed handling of it is
// exactly what the test using this asserts. Going through Enqueue would test
// the new guard instead of the old behaviour, and silently stop covering it.
func (h *harness) seedRow(t *testing.T, ch domain.Channel, cat domain.Category) *domain.Notification {
	t.Helper()

	n, err := domain.NewNotification(domain.NewNotificationParams{
		ID:          uuid.New(),
		RecipientID: testUser,
		Channel:     ch,
		Category:    cat,
		TemplateKey: "security.password_changed",
		Payload:     map[string]any{"ip": "203.0.113.7"},
	}, h.clock.Now())
	if err != nil {
		t.Fatalf("NewNotification: %v", err)
	}
	if err := h.notifications.Enqueue(context.Background(), n); err != nil {
		t.Fatalf("seed row: %v", err)
	}

	h.notifications.mu.Lock()
	defer h.notifications.mu.Unlock()
	return h.notifications.rows[len(h.notifications.rows)-1]
}

func (h *harness) enqueue(t *testing.T, ch domain.Channel, cat domain.Category, key string) *domain.Notification {
	t.Helper()

	before := h.notifications.count()
	req := EnqueueRequest{
		RecipientID:    testUser,
		Channel:        ch,
		Category:       cat,
		TemplateKey:    "security.password_changed",
		Payload:        map[string]any{"ip": "203.0.113.7"},
		IdempotencyKey: key,
	}
	if err := h.svc.Enqueue(context.Background(), req); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if got := h.notifications.count(); got != before+1 {
		t.Fatalf("outbox holds %d rows after Enqueue, want %d", got, before+1)
	}

	h.notifications.mu.Lock()
	defer h.notifications.mu.Unlock()
	return h.notifications.rows[len(h.notifications.rows)-1]
}
