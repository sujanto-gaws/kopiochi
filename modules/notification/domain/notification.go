// Package domain holds the notification module's entities and invariants: the
// outbox row, its delivery state machine, the retry schedule, and the per-user
// delivery preferences that filter an enqueue.
//
// Nothing in this package performs I/O or reads the clock. Every method that
// needs the time takes it as an argument, so the retry schedule is asserted on
// exact values rather than on tolerances, and the repository interfaces in
// repository.go are ports this package owns and infrastructure implements.
package domain

import (
	"fmt"
	"math"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Channel is how a notification reaches its recipient.
type Channel string

const (
	ChannelEmail   Channel = "email"
	ChannelInApp   Channel = "inapp"
	ChannelWebhook Channel = "webhook"
)

// Channels returns every channel, in a fixed order.
//
// The effective-preference matrix is the cross product of channels and
// categories, and a caller that hardcodes the list silently stops covering a
// channel added later.
func Channels() []Channel {
	return []Channel{ChannelEmail, ChannelInApp, ChannelWebhook}
}

// Valid reports whether c is a channel this module knows how to route.
func (c Channel) Valid() bool {
	switch c {
	case ChannelEmail, ChannelInApp, ChannelWebhook:
		return true
	default:
		return false
	}
}

// Category is what a notification is about. It is the unit users express
// preferences over, which is why it is coarser than TemplateKey.
type Category string

const (
	CategorySecurity Category = "security"
	CategoryAccount  Category = "account"
	CategorySystem   Category = "system"
)

// Categories returns every category, in a fixed order. See Channels.
func Categories() []Category {
	return []Category{CategorySecurity, CategoryAccount, CategorySystem}
}

// Valid reports whether c is a known category.
func (c Category) Valid() bool {
	switch c {
	case CategorySecurity, CategoryAccount, CategorySystem:
		return true
	default:
		return false
	}
}

// Status is a notification's position in the delivery state machine.
//
//	pending ──► sending ──┬──► sent           (terminal)
//	   ▲                  ├──► failed ──┬──► pending   (retry)
//	   │                  │             └──► dead      (terminal)
//	   └──────────────────┘             │
//	                      └──► dead ────┘     (non-retryable, terminal)
type Status string

const (
	StatusPending Status = "pending"
	StatusSending Status = "sending"
	StatusSent    Status = "sent"
	StatusFailed  Status = "failed"
	StatusDead    Status = "dead"
)

type transition struct{ from, to Status }

// allowedTransitions is the whole state machine. Anything absent is refused,
// including every self-transition and every move out of sent or dead.
//
// sending -> dead has no failed step on purpose: it is the non-retryable path,
// taken when the failure is a configuration bug rather than a transient one —
// an unknown template key, or no sender registered for the channel. Routing
// those through failed would spend the row's whole retry budget re-discovering
// that the deployment is still misconfigured.
var allowedTransitions = map[transition]struct{}{
	{StatusPending, StatusSending}: {},
	{StatusSending, StatusSent}:    {},
	{StatusSending, StatusFailed}:  {},
	{StatusSending, StatusDead}:    {},
	{StatusFailed, StatusPending}:  {},
	{StatusFailed, StatusDead}:     {},
}

// Notification is one row of the outbox: an intent to deliver, committed inside
// the business transaction that caused it and delivered later by the
// dispatcher.
type Notification struct {
	ID uuid.UUID

	// RecipientID is a user id and nothing more. This module holds no foreign
	// key into another module's table and makes no claim about what a user is;
	// resolving the id to an address is the sender's job at dispatch time.
	RecipientID uuid.UUID

	Channel  Channel
	Category Category

	// TemplateKey names the template family to render, e.g.
	// "security.password_changed".
	TemplateKey string

	// Payload is template data, and only template data. Rendering happens at
	// dispatch rather than at enqueue so that a fix to a template reaches rows
	// that are already queued; storing rendered content here would freeze
	// every typo into the outbox.
	Payload map[string]any

	Status Status

	// Attempts counts delivery attempts that failed. A successful send does not
	// increment it. That is what makes it the right input to both retry
	// decisions: Backoff(Attempts, ...) for how long to wait, and
	// Attempts >= maxAttempts for whether to stop.
	Attempts int

	// NextAttemptAt is the dispatcher's claim predicate: a pending row is
	// eligible once NextAttemptAt has passed. It is set at construction, so a
	// freshly enqueued row is immediately claimable.
	NextAttemptAt time.Time

	// IdempotencyKey deduplicates enqueues that the same business event may
	// trigger more than once — a retried request, a replayed message. An empty
	// string means "no key", which the repository maps to SQL NULL so that
	// unkeyed rows do not collide with each other.
	IdempotencyKey string

	// ReadAt is meaningful for the in-app channel only; the other channels
	// leave the mailbox, and this module never learns whether anyone opened
	// them.
	ReadAt *time.Time

	// LastError is the truncated reason the most recent attempt failed. It is
	// for operators and is never shown to a user: it can carry SMTP server
	// responses, webhook bodies and internal host names.
	LastError string

	CreatedAt time.Time
	SentAt    *time.Time
}

// maxLastErrorLen caps LastError. Failure reasons arrive from SMTP servers and
// webhook endpoints, which are free to return a megabyte of HTML; the outbox is
// not a log store, and the first few hundred bytes are what identifies the
// fault.
const maxLastErrorLen = 512

// NewNotificationParams carries the caller-supplied half of a new notification.
// Everything derived — status, attempt count, timestamps — is set by
// NewNotification.
type NewNotificationParams struct {
	// ID is supplied by the caller rather than generated here so that
	// construction is deterministic and a test can assert on the id it chose.
	ID          uuid.UUID
	RecipientID uuid.UUID
	Channel     Channel
	Category    Category
	TemplateKey string
	Payload     map[string]any

	// IdempotencyKey is optional; "" means no deduplication.
	IdempotencyKey string
}

// NewNotification builds a pending notification that is immediately claimable.
//
// It is the only way to obtain a valid entity: it rejects the unknown channel
// and the missing template key here, at enqueue, where the caller still has the
// context to fix them — rather than at dispatch, hours later, on a background
// worker whose only recourse is to dead-letter the row.
func NewNotification(p NewNotificationParams, now time.Time) (*Notification, error) {
	if p.ID == uuid.Nil {
		return nil, fmt.Errorf("%w: id is required", ErrInvalidNotification)
	}
	if p.RecipientID == uuid.Nil {
		return nil, fmt.Errorf("%w: recipient id is required", ErrInvalidNotification)
	}
	if !p.Channel.Valid() {
		return nil, fmt.Errorf("%w: unknown channel %q", ErrInvalidNotification, p.Channel)
	}
	if !p.Category.Valid() {
		return nil, fmt.Errorf("%w: unknown category %q", ErrInvalidNotification, p.Category)
	}
	if p.TemplateKey == "" {
		return nil, fmt.Errorf("%w: template key is required", ErrInvalidNotification)
	}

	payload := p.Payload
	if payload == nil {
		payload = map[string]any{}
	}

	return &Notification{
		ID:             p.ID,
		RecipientID:    p.RecipientID,
		Channel:        p.Channel,
		Category:       p.Category,
		TemplateKey:    p.TemplateKey,
		Payload:        payload,
		Status:         StatusPending,
		Attempts:       0,
		NextAttemptAt:  now,
		IdempotencyKey: p.IdempotencyKey,
		CreatedAt:      now,
	}, nil
}

// Transition moves the notification to next, or returns ErrInvalidTransition
// and leaves it untouched.
//
// This is the primitive every other state change goes through. It sets no
// timestamps and touches no other field: a caller that needs SentAt or a retry
// schedule uses MarkSent, MarkDead or RecordFailure, which set them together
// with the status so the two cannot drift apart.
func (n *Notification) Transition(next Status) error {
	if _, ok := allowedTransitions[transition{n.Status, next}]; !ok {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, n.Status, next)
	}
	n.Status = next
	return nil
}

// MarkSent settles a successful delivery, stamping SentAt and clearing any
// error left by an earlier attempt — the row succeeded, and a stale LastError
// on a sent row reads like a fault that never happened.
func (n *Notification) MarkSent(now time.Time) error {
	if err := n.Transition(StatusSent); err != nil {
		return err
	}
	sentAt := now
	n.SentAt = &sentAt
	n.LastError = ""
	return nil
}

// MarkDead settles a delivery that will never be retried: from sending for a
// non-retryable failure, or from failed once the retry budget is gone.
//
// It does not increment Attempts. Attempts drives the retry schedule and this
// row has left it; what the operator needs is reason, which is preserved.
func (n *Notification) MarkDead(reason string) error {
	if err := n.Transition(StatusDead); err != nil {
		return err
	}
	n.LastError = truncateReason(reason)
	return nil
}

// RecordFailure settles a delivery attempt that failed for a retryable reason.
//
// It increments Attempts, records reason, and then routes: a notification that
// has spent its budget goes to dead and stays there; anything else returns to
// pending with NextAttemptAt = now + retryAfter, which is the predicate
// ClaimBatch selects on. Callers compute retryAfter with Backoff before
// calling, while Attempts still holds the count of prior failures — so the
// first failure waits base, not base*2.
//
// The exhaustion check lives here rather than in the caller because
// "Attempts >= maxAttempts means dead" is the only thing stopping a
// permanently broken recipient from being retried forever, and a caller that
// forgets it gets an infinite loop instead of a compile error. maxAttempts is a
// parameter, not a constant, because it is operator-tunable config; a value
// below 1 makes the first failure fatal.
//
// The trip through failed is deliberate: the state machine says a retry is
// sending -> failed -> pending, and passing through the intermediate state here
// means the same arrows are exercised whether a row is settled by this method
// or by Transition directly.
func (n *Notification) RecordFailure(now time.Time, retryAfter time.Duration, maxAttempts int, reason string) error {
	if err := n.Transition(StatusFailed); err != nil {
		return err
	}

	n.Attempts++
	n.LastError = truncateReason(reason)

	if n.Attempts >= maxAttempts {
		// Cannot fail: failed -> dead is in the table.
		_ = n.Transition(StatusDead)
		return nil
	}

	// Cannot fail: failed -> pending is in the table.
	_ = n.Transition(StatusPending)
	n.NextAttemptAt = now.Add(retryAfter)
	return nil
}

// truncateReason clips s to maxLastErrorLen bytes without splitting a rune. A
// half-rune would be stored, logged and eventually rendered as a replacement
// character, which looks like corruption at exactly the moment an operator is
// trying to read a failure.
func truncateReason(s string) string {
	if len(s) <= maxLastErrorLen {
		return s
	}
	cut := s[:maxLastErrorLen]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut
}

// Backoff returns the retry delay for a notification that has already failed
// attempt times: min(base * 2^attempt, cap), never negative.
//
// It is pure and jitter-free, which is what makes the schedule assertable on
// exact values. BackoffWithJitter adds the spread; the two are split so that
// the arithmetic — including the saturation below — is tested without a random
// source anywhere near it.
//
// The clamp is a plain min/max: a non-positive cap yields 0, a non-positive
// base yields 0, and a negative attempt is treated as 0. There is no "cap <= 0
// means unlimited" special case, because that reading turns a zeroed config
// field into an unbounded retry delay.
func Backoff(attempt int, base, cap time.Duration) time.Duration {
	d := scaleBase(attempt, base)
	if d > cap {
		d = cap
	}
	if d < 0 {
		d = 0
	}
	return d
}

// scaleBase returns base * 2^attempt, saturating at the largest Duration
// instead of wrapping.
//
// time.Duration is an int64 of nanoseconds, so base << attempt overflows and
// goes negative for attempt values a stuck row reaches in a few days. A
// negative delay would set NextAttemptAt in the past, the dispatcher would
// re-claim the row on its very next tick, and the backoff would become a hot
// loop precisely when the downstream is already failing.
func scaleBase(attempt int, base time.Duration) time.Duration {
	if base <= 0 {
		return 0
	}
	if attempt <= 0 {
		return base
	}
	// A Duration holds 63 value bits, so any shift at or beyond that saturates
	// whatever the base.
	if attempt >= 63 {
		return time.Duration(math.MaxInt64)
	}
	if base > time.Duration(math.MaxInt64)>>uint(attempt) {
		return time.Duration(math.MaxInt64)
	}
	return base << uint(attempt)
}

// JitterSource yields a float in [0, 1). *math/rand.Rand and *math/rand/v2.Rand
// both satisfy it.
//
// It is an interface parameter rather than a package-level generator because
// the dispatcher runs several workers concurrently: a shared mutable source in
// this package would be a data race that only shows up under load, and a
// per-package mutex would serialise workers on a coin flip. Passing the source
// in also makes the schedule reproducible in a test with a two-line fake.
type JitterSource interface {
	Float64() float64
}

// BackoffWithJitter returns Backoff(attempt, base, cap) spread over the top
// half of its range: the result is in [d/2, d) where d = Backoff(...), and it
// therefore never exceeds cap.
//
// Jitter is applied inside the cap, not added on top of it, because cap is a
// configured maximum wait — a result above it would break the promise the
// config field makes, and config validation (backoff_base <= backoff_cap) reads
// as if it bounds the outcome.
//
// Half the delay stays deterministic. Full jitter — a uniform draw over [0, d)
// — decorrelates better but can schedule a retry almost immediately, which
// turns a broken SMTP host into a tight loop; keeping a floor of d/2 preserves
// the exponential shape while still breaking up a herd of rows that all failed
// on the same outage.
//
// A nil source means no jitter rather than a panic: an unjittered retry is a
// worse schedule, but a panicking dispatcher loses the batch.
func BackoffWithJitter(attempt int, base, cap time.Duration, src JitterSource) time.Duration {
	d := Backoff(attempt, base, cap)
	if src == nil || d <= 0 {
		return d
	}
	half := d / 2
	f := src.Float64()
	if f < 0 {
		f = 0
	}
	if f >= 1 {
		// Defensive: the contract is [0, 1), and a source that returns 1 would
		// push the result to exactly d. Clamping keeps the documented
		// half-open range true of every source.
		f = math.Nextafter(1, 0)
	}
	return half + time.Duration(f*float64(d-half))
}
