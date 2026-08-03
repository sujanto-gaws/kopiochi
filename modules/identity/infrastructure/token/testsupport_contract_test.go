package token

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/sujanto-gaws/kopiochi/internal/testsupport"
	domain "github.com/sujanto-gaws/kopiochi/modules/identity/domain"
)

// internal/testsupport mints JWTs with golang-jwt directly rather than through
// this package, because dependency rule R3 forbids internal/** from importing
// modules/**. That duplication buys independence — a fixture built from the
// code under test cannot detect that code changing — but it only stays honest
// if something checks the two still agree.
//
// This file is that check, and it lives here because this is the one side of
// the boundary allowed to see both: modules/** may import internal/**.
//
// If a claim name, the signing algorithm, or the class vocabulary changes in
// jwt.go, these tests fail and internal/testsupport/auth.go needs the same
// change. Without them the failure mode is silent and much worse: every
// authenticated test keeps passing against a token shape the application
// stopped issuing.

// serviceForKeypair builds a JWTService over a testsupport keypair, so both
// sides are working with the same key material.
func serviceForKeypair(t *testing.T, kp testsupport.Keypair) *JWTService {
	t.Helper()

	svc, err := NewJWTService(kp.PrivatePath, kp.PublicPath,
		testsupport.TestIssuer, testsupport.TestAudience, time.Second)
	require.NoError(t, err)
	return svc
}

func TestTestSupportToken_ValidatesAsAGenuineAccessToken(t *testing.T) {
	t.Parallel()

	kp := testsupport.NewKeypair(t)
	svc := serviceForKeypair(t, kp)
	subject := uuid.New().String()

	claims, err := svc.Validate(testsupport.AccessToken(t, kp, subject), domain.ClassAccess)
	require.NoError(t, err,
		"internal/testsupport mints a token this service rejects — the two have drifted apart")
	require.Equal(t, subject, claims.Subject)
	require.Equal(t, domain.ClassAccess, claims.Class)
}

// TestTestSupportClassConstants_MatchTheDomain catches the cheapest way for
// the two to drift: renaming a class in one place only.
func TestTestSupportClassConstants_MatchTheDomain(t *testing.T) {
	t.Parallel()

	require.Equal(t, string(domain.ClassAccess), testsupport.ClassAccess)
	require.Equal(t, string(domain.ClassMFA), testsupport.ClassMFA)
	require.Equal(t, string(domain.ClassID), testsupport.ClassID)
}

// TestTestSupportToken_RejectionCasesAreActuallyRejected is the reason the
// helper can mint arbitrary claims at all. These tokens cannot be obtained
// from the login endpoint, so without minting them there is no way to test
// that the application refuses them — and "MFA token accepted as an access
// token" is a complete authentication bypass.
func TestTestSupportToken_RejectionCasesAreActuallyRejected(t *testing.T) {
	t.Parallel()

	kp := testsupport.NewKeypair(t)
	svc := serviceForKeypair(t, kp)
	subject := uuid.New().String()

	tests := []struct {
		name  string
		token func() string
	}{
		{
			name:  "expired",
			token: func() string { return testsupport.ExpiredToken(t, kp, subject) },
		},
		{
			name: "wrong issuer",
			token: func() string {
				return testsupport.MintToken(t, kp, testsupport.Claims{
					Subject: subject, Issuer: "https://attacker.example",
				})
			},
		},
		{
			name: "wrong audience",
			token: func() string {
				return testsupport.MintToken(t, kp, testsupport.Claims{
					Subject: subject, Audience: "some-other-client",
				})
			},
		},
		{
			name: "mfa token presented as an access token",
			token: func() string {
				return testsupport.MintToken(t, kp, testsupport.Claims{
					Subject: subject, Class: testsupport.ClassMFA,
				})
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := svc.Validate(tc.token(), domain.ClassAccess)
			require.Error(t, err, "a %s token was accepted as a valid access token", tc.name)
		})
	}
}

// TestTestSupportToken_SignedByADifferentKeyIsRejected guards the other
// direction: the helper must not be able to mint a token any service accepts,
// only one signed by the keypair that service was built with.
func TestTestSupportToken_SignedByADifferentKeyIsRejected(t *testing.T) {
	t.Parallel()

	svc := serviceForKeypair(t, testsupport.NewKeypair(t))
	other := testsupport.NewKeypair(t)

	_, err := svc.Validate(testsupport.AccessToken(t, other, uuid.New().String()), domain.ClassAccess)
	require.Error(t, err, "a token signed by an unrelated key was accepted")
}
