package auth

import "time"

// claimsContextKey is an unexported type for context keys in this package.
// Using an unexported type prevents collisions with keys from other packages.
type claimsContextKey struct{ name string }

// ClaimsKey is the context key under which JWT Claims are stored by AuthRequired middleware.
var ClaimsKey = &claimsContextKey{"claims"}

type Claims struct {
	Subject     string   `json:"sub"`
	Email       string   `json:"email"`
	Name        string   `json:"name"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
	Scope       string   `json:"scope"`
	IssuedAt    int64    `json:"iat"`
	ExpiresAt   int64    `json:"exp"`
}

type TokenIssuer interface {
	IssueAccessToken(user User, ttl time.Duration) (string, error)
	IssueIDToken(user User, clientID string) (string, error)
	IssueMFAToken(user User) (string, error)
	Validate(tokenStr string) (*Claims, error)
}

type PasswordHasher interface {
	Hash(plain string) (string, error)
	Verify(plain, hashed string) bool
}

type MFAService interface {
	GenerateSecret(email string) (secret string, qrCodeURL string, err error)
	ValidateCode(secret, code string) bool
}
