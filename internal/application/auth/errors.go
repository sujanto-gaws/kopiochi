package auth

import "errors"

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrAccountLocked      = errors.New("account locked")
	ErrMFARequired        = errors.New("mfa required")
	ErrInvalidMFAToken    = errors.New("invalid mfa token")
	ErrInvalidMFACode     = errors.New("invalid mfa code")
	ErrRefreshTokenInvalid = errors.New("refresh token invalid or expired")
)

type MFAError struct {
	Token string
	User  UserDTO
}

func (e *MFAError) Error() string {
	return "mfa required"
}