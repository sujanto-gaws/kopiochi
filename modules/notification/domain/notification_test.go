package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

var (
	testID        = uuid.MustParse("11111111-1111-4111-8111-111111111111")
	testRecipient = uuid.MustParse("22222222-2222-4222-8222-222222222222")
	testNow       = time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
)

func validParams() NewNotificationParams {
	return NewNotificationParams{
		ID:          testID,
		RecipientID: testRecipient,
		Channel:     ChannelEmail,
		Category:    CategorySecurity,
		TemplateKey: "security.password_changed",
		Payload:     map[string]any{"ip": "203.0.113.7"},
	}
}

func TestNewNotificationDefaults(t *testing.T) {
	p := validParams()
	p.IdempotencyKey = "pwchange:u1:e1"

	n, err := NewNotification(p, testNow)
	if err != nil {
		t.Fatalf("NewNotification: %v", err)
	}

	if n.Status != StatusPending {
		t.Errorf("Status = %q, want %q", n.Status, StatusPending)
	}
	if n.Attempts != 0 {
		t.Errorf("Attempts = %d, want 0", n.Attempts)
	}
	// Claimable immediately: the claim predicate is next_attempt_at <= now.
	if !n.NextAttemptAt.Equal(testNow) {
		t.Errorf("NextAttemptAt = %v, want %v", n.NextAttemptAt, testNow)
	}
	if !n.CreatedAt.Equal(testNow) {
		t.Errorf("CreatedAt = %v, want %v", n.CreatedAt, testNow)
	}
	if n.SentAt != nil || n.ReadAt != nil {
		t.Errorf("SentAt = %v, ReadAt = %v, want both nil", n.SentAt, n.ReadAt)
	}
	if n.LastError != "" {
		t.Errorf("LastError = %q, want empty", n.LastError)
	}
	if n.IdempotencyKey != "pwchange:u1:e1" {
		t.Errorf("IdempotencyKey = %q", n.IdempotencyKey)
	}
	if n.ID != testID || n.RecipientID != testRecipient {
		t.Errorf("ids not carried through: %v / %v", n.ID, n.RecipientID)
	}
	if got := n.Payload["ip"]; got != "203.0.113.7" {
		t.Errorf("Payload[ip] = %v", got)
	}
}

// A nil payload becomes an empty map so that every consumer — the renderer, the
// JSON column — sees a map rather than having to nil-check one.
func TestNewNotificationNilPayloadBecomesEmptyMap(t *testing.T) {
	p := validParams()
	p.Payload = nil

	n, err := NewNotification(p, testNow)
	if err != nil {
		t.Fatalf("NewNotification: %v", err)
	}
	if n.Payload == nil {
		t.Fatal("Payload = nil, want an empty map")
	}
	if len(n.Payload) != 0 {
		t.Fatalf("Payload = %v, want empty", n.Payload)
	}
}

func TestNewNotificationValidation(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*NewNotificationParams)
		wantMsg string
	}{
		{"missing id", func(p *NewNotificationParams) { p.ID = uuid.Nil }, "id is required"},
		{"missing recipient", func(p *NewNotificationParams) { p.RecipientID = uuid.Nil }, "recipient id is required"},
		{"unknown channel", func(p *NewNotificationParams) { p.Channel = "carrier-pigeon" }, "unknown channel"},
		{"empty channel", func(p *NewNotificationParams) { p.Channel = "" }, "unknown channel"},
		{"unknown category", func(p *NewNotificationParams) { p.Category = "marketing" }, "unknown category"},
		{"empty category", func(p *NewNotificationParams) { p.Category = "" }, "unknown category"},
		{"missing template key", func(p *NewNotificationParams) { p.TemplateKey = "" }, "template key is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := validParams()
			tt.mutate(&p)

			n, err := NewNotification(p, testNow)
			if !errors.Is(err, ErrInvalidNotification) {
				t.Fatalf("err = %v, want ErrInvalidNotification", err)
			}
			if n != nil {
				t.Fatalf("entity = %+v, want nil on error", n)
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Fatalf("error %q does not mention %q", err, tt.wantMsg)
			}
		})
	}
}

func TestChannelValid(t *testing.T) {
	for _, c := range Channels() {
		if !c.Valid() {
			t.Errorf("Channels() returned %q, which Valid() rejects", c)
		}
	}
	if len(Channels()) != 3 {
		t.Errorf("Channels() = %v, want 3 entries", Channels())
	}
	for _, c := range []Channel{"", "sms", "EMAIL"} {
		if c.Valid() {
			t.Errorf("Channel(%q).Valid() = true", c)
		}
	}
}

func TestCategoryValid(t *testing.T) {
	for _, c := range Categories() {
		if !c.Valid() {
			t.Errorf("Categories() returned %q, which Valid() rejects", c)
		}
	}
	if len(Categories()) != 3 {
		t.Errorf("Categories() = %v, want 3 entries", Categories())
	}
	for _, c := range []Category{"", "billing", "Security"} {
		if c.Valid() {
			t.Errorf("Category(%q).Valid() = true", c)
		}
	}
}

func TestMarkSent(t *testing.T) {
	n := &Notification{Status: StatusSending, LastError: "temporary failure, retrying"}

	if err := n.MarkSent(testNow); err != nil {
		t.Fatalf("MarkSent: %v", err)
	}
	if n.Status != StatusSent {
		t.Errorf("Status = %q, want %q", n.Status, StatusSent)
	}
	if n.SentAt == nil || !n.SentAt.Equal(testNow) {
		t.Errorf("SentAt = %v, want %v", n.SentAt, testNow)
	}
	// A stale error on a delivered row reads like a fault that never happened.
	if n.LastError != "" {
		t.Errorf("LastError = %q, want cleared", n.LastError)
	}
	if n.Attempts != 0 {
		t.Errorf("Attempts = %d: a success is not a failed attempt", n.Attempts)
	}
}

func TestMarkSentRejectedFromPending(t *testing.T) {
	n := &Notification{Status: StatusPending}

	err := n.MarkSent(testNow)
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("err = %v, want ErrInvalidTransition", err)
	}
	if n.SentAt != nil {
		t.Errorf("SentAt = %v, want nil after a refused transition", n.SentAt)
	}
	if n.Status != StatusPending {
		t.Errorf("Status = %q, want unchanged", n.Status)
	}
}

func TestMarkDeadFromSendingIsTheNonRetryablePath(t *testing.T) {
	n := &Notification{Status: StatusSending}

	if err := n.MarkDead("no sender registered for channel webhook"); err != nil {
		t.Fatalf("MarkDead: %v", err)
	}
	if n.Status != StatusDead {
		t.Errorf("Status = %q, want %q", n.Status, StatusDead)
	}
	if n.LastError != "no sender registered for channel webhook" {
		t.Errorf("LastError = %q", n.LastError)
	}
	// The row has left the retry schedule; the count is what an operator reads,
	// not something to inflate on the way out.
	if n.Attempts != 0 {
		t.Errorf("Attempts = %d, want 0", n.Attempts)
	}
}

func TestMarkDeadFromFailed(t *testing.T) {
	n := &Notification{Status: StatusFailed, Attempts: 6}

	if err := n.MarkDead("gave up"); err != nil {
		t.Fatalf("MarkDead: %v", err)
	}
	if n.Status != StatusDead {
		t.Errorf("Status = %q, want %q", n.Status, StatusDead)
	}
}

func TestMarkDeadRejectedFromTerminal(t *testing.T) {
	n := &Notification{Status: StatusSent, LastError: ""}

	if err := n.MarkDead("too late"); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("err = %v, want ErrInvalidTransition", err)
	}
	if n.LastError != "" {
		t.Errorf("LastError = %q, want untouched", n.LastError)
	}
}

func TestRecordFailureSchedulesRetry(t *testing.T) {
	n := &Notification{Status: StatusSending, Attempts: 1, NextAttemptAt: testNow}

	if err := n.RecordFailure(testNow, 90*time.Second, 6, "smtp: 421 service not available"); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}

	if n.Status != StatusPending {
		t.Errorf("Status = %q, want %q — a retryable failure returns to the claim queue", n.Status, StatusPending)
	}
	if n.Attempts != 2 {
		t.Errorf("Attempts = %d, want 2", n.Attempts)
	}
	want := testNow.Add(90 * time.Second)
	if !n.NextAttemptAt.Equal(want) {
		t.Errorf("NextAttemptAt = %v, want %v", n.NextAttemptAt, want)
	}
	if n.LastError != "smtp: 421 service not available" {
		t.Errorf("LastError = %q", n.LastError)
	}
}

func TestRecordFailureDeadLettersOnExhaustion(t *testing.T) {
	// Fifth failure of a six-attempt budget: still retrying.
	n := &Notification{Status: StatusSending, Attempts: 4}
	if err := n.RecordFailure(testNow, time.Minute, 6, "boom"); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}
	if n.Status != StatusPending {
		t.Fatalf("Attempts 5 of 6: Status = %q, want %q", n.Status, StatusPending)
	}

	// Sixth: the budget is gone.
	n = &Notification{Status: StatusSending, Attempts: 5}
	if err := n.RecordFailure(testNow, time.Minute, 6, "boom"); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}
	if n.Status != StatusDead {
		t.Fatalf("Attempts 6 of 6: Status = %q, want %q", n.Status, StatusDead)
	}
	if n.Attempts != 6 {
		t.Errorf("Attempts = %d, want 6", n.Attempts)
	}
	// Nothing schedules a dead row, so the retry stamp must stay untouched.
	if !n.NextAttemptAt.IsZero() {
		t.Errorf("NextAttemptAt = %v, want unset on a dead row", n.NextAttemptAt)
	}
}

// maxAttempts is a parameter, so an operator who tunes it to 1 gets a single
// attempt and no retry. Driving it from the test rather than a constant is the
// point of the parameter.
func TestRecordFailureMaxAttemptsIsCallerSupplied(t *testing.T) {
	for _, max := range []int{-1, 0, 1} {
		n := &Notification{Status: StatusSending}
		if err := n.RecordFailure(testNow, time.Minute, max, "boom"); err != nil {
			t.Fatalf("maxAttempts=%d: %v", max, err)
		}
		if n.Status != StatusDead {
			t.Errorf("maxAttempts=%d: Status = %q, want %q", max, n.Status, StatusDead)
		}
	}

	n := &Notification{Status: StatusSending}
	if err := n.RecordFailure(testNow, time.Minute, 2, "boom"); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}
	if n.Status != StatusPending {
		t.Errorf("maxAttempts=2, first failure: Status = %q, want %q", n.Status, StatusPending)
	}
}

func TestRecordFailureRejectedOutsideSending(t *testing.T) {
	for _, from := range []Status{StatusPending, StatusFailed, StatusSent, StatusDead} {
		n := &Notification{Status: from, Attempts: 3, NextAttemptAt: testNow}

		err := n.RecordFailure(testNow.Add(time.Hour), time.Minute, 6, "boom")
		if !errors.Is(err, ErrInvalidTransition) {
			t.Errorf("from %q: err = %v, want ErrInvalidTransition", from, err)
		}
		if n.Attempts != 3 {
			t.Errorf("from %q: Attempts = %d, want untouched", from, n.Attempts)
		}
		if !n.NextAttemptAt.Equal(testNow) {
			t.Errorf("from %q: NextAttemptAt moved on a refused transition", from)
		}
	}
}

func TestLastErrorIsTruncated(t *testing.T) {
	long := strings.Repeat("x", maxLastErrorLen+200)

	n := &Notification{Status: StatusSending}
	if err := n.RecordFailure(testNow, time.Minute, 6, long); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}
	if len(n.LastError) != maxLastErrorLen {
		t.Fatalf("len(LastError) = %d, want %d", len(n.LastError), maxLastErrorLen)
	}

	d := &Notification{Status: StatusSending}
	if err := d.MarkDead(long); err != nil {
		t.Fatalf("MarkDead: %v", err)
	}
	if len(d.LastError) != maxLastErrorLen {
		t.Fatalf("MarkDead: len(LastError) = %d, want %d", len(d.LastError), maxLastErrorLen)
	}
}

// A reason exactly at the limit is kept whole; nothing shorter is touched.
func TestLastErrorShorterThanLimitIsUntouched(t *testing.T) {
	exact := strings.Repeat("y", maxLastErrorLen)

	n := &Notification{Status: StatusSending}
	if err := n.MarkDead(exact); err != nil {
		t.Fatalf("MarkDead: %v", err)
	}
	if n.LastError != exact {
		t.Fatalf("len(LastError) = %d, want %d unchanged", len(n.LastError), maxLastErrorLen)
	}
}

// Truncation must not split a rune: a half-character is stored, logged and then
// rendered as U+FFFD, which looks like corruption to whoever is reading the
// failure.
func TestLastErrorTruncationKeepsRunesWhole(t *testing.T) {
	// 511 ASCII bytes then a 3-byte rune, so the byte cut lands mid-rune.
	reason := strings.Repeat("a", maxLastErrorLen-1) + "☃" + "tail"

	n := &Notification{Status: StatusSending}
	if err := n.MarkDead(reason); err != nil {
		t.Fatalf("MarkDead: %v", err)
	}
	if !utf8.ValidString(n.LastError) {
		t.Fatalf("LastError is not valid UTF-8: %q", n.LastError)
	}
	if len(n.LastError) != maxLastErrorLen-1 {
		t.Fatalf("len(LastError) = %d, want %d (the partial rune dropped)", len(n.LastError), maxLastErrorLen-1)
	}
}
