// Package models holds the identity module's bun row structs.
//
// These are the storage shape, not the domain shape: repositories translate
// between the two so the domain never depends on the ORM.
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

	ID          uuid.UUID `bun:"id,pk,type:uuid,default:gen_random_uuid()"`
	Username    string    `bun:"username,notnull"`
	Email       string    `bun:"email,notnull"`
	Name        string    `bun:"name"`
	Roles       []string  `bun:"roles,array"`
	Permissions []string  `bun:"permissions,array"`
	// PasswordHash and MFASecret are pointers because their columns are
	// NULLABLE, and "never set" is a different fact from "set to the empty
	// string". A plain string made those arrive identically, which is BL33's
	// class and E10's instance: an account that never ran MFA setup presented
	// as one whose secret is "", and the TOTP code for the empty secret is
	// computable by anyone with a clock.
	//
	// The conversion to the domain's plain strings is in toDomainUser, where it
	// is a decision somebody wrote down rather than something bun's scanner did
	// silently.
	PasswordHash        *string    `bun:"password_hash"`
	MFAEnabled          bool       `bun:"mfa_enabled,default:false"`
	MFASecret           *string    `bun:"mfa_secret"`
	FailedLoginAttempts int        `bun:"failed_login_attempts,default:0"`
	LockedUntil         *time.Time `bun:"locked_until"`
	CreatedAt           time.Time  `bun:"created_at,default:now()"`
	UpdatedAt           time.Time  `bun:"updated_at,default:now()"`
}

// RefreshTokenRow maps to "auth_refresh_tokens".
type RefreshTokenRow struct {
	bun.BaseModel `bun:"table:auth_refresh_tokens,alias:rt"`

	ID        uuid.UUID `bun:"id,pk,type:uuid,default:gen_random_uuid()"`
	UserID    uuid.UUID `bun:"user_id,notnull"`
	TokenHash string    `bun:"token_hash,notnull"`
	ExpiresAt time.Time `bun:"expires_at,notnull"`
	Revoked   bool      `bun:"revoked,default:false"`
	CreatedAt time.Time `bun:"created_at,default:now()"`

	// FamilyID chains every token descending from one login. Reuse anywhere in
	// the chain revokes all of it — see migration 00006.
	FamilyID uuid.UUID `bun:"family_id,type:uuid,default:gen_random_uuid()"`
	// UsedAt is set when the token is exchanged. Presenting a token that
	// already has one is the theft signal.
	UsedAt *time.Time `bun:"used_at"`
	// ReuseDetectedAt distinguishes a family revoked for a security event from
	// one revoked by an ordinary logout.
	ReuseDetectedAt *time.Time `bun:"reuse_detected_at"`
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
