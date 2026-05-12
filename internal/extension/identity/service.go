package identity

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Service defines the interface for identity business logic
type Service interface {
	// User operations
	CreateUser(ctx context.Context, userName, email, password string) (*User, error)
	GetUserByID(ctx context.Context, id string) (*User, error)
	GetUserByUserName(ctx context.Context, userName string) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	UpdateUser(ctx context.Context, user *User) error
	DeleteUser(ctx context.Context, id string) error
	VerifyPassword(user *User, password string) bool
	HashPassword(password string) (string, error)
	GenerateSecurityStamp() string
	GenerateConcurrencyStamp() string

	// Email confirmation
	ConfirmEmail(ctx context.Context, userID string) error

	// Phone confirmation
	ConfirmPhoneNumber(ctx context.Context, userID string) error

	// Lockout management
	IsLockedOut(user *User) bool
	RecordFailedAccess(ctx context.Context, user *User) error
	ResetAccessFailedCount(ctx context.Context, user *User) error

	// Role operations
	CreateRole(ctx context.Context, name string) (*Role, error)
	GetRoleByID(ctx context.Context, id string) (*Role, error)
	GetRoleByName(ctx context.Context, name string) (*Role, error)
	AddUserToRole(ctx context.Context, userID, roleID string) error
	RemoveUserFromRole(ctx context.Context, userID, roleID string) error
	GetRolesForUser(ctx context.Context, userID string) ([]*Role, error)
	GetUsersForRole(ctx context.Context, roleID string) ([]*User, error)

	// Claim operations
	AddUserClaim(ctx context.Context, userID, claimType, claimValue string) error
	DeleteUserClaim(ctx context.Context, claimID int) error
	GetClaimsForUser(ctx context.Context, userID string) ([]*UserClaim, error)
	AddRoleClaim(ctx context.Context, roleID, claimType, claimValue string) error
	DeleteRoleClaim(ctx context.Context, claimID int) error
	GetClaimsForRole(ctx context.Context, roleID string) ([]*RoleClaim, error)

	// External login operations
	AddExternalLogin(ctx context.Context, userID, loginProvider, providerKey, providerDisplayName string) error
	GetUserByExternalLogin(ctx context.Context, loginProvider, providerKey string) (*User, error)
	RemoveExternalLogin(ctx context.Context, loginProvider, providerKey string) error
	GetExternalLogins(ctx context.Context, userID string) ([]*UserLogin, error)

	// Token operations
	SetUserToken(ctx context.Context, userID, loginProvider, name, value string) error
	GetUserToken(ctx context.Context, userID, loginProvider, name string) (string, error)
	RemoveUserToken(ctx context.Context, userID, loginProvider, name string) error

	// Passkey operations
	RegisterPasskey(ctx context.Context, userID, id string, credentialID, publicKey []byte, algorithm int, transports []string, aaguid string, attestationObject []byte, clientDataJSON []byte) error
	GetPasskeysForUser(ctx context.Context, userID string) ([]*UserPasskey, error)
	GetPasskeyByCredentialID(ctx context.Context, credentialID []byte) (*UserPasskey, error)
	UpdatePasskeyCounter(ctx context.Context, passkeyID string, counter int) error
	DeletePasskey(ctx context.Context, passkeyID string) error
}

// service implements the Service interface
type service struct {
	repo              Repository
	maxFailedAccesses int
	lockoutDuration   time.Duration
}

// ServiceConfig holds configuration for the identity service
type ServiceConfig struct {
	MaxFailedAccesses int           // Maximum failed access attempts before lockout
	LockoutDuration   time.Duration // Duration to lock out a user after max failed attempts
}

// DefaultServiceConfig returns default service configuration
func DefaultServiceConfig() ServiceConfig {
	return ServiceConfig{
		MaxFailedAccesses: 5,
		LockoutDuration:   15 * time.Minute,
	}
}

// NewService creates a new identity service with the given repository and config
func NewService(repo Repository, config ServiceConfig) Service {
	return &service{
		repo:              repo,
		maxFailedAccesses: config.MaxFailedAccesses,
		lockoutDuration:   config.LockoutDuration,
	}
}

// CreateUser creates a new user with the given credentials
func (s *service) CreateUser(ctx context.Context, userName, email, password string) (*User, error) {
	id := generateID()
	now := time.Now()

	passwordHash, err := s.HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	user := &User{
		ID:                   id,
		UserName:             &userName,
		Email:                &email,
		NormalizedEmail:      stringPtr(strings.ToUpper(email)),
		EmailConfirmed:       false,
		PasswordHash:         &passwordHash,
		SecurityStamp:        stringPtr(s.GenerateSecurityStamp()),
		ConcurrencyStamp:     stringPtr(s.GenerateConcurrencyStamp()),
		PhoneNumberConfirmed: false,
		TwoFactorEnabled:     false,
		LockoutEnabled:       true,
		AccessFailedCount:    0,
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	if err := s.repo.CreateUser(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

// GetUserByID retrieves a user by ID
func (s *service) GetUserByID(ctx context.Context, id string) (*User, error) {
	return s.repo.GetUserByID(ctx, id)
}

// GetUserByUserName retrieves a user by username
func (s *service) GetUserByUserName(ctx context.Context, userName string) (*User, error) {
	return s.repo.GetUserByUserName(ctx, userName)
}

// GetUserByEmail retrieves a user by email
func (s *service) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	return s.repo.GetUserByEmail(ctx, email)
}

// UpdateUser updates an existing user
func (s *service) UpdateUser(ctx context.Context, user *User) error {
	return s.repo.UpdateUser(ctx, user)
}

// DeleteUser removes a user by ID
func (s *service) DeleteUser(ctx context.Context, id string) error {
	return s.repo.DeleteUser(ctx, id)
}

// VerifyPassword checks if the provided password matches the user's password hash
func (s *service) VerifyPassword(user *User, password string) bool {
	if user.PasswordHash == nil {
		return false
	}
	err := bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(password))
	return err == nil
}

// HashPassword hashes a password using bcrypt
func (s *service) HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// GenerateSecurityStamp generates a random security stamp
func (s *service) GenerateSecurityStamp() string {
	return generateRandomString(32)
}

// GenerateConcurrencyStamp generates a random concurrency stamp
func (s *service) GenerateConcurrencyStamp() string {
	return generateRandomString(36)
}

// ConfirmEmail marks a user's email as confirmed
func (s *service) ConfirmEmail(ctx context.Context, userID string) error {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	user.EmailConfirmed = true
	return s.repo.UpdateUser(ctx, user)
}

// ConfirmPhoneNumber marks a user's phone number as confirmed
func (s *service) ConfirmPhoneNumber(ctx context.Context, userID string) error {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	user.PhoneNumberConfirmed = true
	return s.repo.UpdateUser(ctx, user)
}

// IsLockedOut checks if a user is currently locked out
func (s *service) IsLockedOut(user *User) bool {
	if !user.LockoutEnabled || user.LockoutEnd == nil {
		return false
	}
	return user.LockoutEnd.After(time.Now())
}

// RecordFailedAccess records a failed access attempt and locks out if necessary
func (s *service) RecordFailedAccess(ctx context.Context, user *User) error {
	user.AccessFailedCount++
	if user.AccessFailedCount >= s.maxFailedAccesses && user.LockoutEnabled {
		lockoutEnd := time.Now().Add(s.lockoutDuration)
		user.LockoutEnd = &lockoutEnd
	}
	return s.repo.UpdateUser(ctx, user)
}

// ResetAccessFailedCount resets the failed access count and clears lockout
func (s *service) ResetAccessFailedCount(ctx context.Context, user *User) error {
	user.AccessFailedCount = 0
	user.LockoutEnd = nil
	return s.repo.UpdateUser(ctx, user)
}

// CreateRole creates a new role
func (s *service) CreateRole(ctx context.Context, name string) (*Role, error) {
	id := generateID()
	role := &Role{
		ID:             id,
		Name:           &name,
		NormalizedName: stringPtr(strings.ToUpper(name)),
	}
	if err := s.repo.CreateRole(ctx, role); err != nil {
		return nil, err
	}
	return role, nil
}

// GetRoleByID retrieves a role by ID
func (s *service) GetRoleByID(ctx context.Context, id string) (*Role, error) {
	return s.repo.GetRoleByID(ctx, id)
}

// GetRoleByName retrieves a role by name
func (s *service) GetRoleByName(ctx context.Context, name string) (*Role, error) {
	return s.repo.GetRoleByName(ctx, name)
}

// AddUserToRole adds a user to a role
func (s *service) AddUserToRole(ctx context.Context, userID, roleID string) error {
	return s.repo.AddUserToRole(ctx, userID, roleID)
}

// RemoveUserFromRole removes a user from a role
func (s *service) RemoveUserFromRole(ctx context.Context, userID, roleID string) error {
	return s.repo.RemoveUserFromRole(ctx, userID, roleID)
}

// GetRolesForUser retrieves all roles for a user
func (s *service) GetRolesForUser(ctx context.Context, userID string) ([]*Role, error) {
	return s.repo.GetRolesForUser(ctx, userID)
}

// GetUsersForRole retrieves all users for a role
func (s *service) GetUsersForRole(ctx context.Context, roleID string) ([]*User, error) {
	return s.repo.GetUsersForRole(ctx, roleID)
}

// AddUserClaim adds a claim for a user
func (s *service) AddUserClaim(ctx context.Context, userID, claimType, claimValue string) error {
	claim := &UserClaim{
		UserID:     userID,
		ClaimType:  &claimType,
		ClaimValue: &claimValue,
	}
	return s.repo.AddUserClaim(ctx, claim)
}

// DeleteUserClaim deletes a user claim by ID
func (s *service) DeleteUserClaim(ctx context.Context, claimID int) error {
	return s.repo.DeleteUserClaim(ctx, claimID)
}

// GetClaimsForUser retrieves all claims for a user
func (s *service) GetClaimsForUser(ctx context.Context, userID string) ([]*UserClaim, error) {
	return s.repo.GetClaimsForUser(ctx, userID)
}

// AddRoleClaim adds a claim for a role
func (s *service) AddRoleClaim(ctx context.Context, roleID, claimType, claimValue string) error {
	claim := &RoleClaim{
		RoleID:     roleID,
		ClaimType:  &claimType,
		ClaimValue: &claimValue,
	}
	return s.repo.AddRoleClaim(ctx, claim)
}

// DeleteRoleClaim deletes a role claim by ID
func (s *service) DeleteRoleClaim(ctx context.Context, claimID int) error {
	return s.repo.DeleteRoleClaim(ctx, claimID)
}

// GetClaimsForRole retrieves all claims for a role
func (s *service) GetClaimsForRole(ctx context.Context, roleID string) ([]*RoleClaim, error) {
	return s.repo.GetClaimsForRole(ctx, roleID)
}

// AddExternalLogin adds an external login provider for a user
func (s *service) AddExternalLogin(ctx context.Context, userID, loginProvider, providerKey, providerDisplayName string) error {
	login := &UserLogin{
		LoginProvider:       loginProvider,
		ProviderKey:         providerKey,
		ProviderDisplayName: &providerDisplayName,
		UserID:              userID,
	}
	return s.repo.AddUserLogin(ctx, login)
}

// GetUserByExternalLogin retrieves a user by external login provider and key
func (s *service) GetUserByExternalLogin(ctx context.Context, loginProvider, providerKey string) (*User, error) {
	return s.repo.GetUserByLogin(ctx, loginProvider, providerKey)
}

// RemoveExternalLogin removes an external login
func (s *service) RemoveExternalLogin(ctx context.Context, loginProvider, providerKey string) error {
	return s.repo.RemoveUserLogin(ctx, loginProvider, providerKey)
}

// GetExternalLogins retrieves all external logins for a user
func (s *service) GetExternalLogins(ctx context.Context, userID string) ([]*UserLogin, error) {
	return s.repo.GetUserLogins(ctx, userID)
}

// SetUserToken sets or updates a user token
func (s *service) SetUserToken(ctx context.Context, userID, loginProvider, name, value string) error {
	token := &UserToken{
		UserID:        userID,
		LoginProvider: loginProvider,
		Name:          name,
		Value:         &value,
	}
	return s.repo.SetUserToken(ctx, token)
}

// GetUserToken retrieves a user token value
func (s *service) GetUserToken(ctx context.Context, userID, loginProvider, name string) (string, error) {
	token, err := s.repo.GetUserToken(ctx, userID, loginProvider, name)
	if err != nil {
		return "", err
	}
	if token.Value == nil {
		return "", nil
	}
	return *token.Value, nil
}

// RemoveUserToken removes a user token
func (s *service) RemoveUserToken(ctx context.Context, userID, loginProvider, name string) error {
	return s.repo.RemoveUserToken(ctx, userID, loginProvider, name)
}

// RegisterPasskey registers a new passkey for a user
func (s *service) RegisterPasskey(ctx context.Context, userID, id string, credentialID, publicKey []byte, algorithm int, transports []string, aaguid string, attestationObject []byte, clientDataJSON []byte) error {
	passkey := &UserPasskey{
		ID:                id,
		UserID:            userID,
		CredentialID:      credentialID,
		PublicKey:         publicKey,
		Algorithm:         algorithm,
		Counter:           0,
		Transports:        marshalJSON(transports),
		AAGUID:            stringPtr(aaguid),
		CreatedAt:         time.Now(),
		IsBackupEligible:  false,
		IsBackedUp:        false,
		AttestationObject: attestationObject,
		ClientDataJSON:    json.RawMessage(clientDataJSON),
	}
	return s.repo.AddPasskey(ctx, passkey)
}

// GetPasskeysForUser retrieves all passkeys for a user
func (s *service) GetPasskeysForUser(ctx context.Context, userID string) ([]*UserPasskey, error) {
	return s.repo.GetPasskeysForUser(ctx, userID)
}

// GetPasskeyByCredentialID retrieves a passkey by credential ID
func (s *service) GetPasskeyByCredentialID(ctx context.Context, credentialID []byte) (*UserPasskey, error) {
	return s.repo.GetPasskeyByCredentialID(ctx, credentialID)
}

// UpdatePasskeyCounter updates the counter for a passkey
func (s *service) UpdatePasskeyCounter(ctx context.Context, passkeyID string, counter int) error {
	return s.repo.UpdatePasskeyCounter(ctx, passkeyID, counter)
}

// DeletePasskey removes a passkey
func (s *service) DeletePasskey(ctx context.Context, passkeyID string) error {
	return s.repo.DeletePasskey(ctx, passkeyID)
}

// Helper functions

func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func generateRandomString(length int) string {
	b := make([]byte, length/2)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func stringPtr(s string) *string {
	return &s
}

// marshalJSON marshals a value to json.RawMessage, returning nil on error.
func marshalJSON(v interface{}) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}
