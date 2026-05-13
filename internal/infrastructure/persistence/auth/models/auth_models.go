package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// BunUser maps to the "users" table.
// It extends your existing User table; add bun tags only for the columns we use in auth.
type BunUser struct {
	bun.BaseModel `bun:"table:auth_users,alias:u"`

	ID                  uuid.UUID  `bun:"id,pk,type:uuid,default:gen_random_uuid()"`
	Username            string     `bun:"username,notnull"`
	Email               string     `bun:"email,notnull"`
	Name                string     `bun:"name"`
	Roles               []string   `bun:"roles,array"`
	Permissions         []string   `bun:"permissions,array"`
	PasswordHash        string     `bun:"password_hash"`
	MFAEnabled          bool       `bun:"mfa_enabled,default:false"`
	MFASecret           string     `bun:"mfa_secret"`
	FailedLoginAttempts int        `bun:"failed_login_attempts,default:0"`
	LockedUntil         *time.Time `bun:"locked_until"`
	CreatedAt           time.Time  `bun:"created_at,default:now()"`
	UpdatedAt           time.Time  `bun:"updated_at,default:now()"`
}

// RefreshTokenRow maps to "refresh_tokens"
type RefreshTokenRow struct {
	bun.BaseModel `bun:"table:auth_refresh_tokens,alias:rt"`

	ID        uuid.UUID `bun:"id,pk,type:uuid,default:gen_random_uuid()"`
	UserID    uuid.UUID `bun:"user_id,notnull"`
	TokenHash string    `bun:"token_hash,notnull"`
	ExpiresAt time.Time `bun:"expires_at,notnull"`
	Revoked   bool      `bun:"revoked,default:false"`
	CreatedAt time.Time `bun:"created_at,default:now()"`
}

// MfaBackupCodeRow maps to "mfa_backup_codes"
type MfaBackupCodeRow struct {
	bun.BaseModel `bun:"table:auth_mfa_backup_codes,alias:bc"`

	ID        uuid.UUID `bun:"id,pk,type:uuid,default:gen_random_uuid()"`
	UserID    uuid.UUID `bun:"user_id,notnull"`
	CodeHash  string    `bun:"code_hash,notnull"`
	Used      bool      `bun:"used,default:false"`
	CreatedAt time.Time `bun:"created_at,default:now()"`
}
