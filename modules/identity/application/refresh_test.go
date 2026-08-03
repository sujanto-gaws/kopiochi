package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "github.com/sujanto-gaws/kopiochi/modules/identity/domain"
)

// login is a helper that performs a successful login and returns the response.
func login(t *testing.T, h *harness, username string) *TokenResponse {
	t.Helper()

	resp, err := h.svc.Login(context.Background(), LoginRequest{Username: username, Password: testPassword})
	if err != nil {
		t.Fatalf("precondition Login() error = %v", err)
	}
	return resp
}

func TestRefresh_ValidToken_IssuesANewPair(t *testing.T) {
	t.Parallel()

	h := newHarness(testUser("alice"))
	first := login(t, h, "alice")

	second, err := h.svc.Refresh(context.Background(), first.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if second.AccessToken == "" {
		t.Error("no access token issued on refresh")
	}
	if second.RefreshToken == "" {
		t.Error("no refresh token issued on refresh")
	}
}

// TestRefresh_RotatesTheRefreshToken: reusing the same refresh token forever
// means a single leaked token grants indefinite access with nothing to detect.
func TestRefresh_RotatesTheRefreshToken(t *testing.T) {
	t.Parallel()

	h := newHarness(testUser("alice"))
	first := login(t, h, "alice")

	second, err := h.svc.Refresh(context.Background(), first.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if second.RefreshToken == first.RefreshToken {
		t.Error("Refresh returned the same refresh token; it must rotate")
	}
}

// TestRefresh_OldTokenStopsWorking is the other half of rotation. Rotating but
// leaving the old token valid gives an attacker who captured it exactly the
// access rotation was meant to remove.
func TestRefresh_OldTokenStopsWorking(t *testing.T) {
	t.Parallel()

	h := newHarness(testUser("alice"))
	first := login(t, h, "alice")

	if _, err := h.svc.Refresh(context.Background(), first.RefreshToken); err != nil {
		t.Fatalf("first Refresh() error = %v", err)
	}

	_, err := h.svc.Refresh(context.Background(), first.RefreshToken)
	if !errors.Is(err, ErrRefreshTokenInvalid) {
		t.Fatalf("reusing a rotated refresh token gave %v, want ErrRefreshTokenInvalid", err)
	}
}

// TestRefresh_DoesNotRevokeOtherSessions records a behaviour change from
// Phase 5.6, and an improvement.
//
// Refresh used to call RevokeAllForUser on every single exchange, so
// refreshing on a phone silently logged out the same user's laptop. Rotation
// is now scoped to the family descending from one login, so concurrent
// sessions are independent — which is also what makes reuse detection
// meaningful: revoking a family is now a signal, not routine housekeeping.
func TestRefresh_DoesNotRevokeOtherSessions(t *testing.T) {
	t.Parallel()

	h := newHarness(testUser("alice"))

	laptop := login(t, h, "alice")
	phone := login(t, h, "alice")

	if _, err := h.svc.Refresh(context.Background(), phone.RefreshToken); err != nil {
		t.Fatalf("refreshing the phone session failed: %v", err)
	}

	if _, err := h.svc.Refresh(context.Background(), laptop.RefreshToken); err != nil {
		t.Errorf("refreshing one session invalidated another: %v", err)
	}
}

// TestRefresh_ReuseRevokesTheWholeFamily is the core of 5.6.
//
// Rotation alone does not survive theft: once the stolen token is spent, both
// parties hold something that looks valid. Refusing only the second request
// leaves whoever moved first — the attacker — with a working session. Revoking
// the family logs out both and forces a real re-authentication.
func TestRefresh_ReuseRevokesTheWholeFamily(t *testing.T) {
	t.Parallel()

	h := newHarness(testUser("alice"))
	stolen := login(t, h, "alice")

	// The attacker refreshes first and gets a valid successor.
	attacker, err := h.svc.Refresh(context.Background(), stolen.RefreshToken)
	if err != nil {
		t.Fatalf("first refresh failed: %v", err)
	}

	// The victim then presents the token they still hold. That is the
	// detection.
	if _, err := h.svc.Refresh(context.Background(), stolen.RefreshToken); !errors.Is(err, ErrRefreshTokenInvalid) {
		t.Fatalf("reuse gave %v, want ErrRefreshTokenInvalid", err)
	}

	// The attacker's successor must now be dead too. If only the replayed
	// token were invalidated, the theft would have succeeded.
	if _, err := h.svc.Refresh(context.Background(), attacker.RefreshToken); !errors.Is(err, ErrRefreshTokenInvalid) {
		t.Fatalf("the attacker's token still works after reuse detection: %v", err)
	}
}

func TestRefresh_ReuseIsAudited(t *testing.T) {
	t.Parallel()

	h := newHarness(testUser("alice"))
	stolen := login(t, h, "alice")

	if _, err := h.svc.Refresh(context.Background(), stolen.RefreshToken); err != nil {
		t.Fatalf("first refresh failed: %v", err)
	}
	_, _ = h.svc.Refresh(context.Background(), stolen.RefreshToken)

	ev, ok := h.audit.find("refresh.reuse")
	if !ok {
		t.Fatal("token reuse was detected but never audited; an incident review would see nothing")
	}
	if ev.FamilyID == "" {
		t.Error("the audit event carries no family id, so the affected session cannot be identified")
	}
	if ev.Revoked < 1 {
		t.Errorf("audit reports %d tokens revoked, want at least 1", ev.Revoked)
	}
}

// TestRefresh_ReuseIsIndistinguishableToTheCaller: telling an attacker that
// the token was recognised and the family is now locked confirms they held a
// real credential and tells them exactly when they were caught.
func TestRefresh_ReuseIsIndistinguishableToTheCaller(t *testing.T) {
	t.Parallel()

	h := newHarness(testUser("alice"))
	stolen := login(t, h, "alice")
	if _, err := h.svc.Refresh(context.Background(), stolen.RefreshToken); err != nil {
		t.Fatalf("first refresh failed: %v", err)
	}

	_, reuseErr := h.svc.Refresh(context.Background(), stolen.RefreshToken)
	_, bogusErr := h.svc.Refresh(context.Background(), "never-issued-at-all")

	if reuseErr.Error() != bogusErr.Error() {
		t.Errorf("reuse returns %q but an unknown token returns %q; the difference confirms a real credential",
			reuseErr, bogusErr)
	}
}

// TestRefresh_UnknownTokenIsNotAudited keeps the signal worth alerting on.
// Every scanner probing the endpoint would otherwise raise a token-theft
// event, and an alert that fires constantly is one nobody reads.
func TestRefresh_UnknownTokenIsNotAudited(t *testing.T) {
	t.Parallel()

	h := newHarness(testUser("alice"))

	_, _ = h.svc.Refresh(context.Background(), "never-issued-at-all")

	if n := h.audit.count("refresh.reuse"); n != 0 {
		t.Errorf("an unknown token produced %d reuse events, want 0", n)
	}
}

// TestRefresh_ExpiredTokenIsNotReuse: an expired token is the ordinary end of
// a session. Treating it as theft would revoke a legitimate user's other
// sessions every time one simply aged out.
func TestRefresh_ExpiredTokenIsNotAudited(t *testing.T) {
	t.Parallel()

	u := testUser("alice")
	h := newHarness(u)

	const plain = "an-expired-refresh-token"
	err := h.tokens.Store(context.Background(), domain.RefreshToken{
		UserID:    u.ID.String(),
		TokenHash: domain.HashToken(plain),
		ExpiresAt: time.Now().Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("seeding the store failed: %v", err)
	}

	if _, err := h.svc.Refresh(context.Background(), plain); !errors.Is(err, ErrRefreshTokenInvalid) {
		t.Fatalf("Refresh(expired) = %v, want ErrRefreshTokenInvalid", err)
	}
	if n := h.audit.count("refresh.reuse"); n != 0 {
		t.Errorf("an expired token produced %d reuse events, want 0", n)
	}
}

// TestRefresh_RevocationFailureStillRejects: if the family revocation itself
// fails, the request must still be refused. Returning a 500 would tell the
// caller something unusual just happened — and returning success would be
// catastrophic.
func TestRefresh_RevocationFailureStillRejects(t *testing.T) {
	t.Parallel()

	h := newHarness(testUser("alice"))
	stolen := login(t, h, "alice")
	if _, err := h.svc.Refresh(context.Background(), stolen.RefreshToken); err != nil {
		t.Fatalf("first refresh failed: %v", err)
	}

	h.tokens.revokeFamilyErr = errors.New("database is down")

	if _, err := h.svc.Refresh(context.Background(), stolen.RefreshToken); !errors.Is(err, ErrRefreshTokenInvalid) {
		t.Fatalf("Refresh() = %v, want ErrRefreshTokenInvalid even when revocation fails", err)
	}

	ev, ok := h.audit.find("refresh.reuse")
	if !ok {
		t.Fatal("a failed revocation was not audited at all")
	}
	if ev.Err == nil {
		t.Error("the audit event does not record that revocation failed — the one case where the attacker keeps a live session")
	}
}

func TestRefresh_EmptyToken_IsRejected(t *testing.T) {
	t.Parallel()

	h := newHarness(testUser("alice"))

	if _, err := h.svc.Refresh(context.Background(), ""); !errors.Is(err, ErrRefreshTokenInvalid) {
		t.Fatalf("Refresh(\"\") error = %v, want ErrRefreshTokenInvalid", err)
	}
}

func TestRefresh_UnknownToken_IsRejected(t *testing.T) {
	t.Parallel()

	h := newHarness(testUser("alice"))

	if _, err := h.svc.Refresh(context.Background(), "never-issued"); !errors.Is(err, ErrRefreshTokenInvalid) {
		t.Fatalf("Refresh(unknown) error = %v, want ErrRefreshTokenInvalid", err)
	}
}

// TestRefresh_ExpiredToken_IsRejected covers the check that the store alone
// does not make: a row can still be present and past its expiry.
func TestRefresh_ExpiredToken_IsRejected(t *testing.T) {
	t.Parallel()

	u := testUser("alice")
	h := newHarness(u)

	const plain = "an-expired-refresh-token"
	err := h.tokens.Store(context.Background(), domain.RefreshToken{
		UserID:    u.ID.String(),
		TokenHash: domain.HashToken(plain),
		ExpiresAt: time.Now().Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("seeding the store failed: %v", err)
	}

	if _, err := h.svc.Refresh(context.Background(), plain); !errors.Is(err, ErrRefreshTokenInvalid) {
		t.Fatalf("Refresh(expired) error = %v, want ErrRefreshTokenInvalid", err)
	}
}

// TestRefresh_TokenForADeletedUser_IsRejected: the token row can outlive the
// user row, and issuing an access token for a subject that no longer exists
// would be an authentication with no account behind it.
func TestRefresh_TokenForADeletedUser_IsRejected(t *testing.T) {
	t.Parallel()

	h := newHarness() // no users at all
	err := h.tokens.Store(context.Background(), domain.RefreshToken{
		UserID:    "11111111-1111-1111-1111-111111111111",
		TokenHash: domain.HashToken("orphan"),
		ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("seeding the store failed: %v", err)
	}

	if _, err := h.svc.Refresh(context.Background(), "orphan"); !errors.Is(err, ErrRefreshTokenInvalid) {
		t.Fatalf("Refresh() for a missing user gave %v, want ErrRefreshTokenInvalid", err)
	}
	if h.issuer.accessTokensIssued() != 0 {
		t.Error("an access token was issued for a user that does not exist")
	}
}

func TestLogout_RevokesEveryRefreshTokenForTheUser(t *testing.T) {
	t.Parallel()

	u := testUser("alice")
	h := newHarness(u)
	first := login(t, h, "alice")

	if err := h.svc.Logout(context.Background(), u.ID.String()); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}

	if _, err := h.svc.Refresh(context.Background(), first.RefreshToken); !errors.Is(err, ErrRefreshTokenInvalid) {
		t.Fatalf("a refresh token still worked after logout: %v", err)
	}
}
