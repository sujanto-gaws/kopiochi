package dispatcher_test

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	domain "github.com/sujanto-gaws/kopiochi/modules/notification/domain"
	"github.com/sujanto-gaws/kopiochi/modules/notification/infrastructure/dispatcher"
)

// The dispatcher owns goroutines and a shutdown protocol, so almost every test
// here is about timing. None of them call t.Parallel: the leak assertions read
// runtime.NumGoroutine, which is process-wide, and a parallel sibling starting
// its own workers would make them report each other's goroutines.

const (
	// tick is short enough that a test does not wait for real polling, and long
	// enough that a loop does not spin the CPU while a test blocks on it.
	tick = 5 * time.Millisecond

	// settle bounds every "did the thing happen" wait. Generous, because it is
	// only reached when a test is about to fail anyway.
	settle = 2 * time.Second
)

type fakeClock struct{ now time.Time }

func (c fakeClock) Now() time.Time { return c.now }

// fakeBatcher records dispatch cycles and can be made to block inside one,
// which is how the shutdown tests get a delivery to be in flight.
type fakeBatcher struct {
	mu    sync.Mutex
	calls int

	// returns is consumed one entry per call; the last entry repeats.
	returns []batchResult

	// entered is signalled on every call, and block (when non-nil) is what the
	// call waits on before returning.
	entered chan struct{}
	block   chan struct{}

	// ctxErr records whether the context the dispatcher handed the use case
	// was already cancelled when the call returned.
	ctxCancelled atomic.Bool
}

type batchResult struct {
	claimed int
	err     error
}

func (b *fakeBatcher) DispatchBatch(ctx context.Context) (int, error) {
	b.mu.Lock()
	n := b.calls
	b.calls++
	var res batchResult
	if len(b.returns) > 0 {
		if n >= len(b.returns) {
			n = len(b.returns) - 1
		}
		res = b.returns[n]
	}
	entered, block := b.entered, b.block
	b.mu.Unlock()

	if entered != nil {
		select {
		case entered <- struct{}{}:
		default:
		}
	}
	if block != nil {
		<-block
	}
	b.ctxCancelled.Store(ctx.Err() != nil)

	return res.claimed, res.err
}

func (b *fakeBatcher) callCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.calls
}

// fakeRepo implements only what the sweep uses. The embedded interface is nil,
// so any other method panics loudly rather than returning a zero value that a
// test could mistake for behaviour.
type fakeRepo struct {
	domain.NotificationRepository

	mu sync.Mutex

	stalled    []*domain.Notification
	claimErr   error
	claimCalls []claimStalledCall

	saved    []*domain.Notification
	saveErrs map[uuid.UUID]error
}

type claimStalledCall struct {
	n             int
	stalledBefore time.Time
	now           time.Time
}

func (r *fakeRepo) ClaimStalled(_ context.Context, n int, stalledBefore, now time.Time) ([]*domain.Notification, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.claimCalls = append(r.claimCalls, claimStalledCall{n: n, stalledBefore: stalledBefore, now: now})
	if r.claimErr != nil {
		return nil, r.claimErr
	}

	out := r.stalled
	r.stalled = nil
	return out, nil
}

func (r *fakeRepo) Save(_ context.Context, n *domain.Notification) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.saveErrs[n.ID]; err != nil {
		return err
	}
	r.saved = append(r.saved, n)
	return nil
}

func (r *fakeRepo) savedRows() []*domain.Notification {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*domain.Notification(nil), r.saved...)
}

func (r *fakeRepo) claims() []claimStalledCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]claimStalledCall(nil), r.claimCalls...)
}

func testConfig() dispatcher.Config {
	return dispatcher.Config{
		PollInterval: tick,
		Workers:      1,
		BatchSize:    10,
		StalledAfter: 5 * time.Minute,
		MaxAttempts:  3,
		BackoffBase:  30 * time.Second,
		BackoffCap:   time.Hour,
		DrainTimeout: settle,
	}
}

func testDeps(b dispatcher.Batcher, r domain.NotificationRepository, now time.Time) dispatcher.Deps {
	return dispatcher.Deps{
		Batcher:       b,
		Notifications: r,
		Clock:         fakeClock{now: now},
		Logger:        zerolog.Nop(),
		// Jitter left nil on purpose: the recovery schedule is then a literal
		// the sweep tests can assert on.
	}
}

func newDispatcher(t *testing.T, deps dispatcher.Deps, cfg dispatcher.Config) *dispatcher.Dispatcher {
	t.Helper()
	d, err := dispatcher.New(deps, cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return d
}

// waitFor polls cond until it holds or the deadline passes. Polling rather than
// a channel because most conditions here are counters that the dispatcher
// increments on its own schedule.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(settle)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

// assertNoGoroutineLeak waits for the process's goroutine count to fall back to
// baseline. It polls because a goroutine that has returned is not counted out
// instantly, and because the alternative — a bare comparison — fails randomly.
//
// This repo has no goleak dependency and this change does not add one.
func assertNoGoroutineLeak(t *testing.T, baseline int) {
	t.Helper()

	deadline := time.Now().Add(settle)
	for {
		got := runtime.NumGoroutine()
		if got <= baseline {
			return
		}
		if time.Now().After(deadline) {
			buf := make([]byte, 1<<16)
			buf = buf[:runtime.Stack(buf, true)]
			t.Fatalf("goroutines still running: %d, baseline %d\n%s", got, baseline, buf)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestNewRefusesIncompleteDependenciesAndSettings(t *testing.T) {
	now := time.Now()

	cases := []struct {
		name string
		deps func(dispatcher.Deps) dispatcher.Deps
		cfg  func(dispatcher.Config) dispatcher.Config
	}{
		{"no batcher", func(d dispatcher.Deps) dispatcher.Deps { d.Batcher = nil; return d }, nil},
		{"no repository", func(d dispatcher.Deps) dispatcher.Deps { d.Notifications = nil; return d }, nil},
		{"no clock", func(d dispatcher.Deps) dispatcher.Deps { d.Clock = nil; return d }, nil},
		{"zero poll interval", nil, func(c dispatcher.Config) dispatcher.Config { c.PollInterval = 0; return c }},
		{"zero workers", nil, func(c dispatcher.Config) dispatcher.Config { c.Workers = 0; return c }},
		{"zero batch size", nil, func(c dispatcher.Config) dispatcher.Config { c.BatchSize = 0; return c }},
		{"zero stalled-after", nil, func(c dispatcher.Config) dispatcher.Config { c.StalledAfter = 0; return c }},
		{"zero max attempts", nil, func(c dispatcher.Config) dispatcher.Config { c.MaxAttempts = 0; return c }},
		{"zero backoff base", nil, func(c dispatcher.Config) dispatcher.Config { c.BackoffBase = 0; return c }},
		{"zero backoff cap", nil, func(c dispatcher.Config) dispatcher.Config { c.BackoffCap = 0; return c }},
		{"zero drain timeout", nil, func(c dispatcher.Config) dispatcher.Config { c.DrainTimeout = 0; return c }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := testDeps(&fakeBatcher{}, &fakeRepo{}, now)
			cfg := testConfig()
			if tc.deps != nil {
				deps = tc.deps(deps)
			}
			if tc.cfg != nil {
				cfg = tc.cfg(cfg)
			}

			d, err := dispatcher.New(deps, cfg)
			if err == nil {
				t.Fatalf("expected an error, got a dispatcher: %+v", d)
			}
		})
	}
}

// The whole shutdown contract in one test: Stop does not return while a
// delivery is in flight, the in-flight call is not cancelled underneath it, and
// nothing is left running afterwards.
func TestStopWaitsForTheInFlightDeliveryToSettle(t *testing.T) {
	baseline := runtime.NumGoroutine()

	batcher := &fakeBatcher{
		entered: make(chan struct{}, 1),
		block:   make(chan struct{}),
		returns: []batchResult{{claimed: 1}},
	}
	d := newDispatcher(t, testDeps(batcher, &fakeRepo{}, time.Now()), testConfig())
	d.Start()

	<-batcher.entered // a cycle is now in flight and blocked

	stopped := make(chan error, 1)
	go func() { stopped <- d.Stop(context.Background()) }()

	// Stop must still be waiting: the row this cycle claimed is marked sending
	// in the database, and abandoning it would leave it stuck there.
	select {
	case err := <-stopped:
		t.Fatalf("Stop returned while a delivery was in flight: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(batcher.block)

	select {
	case err := <-stopped:
		if err != nil {
			t.Fatalf("Stop: %v", err)
		}
	case <-time.After(settle):
		t.Fatal("Stop did not return after the in-flight delivery finished")
	}

	if batcher.ctxCancelled.Load() {
		t.Error("the in-flight cycle saw a cancelled context; shutdown must stop claiming, not abandon work")
	}
	assertNoGoroutineLeak(t, baseline)
}

// The other half: a delivery that never finishes must not hang the process
// forever. Stop reports it and cuts the work loose, which leaves the row in
// sending for the next process's sweep — the at-least-once trade the package
// comment describes.
func TestStopReportsADrainThatDoesNotFinish(t *testing.T) {
	baseline := runtime.NumGoroutine()

	release := make(chan struct{})
	batcher := &fakeBatcher{
		entered: make(chan struct{}, 1),
		block:   release,
		returns: []batchResult{{claimed: 1}},
	}

	cfg := testConfig()
	cfg.DrainTimeout = 20 * time.Millisecond

	d := newDispatcher(t, testDeps(batcher, &fakeRepo{}, time.Now()), cfg)
	d.Start()
	<-batcher.entered

	err := d.Stop(context.Background())
	if err == nil {
		t.Fatal("Stop reported success while a delivery was still stuck")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Stop error does not carry the deadline: %v", err)
	}

	close(release)
	assertNoGoroutineLeak(t, baseline)
}

func TestStopIsIdempotentAndSafeWithoutStart(t *testing.T) {
	d := newDispatcher(t, testDeps(&fakeBatcher{}, &fakeRepo{}, time.Now()), testConfig())

	// Never started: there is nothing to stop, and saying so with a panic
	// would make a failed boot's cleanup path the second failure.
	if err := d.Stop(context.Background()); err != nil {
		t.Fatalf("Stop before Start: %v", err)
	}

	started := newDispatcher(t, testDeps(&fakeBatcher{}, &fakeRepo{}, time.Now()), testConfig())
	started.Start()
	if err := started.Stop(context.Background()); err != nil {
		t.Fatalf("first Stop: %v", err)
	}
	if err := started.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

// A full batch means there was more work than one claim could take. Waiting a
// whole poll interval for the rest would make the drain rate a function of the
// tick instead of the database.
func TestAFullBatchIsFollowedByAnotherClaimImmediately(t *testing.T) {
	baseline := runtime.NumGoroutine()

	// A long poll interval is what gives this test its teeth. With the 5ms tick
	// the other tests use, three cycles would arrive in 15ms whether the drain
	// loop existed or not, and the assertion would pass on an implementation
	// that only ever polls. At 300ms there is exactly one tick inside the
	// window below, so a second and third cycle can only have come from the
	// drain loop.
	const pollInterval = 300 * time.Millisecond

	cfg := testConfig()
	cfg.PollInterval = pollInterval

	batcher := &fakeBatcher{returns: []batchResult{
		{claimed: cfg.BatchSize},
		{claimed: cfg.BatchSize},
		{claimed: 3}, // a short batch: the outbox is empty, stop asking
	}}

	d := newDispatcher(t, testDeps(batcher, &fakeRepo{}, time.Now()), cfg)
	d.Start()

	waitFor(t, "the first tick", func() bool { return batcher.callCount() >= 1 })

	window := time.Now().Add(pollInterval / 3)
	for batcher.callCount() < 3 {
		if time.Now().After(window) {
			t.Fatalf("only %d dispatch cycles ran within a third of a poll interval; a full batch must be followed by another claim",
				batcher.callCount())
		}
		time.Sleep(time.Millisecond)
	}

	// And it stops asking once a batch comes back short, rather than spinning.
	time.Sleep(pollInterval / 3)
	if got := batcher.callCount(); got != 3 {
		t.Errorf("dispatch cycles = %d, want 3: the drain loop kept claiming after a short batch", got)
	}

	if err := d.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	assertNoGoroutineLeak(t, baseline)
}

// A failed cycle is the infrastructure kind — a claim that failed, a row that
// would not save. The next tick is the retry; killing the worker would turn one
// bad minute into a dispatcher that never runs again.
func TestAFailedCycleDoesNotStopTheWorker(t *testing.T) {
	baseline := runtime.NumGoroutine()

	batcher := &fakeBatcher{returns: []batchResult{
		{err: errors.New("claim notifications: connection refused")},
		{claimed: 0},
	}}

	d := newDispatcher(t, testDeps(batcher, &fakeRepo{}, time.Now()), testConfig())
	d.Start()

	waitFor(t, "a cycle after the failed one", func() bool { return batcher.callCount() >= 2 })

	if err := d.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	assertNoGoroutineLeak(t, baseline)
}

func stalledRow(t *testing.T, attempts int, now time.Time) *domain.Notification {
	t.Helper()

	n, err := domain.NewNotification(domain.NewNotificationParams{
		ID:          uuid.New(),
		RecipientID: uuid.New(),
		Channel:     domain.ChannelEmail,
		Category:    domain.CategorySecurity,
		TemplateKey: "security.password_changed",
	}, now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("build notification: %v", err)
	}

	// The state a killed worker leaves behind: claimed, never settled.
	n.Status = domain.StatusSending
	n.Attempts = attempts
	return n
}

func TestSweepRecoversStalledRowsOnTheOrdinaryRetrySchedule(t *testing.T) {
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	cfg := testConfig()

	row := stalledRow(t, 0, now)
	repo := &fakeRepo{stalled: []*domain.Notification{row}}

	d := newDispatcher(t, testDeps(&fakeBatcher{}, repo, now), cfg)

	recovered, err := d.Sweep(context.Background())
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if recovered != 1 {
		t.Fatalf("recovered = %d, want 1", recovered)
	}

	// The window is the caller's policy, computed from the injected clock.
	claims := repo.claims()
	if len(claims) != 1 {
		t.Fatalf("ClaimStalled called %d times, want 1", len(claims))
	}
	if want := now.Add(-cfg.StalledAfter); !claims[0].stalledBefore.Equal(want) {
		t.Errorf("stalledBefore = %s, want %s", claims[0].stalledBefore, want)
	}
	if !claims[0].now.Equal(now) {
		t.Errorf("now = %s, want %s", claims[0].now, now)
	}
	if claims[0].n != cfg.BatchSize {
		t.Errorf("claimed %d rows at a time, want the batch size %d", claims[0].n, cfg.BatchSize)
	}

	saved := repo.savedRows()
	if len(saved) != 1 {
		t.Fatalf("saved %d rows, want 1", len(saved))
	}
	got := saved[0]
	if got.Status != domain.StatusPending {
		t.Errorf("status = %q, want %q", got.Status, domain.StatusPending)
	}
	// Recovery costs an attempt. A row that hangs its worker every time it is
	// claimed is indistinguishable from one that fails every time it is sent,
	// and recovering it for free would retry it forever.
	if got.Attempts != 1 {
		t.Errorf("attempts = %d, want 1", got.Attempts)
	}
	if want := now.Add(domain.Backoff(0, cfg.BackoffBase, cfg.BackoffCap)); !got.NextAttemptAt.Equal(want) {
		t.Errorf("next attempt = %s, want %s", got.NextAttemptAt, want)
	}
	if got.LastError == "" {
		t.Error("a recovered row must say why it was recovered")
	}
}

// The budget applies to stall recovery exactly as it does to a reported
// failure, or a row that kills its worker every time is retried forever.
func TestSweepDeadLettersARowThatHasSpentItsBudget(t *testing.T) {
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	cfg := testConfig()

	row := stalledRow(t, cfg.MaxAttempts-1, now)
	repo := &fakeRepo{stalled: []*domain.Notification{row}}

	d := newDispatcher(t, testDeps(&fakeBatcher{}, repo, now), cfg)
	if _, err := d.Sweep(context.Background()); err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	saved := repo.savedRows()
	if len(saved) != 1 {
		t.Fatalf("saved %d rows, want 1", len(saved))
	}
	if saved[0].Status != domain.StatusDead {
		t.Errorf("status = %q, want %q", saved[0].Status, domain.StatusDead)
	}
}

// One unsaveable row costs one row. The sweep is a batch, and a batch that
// abandons its remaining work on the first failure leaves rows stuck for
// another whole stall window.
func TestSweepKeepsGoingAfterOneRowFails(t *testing.T) {
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)

	bad := stalledRow(t, 0, now)
	good := stalledRow(t, 0, now)
	repo := &fakeRepo{
		stalled:  []*domain.Notification{bad, good},
		saveErrs: map[uuid.UUID]error{bad.ID: errors.New("connection reset")},
	}

	d := newDispatcher(t, testDeps(&fakeBatcher{}, repo, now), testConfig())

	recovered, err := d.Sweep(context.Background())
	if err == nil {
		t.Fatal("Sweep hid the failed row")
	}
	if recovered != 1 {
		t.Errorf("recovered = %d, want 1", recovered)
	}

	saved := repo.savedRows()
	if len(saved) != 1 || saved[0].ID != good.ID {
		t.Errorf("the healthy row was not recovered: %+v", saved)
	}
}

func TestSweepReportsAFailedClaim(t *testing.T) {
	now := time.Now()
	repo := &fakeRepo{claimErr: errors.New("connection refused")}

	d := newDispatcher(t, testDeps(&fakeBatcher{}, repo, now), testConfig())

	recovered, err := d.Sweep(context.Background())
	if err == nil {
		t.Fatal("expected the claim failure to be reported")
	}
	if recovered != 0 {
		t.Errorf("recovered = %d, want 0", recovered)
	}
}

// Nothing stalled is the normal case, and it must not be reported as anything.
func TestSweepIsQuietWhenNothingIsStalled(t *testing.T) {
	d := newDispatcher(t, testDeps(&fakeBatcher{}, &fakeRepo{}, time.Now()), testConfig())

	recovered, err := d.Sweep(context.Background())
	if err != nil || recovered != 0 {
		t.Fatalf("Sweep = (%d, %v), want (0, nil)", recovered, err)
	}
}

// The sweeper runs on the same cycle as the workers, so a stalled row is
// recovered without anyone calling Sweep by hand.
func TestTheRunningDispatcherSweeps(t *testing.T) {
	baseline := runtime.NumGoroutine()
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)

	repo := &fakeRepo{stalled: []*domain.Notification{stalledRow(t, 0, now)}}
	d := newDispatcher(t, testDeps(&fakeBatcher{}, repo, now), testConfig())
	d.Start()

	waitFor(t, "the sweep to recover the stalled row", func() bool { return len(repo.savedRows()) == 1 })

	if err := d.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	assertNoGoroutineLeak(t, baseline)
}

// Every worker plus the sweeper plus the goroutine that closes done: whatever
// the worker count, Stop leaves none of them behind.
func TestEveryWorkerStops(t *testing.T) {
	baseline := runtime.NumGoroutine()

	cfg := testConfig()
	cfg.Workers = 4

	batcher := &fakeBatcher{}
	d := newDispatcher(t, testDeps(batcher, &fakeRepo{}, time.Now()), cfg)
	d.Start()

	waitFor(t, "the workers to poll", func() bool { return batcher.callCount() >= cfg.Workers })

	if err := d.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	assertNoGoroutineLeak(t, baseline)
}
