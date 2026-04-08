package ofbizuser

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidUserLoginID  = errors.New("invalid user login ID")
	ErrInvalidPassword     = errors.New("invalid password")
	ErrUserLoginNotFound   = errors.New("user login not found")
	ErrUserLoginDisabled   = errors.New("user login is disabled")
)

// UserLogin is the domain entity representing an Apache OFBiz UserLogin
// This is a pure domain entity without infrastructure concerns
type UserLogin struct {
	UserLoginID         string
	CurrentPassword     string
	PasswordHint        string
	IsEnabled           bool
	DisabledDateTime    *time.Time
	SuccessiveFailedLogins int
	LastFailedLoginTime *time.Time
	RequirePasswordChange bool
	ExternalAuthID      string
	PartyID             string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// Validate validates the UserLogin entity against business rules
func (u *UserLogin) Validate() error {
	if strings.TrimSpace(u.UserLoginID) == "" {
		return ErrInvalidUserLoginID
	}
	if strings.TrimSpace(u.CurrentPassword) == "" {
		return ErrInvalidPassword
	}
	return nil
}

// IsAccountLocked checks if the account is locked due to failed login attempts
func (u *UserLogin) IsAccountLocked() bool {
	return !u.IsEnabled || (u.DisabledDateTime != nil)
}

// RecordFailedLogin records a failed login attempt
func (u *UserLogin) RecordFailedLogin() {
	u.SuccessiveFailedLogins++
	now := time.Now()
	u.LastFailedLoginTime = &now
}

// ResetFailedLoginCounter resets the failed login counter
func (u *UserLogin) ResetFailedLoginCounter() {
	u.SuccessiveFailedLogins = 0
	u.LastFailedLoginTime = nil
}
