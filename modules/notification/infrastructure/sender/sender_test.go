package sender_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	domain "github.com/sujanto-gaws/kopiochi/modules/notification/domain"
	"github.com/sujanto-gaws/kopiochi/modules/notification/infrastructure/sender"
)

// channelSender is application.ChannelSender, copied rather than imported.
//
// Infrastructure may not import application (R1, E11) and tools/archtest loads
// test files too, so naming the real port here would fail the architecture
// rules these adapters exist to respect. Go satisfies interfaces structurally:
// if these assertions compile, the service can register both senders.
type channelSender interface {
	Channel() domain.Channel
	Send(ctx context.Context, msg domain.RenderedMessage) error
}

var (
	_ channelSender = (*sender.Log)(nil)
	_ channelSender = (*sender.InApp)(nil)
)

// sensitive stands in for what a rendered body can legitimately contain — a
// one-time link. The point of the info/debug split is that this string does not
// reach a production log.
const sensitive = "https://app.example.test/reset/one-time-link"

func message() domain.RenderedMessage {
	return domain.RenderedMessage{
		NotificationID: uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		RecipientID:    uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		Channel:        domain.ChannelEmail,
		Subject:        "Your password was changed",
		Body:           "Reset it here: " + sensitive,
		HTMLBody:       "<p>Reset it here: " + sensitive + "</p>",
	}
}

func newLog(t *testing.T, log zerolog.Logger, channel domain.Channel) *sender.Log {
	t.Helper()
	s, err := sender.NewLog(log, channel)
	if err != nil {
		t.Fatalf("NewLog(%q): %v", channel, err)
	}
	return s
}

// records decodes every JSON line written to buf.
func records(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()

	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("log line is not JSON: %v (%q)", err, line)
		}
		out = append(out, rec)
	}
	return out
}

// The summary is what production sees. A rendered body can carry a reset link
// or a one-time code, and the dev-mode sender must not be the reason one lands
// in a log aggregator that keeps it for a year.
func TestLogSenderSummaryAtInfoOmitsTheBody(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	msg := message()
	if err := newLog(t, zerolog.New(&buf).Level(zerolog.InfoLevel), domain.ChannelEmail).Send(context.Background(), msg); err != nil {
		t.Fatalf("send: %v", err)
	}

	if strings.Contains(buf.String(), sensitive) {
		t.Fatalf("the body reached the info-level log:\n%s", buf.String())
	}

	recs := records(t, &buf)
	if len(recs) != 1 {
		t.Fatalf("expected exactly one record at info, got %d:\n%s", len(recs), buf.String())
	}

	rec := recs[0]
	for field, want := range map[string]any{
		"notification_id": msg.NotificationID.String(),
		"recipient_id":    msg.RecipientID.String(),
		"channel":         string(msg.Channel),
		"subject":         msg.Subject,
		"body_bytes":      float64(len(msg.Body)),
		"html":            true,
	} {
		if got := rec[field]; got != want {
			t.Errorf("%s = %v, want %v", field, got, want)
		}
	}
}

func TestLogSenderWritesTheBodyAtDebug(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	msg := message()
	if err := newLog(t, zerolog.New(&buf).Level(zerolog.DebugLevel), domain.ChannelEmail).Send(context.Background(), msg); err != nil {
		t.Fatalf("send: %v", err)
	}

	recs := records(t, &buf)
	if len(recs) != 2 {
		t.Fatalf("expected a summary and a body record, got %d:\n%s", len(recs), buf.String())
	}
	if recs[1]["body"] != msg.Body {
		t.Errorf("body = %v, want %q", recs[1]["body"], msg.Body)
	}
	if recs[1]["html_body"] != msg.HTMLBody {
		t.Errorf("html_body = %v, want %q", recs[1]["html_body"], msg.HTMLBody)
	}
}

// "html": false is not cosmetic — it is how an operator running the dev sender
// sees that a family ships no rich part, which is a normal state and not a
// failure.
func TestLogSenderReportsAMessageWithNoHTML(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	msg := message()
	msg.HTMLBody = ""
	if err := newLog(t, zerolog.New(&buf).Level(zerolog.InfoLevel), domain.ChannelInApp).Send(context.Background(), msg); err != nil {
		t.Fatalf("send: %v", err)
	}
	rec := records(t, &buf)[0]
	if html, ok := rec["html"].(bool); !ok || html {
		t.Errorf("html = %v, want false", rec["html"])
	}
}

// It is the always-succeeds channel: a row that reaches it always settles as
// sent, including for a message no renderer would produce.
func TestLogSenderAlwaysSucceeds(t *testing.T) {
	t.Parallel()

	s := newLog(t, zerolog.Nop(), domain.ChannelEmail)
	if err := s.Send(context.Background(), domain.RenderedMessage{}); err != nil {
		t.Errorf("empty message: %v", err)
	}

	// Cancelled too. Send touches no network, so there is nothing for a
	// cancellation to abandon, and returning ctx.Err() here would dead-letter
	// rows during a shutdown that the dispatcher is deliberately draining.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Send(ctx, message()); err != nil {
		t.Errorf("cancelled context: %v", err)
	}
}

func TestNewLogClaimsTheChannelItWasGiven(t *testing.T) {
	t.Parallel()

	for _, channel := range domain.Channels() {
		if got := newLog(t, zerolog.Nop(), channel).Channel(); got != channel {
			t.Errorf("Channel() = %q, want %q", got, channel)
		}
	}
}

// A sender filed under a channel that does not exist is invisible: the module
// builds, boots, and silently has no sender for the channel the operator
// thought they configured.
func TestNewLogRejectsAnUnknownChannel(t *testing.T) {
	t.Parallel()

	for _, channel := range []domain.Channel{"", "telegram", "EMAIL", "e-mail"} {
		if _, err := sender.NewLog(zerolog.Nop(), channel); err == nil {
			t.Errorf("NewLog accepted channel %q", channel)
		}
	}
}

func TestInAppSenderIsANoOpOnItsOwnChannel(t *testing.T) {
	t.Parallel()

	s := sender.NewInApp()
	if got := s.Channel(); got != domain.ChannelInApp {
		t.Errorf("Channel() = %q, want %q", got, domain.ChannelInApp)
	}

	// Nothing is transmitted, so nothing can fail: the row committed at
	// enqueue is the notification, and Send returning nil is what lets the
	// dispatch cycle settle it as sent — meaning "visible in the mailbox".
	if err := s.Send(context.Background(), message()); err != nil {
		t.Errorf("send: %v", err)
	}
	if err := s.Send(context.Background(), domain.RenderedMessage{}); err != nil {
		t.Errorf("send of a zero message: %v", err)
	}
}
