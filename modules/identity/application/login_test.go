package application

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "github.com/sujanto-gaws/kopiochi/modules/identity/domain"
)

func TestLogin_ValidCredentials_IssuesTokens(t *testing.T) {
	t.Parallel()

	u := testUser("alice")
	h := newHarness(u)

	resp, err := h.svc.Login(context.Background(), LoginRequest{Username: "alice", Password: testPassword})
	if err != nil {
		t.Fatalf("Login() error = %v, want nil", err)
	}
	if resp.AccessToken == "" {
		t.Error("no access token issued")
	}
	if resp.RefreshToken == "" {
		t.Error("no refresh token issued")
	}
	if resp.TokenType != "Bearer" {
		t.Errorf("TokenType = %q, want Bearer", resp.TokenType)
	}
	if resp.ExpiresIn != int(testConfig().AccessTokenTTL.Seconds()) {
		t.Errorf("ExpiresIn = %d, want %d", resp.ExpiresIn, int(testConfig().AccessTokenTTL.Seconds()))
	}
}

// TestLogin_StoresOnlyTheHashOfTheRefreshToken: the plaintext goes to the
// client, the hash goes to the database. Storing the plaintext would make a
// table dump equivalent to a set of live sessions.
func TestLogin_StoresOnlyTheHashOfTheRefreshToken(t *testing.T) {
	t.Parallel()

	u := testUser("alice")
	h := newHarness(u)

	resp, err := h.svc.Login(context.Background(), LoginRequest{Username: "alice", Password: testPassword})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	if _, ok := h.tokens.byHash[resp.RefreshToken]; ok {
		t.Fatal("the refresh token plaintext is a key in the store; it must be hashed")
	}
	stored, err := h.tokens.FindValid(context.Background(), domain.HashToken(resp.RefreshToken))
	if err != nil {
		t.Fatalf("the hash of the issued refresh token is not in the store: %v", err)
	}
	if stored.UserID != u.ID.String() {
		t.Errorf("stored token belongs to %q, want %q", stored.UserID, u.ID.String())
	}
}

func TestLogin_UnknownUser_ReturnsInvalidCredentials(t *testing.T) {
	t.Parallel()

	h := newHarness(testUser("alice"))

	_, err := h.svc.Login(context.Background(), LoginRequest{Username: "nobody", Password: testPassword})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Login() error = %v, want ErrInvalidCredentials", err)
	}
}

// TestLogin_UnknownUserAndWrongPassword_AreIndistinguishable: a different
// error for "no such user" turns the login endpoint into a user enumeration
// oracle.
func TestLogin_UnknownUserAndWrongPassword_AreIndistinguishable(t *testing.T) {
	t.Parallel()

	h := newHarness(testUser("alice"))

	_, unknownErr := h.svc.Login(context.Background(), LoginRequest{Username: "nobody", Password: "x"})
	_, wrongPwErr := h.svc.Login(context.Background(), LoginRequest{Username: "alice", Password: "wrong"})

	if unknownErr.Error() != wrongPwErr.Error() {
		t.Errorf("unknown user gives %q but wrong password gives %q; the difference enumerates users",
			unknownErr, wrongPwErr)
	}
}

func TestLogin_WrongPassword_IssuesNoTokens(t *testing.T) {
	t.Parallel()

	h := newHarness(testUser("alice"))

	resp, err := h.svc.Login(context.Background(), LoginRequest{Username: "alice", Password: "wrong"})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Login() error = %v, want ErrInvalidCredentials", err)
	}
	if resp != nil {
		t.Errorf("Login() = %+v, want nil response on failure", resp)
	}
	if h.issuer.accessTokensIssued() != 0 {
		t.Error("an access token was minted for a failed login")
	}
}

// TestLogin_WrongPassword_PersistsTheFailedAttempt is the assertion that makes
// lockout real. The domain increments the counter, but if the service does not
// persist it the count resets on every request and the account never locks —
// the rule would look correct in the entity and do nothing in production.
func TestLogin_WrongPassword_PersistsTheFailedAttempt(t *testing.T) {
	t.Parallel()

	u := testUser("alice")
	h := newHarness(u)

	before := h.users.saves()
	_, _ = h.svc.Login(context.Background(), LoginRequest{Username: "alice", Password: "wrong"})

	if h.users.saves() == before {
		t.Fatal("the failed attempt was never saved; the counter resets each request and lockout never triggers")
	}
	if u.FailedLoginAttempts != 1 {
		t.Errorf("FailedLoginAttempts = %d, want 1", u.FailedLoginAttempts)
	}
}

// TestLogin_LocksAfterRepeatedFailures walks the whole path end to end:
// enough wrong passwords must produce ErrAccountLocked, and from then on even
// the *correct* password must be refused.
func TestLogin_LocksAfterRepeatedFailures(t *testing.T) {
	t.Parallel()

	h := newHarness(testUser("alice"))
	max := testConfig().MaxFailedAttempts

	for i := 0; i < max; i++ {
		_, err := h.svc.Login(context.Background(), LoginRequest{Username: "alice", Password: "wrong"})
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("attempt %d: error = %v, want ErrInvalidCredentials", i+1, err)
		}
	}

	_, err := h.svc.Login(context.Background(), LoginRequest{Username: "alice", Password: testPassword})
	if !errors.Is(err, ErrAccountLocked) {
		t.Fatalf("after %d failures the correct password gave %v, want ErrAccountLocked", max, err)
	}
}

func TestLogin_SuccessfulLogin_ClearsTheFailureCount(t *testing.T) {
	t.Parallel()

	u := testUser("alice")
	h := newHarness(u)

	_, _ = h.svc.Login(context.Background(), LoginRequest{Username: "alice", Password: "wrong"})
	if u.FailedLoginAttempts == 0 {
		t.Fatal("precondition failed: the failed attempt was not recorded")
	}

	if _, err := h.svc.Login(context.Background(), LoginRequest{Username: "alice", Password: testPassword}); err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if u.FailedLoginAttempts != 0 {
		t.Errorf("FailedLoginAttempts = %d after a successful login, want 0", u.FailedLoginAttempts)
	}
}

// TestLogin_MFAEnabled_ReturnsMFAErrorAndNoAccessToken is the escalation guard
// at the service boundary: a user with a second factor must get a token that
// is useless for API access until that factor is verified.
func TestLogin_MFAEnabled_ReturnsMFAErrorAndNoAccessToken(t *testing.T) {
	t.Parallel()

	u := testUser("alice")
	u.MFAEnabled = true
	u.MFASecret = "SECRET"
	h := newHarness(u)

	resp, err := h.svc.Login(context.Background(), LoginRequest{Username: "alice", Password: testPassword})
	if resp != nil {
		t.Errorf("Login() = %+v, want nil while MFA is outstanding", resp)
	}

	var mfaErr *MFAError
	if !errors.As(err, &mfaErr) {
		t.Fatalf("Login() error = %v, want *MFAError", err)
	}
	if mfaErr.Token == "" {
		t.Error("MFAError carries no token; the client cannot continue")
	}
	if h.issuer.accessTokensIssued() != 0 {
		t.Error("an access token was issued before the second factor was verified")
	}
	if mfaErr.User.ID != u.ID.String() {
		t.Errorf("MFAError.User.ID = %q, want %q", mfaErr.User.ID, u.ID.String())
	}
}

// TestLogin_LockedAccount_IsRejectedBeforeThePasswordIsChecked: the order
// matters. Checking the password first would let an attacker keep testing
// guesses against a locked account and learn when one is right.
func TestLogin_LockedAccount_IsRejectedBeforeThePasswordIsChecked(t *testing.T) {
	t.Parallel()

	u := testUser("alice")
	until := time.Now().Add(time.Hour)
	u.LockedUntil = &until
	h := newHarness(u)

	_, err := h.svc.Login(context.Background(), LoginRequest{Username: "alice", Password: testPassword})
	if !errors.Is(err, ErrAccountLocked) {
		t.Fatalf("Login() error = %v, want ErrAccountLocked", err)
	}
	if h.issuer.accessTokensIssued() != 0 {
		t.Error("a token was issued for a locked account")
	}
}

// TestLogin_ExpiredLock_LetsTheUserBackIn: a lock that never expired would
// make a brute-force attempt a permanent denial of service against the victim.
func TestLogin_ExpiredLock_LetsTheUserBackIn(t *testing.T) {
	t.Parallel()

	u := testUser("alice")
	past := time.Now().Add(-time.Hour)
	u.LockedUntil = &past
	u.FailedLoginAttempts = 99
	h := newHarness(u)

	if _, err := h.svc.Login(context.Background(), LoginRequest{Username: "alice", Password: testPassword}); err != nil {
		t.Fatalf("Login() error = %v, want the expired lock to be ignored", err)
	}
}

// TestLogin_RefreshTokenStoreFailure_FailsTheLogin: returning tokens after the
// refresh token failed to persist would hand the client a refresh token that
// can never be redeemed.
func TestLogin_RefreshTokenStoreFailure_FailsTheLogin(t *testing.T) {
	t.Parallel()

	h := newHarness(testUser("alice"))
	h.tokens.storeErr = errors.New("database is down")

	resp, err := h.svc.Login(context.Background(), LoginRequest{Username: "alice", Password: testPassword})
	if err == nil {
		t.Fatal("Login() error = nil, want the store failure surfaced")
	}
	if resp != nil {
		t.Errorf("Login() = %+v, want nil", resp)
	}
}

// TestLogin_EachLoginIssuesADistinctRefreshToken: a predictable or reused
// refresh token would let one session's token be guessed from another's.
func TestLogin_EachLoginIssuesADistinctRefreshToken(t *testing.T) {
	t.Parallel()

	h := newHarness(testUser("alice"))

	seen := map[string]bool{}
	for i := 0; i < 5; i++ {
		resp, err := h.svc.Login(context.Background(), LoginRequest{Username: "alice", Password: testPassword})
		if err != nil {
			t.Fatalf("Login() error = %v", err)
		}
		if seen[resp.RefreshToken] {
			t.Fatalf("refresh token %q issued twice", resp.RefreshToken)
		}
		seen[resp.RefreshToken] = true
	}
}
