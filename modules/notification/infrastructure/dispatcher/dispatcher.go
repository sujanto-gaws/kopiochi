// Package dispatcher runs the outbox drain as a background worker: a ticker,
// a fixed set of workers calling the dispatch use case, and the sweep that
// recovers rows a dead worker left behind.
//
// It holds no delivery logic of its own. Claiming, rendering, sending and
// settling are one use-case call (Batcher); this package decides only when to
// make that call, how many at once, and when to stop.
//
// # Delivery is at-least-once, deliberately
//
// A worker can send a message and then fail to record that it did — the
// process is killed between the SMTP handshake and the UPDATE, or the database
// connection dies in that window. The row stays in sending with nobody left to
// settle it, the sweep below recovers it, and it is delivered again.
//
// The alternative is at-most-once: settle first, send after. That trades a
// duplicate mail for a lost one, and the notification most likely to be in
// flight when a process dies is the one it was retrying — a security mail.
// Duplicates are visible and survivable; a silently lost password-changed
// notification is neither. Consumers that cannot tolerate a duplicate must
// deduplicate on NotificationID, which is stable across every attempt.
//
// # Shutdown
//
// Stop separates "stop claiming" from "abandon what is in flight", because
// they are not the same instant. A claimed row is already marked sending in the
// database; abandoning it mid-cycle leaves it stuck until the sweep finds it
// five minutes later. So the run loop and the work handed to the use case get
// two different contexts: cancelling the first ends the loop, and the second
// stays live until the drain finishes or DrainTimeout runs out.
package dispatcher

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"

	domain "github.com/sujanto-gaws/kopiochi/modules/notification/domain"
)

// Batcher is the dispatch use case this worker drives.
//
// It is declared here and satisfied structurally by *application.Service: the
// infrastructure layer may not import application (R1, E11), and Go needs no
// import to satisfy an interface. The same reason the senders name only domain
// types.
type Batcher interface {
	// DispatchBatch runs one claim-render-send-settle cycle and reports how
	// many rows it claimed.
	DispatchBatch(ctx context.Context) (int, error)
}

// Clock reports the current time. The sweep's "claimed before this instant"
// predicate is computed from it, so a test fixes the clock and the window is a
// literal.
type Clock interface{ Now() time.Time }

// Config is the worker's schedule and the sweep's policy. Every field is
// required; New refuses a zero.
type Config struct {
	PollInterval time.Duration
	Workers      int
	BatchSize    int

	// StalledAfter is how long a row may sit in sending before the sweep owns
	// it. It is policy and lives here rather than in the repository, which has
	// no opinion about how long a send may take.
	StalledAfter time.Duration

	// The retry schedule a recovered row is rescheduled on. They are the same
	// values the dispatch use case settles ordinary failures with, passed
	// again rather than shared, because a stalled row is an ordinary failure
	// with no reporter and must not get a schedule of its own.
	MaxAttempts int
	BackoffBase time.Duration
	BackoffCap  time.Duration

	// DrainTimeout bounds Stop when its caller supplies no deadline.
	DrainTimeout time.Duration
}

// Deps are the collaborators the worker cannot build for itself.
type Deps struct {
	Batcher Batcher

	// Notifications is used by the sweep alone. The dispatch cycle reaches the
	// repository through Batcher; this is here because recovering a stalled
	// row is a claim plus a domain transition plus a save, and there is no use
	// case that spells it.
	Notifications domain.NotificationRepository

	Clock  Clock
	Logger zerolog.Logger

	// Jitter spreads recovery reschedules. Optional: nil means an unjittered
	// schedule, which is a worse schedule and not a broken one.
	Jitter domain.JitterSource
}

// Dispatcher is the background worker. It is started once, by the module's
// constructor, and stopped once, by the module's Close.
type Dispatcher struct {
	deps Deps
	cfg  Config

	// stopClaiming ends the run loops; abortWork cancels whatever they handed
	// to the use case. Two cancels, because shutdown means stop taking new
	// work, not drop the work in hand.
	stopClaiming context.CancelFunc
	abortWork    context.CancelFunc

	// done is closed once every goroutine this type started has returned.
	done chan struct{}

	startOnce sync.Once
	stopOnce  sync.Once
	stopErr   error
}

// New validates the worker's dependencies and settings and returns it stopped.
//
// Everything is checked here rather than at the first tick: a nil repository or
// a zero interval discovered by a background goroutine surfaces as a panic in a
// log nobody is reading, hours after the boot that caused it.
func New(deps Deps, cfg Config) (*Dispatcher, error) {
	var errs []error

	if deps.Batcher == nil {
		errs = append(errs, errors.New("dispatcher: a batcher is required"))
	}
	if deps.Notifications == nil {
		errs = append(errs, errors.New("dispatcher: a notification repository is required"))
	}
	if deps.Clock == nil {
		errs = append(errs, errors.New("dispatcher: a clock is required"))
	}
	if cfg.PollInterval <= 0 {
		errs = append(errs, errors.New("dispatcher: poll interval must be positive"))
	}
	if cfg.Workers <= 0 {
		errs = append(errs, errors.New("dispatcher: workers must be positive"))
	}
	if cfg.BatchSize <= 0 {
		errs = append(errs, errors.New("dispatcher: batch size must be positive"))
	}
	if cfg.StalledAfter <= 0 {
		errs = append(errs, errors.New("dispatcher: stalled-after must be positive"))
	}
	if cfg.MaxAttempts <= 0 {
		errs = append(errs, errors.New("dispatcher: max attempts must be positive"))
	}
	if cfg.BackoffBase <= 0 || cfg.BackoffCap <= 0 {
		errs = append(errs, errors.New("dispatcher: backoff base and cap must be positive"))
	}
	if cfg.DrainTimeout <= 0 {
		errs = append(errs, errors.New("dispatcher: drain timeout must be positive"))
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	return &Dispatcher{deps: deps, cfg: cfg, done: make(chan struct{})}, nil
}

// Start launches the workers and the sweeper. It returns immediately and is a
// no-op on a second call.
//
// The first poll is a full PollInterval away rather than immediate, so
// constructing and closing a dispatcher — which is what a boot check and most
// tests do — touches the database exactly never.
func (d *Dispatcher) Start() {
	d.startOnce.Do(func() {
		claimCtx, stopClaiming := context.WithCancel(context.Background())
		workCtx, abortWork := context.WithCancel(context.Background())
		d.stopClaiming, d.abortWork = stopClaiming, abortWork

		var wg sync.WaitGroup
		wg.Add(d.cfg.Workers + 1)

		for i := 0; i < d.cfg.Workers; i++ {
			go func(worker int) {
				defer wg.Done()
				d.runWorker(claimCtx, workCtx, worker)
			}(i)
		}

		// One sweeper regardless of the worker count. ClaimStalled is
		// exclusive, so N sweepers would be correct — and would be N times the
		// query for a table that is empty of stalled rows almost always.
		go func() {
			defer wg.Done()
			d.runSweeper(claimCtx, workCtx)
		}()

		go func() {
			wg.Wait()
			close(d.done)
		}()

		d.deps.Logger.Info().
			Int("workers", d.cfg.Workers).
			Dur("poll_interval", d.cfg.PollInterval).
			Msg("notification dispatcher started")
	})
}

// Stop stops claiming and waits for the in-flight cycle to settle its rows.
//
// It returns an error only when the drain did not finish inside ctx or
// DrainTimeout, whichever comes first — and in that case it cancels the
// in-flight work rather than waiting forever, which leaves those rows in
// sending for the next process's sweep to recover. That is the honest outcome:
// the alternative is a shutdown that never completes.
//
// Safe to call on a dispatcher that was never started, and idempotent.
func (d *Dispatcher) Stop(ctx context.Context) error {
	d.stopOnce.Do(func() {
		if d.stopClaiming == nil {
			return // never started
		}

		ctx, cancel := context.WithTimeout(ctx, d.cfg.DrainTimeout)
		defer cancel()

		d.stopClaiming()

		select {
		case <-d.done:
			d.deps.Logger.Info().Msg("notification dispatcher stopped")
		case <-ctx.Done():
			d.stopErr = fmt.Errorf("notification dispatcher: in-flight deliveries did not settle within %s: %w", d.cfg.DrainTimeout, ctx.Err())
			d.deps.Logger.Error().Err(d.stopErr).Msg("notification dispatcher drain timed out")
		}

		// Released on both paths: on the happy one nothing is using it, and on
		// the unhappy one this is what unblocks whatever is still waiting on
		// the database.
		d.abortWork()
	})

	return d.stopErr
}

// runWorker polls, and drains what it finds.
func (d *Dispatcher) runWorker(claim, work context.Context, worker int) {
	ticker := time.NewTicker(d.cfg.PollInterval)
	defer ticker.Stop()

	log := d.deps.Logger.With().Int("worker", worker).Logger()

	for {
		select {
		case <-claim.Done():
			return
		case <-ticker.C:
		}

		d.drain(claim, work, log)
	}
}

// drain calls the use case until the outbox runs dry or shutdown begins.
//
// A cycle that claimed a full batch means there was more work than one claim
// could take, and waiting a whole poll interval to take the rest would make the
// drain rate a function of the tick rather than of the database. A short batch
// means the queue is empty and there is nothing to gain from asking again.
func (d *Dispatcher) drain(claim, work context.Context, log zerolog.Logger) {
	for {
		claimed, err := d.deps.Batcher.DispatchBatch(work)
		if err != nil {
			// Not fatal to the loop. DispatchBatch's error is the
			// infrastructure kind — a failed claim, a row that would not save
			// — and the next tick is the retry. Delivery failures are not
			// errors here at all; they are outcomes recorded on the row.
			log.Error().Err(err).Int("claimed", claimed).Msg("notification dispatch cycle failed")
		}

		// claimed == 0 is checked separately from the batch comparison: it is
		// the ordinary "nothing to do", and it is also the only guard that
		// stops a misconfigured non-positive batch size from spinning here.
		if claimed == 0 || claimed < d.cfg.BatchSize || claim.Err() != nil {
			return
		}
	}
}

// runSweeper recovers rows that were claimed and never settled.
func (d *Dispatcher) runSweeper(claim, work context.Context) {
	ticker := time.NewTicker(d.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-claim.Done():
			return
		case <-ticker.C:
		}

		if _, err := d.Sweep(work); err != nil {
			d.deps.Logger.Error().Err(err).Msg("notification stalled-row sweep failed")
		}
	}
}

// Sweep recovers up to one batch of rows that have been sending for longer than
// StalledAfter, and reports how many it recovered.
//
// The recovery itself is the domain's: ClaimStalled decides only which rows
// this sweeper owns and deliberately leaves the status alone, so the transition
// runs through RecoverStalled here, one row at a time. A set-based UPDATE would
// be a second copy of the state machine written in SQL, which no domain test
// covers (E9c).
//
// A recovered row costs an attempt, exactly as a reported failure does. A
// notification that hangs its worker every time it is claimed is
// indistinguishable from one that fails every time it is sent, and recovering
// it for free would retry it forever.
//
// Exported so the behaviour is testable on its own, without a ticker.
func (d *Dispatcher) Sweep(ctx context.Context) (int, error) {
	now := d.deps.Clock.Now()

	stalled, err := d.deps.Notifications.ClaimStalled(ctx, d.cfg.BatchSize, now.Add(-d.cfg.StalledAfter), now)
	if err != nil {
		return 0, fmt.Errorf("claim stalled notifications: %w", err)
	}

	var (
		recovered int
		errs      []error
	)
	for _, n := range stalled {
		if n == nil {
			errs = append(errs, errors.New("claimed a nil stalled notification"))
			continue
		}

		// Computed before RecoverStalled increments Attempts, for the same
		// reason the dispatch cycle computes it before RecordFailure: Attempts
		// holds the count of prior failures, so the first one waits
		// BackoffBase rather than twice it.
		retryAfter := domain.BackoffWithJitter(n.Attempts, d.cfg.BackoffBase, d.cfg.BackoffCap, d.deps.Jitter)
		if err := n.RecoverStalled(now, retryAfter, d.cfg.MaxAttempts); err != nil {
			errs = append(errs, fmt.Errorf("notification %s: recover stalled: %w", n.ID, err))
			continue
		}
		if err := d.deps.Notifications.Save(ctx, n); err != nil {
			errs = append(errs, fmt.Errorf("notification %s: save recovered: %w", n.ID, err))
			continue
		}

		recovered++
		// Warn, not info: a stalled row means a worker died mid-delivery, and
		// the message may have been sent already — see the package comment on
		// at-least-once.
		d.deps.Logger.Warn().
			Str("notification_id", n.ID.String()).
			Str("status", string(n.Status)).
			Int("attempts", n.Attempts).
			Msg("recovered a stalled notification")
	}

	return recovered, errors.Join(errs...)
}
