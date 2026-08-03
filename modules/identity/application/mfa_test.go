package auth

import (
	"context"
	"errors"
	"testing"

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
