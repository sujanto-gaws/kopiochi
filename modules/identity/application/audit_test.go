package auth

import (
	"context"
	"errors"
	"testing"
)

// TestNewService_NilAuditorIsReplaced: a nil Auditor would panic on the first
// security event — that is, at exactly the moment a token theft was detected,
// converting a detection into a 500 and losing the record.
func TestNewService_NilAuditorIsReplaced(t *testing.T) {
	t.Parallel()

	users := newFakeUserRepo(testUser("alice"))
	svc := NewService(users, fakeHasher{}, &fakeTokenIssuer{}, newFakeTokenStore(),
		testConfig(), fakeMFAService{validCode: "123456"}, &fakeMFAStore{}, nil)

	// Any path that audits will panic if the nil was kept.
	_, _ = svc.Login(context.Background(), LoginRequest{Username: "nobody", Password: "x"})
}

func TestNopAuditor_SwallowsEverything(t *testing.T) {
	t.Parallel()

	a := NopAuditor()
	ctx := context.Background()

	a.LoginSucceeded(ctx, "u")
	a.LoginFailed(ctx, "u", ReasonBadPassword)
	a.AccountLocked(ctx, "u")
	a.LogoutSucceeded(ctx, "u")
	a.MFAEnrolled(ctx, "u")
	a.MFAFailed(ctx, "u", ReasonInvalidCode)
	a.RefreshReuseDetected(ctx, "u", "f", 1, errors.New("x"))
}

// TestLogin_AuditsTheOutcome covers the events an incident review actually
// reads, and the distinction that makes them trustworthy: a failed attempt
// records the *attempted* username as a subject, never as a confirmed actor.
func TestLogin_AuditsTheOutcome(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		u := testUser("alice")
		h := newHarness(u)

		if _, err := h.svc.Login(context.Background(), LoginRequest{Username: "alice", Password: testPassword}); err != nil {
			t.Fatalf("Login() error = %v", err)
		}

		ev, ok := h.audit.find("login.success")
		if !ok {
			t.Fatal("a successful login was not audited")
		}
		if ev.ActorID != u.ID.String() {
			t.Errorf("actor = %q, want the user id %q", ev.ActorID, u.ID.String())
		}
	})

	t.Run("wrong password", func(t *testing.T) {
		t.Parallel()

		h := newHarness(testUser("alice"))
		_, _ = h.svc.Login(context.Background(), LoginRequest{Username: "alice", Password: "wrong"})

		ev, ok := h.audit.find("login.failure")
		if !ok {
			t.Fatal("a failed login was not audited")
		}
		if ev.Reason != ReasonBadPassword {
			t.Errorf("reason = %q, want %q", ev.Reason, ReasonBadPassword)
		}
		if ev.ActorID != "" {
			t.Errorf("actor_id = %q on a failed login; nobody authenticated, so it must be a subject", ev.ActorID)
		}
		if ev.Subject != "alice" {
			t.Errorf("subject = %q, want the attempted username", ev.Subject)
		}
	})

	t.Run("unknown user", func(t *testing.T) {
		t.Parallel()

		h := newHarness(testUser("alice"))
		_, _ = h.svc.Login(context.Background(), LoginRequest{Username: "nobody", Password: "x"})

		ev, ok := h.audit.find("login.failure")
		if !ok {
			t.Fatal("a login for an unknown user was not audited")
		}
		if ev.Reason != ReasonUnknownUser {
			t.Errorf("reason = %q, want %q", ev.Reason, ReasonUnknownUser)
		}
	})
}

// TestLogin_AccountLockedIsAuditedOnceOnTheTransition: emitting it on every
// subsequent attempt would make the event mean "someone tried a locked
// account" rather than "this account just locked", and an alert on it would
// fire continuously during any brute-force attempt.
func TestLogin_AccountLockedIsAuditedOnceOnTheTransition(t *testing.T) {
	t.Parallel()

	h := newHarness(testUser("alice"))
	max := testConfig().MaxFailedAttempts

	// Enough attempts to lock, then several more against the locked account.
	for i := 0; i < max+3; i++ {
		_, _ = h.svc.Login(context.Background(), LoginRequest{Username: "alice", Password: "wrong"})
	}

	if n := h.audit.count("account.locked"); n != 1 {
		t.Errorf("account.locked emitted %d times, want exactly 1 (the transition)", n)
	}
}

// TestLogin_MFARequiredIsNotASuccess: the password was right but the second
// factor is outstanding. Recording a success here would make the audit trail
// claim an authentication that has not happened.
func TestLogin_MFARequiredIsNotASuccess(t *testing.T) {
	t.Parallel()

	u := testUser("alice")
	u.MFAEnabled = true
	u.MFASecret = "SECRET"
	h := newHarness(u)

	_, _ = h.svc.Login(context.Background(), LoginRequest{Username: "alice", Password: testPassword})

	if n := h.audit.count("login.success"); n != 0 {
		t.Errorf("login.success emitted %d times while MFA was still outstanding, want 0", n)
	}
}

func TestLogout_IsAudited(t *testing.T) {
	t.Parallel()

	u := testUser("alice")
	h := newHarness(u)

	if err := h.svc.Logout(context.Background(), u.ID.String()); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}

	if _, ok := h.audit.find("logout"); !ok {
		t.Error("logout was not audited")
	}
}
