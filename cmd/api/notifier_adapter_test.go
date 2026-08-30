package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	notifapp "github.com/sujanto-gaws/kopiochi/modules/notification/application"
	notifdomain "github.com/sujanto-gaws/kopiochi/modules/notification/domain"
)

// fakeEnqueuer records every EnqueueRequest it was handed, and returns
// enqueueErr for every call when set — enough to assert both what the adapter
// sends and that a failure does not stop it from trying the next channel.
type fakeEnqueuer struct {
	mu         sync.Mutex
	requests   []notifapp.EnqueueRequest
	enqueueErr error
}

func (f *fakeEnqueuer) Enqueue(_ context.Context, req notifapp.EnqueueRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, req)
	return f.enqueueErr
}

func (f *fakeEnqueuer) calls() []notifapp.EnqueueRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]notifapp.EnqueueRequest(nil), f.requests...)
}

func TestSecurityNotifierAdapter_AccountLocked_EnqueuesBothChannels(t *testing.T) {
	t.Parallel()

	fe := &fakeEnqueuer{}
	a := newSecurityNotifierAdapter(fe, zerolog.Nop())

	userID := uuid.New()
	lockedUntil := time.Date(2026, 8, 30, 9, 14, 0, 0, time.UTC)

	a.AccountLocked(context.Background(), userID.String(), lockedUntil)

	calls := fe.calls()
	require.Len(t, calls, len(securityChannels), "one enqueue per security channel")

	seenChannels := map[notifdomain.Channel]bool{}
	for _, req := range calls {
		require.Equal(t, userID, req.RecipientID)
		require.Equal(t, notifdomain.CategorySecurity, req.Category)
		require.Equal(t, "security.account_locked", req.TemplateKey)
		require.Equal(t, "30 August 2026 at 09:14 UTC", req.Payload["LockedUntil"])
		require.NotEmpty(t, req.IdempotencyKey)
		seenChannels[req.Channel] = true
	}
	require.True(t, seenChannels[notifdomain.ChannelEmail], "no email enqueue")
	require.True(t, seenChannels[notifdomain.ChannelInApp], "no in-app enqueue")
}

func TestSecurityNotifierAdapter_MFAEnabled_EnqueuesBothChannels(t *testing.T) {
	t.Parallel()

	fe := &fakeEnqueuer{}
	a := newSecurityNotifierAdapter(fe, zerolog.Nop())

	userID := uuid.New()
	enabledAt := time.Date(2026, 8, 30, 9, 14, 0, 0, time.UTC)

	a.MFAEnabled(context.Background(), userID.String(), enabledAt)

	calls := fe.calls()
	require.Len(t, calls, len(securityChannels))
	for _, req := range calls {
		require.Equal(t, "security.mfa_enabled", req.TemplateKey)
		require.Equal(t, "30 August 2026 at 09:14 UTC", req.Payload["EnabledAt"])
	}
}

// TestSecurityNotifierAdapter_IdempotencyKeysAreDistinctPerChannel is the
// load-bearing assertion: idx_notifications_idem is one partial-unique index
// over the whole outbox, not scoped by channel, so two calls sharing a key
// would collide and the second insert would be silently dropped as a
// duplicate — losing that channel's copy rather than deduplicating a retry.
func TestSecurityNotifierAdapter_IdempotencyKeysAreDistinctPerChannel(t *testing.T) {
	t.Parallel()

	fe := &fakeEnqueuer{}
	a := newSecurityNotifierAdapter(fe, zerolog.Nop())

	a.AccountLocked(context.Background(), uuid.New().String(), time.Now())

	calls := fe.calls()
	require.Len(t, calls, 2)
	require.NotEqual(t, calls[0].IdempotencyKey, calls[1].IdempotencyKey)
}

// TestSecurityNotifierAdapter_SameEpisodeIsIdempotentPerChannel: the same
// lockedUntil (the deterministic id of one lockout episode) reported twice
// produces the same key for the same channel, which is what lets the outbox's
// unique index deduplicate a retried report.
func TestSecurityNotifierAdapter_SameEpisodeIsIdempotentPerChannel(t *testing.T) {
	t.Parallel()

	fe := &fakeEnqueuer{}
	a := newSecurityNotifierAdapter(fe, zerolog.Nop())

	userID := uuid.New().String()
	lockedUntil := time.Now().Add(time.Hour)

	a.AccountLocked(context.Background(), userID, lockedUntil)
	a.AccountLocked(context.Background(), userID, lockedUntil)

	calls := fe.calls()
	require.Len(t, calls, 4)

	keysByChannel := map[notifdomain.Channel][]string{}
	for _, req := range calls {
		keysByChannel[req.Channel] = append(keysByChannel[req.Channel], req.IdempotencyKey)
	}
	for ch, keys := range keysByChannel {
		require.Len(t, keys, 2, "channel %s", ch)
		require.Equal(t, keys[0], keys[1], "channel %s: same episode must produce the same key", ch)
	}
}

// TestSecurityNotifierAdapter_EnqueueFailureDoesNotPanicOrStopTheOtherChannel:
// SecurityNotifier's methods return nothing by design, so this is the whole
// contract — a failed enqueue is swallowed (logged, not propagated), and it
// must not stop the adapter from attempting the remaining channel.
func TestSecurityNotifierAdapter_EnqueueFailureDoesNotPanicOrStopTheOtherChannel(t *testing.T) {
	t.Parallel()

	fe := &fakeEnqueuer{enqueueErr: errors.New("outbox is down")}
	a := newSecurityNotifierAdapter(fe, zerolog.Nop())

	require.NotPanics(t, func() {
		a.AccountLocked(context.Background(), uuid.New().String(), time.Now())
	})
	require.Len(t, fe.calls(), len(securityChannels), "a failure on one channel must not prevent the other from being attempted")
}

// TestSecurityNotifierAdapter_InvalidUserIDDoesNotPanic: userID arrives as a
// bare string from identity's application layer; a value that does not parse
// as a UUID must be refused quietly, not crash the caller that is mid-login.
func TestSecurityNotifierAdapter_InvalidUserIDDoesNotPanic(t *testing.T) {
	t.Parallel()

	fe := &fakeEnqueuer{}
	a := newSecurityNotifierAdapter(fe, zerolog.Nop())

	require.NotPanics(t, func() {
		a.AccountLocked(context.Background(), "not-a-uuid", time.Now())
	})
	require.Empty(t, fe.calls())
}
