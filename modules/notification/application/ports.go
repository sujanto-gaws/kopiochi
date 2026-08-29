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

// Clock reports the current time. It exists so that the dispatch cycle is
// deterministic: the claim predicate, the sent-at stamp and the retry schedule
// all read this and nothing else.
type Clock interface{ Now() time.Time }

// The sender error contract — domain.ErrNonRetryable — lives in domain, because
// it decides a domain state transition and because a sender in infrastructure
// must reference it and may not import this package (R1). See E11.

// retryable reports whether a failed delivery is worth another attempt.
func retryable(err error) bool { return !errors.Is(err, domain.ErrNonRetryable) }
