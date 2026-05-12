// This file defines the ASP.NET Identity-inspired data model for the identity extension
// (tables: sec_user, sec_roles, sec_user_roles, sec_user_claim, sec_role_claim).
// This is the most complete user/role/claims system in the codebase. It is distinct from:
//   - domain/auth  (auth_users): the core login identity used by the HTTP auth layer
//   - domain/user  (users): a simple profile CRUD entity
//   - domain/ofbizuser (user_login): OFBiz compatibility
package identity

import (
	"encoding/json"
	"time"
)

// User represents the sec_user table entity
type User struct {
	ID                   string     `bun:"id,pk"`
	UserName             *string    `bun:"user_name"`
	Email                *string    `bun:"email"`
	NormalizedEmail      *string    `bun:"normalized_email"`
	EmailConfirmed       bool       `bun:"email_confirmed,notnull,default:false"`
	PasswordHash         *string    `bun:"password_hash"`
	SecurityStamp        *string    `bun:"security_stamp"`
	ConcurrencyStamp     *string    `bun:"concurrency_stamp"`
	PhoneNumber          *string    `bun:"phone_number"`
	PhoneNumberConfirmed bool       `bun:"phone_number_confirmed,notnull,default:false"`
	TwoFactorEnabled     bool       `bun:"two_factor_enabled,notnull,default:false"`
	LockoutEnd           *time.Time `bun:"lockout_end"`
	LockoutEnabled       bool       `bun:"lockout_enable,notnull,default:false"`
	AccessFailedCount    int        `bun:"access_failed_count,notnull,default:0"`
	CreatedAt            time.Time  `bun:"created_at,notnull,default:now()"`
	UpdatedAt            time.Time  `bun:"updated_at,notnull,default:now()"`
}

// TableName overrides the table name used by Bun
func (User) TableName() string {
	return "sec_user"
}

// Role represents the sec_roles table entity
type Role struct {
	ID             string  `bun:"id,pk"`
	Name           *string `bun:"name"`
	NormalizedName *string `bun:"normalized_name"`
}

// TableName overrides the table name used by Bun
func (Role) TableName() string {
	return "sec_roles"
}

// UserRole represents the sec_user_roles junction table
type UserRole struct {
	UserID string `bun:"sec_user_id,pk"`
	RoleID string `bun:"sec_role_id,pk"`
}

// TableName overrides the table name used by Bun
func (UserRole) TableName() string {
	return "sec_user_roles"
}

// UserClaim represents the sec_user_claim table entity
type UserClaim struct {
	ID         int     `bun:"id,pk,autoincrement"`
	UserID     string  `bun:"sec_user_id,notnull"`
	ClaimType  *string `bun:"claim_type"`
	ClaimValue *string `bun:"claim_value"`
}

// TableName overrides the table name used by Bun
func (UserClaim) TableName() string {
	return "sec_user_claim"
}

// RoleClaim represents the sec_role_claim table entity
type RoleClaim struct {
	ID         int     `bun:"id,pk,autoincrement"`
	RoleID     string  `bun:"sec_role_id,notnull"`
	ClaimType  *string `bun:"claim_type"`
	ClaimValue *string `bun:"claim_value"`
}

// TableName overrides the table name used by Bun
func (RoleClaim) TableName() string {
	return "sec_role_claim"
}

// UserLogin represents the sec_user_login table entity for external logins
type UserLogin struct {
	LoginProvider       string  `bun:"login_provider,pk"`
	ProviderKey         string  `bun:"provider_key,pk"`
	ProviderDisplayName *string `bun:"provider_display_name"`
	UserID              string  `bun:"sec_user_id,notnull"`
}

// TableName overrides the table name used by Bun
func (UserLogin) TableName() string {
	return "sec_user_login"
}

// UserToken represents the sec_user_token table entity
type UserToken struct {
	UserID        string  `bun:"sec_user_id,pk"`
	LoginProvider string  `bun:"login_provider,pk"`
	Name          string  `bun:"name,pk"`
	Value         *string `bun:"value"`
}

// TableName overrides the table name used by Bun
func (UserToken) TableName() string {
	return "sec_user_token"
}

// UserPasskey represents the sec_user_passkeys table entity for FIDO2/WebAuthn credentials
type UserPasskey struct {
	ID                string          `bun:"id,pk"`
	UserID            string          `bun:"sec_user_id,notnull"`
	CredentialID      []byte          `bun:"credential_id,notnull"`
	PublicKey         []byte          `bun:"public_key,notnull"`
	Algorithm         int             `bun:"algorithm,notnull"`
	Counter           int             `bun:"counter,notnull,default:0"`
	Transports        json.RawMessage `bun:"transports"`
	AAGUID            *string         `bun:"aaguid"` // UUID stored as string for compatibility
	CreatedAt         time.Time       `bun:"created_at,notnull,default:now()"`
	LastUsedAt        *time.Time      `bun:"last_used_at"`
	IsBackupEligible  bool            `bun:"is_backup_eligible,notnull,default:false"`
	IsBackedUp        bool            `bun:"is_backed_up,notnull,default:false"`
	AttestationObject []byte          `bun:"attestation_object"`
	ClientDataJSON    json.RawMessage `bun:"client_data_json"`
}

// TableName overrides the table name used by Bun
func (UserPasskey) TableName() string {
	return "sec_user_passkeys"
}
