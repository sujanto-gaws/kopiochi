package application

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestNewService_NilNotifierIsReplaced: a nil SecurityNotifier would panic on
// the first security event this service raises — that is, at exactly the
// moment an account locked, converting the lockout into a 500 instead of a
// response. Mirrors TestNewService_NilAuditorIsReplaced.
func TestNewService_NilNotifierIsReplaced(t *testing.T) {
	t.Parallel()

	u := testUser("alice")
	users := newFakeUserRepo(u)
	svc := NewService(users, fakeHasher{}, &fakeTokenIssuer{}, newFakeTokenStore(),
		testConfig(), fakeMFAService{validCode: "123456"}, &fakeMFAStore{}, nil, nil)

	// Enough wrong passwords to cross the lockout threshold and reach the
	// AccountLocked call. A kept nil panics here.
	max := testConfig().MaxFailedAttempts
	for i := 0; i < max; i++ {
		_, _ = svc.Login(context.Background(), LoginRequest{Username: "alice", Password: "wrong"})
	}
}

func TestNopNotifier_SwallowsEverything(t *testing.T) {
	t.Parallel()

	n := NopNotifier()
	ctx := context.Background()

	n.AccountLocked(ctx, "u", time.Now())
	n.MFAEnabled(ctx, "u", time.Now())
}

// TestLogin_NotifiesAccountLockedOnceOnTheTransition mirrors
// TestLogin_AccountLockedIsAuditedOnceOnTheTransition: the notifier gets the
// same guard as the audit port, on purpose — the event means "this account
// just locked", not "someone tried a locked account", and a continuous brute
// force must not raise a fresh notification on every attempt.
func TestLogin_NotifiesAccountLockedOnceOnTheTransition(t *testing.T) {
	t.Parallel()

	h := newHarness(testUser("alice"))
	max := testConfig().MaxFailedAttempts

	for i := 0; i < max+3; i++ {
		_, _ = h.svc.Login(context.Background(), LoginRequest{Username: "alice", Password: "wrong"})
	}

	if n := h.notifier.count("account.locked"); n != 1 {
		t.Errorf("account.locked notified %d times, want exactly 1 (the transition)", n)
	}
}

// TestLogin_NotifiesAccountLockedWithTheLockExpiry: the notifier's event id is
// the deterministic identity of the lockout episode, and this is the value
// login.go actually passes.
func TestLogin_NotifiesAccountLockedWithTheLockExpiry(t *testing.T) {
	t.Parallel()

	u := testUser("alice")
	h := newHarness(u)
	max := testConfig().MaxFailedAttempts

	before := time.Now()
	for i := 0; i < max; i++ {
		_, _ = h.svc.Login(context.Background(), LoginRequest{Username: "alice", Password: "wrong"})
	}
	after := time.Now()

	ev, ok := h.notifier.find("account.locked")
	if !ok {
		t.Fatal("account.locked was not raised")
	}
	if ev.UserID != u.ID.String() {
		t.Errorf("user id = %q, want %q", ev.UserID, u.ID.String())
	}
	if ev.At.Before(before) || ev.At.After(after.Add(testConfig().LockDuration+time.Second)) {
		t.Errorf("lockedUntil = %v, want roughly now+%v", ev.At, testConfig().LockDuration)
	}
	if u.LockedUntil == nil || !ev.At.Equal(*u.LockedUntil) {
		t.Errorf("notifier received %v, want the exact value persisted on the user (%v)", ev.At, u.LockedUntil)
	}
}

// TestLogin_DoesNotNotifyOnAnUnlockedFailure: a wrong password that does not
// cross the lockout threshold is not a security event this notifier reports.
func TestLogin_DoesNotNotifyOnAnUnlockedFailure(t *testing.T) {
	t.Parallel()

	h := newHarness(testUser("alice"))

	_, _ = h.svc.Login(context.Background(), LoginRequest{Username: "alice", Password: "wrong"})

	if n := h.notifier.count("account.locked"); n != 0 {
		t.Errorf("account.locked notified %d times for a single failed attempt, want 0", n)
	}
}

// TestVerifyMFASetup_NotifiesMFAEnabled mirrors the audit-side assertions in
// mfa_test.go for the notifier port.
func TestVerifyMFASetup_NotifiesMFAEnabled(t *testing.T) {
	t.Parallel()

	u := testUser("alice")
	u.MFASecret = "SECRET"
	h := newHarness(u)

	before := time.Now()
	if _, err := h.svc.VerifyMFASetup(context.Background(), u.ID.String(), "123456"); err != nil {
		t.Fatalf("VerifyMFASetup() error = %v", err)
	}
	after := time.Now()

	ev, ok := h.notifier.find("mfa.enabled")
	if !ok {
		t.Fatal("mfa.enabled was not raised")
	}
	if ev.UserID != u.ID.String() {
		t.Errorf("user id = %q, want %q", ev.UserID, u.ID.String())
	}
	if ev.At.Before(before) || ev.At.After(after) {
		t.Errorf("enabledAt = %v, want between %v and %v", ev.At, before, after)
	}
}

// TestVerifyMFASetup_WrongCodeDoesNotNotify: an unconfirmed second factor is
// not "enabled", and nothing here must claim otherwise.
func TestVerifyMFASetup_WrongCodeDoesNotNotify(t *testing.T) {
	t.Parallel()

	u := testUser("alice")
	u.MFASecret = "SECRET"
	h := newHarness(u)

	_, _ = h.svc.VerifyMFASetup(context.Background(), u.ID.String(), "000000")

	if n := h.notifier.count("mfa.enabled"); n != 0 {
		t.Errorf("mfa.enabled notified %d times for a rejected confirmation, want 0", n)
	}
}

// TestVerifyMFASetup_SaveFailureDoesNotNotify: reporting an enablement that
// never persisted would tell a user their account gained a second factor it
// does not actually have.
func TestVerifyMFASetup_SaveFailureDoesNotNotify(t *testing.T) {
	t.Parallel()

	u := testUser("alice")
	u.MFASecret = "SECRET"
	h := newHarness(u)
	h.users.saveErr = errors.New("database is down")

	_, err := h.svc.VerifyMFASetup(context.Background(), u.ID.String(), "123456")
	if err == nil {
		t.Fatal("VerifyMFASetup() error = nil despite the save failing")
	}
	if n := h.notifier.count("mfa.enabled"); n != 0 {
		t.Errorf("mfa.enabled notified %d times despite the save failing, want 0", n)
	}
}
