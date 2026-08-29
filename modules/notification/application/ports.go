// Package application holds the notification module's use cases: writing an
// intent into the outbox, the dispatch cycle that drains it, and the read model
// over a user's in-app mailbox and delivery preferences.
//
// It imports this module's domain package and the standard library, and nothing
// else — no ORM, no HTTP, no logger. Everything that touches the outside world
// arrives through one of the three ports below, which is what lets the whole
// dispatch cycle, including the retry schedule, be asserted on exact values
// against hand-written fakes.
//
// Time is one of those things. Nothing in this package calls time.Now(); every
// timestamp comes from the injected Clock, so a test fixes the clock and the
// retry ladder is a literal, not a tolerance.
package application

import (
	"context"
	"errors"
	"time"

	domain "github.com/sujanto-gaws/kopiochi/modules/notification/domain"
)

// ChannelSender delivers a rendered message over exactly one channel.
//
// Channel is on the sender rather than in a registry key so that the two cannot
// disagree: NewService builds its routing table from this method, so a sender
// filed under the wrong channel is not expressible.
//
// Send's error decides the row's fate — see domain.ErrNonRetryable.
type ChannelSender interface {
	Channel() domain.Channel
	Send(ctx context.Context, msg domain.RenderedMessage) error
}

// TemplateRenderer turns a template key and its payload into a message.
//
// Rendering happens at dispatch and not at enqueue, so a fixed template reaches
// rows that are already queued. Every error it returns is treated as
// non-retryable: an unknown key and a payload the template cannot execute are
// both deployment or producer bugs, and no amount of waiting fixes either.
type TemplateRenderer interface {
	Render(key string, channel domain.Channel, payload map[string]any) (domain.RenderedMessage, error)
}

// DispatchObserver is told the outcome of every settled delivery. E12.
//
// It exists because settle computes the channel, the category, the outcome and
// the failure, and then discards all of it: DispatchBatch returns len(claimed)
// and a joined error, which cannot carry per-channel counters, a latency
// histogram, or the audit event a security-category dead-letter warrants.
//
// Optional. A nil observer is a no-op, so nothing here is required to have one
// and no test has to supply one.
//
// EVERY PARAMETER IS A DOMAIN OR STDLIB TYPE, DELIBERATELY. The implementations
// live in infrastructure — a metrics adapter, an audit adapter — and R1 forbids
// infrastructure importing application. An application-defined outcome enum
// here would have forced exactly the choice E11 refused: an adapter that cannot
// be written, or a weakened rule. domain.Status already carries the vocabulary.
// The interface itself may stay in this package because Go satisfies interfaces
// structurally: an adapter never names DispatchObserver, only its parameters.
//
// took is how long the delivery attempt took, and is the reason this signature
// has one more parameter than E12 proposed. Without it the port answers two of
// E12's three needs and silently drops the histogram. It is measured with the
// injected clock, so a test with a fixed clock sees zero.
//
// Settled is called once per row, AFTER the outcome has been persisted — a row
// whose Save failed has not settled, and reporting it would mean an audit event
// for a dead-letter that did not happen. The one exception is a panic below
// settle, which is reported with an error wrapping ErrPanicked precisely
// because nothing else makes it visible (BL25).
//
// Implementations must not panic and must not block: they run inside the
// dispatch loop, and settleSafely's recover is a backstop for bugs, not a
// licence to write them.
type DispatchObserver interface {
	Settled(ctx context.Context, n domain.Notification, outcome domain.Status, took time.Duration, err error)
}

// Clock reports the current time. It exists so that the dispatch cycle is
// deterministic: the claim predicate, the sent-at stamp and the retry schedule
// all read this and nothing else.
type Clock interface{ Now() time.Time }

// The sender error contract — domain.ErrNonRetryable — lives in domain, because
// it decides a domain state transition and because a sender in infrastructure
// must reference it and may not import this package (R1). See E11.

// retryable reports whether a failed delivery is worth another attempt.
func retryable(err error) bool { return !errors.Is(err, domain.ErrNonRetryable) }
