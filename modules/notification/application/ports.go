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

	"github.com/google/uuid"

	domain "github.com/sujanto-gaws/kopiochi/modules/notification/domain"
)

// RenderedMessage is one notification turned into the content a sender
// transmits.
//
// It is filled in two halves. TemplateRenderer supplies Subject and Body, which
// are all a template can know; DispatchBatch then stamps NotificationID,
// RecipientID and Channel from the outbox row, because routing is the row's
// property and not the template's. A renderer that sets the routing fields
// itself will have them overwritten.
//
// The recipient is an id, not an address. Resolving it — to a mailbox, a
// webhook URL, or to nothing at all because the row *is* the in-app
// notification — is the sender's job, and is the reason no address appears
// anywhere in this module.
type RenderedMessage struct {
	NotificationID uuid.UUID
	RecipientID    uuid.UUID
	Channel        domain.Channel

	// Subject is the message's one-line heading. Channels that have no notion
	// of one ignore it.
	Subject string

	// Body is the rendered content for Channel. One body per channel, not one
	// per MIME type: the renderer resolves a template per (key, channel), so a
	// channel that needs several representations is a channel that names
	// several templates.
	Body string
}

// ChannelSender delivers a rendered message over exactly one channel.
//
// Channel is on the sender rather than in a registry key so that the two cannot
// disagree: NewService builds its routing table from this method, so a sender
// filed under the wrong channel is not expressible.
//
// Send's error decides the row's fate — see ErrNonRetryable.
type ChannelSender interface {
	Channel() domain.Channel
	Send(ctx context.Context, msg RenderedMessage) error
}

// TemplateRenderer turns a template key and its payload into a message.
//
// Rendering happens at dispatch and not at enqueue, so a fixed template reaches
// rows that are already queued. Every error it returns is treated as
// non-retryable: an unknown key and a payload the template cannot execute are
// both deployment or producer bugs, and no amount of waiting fixes either.
type TemplateRenderer interface {
	Render(key string, channel domain.Channel, payload map[string]any) (RenderedMessage, error)
}

// Clock reports the current time. It exists so that the dispatch cycle is
// deterministic: the claim predicate, the sent-at stamp and the retry schedule
// all read this and nothing else.
type Clock interface{ Now() time.Time }

// ErrNonRetryable marks a delivery failure that another attempt cannot fix, and
// is the whole of the error contract between this package and the senders in
// infrastructure.
//
// The classification has one rule and one default:
//
//   - A sender error that wraps this sentinel settles the row as dead
//     immediately, with the error text kept as LastError for the operator.
//   - Anything else is retryable, and the row goes back to pending with the
//     backoff schedule until its attempt budget runs out.
//
// Retryable is the default because the two mistakes are not symmetric.
// Misclassifying a permanent failure as retryable wastes a bounded number of
// attempts and still ends at dead; misclassifying a transient one as permanent
// destroys the notification on the first network blip — and the notification
// most likely to be sent during an outage is a security mail.
//
// A sender opts out by wrapping:
//
//	return fmt.Errorf("%w: smtp 550 mailbox unavailable", application.ErrNonRetryable)
//
// A sentinel plus errors.Is rather than an interface with a Retryable() bool
// method: wrapping composes with %w through every layer an SMTP or HTTP client
// puts in the way, and it needs no new type in either package.
var ErrNonRetryable = errors.New("notification: non-retryable delivery failure")

// retryable reports whether a failed delivery is worth another attempt.
func retryable(err error) bool { return !errors.Is(err, ErrNonRetryable) }
