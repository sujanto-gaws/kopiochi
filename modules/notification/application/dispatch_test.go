package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	domain "github.com/sujanto-gaws/kopiochi/modules/notification/domain"
)

// Every claimed row must be delivered through the sender registered for its own
// channel, and the message must be addressed from the row rather than from
// whatever the renderer put in the routing fields.
func TestDispatchBatchDeliversOnEveryChannel(t *testing.T) {
	for _, ch := range domain.Channels() {
		t.Run(string(ch), func(t *testing.T) {
			h := newHarness(t)
			row := h.enqueue(t, ch, domain.CategoryAccount, "")

			claimed, err := h.svc.DispatchBatch(context.Background())
			if err != nil {
				t.Fatalf("DispatchBatch: %v", err)
			}
			if claimed != 1 {
				t.Fatalf("claimed = %d, want 1", claimed)
			}

			settled := h.notifications.find(row.ID)
			if settled.Status != domain.StatusSent {
				t.Errorf("status = %q, want %q (last error %q)", settled.Status, domain.StatusSent, settled.LastError)
			}
			if settled.SentAt == nil || !settled.SentAt.Equal(testNow) {
				t.Errorf("SentAt = %v, want %v", settled.SentAt, testNow)
			}
			if settled.Attempts != 0 {
				t.Errorf("Attempts = %d, want 0: a successful send is not an attempt", settled.Attempts)
			}
			if settled.LastError != "" {
				t.Errorf("LastError = %q, want empty", settled.LastError)
			}

			sender := h.senders[ch]
			if len(sender.sent) != 1 {
				t.Fatalf("sender for %q received %d messages, want 1", ch, len(sender.sent))
			}
			msg := sender.sent[0]
			if msg.NotificationID != row.ID || msg.RecipientID != testUser || msg.Channel != ch {
				t.Errorf("routing = {%s, %s, %s}, want {%s, %s, %s}: the row's routing must overwrite the renderer's",
					msg.NotificationID, msg.RecipientID, msg.Channel, row.ID, testUser, ch)
			}
			if want := "subject:security.password_changed"; msg.Subject != want {
				t.Errorf("Subject = %q, want %q", msg.Subject, want)
			}
			if want := fmt.Sprintf("body:security.password_changed:%s", ch); msg.Body != want {
				t.Errorf("Body = %q, want %q", msg.Body, want)
			}

			for other, s := range h.senders {
				if other != ch && len(s.sent) != 0 {
					t.Errorf("sender for %q received %d messages, want none", other, len(s.sent))
				}
			}

			// A sent row is terminal: the next cycle must not find it again.
			if again, err := h.svc.DispatchBatch(context.Background()); err != nil || again != 0 {
				t.Errorf("second DispatchBatch = (%d, %v), want (0, nil)", again, err)
			}
		})
	}
}

func TestDispatchBatchClaimsWithTheConfiguredSizeAndTheInjectedClock(t *testing.T) {
	h := newHarness(t)
	h.clock.advance(time.Hour)

	if _, err := h.svc.DispatchBatch(context.Background()); err != nil {
		t.Fatalf("DispatchBatch: %v", err)
	}

	if len(h.notifications.claims) != 1 {
		t.Fatalf("ClaimBatch called %d times, want 1", len(h.notifications.claims))
	}
	got := h.notifications.claims[0]
	if got.n != testDispatchConfig.BatchSize {
		t.Errorf("claimed n = %d, want the configured %d", got.n, testDispatchConfig.BatchSize)
	}
	if want := testNow.Add(time.Hour); !got.now.Equal(want) {
		t.Errorf("claim now = %v, want the injected clock's %v", got.now, want)
	}
}

// The literal retry ladder. With base 30s and no jitter the delays are
// 30s, 60s, 120s — not 60s, 120s, 240s, which is what this looks like when the
// backoff is computed after RecordFailure has already incremented Attempts.
// Asserting the exact second is the only way to catch that off-by-one.
func TestDispatchBatchSchedulesTheLiteralRetryLadder(t *testing.T) {
	h := newHarness(t)
	h.senders[domain.ChannelEmail].err = errors.New("smtp: connection reset by peer")
	row := h.enqueue(t, domain.ChannelEmail, domain.CategorySecurity, "")

	ladder := []time.Duration{30 * time.Second, 60 * time.Second, 120 * time.Second}
	for i, delay := range ladder {
		attemptedAt := h.clock.Now()

		claimed, err := h.svc.DispatchBatch(context.Background())
		if err != nil {
			t.Fatalf("attempt %d: DispatchBatch: %v", i+1, err)
		}
		if claimed != 1 {
			t.Fatalf("attempt %d: claimed = %d, want 1", i+1, claimed)
		}

		settled := h.notifications.find(row.ID)
		if settled.Status != domain.StatusPending {
			t.Fatalf("attempt %d: status = %q, want %q", i+1, settled.Status, domain.StatusPending)
		}
		if settled.Attempts != i+1 {
			t.Errorf("attempt %d: Attempts = %d, want %d", i+1, settled.Attempts, i+1)
		}
		if want := attemptedAt.Add(delay); !settled.NextAttemptAt.Equal(want) {
			t.Errorf("attempt %d: NextAttemptAt = %v, want %v (delay %v, got %v)",
				i+1, settled.NextAttemptAt, want, delay, settled.NextAttemptAt.Sub(attemptedAt))
		}
		if !strings.Contains(settled.LastError, "connection reset by peer") {
			t.Errorf("attempt %d: LastError = %q, want the sender's reason", i+1, settled.LastError)
		}

		// Not due until the scheduled moment.
		if early, err := h.svc.DispatchBatch(context.Background()); err != nil || early != 0 {
			t.Fatalf("attempt %d: a row scheduled for later was claimed: (%d, %v)", i+1, early, err)
		}
		h.clock.advance(delay)
	}

	// The fourth failure spends the last of MaxAttempts=4.
	if _, err := h.svc.DispatchBatch(context.Background()); err != nil {
		t.Fatalf("final DispatchBatch: %v", err)
	}
	if settled := h.notifications.find(row.ID); settled.Status != domain.StatusDead {
		t.Errorf("status after the last attempt = %q, want %q", settled.Status, domain.StatusDead)
	}
}

// The retry budget is the only thing stopping a permanently broken recipient
// from being retried forever.
func TestDispatchBatchDeadLettersOnAttemptExhaustion(t *testing.T) {
	cfg := testDispatchConfig
	cfg.MaxAttempts = 2

	h := newHarnessWith(t, domain.Channels(), nil, cfg)
	h.senders[domain.ChannelWebhook].err = errors.New("dial tcp: i/o timeout")
	row := h.enqueue(t, domain.ChannelWebhook, domain.CategorySystem, "")

	if _, err := h.svc.DispatchBatch(context.Background()); err != nil {
		t.Fatalf("first DispatchBatch: %v", err)
	}
	if settled := h.notifications.find(row.ID); settled.Status != domain.StatusPending || settled.Attempts != 1 {
		t.Fatalf("after one failure: status = %q, attempts = %d, want pending/1", settled.Status, settled.Attempts)
	}

	h.clock.advance(cfg.BackoffBase)
	if _, err := h.svc.DispatchBatch(context.Background()); err != nil {
		t.Fatalf("second DispatchBatch: %v", err)
	}

	settled := h.notifications.find(row.ID)
	if settled.Status != domain.StatusDead {
		t.Fatalf("status = %q, want %q once the budget is spent", settled.Status, domain.StatusDead)
	}
	if settled.Attempts != 2 {
		t.Errorf("Attempts = %d, want 2", settled.Attempts)
	}
	if !strings.Contains(settled.LastError, "i/o timeout") {
		t.Errorf("LastError = %q, want the last failure's reason", settled.LastError)
	}
	if settled.SentAt != nil {
		t.Errorf("SentAt = %v, want nil on a dead row", settled.SentAt)
	}

	// Terminal: nothing claims it again.
	h.clock.advance(24 * time.Hour)
	if again, err := h.svc.DispatchBatch(context.Background()); err != nil || again != 0 {
		t.Errorf("DispatchBatch after death = (%d, %v), want (0, nil)", again, err)
	}
}

// Three ways to fail without a retry. Each is a bug that another attempt cannot
// fix, so the row must not spend its budget re-discovering it.
func TestDispatchBatchDeadLettersWithoutRetrying(t *testing.T) {
	t.Run("the sender says the failure is permanent", func(t *testing.T) {
		h := newHarness(t)
		h.senders[domain.ChannelEmail].err = fmt.Errorf("%w: 550 mailbox unavailable", domain.ErrNonRetryable)
		row := h.enqueue(t, domain.ChannelEmail, domain.CategorySecurity, "")

		if _, err := h.svc.DispatchBatch(context.Background()); err != nil {
			t.Fatalf("DispatchBatch: %v", err)
		}

		settled := h.notifications.find(row.ID)
		if settled.Status != domain.StatusDead {
			t.Fatalf("status = %q, want %q", settled.Status, domain.StatusDead)
		}
		if settled.Attempts != 0 {
			t.Errorf("Attempts = %d, want 0: a row that skipped the retry path spent no budget", settled.Attempts)
		}
		if !strings.Contains(settled.LastError, "550 mailbox unavailable") {
			t.Errorf("LastError = %q, want the sender's reason", settled.LastError)
		}
	})

	t.Run("no sender is registered for the channel", func(t *testing.T) {
		// A deployment with email switched off. The row is a configuration
		// mismatch, not a transient outage.
		//
		// Seeded directly rather than through Enqueue: since E13, Enqueue
		// refuses an unroutable channel outright, so this row can only arise
		// the way it does in production — queued while the sender was wired,
		// drained after it was removed. Routing it through Enqueue here would
		// test the new guard and quietly stop covering the dispatcher's
		// fail-closed handling, which is what this case is for.
		h := newHarnessWith(t, []domain.Channel{domain.ChannelInApp}, nil, testDispatchConfig)
		row := h.seedRow(t, domain.ChannelEmail, domain.CategorySecurity)

		if _, err := h.svc.DispatchBatch(context.Background()); err != nil {
			t.Fatalf("DispatchBatch: %v", err)
		}

		settled := h.notifications.find(row.ID)
		if settled.Status != domain.StatusDead {
			t.Fatalf("status = %q, want %q", settled.Status, domain.StatusDead)
		}
		if !strings.Contains(settled.LastError, `no sender registered for channel "email"`) {
			t.Errorf("LastError = %q, want it to name the missing sender", settled.LastError)
		}
		if len(h.renderer.calls) != 0 {
			t.Errorf("renderer called %d times, want none: there is nobody to send to", len(h.renderer.calls))
		}
	})

	t.Run("the template cannot be rendered", func(t *testing.T) {
		h := newHarness(t)
		h.renderer.errFor = map[string]error{"security.password_changed": errors.New("no template for key")}
		row := h.enqueue(t, domain.ChannelInApp, domain.CategorySecurity, "")

		if _, err := h.svc.DispatchBatch(context.Background()); err != nil {
			t.Fatalf("DispatchBatch: %v", err)
		}

		settled := h.notifications.find(row.ID)
		if settled.Status != domain.StatusDead {
			t.Fatalf("status = %q, want %q", settled.Status, domain.StatusDead)
		}
		if !strings.Contains(settled.LastError, "no template for key") {
			t.Errorf("LastError = %q, want the renderer's reason", settled.LastError)
		}
		if sent := h.senders[domain.ChannelInApp].sent; len(sent) != 0 {
			t.Errorf("sender received %d messages, want none: nothing was rendered", len(sent))
		}
	})
}

// A panic is not a classification, so it takes the retryable path: the row
// survives for as long as its budget lasts, which is the window in which a fix
// can ship. Dead-lettering instead would turn one bug in one sender into
// permanently destroyed notifications.
func TestDispatchBatchTreatsAPanicAsARetryableFailure(t *testing.T) {
	cases := []struct {
		name  string
		arm   func(h *harness)
		inErr string
	}{
		{
			name:  "in the sender",
			arm:   func(h *harness) { h.senders[domain.ChannelInApp].panicWith = "assignment to entry in nil map" },
			inErr: "assignment to entry in nil map",
		},
		{
			name:  "in the renderer",
			arm:   func(h *harness) { h.renderer.panicWith = "index out of range" },
			inErr: "index out of range",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			tc.arm(h)
			row := h.enqueue(t, domain.ChannelInApp, domain.CategoryAccount, "")

			claimed, err := h.svc.DispatchBatch(context.Background())
			if err != nil {
				t.Fatalf("DispatchBatch: %v", err)
			}
			if claimed != 1 {
				t.Fatalf("claimed = %d, want 1", claimed)
			}

			settled := h.notifications.find(row.ID)
			if settled.Status != domain.StatusPending {
				t.Fatalf("status = %q, want %q", settled.Status, domain.StatusPending)
			}
			if settled.Attempts != 1 {
				t.Errorf("Attempts = %d, want 1", settled.Attempts)
			}
			if want := testNow.Add(30 * time.Second); !settled.NextAttemptAt.Equal(want) {
				t.Errorf("NextAttemptAt = %v, want %v", settled.NextAttemptAt, want)
			}
			if !strings.Contains(settled.LastError, "panic during delivery") || !strings.Contains(settled.LastError, tc.inErr) {
				t.Errorf("LastError = %q, want it to record the panic", settled.LastError)
			}
		})
	}
}

// One bad row costs one row. Rows claimed after it are still settled, and the
// failure is reported rather than swallowed.
func TestDispatchBatchIsolatesOneBadRowFromTheRest(t *testing.T) {
	saveFailed := errors.New("connection closed")

	cases := []struct {
		name  string
		arm   func(h *harness, victim *domain.Notification)
		inErr string
	}{
		{
			name: "the save fails",
			arm: func(h *harness, victim *domain.Notification) {
				h.notifications.saveErrs[victim.ID] = saveFailed
			},
			inErr: "connection closed",
		},
		{
			name: "the save panics",
			arm: func(h *harness, victim *domain.Notification) {
				h.notifications.savePanics[victim.ID] = "sql: database is closed"
			},
			inErr: "panic settling notification: sql: database is closed",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)

			rows := make([]*domain.Notification, 0, 4)
			for i := 0; i < 4; i++ {
				rows = append(rows, h.enqueue(t, domain.ChannelInApp, domain.CategoryAccount, fmt.Sprintf("row-%d", i)))
			}
			victim := rows[1]
			tc.arm(h, victim)

			claimed, err := h.svc.DispatchBatch(context.Background())
			if claimed != 4 {
				t.Fatalf("claimed = %d, want 4", claimed)
			}
			if err == nil {
				t.Fatal("DispatchBatch returned no error, want the bad row reported")
			}
			if !strings.Contains(err.Error(), victim.ID.String()) || !strings.Contains(err.Error(), tc.inErr) {
				t.Errorf("error = %v, want it to name row %s and the cause", err, victim.ID)
			}

			for i, row := range rows {
				settled := h.notifications.find(row.ID)
				if row.ID == victim.ID {
					// Left claimed: settling it is exactly what failed.
					if settled.Status != domain.StatusSending {
						t.Errorf("row %d: status = %q, want %q", i, settled.Status, domain.StatusSending)
					}
					continue
				}
				if settled.Status != domain.StatusSent {
					t.Errorf("row %d: status = %q, want %q — one bad row must not abandon the others", i, settled.Status, domain.StatusSent)
				}
			}

			if len(h.notifications.savedIDs) != 4 {
				t.Errorf("Save called %d times, want 4", len(h.notifications.savedIDs))
			}
		})
	}
}

// A claim that hands back something no repository should produce — a nil, or a
// row that is not in sending and so cannot be settled — must cost that row and
// nothing else. The alternative is a nil dereference or a silent corruption
// taking the rest of the batch with it.
func TestDispatchBatchSurvivesAMalformedClaim(t *testing.T) {
	terminal := func(status domain.Status) *domain.Notification {
		return &domain.Notification{
			ID:          uuid.MustParse("77777777-7777-4777-8777-777777777777"),
			RecipientID: testUser,
			Channel:     domain.ChannelInApp,
			Category:    domain.CategoryAccount,
			TemplateKey: "system.maintenance",
			Status:      status,
		}
	}

	cases := []struct {
		name  string
		arm   func(h *harness)
		inErr string
	}{
		{
			name:  "a nil row",
			arm:   func(h *harness) { h.notifications.injectRows = []*domain.Notification{nil} },
			inErr: "nil notification",
		},
		{
			// Cannot be marked sent: sent -> sent is not in the table.
			name:  "a row that is already sent",
			arm:   func(h *harness) { h.notifications.injectRows = []*domain.Notification{terminal(domain.StatusSent)} },
			inErr: "mark sent",
		},
		{
			// Cannot be marked dead either: dead is terminal.
			name: "a dead row, on the non-retryable path",
			arm: func(h *harness) {
				h.notifications.injectRows = []*domain.Notification{terminal(domain.StatusDead)}
				h.senders[domain.ChannelInApp].errs = []error{fmt.Errorf("%w: gone", domain.ErrNonRetryable)}
			},
			inErr: "mark dead",
		},
		{
			// Cannot be failed: pending -> failed is not in the table.
			name: "a pending row, on the retry path",
			arm: func(h *harness) {
				h.notifications.injectRows = []*domain.Notification{terminal(domain.StatusPending)}
				h.senders[domain.ChannelInApp].errs = []error{errors.New("temporary")}
			},
			inErr: "record failure",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			tc.arm(h)
			row := h.enqueue(t, domain.ChannelInApp, domain.CategoryAccount, "")

			claimed, err := h.svc.DispatchBatch(context.Background())
			if claimed != 2 {
				t.Fatalf("claimed = %d, want 2 (the malformed row and the real one)", claimed)
			}
			if err == nil || !strings.Contains(err.Error(), tc.inErr) {
				t.Fatalf("error = %v, want it to report %q", err, tc.inErr)
			}
			if settled := h.notifications.find(row.ID); settled.Status != domain.StatusSent {
				t.Errorf("status = %q, want %q — the good row must still be delivered", settled.Status, domain.StatusSent)
			}
		})
	}
}

func TestDispatchBatchReportsAFailedClaim(t *testing.T) {
	h := newHarness(t)
	boom := errors.New("pool exhausted")
	h.notifications.claimErr = boom

	claimed, err := h.svc.DispatchBatch(context.Background())
	if claimed != 0 {
		t.Errorf("claimed = %d, want 0", claimed)
	}
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want it to wrap the repository's", err)
	}
}

func TestDispatchBatchOnAnEmptyOutbox(t *testing.T) {
	h := newHarness(t)

	claimed, err := h.svc.DispatchBatch(context.Background())
	if claimed != 0 || err != nil {
		t.Fatalf("DispatchBatch = (%d, %v), want (0, nil)", claimed, err)
	}
}

// The injected jitter source reaches domain.BackoffWithJitter, which spreads a
// delay over [d/2, d) — inside the cap, never on top of it. With base 30s: a
// draw of 0 schedules at 15s, and 0.5 at 22.5s.
func TestDispatchBatchAppliesTheInjectedJitter(t *testing.T) {
	cases := []struct {
		draw float64
		want time.Duration
	}{
		{draw: 0, want: 15 * time.Second},
		{draw: 0.5, want: 22500 * time.Millisecond},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("draw %v", tc.draw), func(t *testing.T) {
			h := newHarnessWith(t, domain.Channels(), fixedJitter{f: tc.draw}, testDispatchConfig)
			h.senders[domain.ChannelWebhook].err = errors.New("502 bad gateway")
			row := h.enqueue(t, domain.ChannelWebhook, domain.CategorySystem, "")

			if _, err := h.svc.DispatchBatch(context.Background()); err != nil {
				t.Fatalf("DispatchBatch: %v", err)
			}

			settled := h.notifications.find(row.ID)
			if got := settled.NextAttemptAt.Sub(testNow); got != tc.want {
				t.Errorf("retry delay = %v, want %v", got, tc.want)
			}
		})
	}
}

// A retry that succeeds settles as sent, keeping the attempt it already spent
// and dropping the error that no longer applies.
func TestDispatchBatchSendsOnASecondAttempt(t *testing.T) {
	h := newHarness(t)
	h.senders[domain.ChannelEmail].errs = []error{errors.New("smtp: temporary failure")}
	row := h.enqueue(t, domain.ChannelEmail, domain.CategoryAccount, "")

	if _, err := h.svc.DispatchBatch(context.Background()); err != nil {
		t.Fatalf("first DispatchBatch: %v", err)
	}
	h.clock.advance(30 * time.Second)
	if _, err := h.svc.DispatchBatch(context.Background()); err != nil {
		t.Fatalf("second DispatchBatch: %v", err)
	}

	settled := h.notifications.find(row.ID)
	if settled.Status != domain.StatusSent {
		t.Fatalf("status = %q, want %q", settled.Status, domain.StatusSent)
	}
	if settled.Attempts != 1 {
		t.Errorf("Attempts = %d, want the failure it already spent", settled.Attempts)
	}
	if settled.LastError != "" {
		t.Errorf("LastError = %q, want it cleared on success", settled.LastError)
	}
	if settled.SentAt == nil || !settled.SentAt.Equal(testNow.Add(30*time.Second)) {
		t.Errorf("SentAt = %v, want %v", settled.SentAt, testNow.Add(30*time.Second))
	}
}

// Observer tests — E12.
//
// What is worth pinning is not that a hook fires. It is that the three things
// DispatchBatch's (int, error) return could never carry now reach a bystander:
// which channel, which outcome, and — the case that pays for the port — a
// security-category dead-letter, which is the event an audit trail exists for.

func TestObserverSeesTheOutcomeOfEverySettledRow(t *testing.T) {
	h := newHarness(t)
	sent := h.enqueue(t, domain.ChannelEmail, domain.CategoryAccount, "ok")

	h.senders[domain.ChannelInApp].err = fmt.Errorf("%w: gone", domain.ErrNonRetryable)
	dead := h.enqueue(t, domain.ChannelInApp, domain.CategorySecurity, "dead")

	if _, err := h.svc.DispatchBatch(context.Background()); err != nil {
		t.Fatalf("DispatchBatch: %v", err)
	}

	calls := h.observer.calls()
	if len(calls) != 2 {
		t.Fatalf("observer saw %d rows, want 2", len(calls))
	}

	byID := map[uuid.UUID]settledCall{}
	for _, c := range calls {
		byID[c.id] = c
	}

	if got := byID[sent.ID]; got.outcome != domain.StatusSent || got.err != nil {
		t.Errorf("sent row reported as outcome=%q err=%v, want sent and no error", got.outcome, got.err)
	}
	if got := byID[sent.ID].channel; got != domain.ChannelEmail {
		t.Errorf("channel = %q, want email: per-channel counters are the point", got)
	}

	// The dead-letter is the one an auditor has to be able to find, and the
	// category is what makes it findable.
	got := byID[dead.ID]
	if got.outcome != domain.StatusDead {
		t.Errorf("dead row reported as %q, want dead", got.outcome)
	}
	if got.err == nil || !errors.Is(got.err, domain.ErrNonRetryable) {
		t.Errorf("dead row reported err=%v, want the non-retryable cause", got.err)
	}
}

// TestObserverIsNotToldAboutARowThatFailedToSave: a row whose Save failed has
// not settled. Reporting it would mean an audit event for a dead-letter that
// never happened, and a counter that disagrees with the table.
func TestObserverIsNotToldAboutARowThatFailedToSave(t *testing.T) {
	h := newHarness(t)
	victim := h.enqueue(t, domain.ChannelInApp, domain.CategoryAccount, "doomed")
	h.notifications.saveErrs[victim.ID] = errors.New("connection closed")

	if _, err := h.svc.DispatchBatch(context.Background()); err == nil {
		t.Fatal("DispatchBatch returned no error, want the failed save reported")
	}

	for _, c := range h.observer.calls() {
		if c.id == victim.ID {
			t.Fatalf("observer was told a row settled whose Save failed: %+v", c)
		}
	}
}

// TestObserverSeesAPanicBelowSettle is BL25. A panic there never reaches the
// normal report, and as one entry in a joined error it is indistinguishable
// from an ordinary delivery failure. The sentinel is what an observer keys on.
func TestObserverSeesAPanicBelowSettle(t *testing.T) {
	h := newHarness(t)
	victim := h.enqueue(t, domain.ChannelInApp, domain.CategoryAccount, "panics")
	h.notifications.savePanics[victim.ID] = "sql: database is closed"

	if _, err := h.svc.DispatchBatch(context.Background()); err == nil {
		t.Fatal("DispatchBatch returned no error, want the panic reported")
	}

	var seen *settledCall
	for _, c := range h.observer.calls() {
		if c.id == victim.ID {
			seen = &c
			break
		}
	}
	if seen == nil {
		t.Fatal("the panicking row was never reported: BL25's gap is still open")
	}
	if !errors.Is(seen.err, ErrPanicked) {
		t.Errorf("reported err=%v, want it to wrap ErrPanicked so a panic is not "+
			"indistinguishable from a delivery failure", seen.err)
	}
}

// TestNilObserverIsANoOp: the port is optional, and every existing caller
// passes nil. If that panicked, the option would not be one.
func TestNilObserverIsANoOp(t *testing.T) {
	h := newHarnessWith(t, []domain.Channel{domain.ChannelInApp}, nil, testDispatchConfig)
	h.observer = nil

	svc, err := NewService(h.notifications, h.preferences, h.renderer,
		[]ChannelSender{h.senders[domain.ChannelInApp]}, h.clock, nil, nil, testDispatchConfig)
	if err != nil {
		t.Fatalf("NewService with a nil observer: %v", err)
	}
	h.svc = svc

	h.enqueue(t, domain.ChannelInApp, domain.CategoryAccount, "quiet")
	if _, err := svc.DispatchBatch(context.Background()); err != nil {
		t.Fatalf("DispatchBatch with a nil observer: %v", err)
	}
}
