package auth

import "time"

type Config struct {
	AccessTokenTTL     time.Duration
	RefreshTokenTTL    time.Duration
	MaxFailedAttempts  int
	LockDuration       time.Duration
	ClientID           string
	MFATemporaryTTL    time.Duration
}
