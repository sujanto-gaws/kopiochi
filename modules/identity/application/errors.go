package application

import "errors"

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrAccountLocked      = errors.New("account locked")
	ErrMFARequired        = errors.New("mfa required")
	ErrInvalidMFAToken    = errors.New("invalid mfa token")
	ErrInvalidMFACode     = errors.New("invalid mfa code")

	// ErrMFANotStarted is returned when a caller tries to confirm an MFA setup
	// that was never begun, so no secret exists to check the code against.
	//
	// Distinct from ErrInvalidMFACode on purpose: the code is not wrong, there
	// is nothing to be right about. Collapsing the two would tell a user their
	// authenticator is broken when the real answer is "call /auth/mfa/setup
	// first", and would hide the state E10 is about from anyone reading logs.
	ErrMFANotStarted       = errors.New("mfa setup not started")
	ErrRefreshTokenInvalid = errors.New("refresh token invalid or expired")
)

type MFAError struct {
	Token string
	User  UserDTO
}

func (e *MFAError) Error() string {
	return "mfa required"
}
