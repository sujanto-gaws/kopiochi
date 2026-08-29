package application

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	domain "github.com/sujanto-gaws/kopiochi/modules/notification/domain"
)

func TestNewServiceRefusesAnIncompleteDependencySet(t *testing.T) {
	var (
		repo     = newFakeNotificationRepo()
		prefs    = &fakePreferenceRepo{}
		renderer = &fakeRenderer{}
		clock    = newFixedClock(testNow)
	)

	tests := []struct {
		name    string
		build   func() (*Service, error)
		wantMsg string
	}{
		{
			name:    "no notification repository",
			build:   func() (*Service, error) { return NewService(nil, prefs, renderer, nil, clock, nil, testDispatchConfig) },
			wantMsg: "notification repository is required",
		},
		{
			name:    "no preference repository",
			build:   func() (*Service, error) { return NewService(repo, nil, renderer, nil, clock, nil, testDispatchConfig) },
			wantMsg: "preference repository is required",
		},
		{
			name:    "no renderer",
			build:   func() (*Service, error) { return NewService(repo, prefs, nil, nil, clock, nil, testDispatchConfig) },
			wantMsg: "template renderer is required",
		},
		{
			// The one dependency whose absence would otherwise surface as a nil
			// dereference inside a background worker hours after boot.
			name:    "no clock",
			build:   func() (*Service, error) { return NewService(repo, prefs, renderer, nil, nil, nil, testDispatchConfig) },
			wantMsg: "clock is required",
		},
		{
			name: "a nil sender",
			build: func() (*Service, error) {
				return NewService(repo, prefs, renderer, []ChannelSender{nil}, clock, nil, testDispatchConfig)
			},
			wantMsg: "sender 0 is nil",
		},
		{
			name: "a sender for an unknown channel",
			build: func() (*Service, error) {
				return NewService(repo, prefs, renderer, []ChannelSender{&fakeSender{ch: "carrier-pigeon"}}, clock, nil, testDispatchConfig)
			},
			wantMsg: `sender 0 claims unknown channel "carrier-pigeon"`,
		},
		{
			name: "two senders for one channel",
			build: func() (*Service, error) {
				senders := []ChannelSender{
					&fakeSender{ch: domain.ChannelEmail},
					&fakeSender{ch: domain.ChannelEmail},
				}
				return NewService(repo, prefs, renderer, senders, clock, nil, testDispatchConfig)
			},
			wantMsg: `two senders registered for channel "email"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, err := tt.build()
			if err == nil {
				t.Fatal("NewService succeeded, want an error")
			}
			if svc != nil {
				t.Errorf("service = %v, want nil alongside the error", svc)
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error = %q, want it to mention %q", err, tt.wantMsg)
			}
		})
	}
}

// No senders at all is a legal deployment — email switched off, say — and not a
// boot failure. Rows for the missing channel dead-letter at dispatch with a
// reason an operator can read.
func TestNewServiceAcceptsNoSenders(t *testing.T) {
	svc, err := NewService(newFakeNotificationRepo(), &fakePreferenceRepo{}, &fakeRenderer{}, nil, newFixedClock(testNow), nil, testDispatchConfig)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if svc == nil {
		t.Fatal("service = nil")
	}
}

// TestEnqueueRefusesAnUnroutableChannel — E13.
//
// domain.Channel accepts email, inapp and webhook, but only a channel with a
// registered sender can be delivered. Before this guard, enqueueing one of the
// others was reported as SUCCESS and the row then died at the dispatch gate
// without a single attempt: a silent drop wearing the costume of a durable
// outbox. E13 found it as "every in-app notification dies at the D6 gate"; the
// shape is general, and webhook would have repeated it verbatim.
//
// The row must not be written. A refused enqueue that still queues something is
// the worst of both: an error for the caller AND a corpse for an operator.
func TestEnqueueRefusesAnUnroutableChannel(t *testing.T) {
	h := newHarnessWith(t, []domain.Channel{domain.ChannelInApp}, nil, testDispatchConfig)

	err := h.svc.Enqueue(context.Background(), EnqueueRequest{
		RecipientID: testUser,
		Channel:     domain.ChannelEmail,
		Category:    domain.CategorySecurity,
		TemplateKey: "security.password_changed",
	})

	if !errors.Is(err, ErrChannelNotRoutable) {
		t.Fatalf("Enqueue error = %v, want ErrChannelNotRoutable", err)
	}
	if !strings.Contains(err.Error(), "email") {
		t.Errorf("error = %q, want it to name the channel", err)
	}
	if got := h.notifications.count(); got != 0 {
		t.Errorf("outbox holds %d rows after a refused enqueue, want 0", got)
	}
}

// TestEnqueueChecksRoutabilityBeforePreferences: an unroutable channel is a
// broken contract between this service and whoever wired it, and it stays
// broken whether or not this user wanted the message. Reporting "delivered
// nothing, successfully" because the recipient had it switched off would hide a
// misconfiguration behind a user's setting.
func TestEnqueueChecksRoutabilityBeforePreferences(t *testing.T) {
	h := newHarnessWith(t, []domain.Channel{domain.ChannelInApp}, nil, testDispatchConfig)
	h.preferences.prefs = []domain.Preference{{
		UserID: testUser, Channel: domain.ChannelEmail,
		Category: domain.CategorySecurity, Enabled: false,
	}}

	err := h.svc.Enqueue(context.Background(), EnqueueRequest{
		RecipientID: testUser,
		Channel:     domain.ChannelEmail,
		Category:    domain.CategorySecurity,
		TemplateKey: "security.password_changed",
	})

	if !errors.Is(err, ErrChannelNotRoutable) {
		t.Fatalf("Enqueue error = %v, want ErrChannelNotRoutable even when the user disabled the channel", err)
	}
}

func TestEnqueueWritesAPendingRow(t *testing.T) {
	h := newHarness(t)

	req := EnqueueRequest{
		RecipientID:    testUser,
		Channel:        domain.ChannelEmail,
		Category:       domain.CategorySecurity,
		TemplateKey:    "security.password_changed",
		Payload:        map[string]any{"ip": "203.0.113.7"},
		IdempotencyKey: "password_changed:user:event",
	}
	if err := h.svc.Enqueue(context.Background(), req); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	row := h.notifications.only(t)
	if row.ID == uuid.Nil {
		t.Error("ID = nil uuid, want a generated one")
	}
	if row.RecipientID != testUser {
		t.Errorf("RecipientID = %s, want %s", row.RecipientID, testUser)
	}
	if row.Channel != domain.ChannelEmail || row.Category != domain.CategorySecurity {
		t.Errorf("routing = %s/%s, want email/security", row.Channel, row.Category)
	}
	if row.TemplateKey != req.TemplateKey || row.IdempotencyKey != req.IdempotencyKey {
		t.Errorf("row = %+v, want the request's template and idempotency key", row)
	}
	if !reflect.DeepEqual(row.Payload, req.Payload) {
		t.Errorf("Payload = %v, want %v", row.Payload, req.Payload)
	}
	if row.Status != domain.StatusPending || row.Attempts != 0 {
		t.Errorf("status = %q, attempts = %d, want pending/0", row.Status, row.Attempts)
	}
	// Both stamps come from the injected clock, and the row is claimable now.
	if !row.CreatedAt.Equal(testNow) || !row.NextAttemptAt.Equal(testNow) {
		t.Errorf("CreatedAt = %v, NextAttemptAt = %v, want both %v", row.CreatedAt, row.NextAttemptAt, testNow)
	}
}

// The caller cannot distinguish the first enqueue from the second, which is the
// point of the key: a retried request must not produce a second mail.
func TestEnqueueIsIdempotent(t *testing.T) {
	h := newHarness(t)

	req := EnqueueRequest{
		RecipientID: testUser,
		Channel:     domain.ChannelEmail,
		Category:    domain.CategorySecurity,
		TemplateKey: "security.password_changed",

		IdempotencyKey: "password_changed:user:event-1",
	}

	for i := 0; i < 3; i++ {
		if err := h.svc.Enqueue(context.Background(), req); err != nil {
			t.Fatalf("Enqueue %d: %v, want success even on a duplicate", i+1, err)
		}
	}
	if got := h.notifications.count(); got != 1 {
		t.Fatalf("outbox holds %d rows, want 1", got)
	}

	// A different key is a different event.
	req.IdempotencyKey = "password_changed:user:event-2"
	if err := h.svc.Enqueue(context.Background(), req); err != nil {
		t.Fatalf("Enqueue with a fresh key: %v", err)
	}
	if got := h.notifications.count(); got != 2 {
		t.Errorf("outbox holds %d rows, want 2", got)
	}

	// Unkeyed enqueues never collide with each other.
	req.IdempotencyKey = ""
	for i := 0; i < 2; i++ {
		if err := h.svc.Enqueue(context.Background(), req); err != nil {
			t.Fatalf("unkeyed Enqueue %d: %v", i+1, err)
		}
	}
	if got := h.notifications.count(); got != 4 {
		t.Errorf("outbox holds %d rows, want 4", got)
	}
}

func TestEnqueueAppliesPreferences(t *testing.T) {
	t.Run("a stored disabled pair filters the enqueue", func(t *testing.T) {
		h := newHarness(t)
		h.preferences.prefs = []domain.Preference{
			{UserID: testUser, Channel: domain.ChannelInApp, Category: domain.CategorySystem, Enabled: false},
		}

		err := h.svc.Enqueue(context.Background(), EnqueueRequest{
			RecipientID: testUser,
			Channel:     domain.ChannelInApp,
			Category:    domain.CategorySystem,
			TemplateKey: "system.maintenance",
		})
		if err != nil {
			t.Fatalf("Enqueue: %v, want a silent success — the user asked not to receive this", err)
		}
		if got := h.notifications.count(); got != 0 {
			t.Errorf("outbox holds %d rows, want none", got)
		}
	})

	t.Run("an absent row means enabled", func(t *testing.T) {
		h := newHarness(t)

		if err := h.svc.Enqueue(context.Background(), EnqueueRequest{
			RecipientID: testUser,
			Channel:     domain.ChannelInApp,
			Category:    domain.CategorySystem,
			TemplateKey: "system.maintenance",
		}); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
		if got := h.notifications.count(); got != 1 {
			t.Errorf("outbox holds %d rows, want 1", got)
		}
	})

	t.Run("a disabled row for another pair does not filter this one", func(t *testing.T) {
		h := newHarness(t)
		h.preferences.prefs = []domain.Preference{
			{UserID: testUser, Channel: domain.ChannelWebhook, Category: domain.CategorySystem, Enabled: false},
		}

		if err := h.svc.Enqueue(context.Background(), EnqueueRequest{
			RecipientID: testUser,
			Channel:     domain.ChannelInApp,
			Category:    domain.CategorySystem,
			TemplateKey: "system.maintenance",
		}); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
		if got := h.notifications.count(); got != 1 {
			t.Errorf("outbox holds %d rows, want 1", got)
		}
	})

	t.Run("another user's disabled row does not filter this user", func(t *testing.T) {
		h := newHarness(t)
		h.preferences.prefs = []domain.Preference{
			{UserID: testOtherUser, Channel: domain.ChannelInApp, Category: domain.CategorySystem, Enabled: false},
		}

		if err := h.svc.Enqueue(context.Background(), EnqueueRequest{
			RecipientID: testUser,
			Channel:     domain.ChannelInApp,
			Category:    domain.CategorySystem,
			TemplateKey: "system.maintenance",
		}); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
		if got := h.notifications.count(); got != 1 {
			t.Errorf("outbox holds %d rows, want 1", got)
		}
	})

	// The security property, and the reason the gate goes through
	// domain.Allowed rather than reading Enabled off the slice here. The row
	// below is *stored* and *disabled* — written before the rule existed, or by
	// hand in psql — and it still must not suppress the mail. Proving that
	// absence defaults to enabled would prove only the default, not this.
	t.Run("a stored disabled row cannot suppress security email", func(t *testing.T) {
		h := newHarness(t)
		h.preferences.prefs = []domain.Preference{{
			UserID:   testUser,
			Channel:  domain.ChannelEmail,
			Category: domain.CategorySecurity,
			Enabled:  false,
		}}

		if err := h.svc.Enqueue(context.Background(), EnqueueRequest{
			RecipientID: testUser,
			Channel:     domain.ChannelEmail,
			Category:    domain.CategorySecurity,
			TemplateKey: "security.password_changed",
		}); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
		if got := h.notifications.count(); got != 1 {
			t.Fatalf("outbox holds %d rows, want the security mail queued anyway", got)
		}
	})

	// The rule covers one pair. Security in the in-app mailbox stays the
	// user's to silence.
	t.Run("security on another channel is still filterable", func(t *testing.T) {
		h := newHarness(t)
		h.preferences.prefs = []domain.Preference{{
			UserID:   testUser,
			Channel:  domain.ChannelInApp,
			Category: domain.CategorySecurity,
			Enabled:  false,
		}}

		if err := h.svc.Enqueue(context.Background(), EnqueueRequest{
			RecipientID: testUser,
			Channel:     domain.ChannelInApp,
			Category:    domain.CategorySecurity,
			TemplateKey: "security.password_changed",
		}); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
		if got := h.notifications.count(); got != 0 {
			t.Errorf("outbox holds %d rows, want none", got)
		}
	})
}

func TestEnqueueRejectsAnInvalidRequest(t *testing.T) {
	tests := []struct {
		name string
		req  EnqueueRequest
	}{
		{"no recipient", EnqueueRequest{Channel: domain.ChannelEmail, Category: domain.CategorySystem, TemplateKey: "k"}},
		{"unknown channel", EnqueueRequest{RecipientID: testUser, Channel: "carrier-pigeon", Category: domain.CategorySystem, TemplateKey: "k"}},
		{"unknown category", EnqueueRequest{RecipientID: testUser, Channel: domain.ChannelEmail, Category: "marketing", TemplateKey: "k"}},
		{"no template key", EnqueueRequest{RecipientID: testUser, Channel: domain.ChannelEmail, Category: domain.CategorySystem}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)

			err := h.svc.Enqueue(context.Background(), tt.req)
			if !errors.Is(err, domain.ErrInvalidNotification) {
				t.Fatalf("error = %v, want ErrInvalidNotification", err)
			}
			if got := h.notifications.count(); got != 0 {
				t.Errorf("outbox holds %d rows, want none", got)
			}
			// Validation comes first: a malformed request does not cost a
			// preference lookup.
			if h.preferences.listCalls != 0 {
				t.Errorf("preferences read %d times, want none", h.preferences.listCalls)
			}
		})
	}
}

func TestEnqueuePropagatesRepositoryFailures(t *testing.T) {
	req := EnqueueRequest{
		RecipientID: testUser,
		Channel:     domain.ChannelEmail,
		Category:    domain.CategorySecurity,
		TemplateKey: "security.password_changed",
	}

	t.Run("the preference lookup fails", func(t *testing.T) {
		h := newHarness(t)
		boom := errors.New("pool exhausted")
		h.preferences.listErr = boom

		err := h.svc.Enqueue(context.Background(), req)
		if !errors.Is(err, boom) {
			t.Fatalf("error = %v, want it to wrap the repository's", err)
		}
		if got := h.notifications.count(); got != 0 {
			t.Errorf("outbox holds %d rows, want none: preferences were never resolved", got)
		}
	})

	t.Run("the insert fails", func(t *testing.T) {
		h := newHarness(t)
		boom := errors.New("deadlock detected")
		h.notifications.enqueueErr = boom

		if err := h.svc.Enqueue(context.Background(), req); !errors.Is(err, boom) {
			t.Fatalf("error = %v, want it to wrap the repository's", err)
		}
	})
}

func TestListForUserMapsRowsAndPassesTheFilterThrough(t *testing.T) {
	h := newHarness(t)

	readAt := testNow.Add(-time.Hour)
	h.notifications.listRows = []*domain.Notification{
		{
			ID:          uuid.MustParse("44444444-4444-4444-8444-444444444444"),
			RecipientID: testUser,
			Channel:     domain.ChannelInApp,
			Category:    domain.CategorySecurity,
			TemplateKey: "security.password_changed",
			Payload:     map[string]any{"ip": "203.0.113.7"},
			Status:      domain.StatusSent,
			CreatedAt:   testNow.Add(-2 * time.Hour),
			ReadAt:      &readAt,
			LastError:   "smtp: 421 service not available",
		},
		{
			ID:          uuid.MustParse("55555555-5555-4555-8555-555555555555"),
			RecipientID: testUser,
			Channel:     domain.ChannelInApp,
			Category:    domain.CategorySystem,
			TemplateKey: "system.maintenance",
			CreatedAt:   testNow.Add(-3 * time.Hour),
		},
	}

	filter := domain.ListFilter{
		UnreadOnly: true,
		Limit:      7,
		Before:     &domain.Cursor{CreatedAt: testNow, ID: testUser},
	}

	views, err := h.svc.ListForUser(context.Background(), testUser, filter)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}

	if h.notifications.listedFor != testUser {
		t.Errorf("listed for %s, want %s", h.notifications.listedFor, testUser)
	}
	if !reflect.DeepEqual(h.notifications.listedWith, filter) {
		t.Errorf("filter = %+v, want it passed through untouched as %+v", h.notifications.listedWith, filter)
	}

	want := []NotificationView{
		{
			ID:          h.notifications.listRows[0].ID,
			Category:    domain.CategorySecurity,
			TemplateKey: "security.password_changed",
			Payload:     map[string]any{"ip": "203.0.113.7"},
			CreatedAt:   testNow.Add(-2 * time.Hour),
			ReadAt:      &readAt,
		},
		{
			ID:          h.notifications.listRows[1].ID,
			Category:    domain.CategorySystem,
			TemplateKey: "system.maintenance",
			CreatedAt:   testNow.Add(-3 * time.Hour),
		},
	}
	if !reflect.DeepEqual(views, want) {
		t.Errorf("views = %+v, want %+v", views, want)
	}
}

func TestListForUserReportsTheRepositoryFailure(t *testing.T) {
	h := newHarness(t)
	boom := errors.New("statement timeout")
	h.notifications.listErr = boom

	views, err := h.svc.ListForUser(context.Background(), testUser, domain.ListFilter{})
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want it to wrap the repository's", err)
	}
	if views != nil {
		t.Errorf("views = %v, want nil alongside the error", views)
	}
}

func TestMarkRead(t *testing.T) {
	t.Run("stamps with the injected clock and scopes to the caller", func(t *testing.T) {
		h := newHarness(t)
		h.clock.advance(90 * time.Minute)

		id := uuid.MustParse("66666666-6666-4666-8666-666666666666")
		if err := h.svc.MarkRead(context.Background(), testUser, id); err != nil {
			t.Fatalf("MarkRead: %v", err)
		}

		if len(h.notifications.markReadCalls) != 1 {
			t.Fatalf("MarkRead called %d times, want 1", len(h.notifications.markReadCalls))
		}
		got := h.notifications.markReadCalls[0]
		if got.recipientID != testUser || got.id != id {
			t.Errorf("call = %+v, want recipient %s and id %s", got, testUser, id)
		}
		if want := testNow.Add(90 * time.Minute); !got.now.Equal(want) {
			t.Errorf("now = %v, want the injected clock's %v", got.now, want)
		}
	})

	// Someone else's id and a nonexistent id are the same answer, so the
	// endpoint cannot be used to probe for ids. The sentinel has to survive the
	// wrapping for transport to keep telling them apart from a 500.
	t.Run("preserves ErrNotFound through the wrap", func(t *testing.T) {
		h := newHarness(t)
		h.notifications.markReadErr = domain.ErrNotFound

		err := h.svc.MarkRead(context.Background(), testUser, uuid.New())
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("error = %v, want ErrNotFound", err)
		}
	})
}

func TestMarkAllRead(t *testing.T) {
	t.Run("returns the number of rows changed", func(t *testing.T) {
		h := newHarness(t)
		h.notifications.markAllN = 12
		h.clock.advance(time.Minute)

		n, err := h.svc.MarkAllRead(context.Background(), testUser)
		if err != nil {
			t.Fatalf("MarkAllRead: %v", err)
		}
		if n != 12 {
			t.Errorf("count = %d, want 12", n)
		}
		if want := testNow.Add(time.Minute); !h.notifications.markAllNow.Equal(want) {
			t.Errorf("now = %v, want the injected clock's %v", h.notifications.markAllNow, want)
		}
	})

	t.Run("reports the repository failure", func(t *testing.T) {
		h := newHarness(t)
		boom := errors.New("deadlock detected")
		h.notifications.markAllErr = boom

		n, err := h.svc.MarkAllRead(context.Background(), testUser)
		if !errors.Is(err, boom) {
			t.Fatalf("error = %v, want it to wrap the repository's", err)
		}
		if n != 0 {
			t.Errorf("count = %d, want 0 alongside the error", n)
		}
	})
}

// The matrix is the cross product of every channel and every category, resolved
// through domain.Allowed — never by reading Enabled off the stored rows. The
// last case is why: a rogue row would otherwise make the UI report "security
// email: off" while the system correctly kept sending.
func TestGetPreferencesReturnsTheEffectiveMatrix(t *testing.T) {
	h := newHarness(t)
	h.preferences.prefs = []domain.Preference{
		{UserID: testUser, Channel: domain.ChannelInApp, Category: domain.CategorySystem, Enabled: false},
		{UserID: testUser, Channel: domain.ChannelEmail, Category: domain.CategorySecurity, Enabled: false},
		{UserID: testOtherUser, Channel: domain.ChannelWebhook, Category: domain.CategoryAccount, Enabled: false},
	}

	views, err := h.svc.GetPreferences(context.Background(), testUser)
	if err != nil {
		t.Fatalf("GetPreferences: %v", err)
	}

	channels := domain.Channels()
	categories := domain.Categories()
	if len(views) != len(channels)*len(categories) {
		t.Fatalf("matrix holds %d cells, want %d — every pair, present or not", len(views), len(channels)*len(categories))
	}

	seen := map[PreferenceView]bool{}
	i := 0
	for _, ch := range channels {
		for _, cat := range categories {
			got := views[i]
			if got.Channel != ch || got.Category != cat {
				t.Fatalf("cell %d = %s/%s, want %s/%s: the order is Channels x Categories", i, got.Channel, got.Category, ch, cat)
			}
			seen[got] = true
			i++
		}
	}

	want := []PreferenceView{
		// The stored disabled row is honoured.
		{Channel: domain.ChannelInApp, Category: domain.CategorySystem, Enabled: false},
		// Absent rows read as enabled.
		{Channel: domain.ChannelInApp, Category: domain.CategoryAccount, Enabled: true},
		{Channel: domain.ChannelWebhook, Category: domain.CategorySystem, Enabled: true},
		// Another user's row is not this user's.
		{Channel: domain.ChannelWebhook, Category: domain.CategoryAccount, Enabled: true},
		// The protected pair reports what will actually happen, whatever the
		// stored row says.
		{Channel: domain.ChannelEmail, Category: domain.CategorySecurity, Enabled: true},
	}
	for _, w := range want {
		if !seen[w] {
			t.Errorf("matrix is missing %s/%s enabled=%v", w.Channel, w.Category, w.Enabled)
		}
	}
}

func TestGetPreferencesReportsTheRepositoryFailure(t *testing.T) {
	h := newHarness(t)
	boom := errors.New("connection refused")
	h.preferences.listErr = boom

	views, err := h.svc.GetPreferences(context.Background(), testUser)
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want it to wrap the repository's", err)
	}
	if views != nil {
		t.Errorf("views = %v, want nil alongside the error", views)
	}
}

func TestUpdatePreferencesWritesAndReturnsTheNewMatrix(t *testing.T) {
	h := newHarness(t)
	h.clock.advance(time.Hour)

	views, err := h.svc.UpdatePreferences(context.Background(), testUser, []PreferenceUpdate{
		{Channel: domain.ChannelInApp, Category: domain.CategorySystem, Enabled: false},
		{Channel: domain.ChannelWebhook, Category: domain.CategoryAccount, Enabled: false},
		// Turning the protected pair on is allowed, so an idempotent "enable
		// everything" does not fail on the one pair that was never off.
		{Channel: domain.ChannelEmail, Category: domain.CategorySecurity, Enabled: true},
	})
	if err != nil {
		t.Fatalf("UpdatePreferences: %v", err)
	}

	if len(h.preferences.upserts) != 1 {
		t.Fatalf("Upsert called %d times, want 1 — the whole set lands together", len(h.preferences.upserts))
	}
	written := h.preferences.upserts[0]
	if len(written) != 3 {
		t.Fatalf("wrote %d preferences, want 3", len(written))
	}
	for _, p := range written {
		if p.UserID != testUser {
			t.Errorf("wrote a preference for %s, want %s", p.UserID, testUser)
		}
		if want := testNow.Add(time.Hour); !p.UpdatedAt.Equal(want) {
			t.Errorf("UpdatedAt = %v, want the injected clock's %v", p.UpdatedAt, want)
		}
	}

	effective := map[PreferenceView]bool{}
	for _, v := range views {
		effective[v] = true
	}
	for _, want := range []PreferenceView{
		{Channel: domain.ChannelInApp, Category: domain.CategorySystem, Enabled: false},
		{Channel: domain.ChannelWebhook, Category: domain.CategoryAccount, Enabled: false},
		{Channel: domain.ChannelEmail, Category: domain.CategorySecurity, Enabled: true},
		{Channel: domain.ChannelInApp, Category: domain.CategoryAccount, Enabled: true},
	} {
		if !effective[want] {
			t.Errorf("returned matrix is missing %s/%s enabled=%v", want.Channel, want.Category, want.Enabled)
		}
	}
}

// A half-legal request is refused whole. The repository writes the slice as one
// unit, so a user who disabled three categories and tried to disable a
// protected fourth would otherwise have to guess which three took effect.
func TestUpdatePreferencesValidatesEverythingBeforeWritingAnything(t *testing.T) {
	tests := []struct {
		name    string
		updates []PreferenceUpdate
		wantErr error
		wantMsg string
	}{
		{
			name: "disabling the protected pair",
			updates: []PreferenceUpdate{
				{Channel: domain.ChannelInApp, Category: domain.CategorySystem, Enabled: false},
				{Channel: domain.ChannelEmail, Category: domain.CategorySecurity, Enabled: false},
			},
			wantErr: domain.ErrPreferenceProtected,
		},
		{
			name:    "an unknown channel",
			updates: []PreferenceUpdate{{Channel: "carrier-pigeon", Category: domain.CategorySystem, Enabled: false}},
			wantErr: domain.ErrInvalidPreference,
			wantMsg: "unknown channel",
		},
		{
			name:    "an unknown category",
			updates: []PreferenceUpdate{{Channel: domain.ChannelInApp, Category: "marketing", Enabled: false}},
			wantErr: domain.ErrInvalidPreference,
			wantMsg: "unknown category",
		},
		{
			// One request naming a pair twice has no single correct outcome,
			// and an upsert asked to write one row twice in one statement is an
			// error at the database rather than a last-one-wins.
			name: "the same pair twice",
			updates: []PreferenceUpdate{
				{Channel: domain.ChannelInApp, Category: domain.CategorySystem, Enabled: false},
				{Channel: domain.ChannelInApp, Category: domain.CategorySystem, Enabled: true},
			},
			wantErr: domain.ErrInvalidPreference,
			wantMsg: "named twice",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)

			views, err := h.svc.UpdatePreferences(context.Background(), testUser, tt.updates)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantMsg != "" && !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error = %q, want it to mention %q", err, tt.wantMsg)
			}
			if views != nil {
				t.Errorf("views = %v, want nil alongside the error", views)
			}
			if len(h.preferences.upserts) != 0 {
				t.Errorf("Upsert called %d times, want none: nothing may land from a refused request", len(h.preferences.upserts))
			}
		})
	}
}

func TestUpdatePreferencesReportsTheWriteFailure(t *testing.T) {
	h := newHarness(t)
	boom := errors.New("serialization failure")
	h.preferences.upsertErr = boom

	views, err := h.svc.UpdatePreferences(context.Background(), testUser, []PreferenceUpdate{
		{Channel: domain.ChannelInApp, Category: domain.CategorySystem, Enabled: false},
	})
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want it to wrap the repository's", err)
	}
	if views != nil {
		t.Errorf("views = %v, want nil alongside the error", views)
	}
}
