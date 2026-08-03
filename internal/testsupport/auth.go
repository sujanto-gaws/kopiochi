package testsupport

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

// Token classes, as they appear in the "cls" claim.
//
// These mirror modules/identity/domain.Class. They are duplicated rather than
// imported because internal/** may not import modules/** (rule R3), and the
// duplication is useful in its own right: a helper that reused the production
// constant could not detect the production constant changing.
const (
	ClassAccess = "access"
	ClassMFA    = "mfa"
	ClassID     = "id"
)

// Claims are the token fields a test may want to control. Zero values get
// working defaults, so a test that only cares about one field sets only that
// one.
type Claims struct {
	Subject     string
	Email       string
	Name        string
	Roles       []string
	Permissions []string
	Class       string
	Issuer      string
	Audience    string
	IssuedAt    time.Time
	ExpiresAt   time.Time
}

// MintToken signs an RS256 token with kp's private key.
//
// It exists so a test can produce a token the application will accept without
// first performing a login, and — more usefully — so it can produce tokens the
// application must *reject*: expired, wrong issuer, wrong audience, wrong
// class. Those are the cases that matter most (an MFA token accepted as an
// access token is a full authentication bypass) and none of them can be
// produced through the login endpoint.
func MintToken(t *testing.T, kp Keypair, c Claims) string {
	t.Helper()

	now := time.Now()
	if c.Class == "" {
		c.Class = ClassAccess
	}
	if c.Issuer == "" {
		c.Issuer = TestIssuer
	}
	if c.Audience == "" {
		c.Audience = TestAudience
	}
	if c.IssuedAt.IsZero() {
		c.IssuedAt = now
	}
	if c.ExpiresAt.IsZero() {
		c.ExpiresAt = now.Add(15 * time.Minute)
	}
	if c.Roles == nil {
		c.Roles = []string{"user"}
	}
	if c.Permissions == nil {
		c.Permissions = []string{}
	}

	claims := jwt.MapClaims{
		"sub":         c.Subject,
		"email":       c.Email,
		"name":        c.Name,
		"roles":       c.Roles,
		"permissions": c.Permissions,
		"scope":       c.Class,
		"cls":         c.Class,
		"iss":         c.Issuer,
		"aud":         c.Audience,
		"iat":         c.IssuedAt.Unix(),
		"exp":         c.ExpiresAt.Unix(),
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(kp.Private)
	require.NoError(t, err)
	return signed
}

// AccessToken is the common case: a valid, unexpired access token for subject.
func AccessToken(t *testing.T, kp Keypair, subject string) string {
	t.Helper()
	return MintToken(t, kp, Claims{Subject: subject, Class: ClassAccess})
}

// ExpiredToken is a token that was valid an hour ago. Its iat is backdated too
// — a token issued in the future and already expired is not a case any
// validator needs to handle, and asserting on it would prove nothing.
func ExpiredToken(t *testing.T, kp Keypair, subject string) string {
	t.Helper()

	now := time.Now()
	return MintToken(t, kp, Claims{
		Subject:   subject,
		Class:     ClassAccess,
		IssuedAt:  now.Add(-2 * time.Hour),
		ExpiresAt: now.Add(-1 * time.Hour),
	})
}
