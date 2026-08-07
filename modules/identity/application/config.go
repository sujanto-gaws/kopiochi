// Package application holds the identity module's application layer: the use cases
// that orchestrate login, refresh, logout and MFA over the domain interfaces.
//
// The package name does not match its directory (application/) because the
// import site reads better as auth.Service than application.Service.
package application

import "time"

type Config struct {
	AccessTokenTTL    time.Duration
	RefreshTokenTTL   time.Duration
	MaxFailedAttempts int
	LockDuration      time.Duration
	ClientID          string
	MFATemporaryTTL   time.Duration
}
