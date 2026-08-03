package mfa

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
)

const testEmail = "alice@example.com"

func newService() *TOTPService { return NewTOTPService("Kopiochi") }

func TestGenerateSecret_ProducesAUsableSecretAndURL(t *testing.T) {
	t.Parallel()

	secret, qrURL, err := newService().GenerateSecret(testEmail)
	if err != nil {
		t.Fatalf("GenerateSecret() error = %v", err)
	}
	if secret == "" {
		t.Error("GenerateSecret returned an empty secret")
	}
	if qrURL == "" {
		t.Error("GenerateSecret returned an empty URL")
	}
}

// TestGenerateSecret_URLIsAValidOtpauthURI: the URL is what an authenticator
// app scans. If the issuer or account is missing the user gets an unlabelled
// entry, and with several accounts enrolled that is unusable.
func TestGenerateSecret_URLIsAValidOtpauthURI(t *testing.T) {
	t.Parallel()

	_, qrURL, err := newService().GenerateSecret(testEmail)
	if err != nil {
		t.Fatalf("GenerateSecret() error = %v", err)
	}

	u, err := url.Parse(qrURL)
	if err != nil {
		t.Fatalf("the QR URL does not parse: %v (%q)", err, qrURL)
	}
	if u.Scheme != "otpauth" {
		t.Errorf("scheme = %q, want otpauth", u.Scheme)
	}
	if u.Host != "totp" {
		t.Errorf("host = %q, want totp", u.Host)
	}
	if got := u.Query().Get("issuer"); got != "Kopiochi" {
		t.Errorf("issuer = %q, want Kopiochi", got)
	}
	if !strings.Contains(u.Path, testEmail) {
		t.Errorf("path %q does not name the account", u.Path)
	}
	if u.Query().Get("secret") == "" {
		t.Error("the URL carries no secret; scanning it enrolls nothing")
	}
}

// TestGenerateSecret_IsDifferentEveryTime: a fixed secret would mean every
// user shares one second factor, and knowing any one of them breaks all.
func TestGenerateSecret_IsDifferentEveryTime(t *testing.T) {
	t.Parallel()

	svc := newService()

	seen := map[string]bool{}
	for i := 0; i < 5; i++ {
		secret, _, err := svc.GenerateSecret(testEmail)
		if err != nil {
			t.Fatalf("GenerateSecret() error = %v", err)
		}
		if seen[secret] {
			t.Fatalf("GenerateSecret returned the same secret twice for the same account")
		}
		seen[secret] = true
	}
}

func TestValidateCode_AcceptsTheCurrentCode(t *testing.T) {
	t.Parallel()

	svc := newService()
	secret, _, err := svc.GenerateSecret(testEmail)
	if err != nil {
		t.Fatalf("GenerateSecret() error = %v", err)
	}

	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("GenerateCode() error = %v", err)
	}

	if !svc.ValidateCode(secret, code) {
		t.Error("ValidateCode rejected a freshly generated code")
	}
}

func TestValidateCode_RejectsAWrongCode(t *testing.T) {
	t.Parallel()

	svc := newService()
	secret, _, err := svc.GenerateSecret(testEmail)
	if err != nil {
		t.Fatalf("GenerateSecret() error = %v", err)
	}

	for _, code := range []string{"", "000000", "123456", "abcdef", "1234567"} {
		// 000000/123456 are astronomically unlikely to be the live code, but
		// skip if one happens to be, rather than failing spuriously.
		if live, _ := totp.GenerateCode(secret, time.Now()); live == code {
			continue
		}
		if svc.ValidateCode(secret, code) {
			t.Errorf("ValidateCode(_, %q) = true, want false", code)
		}
	}
}

// TestValidateCode_RejectsACodeFromAnotherSecret is the property that makes
// TOTP an authentication factor at all: a code is only valid against the
// secret it was derived from.
func TestValidateCode_RejectsACodeFromAnotherSecret(t *testing.T) {
	t.Parallel()

	svc := newService()

	mine, _, err := svc.GenerateSecret(testEmail)
	if err != nil {
		t.Fatalf("GenerateSecret() error = %v", err)
	}
	theirs, _, err := svc.GenerateSecret("mallory@example.com")
	if err != nil {
		t.Fatalf("GenerateSecret() error = %v", err)
	}

	theirCode, err := totp.GenerateCode(theirs, time.Now())
	if err != nil {
		t.Fatalf("GenerateCode() error = %v", err)
	}

	if svc.ValidateCode(mine, theirCode) {
		t.Error("a code generated from a different secret was accepted")
	}
}

// TestValidateCode_RejectsAnExpiredCode: TOTP steps are 30 seconds and the
// default validator allows no skew, so a code from several minutes ago must
// not work. Without this, a code captured once could be replayed indefinitely.
func TestValidateCode_RejectsAnExpiredCode(t *testing.T) {
	t.Parallel()

	svc := newService()
	secret, _, err := svc.GenerateSecret(testEmail)
	if err != nil {
		t.Fatalf("GenerateSecret() error = %v", err)
	}

	stale, err := totp.GenerateCode(secret, time.Now().Add(-10*time.Minute))
	if err != nil {
		t.Fatalf("GenerateCode() error = %v", err)
	}

	if svc.ValidateCode(secret, stale) {
		t.Error("a code from ten minutes ago was accepted")
	}
}

func TestValidateCode_RejectsAnEmptySecret(t *testing.T) {
	t.Parallel()

	// A user with MFAEnabled but no secret must not be able to authenticate
	// with anything at all.
	if newService().ValidateCode("", "123456") {
		t.Error("ValidateCode(\"\", ...) = true, want false")
	}
}
