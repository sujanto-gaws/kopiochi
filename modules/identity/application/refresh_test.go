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

	u := testUser("alice")
	h := newHarness(u)
	first := login(t, h, "alice")

	if _, err := h.svc.Refresh(context.Background(), first.RefreshToken); err != nil {
		t.Fatalf("first Refresh() error = %v", err)
	}
	if !h.tokens.revokedFor(u.ID.String()) {
		t.Error("the user's existing refresh tokens were not revoked")
	}

	_, err := h.svc.Refresh(context.Background(), first.RefreshToken)
	if !errors.Is(err, ErrRefreshTokenInvalid) {
		t.Fatalf("reusing a rotated refresh token gave %v, want ErrRefreshTokenInvalid", err)
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
