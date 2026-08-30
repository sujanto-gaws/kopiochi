// Package sender holds the module's ChannelSender adapters: one delivery
// mechanism per file, each satisfying the port structurally.
//
// No file here imports the application layer. A sender names
// domain.RenderedMessage to spell its Send method and domain.ErrNonRetryable to
// classify a failure, and Go's structural interfaces do the rest — which is the
// arrangement R1 depends on and tools/archtest enforces.
//
// The error contract is domain.ErrNonRetryable's alone: wrap it when another
// attempt cannot help, return anything else when it might, and never decide the
// row's fate here. Both senders in this package always succeed, so neither
// exercises it; the SMTP sender (D8) is where it earns its keep.
package sender

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"

	domain "github.com/sujanto-gaws/kopiochi/modules/notification/domain"
)

// Log is the sender that delivers to the log and nowhere else.
//
// It is the development and test implementation of a real channel: wired in
// place of SMTP, it makes the whole outbox — enqueue, claim, render, settle —
// exercisable end to end with no mail server, no credentials and no network.
// Send always succeeds, so a row that reaches it always ends at sent.
//
// The channel it claims is a constructor argument rather than a constant
// because standing in for a channel is the entire point: an operator running
// locally sets the email channel to the log sender and gets the mail in their
// terminal.
type Log struct {
	log     zerolog.Logger
	channel domain.Channel
}

// NewLog returns a Log that claims channel.
//
// An unknown channel is refused rather than accepted and filed under a key
// nothing routes to. A sender registered under a channel that does not exist is
// invisible: the module builds, boots, and silently has no sender for the
// channel the operator thought they had configured.
func NewLog(log zerolog.Logger, channel domain.Channel) (*Log, error) {
	if !channel.Valid() {
		return nil, fmt.Errorf("log sender: unknown channel %q", channel)
	}
	return &Log{log: log, channel: channel}, nil
}

// Channel reports the channel this sender is registered for.
func (s *Log) Channel() domain.Channel { return s.channel }

// Send writes a summary of msg at info and the rendered bodies at debug, and
// returns nil.
//
// The split is deliberate. A rendered body can carry a password-reset link, a
// one-time code, or an address — content that is fine in a mailbox and not fine
// in a log aggregator six systems away, kept for a year. The summary at info is
// what an operator needs to see that delivery happened and to correlate it with
// the row; the body is behind debug, which is the level a developer turns on
// deliberately on their own machine and which production does not run.
//
// ctx is unused: writing a log line touches no network and cannot be
// meaningfully cancelled. It stays in the signature because the port has it and
// every other sender needs it.
func (s *Log) Send(ctx context.Context, msg domain.RenderedMessage) error {
	s.log.Info().
		Str("notification_id", msg.NotificationID.String()).
		Str("recipient_id", msg.RecipientID.String()).
		Str("channel", string(msg.Channel)).
		Str("subject", msg.Subject).
		Int("body_bytes", len(msg.Body)).
		Bool("html", msg.HTMLBody != "").
		Msg("notification delivered to log sender")

	s.log.Debug().
		Str("notification_id", msg.NotificationID.String()).
		Str("body", msg.Body).
		Str("html_body", msg.HTMLBody).
		Msg("notification body")

	return nil
}
