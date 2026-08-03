package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

type RefreshToken struct {
	UserID    string
	TokenHash string
	ExpiresAt time.Time

	// FamilyID chains every refresh token descending from a single login.
	//
	// Rotation alone does not survive theft: after a stolen token is used,
	// both the attacker and the user hold something that looks valid, and
	// whoever refreshes second presents an already-used token. Revoking only
	// that one token leaves the other party — usually the attacker, who acts
	// faster — with a working session. Revoking the family logs out both and
	// forces a real re-authentication.
	FamilyID string
	// Used reports whether this token has already been exchanged. Presenting a
	// used token is the reuse signal.
	Used bool
	// Revoked reports whether the token was invalidated, by logout, by
	// rotation, or by a reuse detection on its family.
	Revoked bool
}

// Expired reports whether the token is past its lifetime, as measured by the
// caller's clock.
//
// The authoritative check is in SQL (`expires_at > now()`), using the
// database's clock — this exists for the service layer to reason about a token
// it already holds, not as a substitute for that.
func (t RefreshToken) Expired(now time.Time) bool {
	return !t.ExpiresAt.After(now)
}

// Usable reports whether this token may be exchanged: not revoked, not already
// used, not expired.
func (t RefreshToken) Usable(now time.Time) bool {
	return !t.Revoked && !t.Used && !t.Expired(now)
}

func HashToken(plain string) string {
	h := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(h[:])
}
