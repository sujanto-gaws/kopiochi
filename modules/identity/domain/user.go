// Package auth defines the authentication identity (table: auth_users, PK: uuid).
// This is the primary login entity — it owns password hashes, MFA state, lockout, roles, and permissions.
// It is distinct from the profile user in domain/user (table: users, PK: int64).
package auth

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID          uuid.UUID
	Username    string
	Email       string
	Name        string
	Roles       []string
	Permissions []string

	PasswordHash        string
	MFAEnabled          bool
	MFASecret           string // TOTP secret, only set when enabled
	FailedLoginAttempts int
	LockedUntil         *time.Time
}

func (u *User) IsLocked() bool {
	if u.LockedUntil == nil {
		return false
	}
	return time.Now().Before(*u.LockedUntil)
}

func (u *User) RecordFailedLogin(maxAttempts int, lockDuration time.Duration) {
	u.FailedLoginAttempts++
	if u.FailedLoginAttempts >= maxAttempts {
		lockUntil := time.Now().Add(lockDuration)
		u.LockedUntil = &lockUntil
	}
}

func (u *User) ResetFailedLogins() {
	u.FailedLoginAttempts = 0
	u.LockedUntil = nil
}
