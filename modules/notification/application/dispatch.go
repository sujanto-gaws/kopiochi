package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	domain "github.com/sujanto-gaws/kopiochi/modules/notification/domain"
)

// DispatchConfig is the operator-tunable half of the dispatch cycle.
//
// It arrives already validated from the module's Config, and this package
// supplies no defaults of its own: a zero BatchSize claims nothing and a
// MaxAttempts below one makes the first failure fatal, both of which are what
// the configured value literally says. Quietly substituting a better number
// here would mean the value an operator reads in the config file is not the
// value the system uses.
type DispatchConfig struct {
	// BatchSize is how many rows one cycle claims.
	BatchSize int

	// MaxAttempts is the retry budget. A row that has failed this many times is
	// dead-lettered instead of rescheduled.
	MaxAttempts int

	// BackoffBase is the wait after the first failure; BackoffCap bounds the
	// doubling. Jitter, when a source is injected, is applied inside the cap.
	BackoffBase time.Duration
	BackoffCap  time.Duration
}

// DispatchBatch runs one dispatch cycle — claim, render, send, settle — and
// returns the number of rows it claimed, whatever became of them. A caller
// polls again immediately when that equals BatchSize and there is more work.
//
// Delivery failures are not errors of this method: they are outcomes recorded
// on the row. The error it returns is the infrastructure kind — the claim
// failed, or a settled row could not be saved — joined across the batch so that
// one unsaveable row costs one row and not the rest of the batch.
//
// It settles every row it claimed even after ctx is cancelled. A claimed row is
// already marked sending in the database; abandoning it mid-cycle leaves it
// stuck in a state nothing else moves out of. Shutdown means stop claiming, not
// stop finishing — so the caller's shutdown context must outlive the drain.
func (s *Service) DispatchBatch(ctx context.Context) (int, error) {
	claimed, err := s.notifications.ClaimBatch(ctx, s.cfg.BatchSize, s.clock.Now())
	if err != nil {
		return 0, fmt.Errorf("claim notifications: %w", err)
	}

	var failures []error
	for _, n := range claimed {
		if n == nil {
			// A repository is not supposed to produce one. Skipping beats
			// dereferencing it and losing the rest of the batch to a panic.
			failures = append(failures, errors.New("claimed a nil notification"))
			continue
		}
		if err := s.settleSafely(ctx, n); err != nil {
			failures = append(failures, fmt.Errorf("notification %s: %w", n.ID, err))
		}
	}

	return len(claimed), errors.Join(failures...)
}

// settleSafely is settle with a last-resort recover.
//
// The recover inside attemptDelivery turns a panicking sender into an ordinary
// failed delivery, which is the case that matters because the row still gets
// settled. This one covers everything below that is not supposed to panic at
// all — a repository, an entity method — and exists so that such a bug costs
// one row rather than the remaining rows of the batch and the worker goroutine
// that owns them.
func (s *Service) settleSafely(ctx context.Context, n *domain.Notification) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic settling notification: %v", r)
		}
	}()

	return s.settle(ctx, n)
}

// settle attempts one delivery and writes the outcome back.
func (s *Service) settle(ctx context.Context, n *domain.Notification) error {
	worthRetrying, failure := s.attemptDelivery(ctx, n)
	now := s.clock.Now()

	switch {
	case failure == nil:
		if err := n.MarkSent(now); err != nil {
			return fmt.Errorf("mark sent: %w", err)
		}

	case !worthRetrying:
		if err := n.MarkDead(failure.Error()); err != nil {
			return fmt.Errorf("mark dead: %w", err)
		}

	default:
		// Computed before RecordFailure, which is what increments Attempts.
		// Attempts counts failures already recorded, so at this point it still
		// holds the count of *prior* ones — and the first failure therefore
		// waits BackoffBase rather than twice it. Moving this line below the
		// call doubles every retry delay in production and no test that only
		// asserts "a delay was set" would notice.
		retryAfter := domain.BackoffWithJitter(n.Attempts, s.cfg.BackoffBase, s.cfg.BackoffCap, s.jitter)
		if err := n.RecordFailure(now, retryAfter, s.cfg.MaxAttempts, failure.Error()); err != nil {
			return fmt.Errorf("record failure: %w", err)
		}
	}

	if err := s.notifications.Save(ctx, n); err != nil {
		return fmt.Errorf("save notification: %w", err)
	}
	return nil
}

// attemptDelivery renders n and hands it to the sender for its channel.
//
// It reports whether another attempt could plausibly succeed, and the failure.
// The flag is meaningless when the error is nil.
//
// Two failures are non-retryable by construction rather than by the sender's
// classification, because both are configuration bugs and a retry budget spent
// re-discovering that the deployment is still misconfigured is a budget wasted:
// no sender registered for the row's channel, and any error from the renderer.
//
// A panicking sender is treated as an ordinary retryable failure. The
// alternative — dead-lettering — turns one bug in one sender into permanently
// destroyed notifications, while a retry leaves the row alive for as long as
// the attempt budget lasts, which is the window in which a fix can ship.
// Recovering at all is what keeps a third-party client's nil dereference from
// killing the dispatcher goroutine.
func (s *Service) attemptDelivery(ctx context.Context, n *domain.Notification) (worthRetrying bool, err error) {
	defer func() {
		if r := recover(); r != nil {
			worthRetrying = true
			err = fmt.Errorf("panic during delivery: %v", r)
		}
	}()

	sender, ok := s.senders[n.Channel]
	if !ok {
		return false, fmt.Errorf("%w: no sender registered for channel %q", domain.ErrNonRetryable, n.Channel)
	}

	msg, err := s.renderer.Render(n.TemplateKey, n.Channel, n.Payload)
	if err != nil {
		return false, fmt.Errorf("%w: render %q for channel %q: %w", domain.ErrNonRetryable, n.TemplateKey, n.Channel, err)
	}

	// Routing is the row's, not the template's. Stamped after rendering so a
	// renderer cannot misaddress a message.
	msg.NotificationID = n.ID
	msg.RecipientID = n.RecipientID
	msg.Channel = n.Channel

	if err := sender.Send(ctx, msg); err != nil {
		return retryable(err), fmt.Errorf("send over %q: %w", n.Channel, err)
	}
	return false, nil
}
