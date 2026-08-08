package application

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	domain "github.com/sujanto-gaws/kopiochi/modules/identity/domain"
)

// mfaUser returns a user with MFA switched on.
func mfaUser(username string) *domain.User {
	u := testUser(username)
	u.MFAEnabled = true
	u.MFASecret = "SECRET"
	return u
}

// acceptMFAToken makes the fake issuer accept exactly one token string, for
// the MFA class only, resolving to the given subject.
func acceptMFAToken(h *harness, token, subject string) {
	h.issuer.validate = func(got string, want domain.Class) (*domain.Claims, error) {
		if got != token {
			return nil, errors.New("unknown token")
		}
		if want != domain.ClassMFA {
			return nil, domain.ErrWrongTokenClass
		}
		return &domain.Claims{Subject: subject, Class: domain.ClassMFA}, nil
	}
}

func TestVerifyMFA_ValidCode_IssuesFullTokens(t *testing.T) {
	t.Parallel()

	u := mfaUser("alice")
	h := newHarness(u)
	acceptMFAToken(h, "the-mfa-token", u.ID.String())

	resp, err := h.svc.VerifyMFA(context.Background(), "the-mfa-token", MfaVerifyRequest{Code: "123456"})
	if err != nil {
		t.Fatalf("VerifyMFA() error = %v", err)
	}
	if resp.AccessToken == "" {
		t.Error("no access token issued after a valid second factor")
	}
	if resp.RefreshToken == "" {
		t.Error("no refresh token issued after a valid second factor")
	}
}

// TestVerifyMFA_DemandsTheMFAClass is the escalation guard. The service asks
// Validate for domain.ClassMFA specifically; if it asked for any valid token
// instead, an access token could be replayed here — but more importantly, this
// test fails if that argument is ever changed.
func TestVerifyMFA_DemandsTheMFAClass(t *testing.T) {
	t.Parallel()

	u := mfaUser("alice")
	h := newHarness(u)

	var asked domain.Class
	h.issuer.validate = func(_ string, want domain.Class) (*domain.Claims, error) {
		asked = want
		return &domain.Claims{Subject: u.ID.String(), Class: want}, nil
	}

	_, _ = h.svc.VerifyMFA(context.Background(), "any", MfaVerifyRequest{Code: "123456"})

	if asked != domain.ClassMFA {
		t.Errorf("VerifyMFA asked Validate for class %q, want %q — any other value lets a different token kind through",
			asked, domain.ClassMFA)
	}
}

// TestVerifyMFA_AccessTokenIsRejected exercises the same guard from the
// caller's side: presenting a fully-authenticated access token here must not
// work, because that is the bypass the class check exists to prevent.
func TestVerifyMFA_AccessTokenIsRejected(t *testing.T) {
	t.Parallel()

	u := mfaUser("alice")
	h := newHarness(u)

	// An issuer that only ever holds an access-class token.
	h.issuer.validate = func(_ string, want domain.Class) (*domain.Claims, error) {
		if want != domain.ClassAccess {
			return nil, domain.ErrWrongTokenClass
		}
		return &domain.Claims{Subject: u.ID.String(), Class: domain.ClassAccess}, nil
	}

	_, err := h.svc.VerifyMFA(context.Background(), "an-access-token", MfaVerifyRequest{Code: "123456"})
	if !errors.Is(err, ErrInvalidMFAToken) {
		t.Fatalf("VerifyMFA() with an access token gave %v, want ErrInvalidMFAToken", err)
	}
	if h.issuer.accessTokensIssued() != 0 {
		t.Error("full tokens were issued from an access token replayed at the MFA step")
	}
}

func TestVerifyMFA_WrongCode_IsRejected(t *testing.T) {
	t.Parallel()

	u := mfaUser("alice")
	h := newHarness(u)
	acceptMFAToken(h, "the-mfa-token", u.ID.String())

	_, err := h.svc.VerifyMFA(context.Background(), "the-mfa-token", MfaVerifyRequest{Code: "000000"})
	if !errors.Is(err, ErrInvalidMFACode) {
		t.Fatalf("VerifyMFA() error = %v, want ErrInvalidMFACode", err)
	}
	if h.issuer.accessTokensIssued() != 0 {
		t.Error("tokens were issued despite a wrong TOTP code")
	}
}

// TestVerifyMFA_NoCodeAtAll_IsRejected: an empty request must not be treated
// as "nothing to check, therefore fine".
func TestVerifyMFA_NoCodeAtAll_IsRejected(t *testing.T) {
	t.Parallel()

	u := mfaUser("alice")
	h := newHarness(u)
	acceptMFAToken(h, "the-mfa-token", u.ID.String())

	_, err := h.svc.VerifyMFA(context.Background(), "the-mfa-token", MfaVerifyRequest{})
	if !errors.Is(err, ErrInvalidMFACode) {
		t.Fatalf("VerifyMFA() with neither code nor backup code gave %v, want ErrInvalidMFACode", err)
	}
}

func TestVerifyMFA_BackupCode_Works(t *testing.T) {
	t.Parallel()

	u := mfaUser("alice")
	h := newHarness(u)
	acceptMFAToken(h, "the-mfa-token", u.ID.String())

	resp, err := h.svc.VerifyMFA(context.Background(), "the-mfa-token",
		MfaVerifyRequest{BackupCode: "backup-code-1"})
	if err != nil {
		t.Fatalf("VerifyMFA() with a backup code error = %v", err)
	}
	if resp.AccessToken == "" {
		t.Error("no access token issued for a valid backup code")
	}
}

// TestVerifyMFA_BackupCodeIsSingleUse: a backup code that keeps working is a
// static password that bypasses the second factor.
func TestVerifyMFA_BackupCodeIsSingleUse(t *testing.T) {
	t.Parallel()

	u := mfaUser("alice")
	h := newHarness(u)
	acceptMFAToken(h, "the-mfa-token", u.ID.String())

	if _, err := h.svc.VerifyMFA(context.Background(), "the-mfa-token",
		MfaVerifyRequest{BackupCode: "backup-code-1"}); err != nil {
		t.Fatalf("first use of the backup code failed: %v", err)
	}

	_, err := h.svc.VerifyMFA(context.Background(), "the-mfa-token",
		MfaVerifyRequest{BackupCode: "backup-code-1"})
	if !errors.Is(err, ErrInvalidMFACode) {
		t.Fatalf("the backup code was accepted a second time (%v); it must be single use", err)
	}
}

func TestVerifyMFA_UnknownBackupCode_IsRejected(t *testing.T) {
	t.Parallel()

	u := mfaUser("alice")
	h := newHarness(u)
	acceptMFAToken(h, "the-mfa-token", u.ID.String())

	_, err := h.svc.VerifyMFA(context.Background(), "the-mfa-token",
		MfaVerifyRequest{BackupCode: "not-a-real-backup-code"})
	if !errors.Is(err, ErrInvalidMFACode) {
		t.Fatalf("VerifyMFA() error = %v, want ErrInvalidMFACode", err)
	}
}

func TestSetupMFA_ReturnsASecretAndQRURL(t *testing.T) {
	t.Parallel()

	u := testUser("alice")
	h := newHarness(u)

	resp, err := h.svc.SetupMFA(context.Background(), u.ID.String())
	if err != nil {
		t.Fatalf("SetupMFA() error = %v", err)
	}
	if resp.Secret == "" {
		t.Error("SetupMFA returned no secret")
	}
	if resp.QRCodeURL == "" {
		t.Error("SetupMFA returned no QR code URL")
	}
}

// TestSetupMFA_DoesNotEnableMFAYet is the whole point of the two-step setup: a
// user who scans a QR code but never confirms it must not be locked out of
// their own account by an MFA requirement they cannot satisfy.
func TestSetupMFA_DoesNotEnableMFAYet(t *testing.T) {
	t.Parallel()

	u := testUser("alice")
	h := newHarness(u)

	_, err := h.svc.SetupMFA(context.Background(), u.ID.String())
	if err != nil {
		t.Fatalf("SetupMFA() error = %v", err)
	}
	if u.MFAEnabled {
		t.Error("SetupMFA enabled MFA before the code was confirmed")
	}
	if u.MFASecret == "" {
		t.Error("SetupMFA did not store the secret; VerifyMFASetup has nothing to check against")
	}
}

func TestSetupMFA_UnknownUserIsAnError(t *testing.T) {
	t.Parallel()

	h := newHarness()

	if _, err := h.svc.SetupMFA(context.Background(), uuid.New().String()); err == nil {
		t.Error("SetupMFA() = nil error for a user that does not exist")
	}
}

func TestSetupMFA_SecretGenerationFailureIsPropagated(t *testing.T) {
	t.Parallel()

	u := testUser("alice")
	h := newHarness(u)
	h.svc = NewService(h.users, fakeHasher{}, h.issuer, h.tokens, testConfig(),
		fakeMFAService{genErr: errors.New("no entropy")}, h.mfaStore, h.audit)

	if _, err := h.svc.SetupMFA(context.Background(), u.ID.String()); err == nil {
		t.Error("SetupMFA() = nil error despite the generator failing")
	}
	if u.MFASecret != "" {
		t.Error("a secret was stored even though generation failed")
	}
}

func TestVerifyMFASetup_ValidCodeEnablesMFAAndReturnsBackupCodes(t *testing.T) {
	t.Parallel()

	u := testUser("alice")
	u.MFASecret = "SECRET"
	h := newHarness(u)

	resp, err := h.svc.VerifyMFASetup(context.Background(), u.ID.String(), "123456")
	if err != nil {
		t.Fatalf("VerifyMFASetup() error = %v", err)
	}
	if !u.MFAEnabled {
		t.Error("MFA was not enabled after a valid confirmation")
	}
	if len(resp.BackupCodes) != 8 {
		t.Errorf("got %d backup codes, want 8", len(resp.BackupCodes))
	}
}

// TestVerifyMFASetup_BackupCodesAreDistinctAndStoredHashed: duplicates would
// silently reduce the number of usable recovery codes, and storing plaintext
// would make a table dump a set of working second factors.
func TestVerifyMFASetup_BackupCodesAreDistinctAndStoredHashed(t *testing.T) {
	t.Parallel()

	u := testUser("alice")
	u.MFASecret = "SECRET"
	h := newHarness(u)

	resp, err := h.svc.VerifyMFASetup(context.Background(), u.ID.String(), "123456")
	if err != nil {
		t.Fatalf("VerifyMFASetup() error = %v", err)
	}

	seen := map[string]bool{}
	for _, c := range resp.BackupCodes {
		if seen[c] {
			t.Errorf("backup code %q was issued twice", c)
		}
		seen[c] = true
	}

	if len(h.mfaStore.stored) != len(resp.BackupCodes) {
		t.Fatalf("stored %d hashes for %d codes", len(h.mfaStore.stored), len(resp.BackupCodes))
	}
	for _, stored := range h.mfaStore.stored {
		for _, plain := range resp.BackupCodes {
			if stored == plain {
				t.Fatalf("backup code %q was stored in plaintext", plain)
			}
		}
	}
}

// TestVerifyMFASetup_WrongCodeLeavesMFADisabled: enabling MFA on an
// unconfirmed secret would lock the user out permanently, since they have no
// authenticator entry that produces valid codes.
func TestVerifyMFASetup_WrongCodeLeavesMFADisabled(t *testing.T) {
	t.Parallel()

	u := testUser("alice")
	u.MFASecret = "SECRET"
	h := newHarness(u)

	_, err := h.svc.VerifyMFASetup(context.Background(), u.ID.String(), "000000")
	if !errors.Is(err, ErrInvalidMFACode) {
		t.Fatalf("VerifyMFASetup() error = %v, want ErrInvalidMFACode", err)
	}
	if u.MFAEnabled {
		t.Error("MFA was enabled despite an invalid confirmation code")
	}
	if len(h.mfaStore.stored) != 0 {
		t.Error("backup codes were issued for a failed confirmation")
	}
}

func TestVerifyMFASetup_UnknownUserIsAnError(t *testing.T) {
	t.Parallel()

	h := newHarness()

	if _, err := h.svc.VerifyMFASetup(context.Background(), uuid.New().String(), "123456"); err == nil {
		t.Error("VerifyMFASetup() = nil error for a user that does not exist")
	}
}

// TestVerifyMFASetup_SaveFailureDoesNotClaimSuccess: returning backup codes
// after the enable failed to persist would leave the user holding recovery
// codes for a second factor that is not switched on.
func TestVerifyMFASetup_SaveFailureDoesNotClaimSuccess(t *testing.T) {
	t.Parallel()

	u := testUser("alice")
	u.MFASecret = "SECRET"
	h := newHarness(u)
	h.users.saveErr = errors.New("database is down")

	resp, err := h.svc.VerifyMFASetup(context.Background(), u.ID.String(), "123456")
	if err == nil {
		t.Fatal("VerifyMFASetup() = nil error despite the save failing")
	}
	if resp != nil {
		t.Errorf("VerifyMFASetup() = %+v, want nil", resp)
	}
}

func TestMFAError_Error(t *testing.T) {
	t.Parallel()

	var err error = &MFAError{Token: "t"}
	if err.Error() == "" {
		t.Error("MFAError.Error() is empty")
	}
}

func TestLogout_UnknownUserIsNotAnError(t *testing.T) {
	t.Parallel()

	h := newHarness()

	// Revoking for a user with no tokens is a no-op, not a failure: logout
	// must be idempotent or a double-click produces a spurious 500.
	if err := h.svc.Logout(context.Background(), uuid.New().String()); err != nil {
		t.Errorf("Logout() error = %v, want nil", err)
	}
}
