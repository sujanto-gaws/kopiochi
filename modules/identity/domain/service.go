package auth

import (
	"errors"
	"time"
)

// claimsContextKey is an unexported type for context keys in this package.
// Using an unexported type prevents collisions with keys from other packages.
type claimsContextKey struct{ name string }

// ClaimsKey is the context key under which JWT Claims are stored by AuthRequired middleware.
var ClaimsKey = &claimsContextKey{"claims"}

// Class identifies what a token is for. It is carried on every token as the
// "cls" claim and MUST be checked by every caller of Validate — that is what
// makes it structurally impossible for one token kind to be accepted where
// another is expected (e.g. a half-authenticated MFA token used as an API
// access token), rather than relying on every call site remembering to
// compare a convention-only field such as "scope".
type Class string

const (
	// ClassAccess is a fully-authenticated API access token.
	ClassAccess Class = "access"
	// ClassMFA is issued after password verification but before the second
	// factor is verified. It grants no API access — it is only ever valid
	// against the /auth/mfa/verify endpoint.
	ClassMFA Class = "mfa"
	// ClassID is an OIDC-style identity token describing the user to a
	// client; it is not an API access credential.
	ClassID Class = "id"
)

// ErrWrongTokenClass is returned by Validate when a token parses and
// verifies correctly but carries a different class than the caller expects.
var ErrWrongTokenClass = errors.New("token: wrong class")

type Claims struct {
	Subject     string   `json:"sub"`
	Email       string   `json:"email"`
	Name        string   `json:"name"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
	Scope       string   `json:"scope"`
	Class       Class    `json:"cls"`
	IssuedAt    int64    `json:"iat"`
	ExpiresAt   int64    `json:"exp"`
}

type TokenIssuer interface {
	IssueAccessToken(user User, ttl time.Duration) (string, error)
	IssueIDToken(user User, clientID string) (string, error)
	IssueMFAToken(user User) (string, error)
	// Validate parses and verifies tokenStr (signature, algorithm, issuer,
	// audience, expiry) and additionally requires its "cls" claim to equal
	// want; any other class is rejected with ErrWrongTokenClass, not by a
	// caller-side string comparison.
	Validate(tokenStr string, want Class) (*Claims, error)
}

type PasswordHasher interface {
	Hash(plain string) (string, error)
	Verify(plain, hashed string) bool
}

type MFAService interface {
	GenerateSecret(email string) (secret string, qrCodeURL string, err error)
	ValidateCode(secret, code string) bool
}

// ErrRefreshTokenAlreadyUsed is returned when a refresh token that has already
// been exchanged is presented again.
//
// It is a domain error rather than a storage detail because of what it means:
// the legitimate holder and someone else both have a token from the same
// chain. That is either theft or a client bug, and either way the family must
// be revoked rather than the request merely refused.
var ErrRefreshTokenAlreadyUsed = errors.New("refresh token: already used")
