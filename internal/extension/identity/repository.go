package identity

import (
	"context"

	"github.com/uptrace/bun"
)

// Repository defines the interface for identity data access
type Repository interface {
	// User operations
	CreateUser(ctx context.Context, user *User) error
	GetUserByID(ctx context.Context, id string) (*User, error)
	GetUserByUserName(ctx context.Context, userName string) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	GetUserByNormalizedEmail(ctx context.Context, normalizedEmail string) (*User, error)
	UpdateUser(ctx context.Context, user *User) error
	DeleteUser(ctx context.Context, id string) error

	// Role operations
	CreateRole(ctx context.Context, role *Role) error
	GetRoleByID(ctx context.Context, id string) (*Role, error)
	GetRoleByName(ctx context.Context, name string) (*Role, error)
	GetRoleByNormalizedName(ctx context.Context, normalizedName string) (*Role, error)
	UpdateRole(ctx context.Context, role *Role) error
	DeleteRole(ctx context.Context, id string) error

	// User-Role operations
	AddUserToRole(ctx context.Context, userID, roleID string) error
	RemoveUserFromRole(ctx context.Context, userID, roleID string) error
	GetRolesForUser(ctx context.Context, userID string) ([]*Role, error)
	GetUsersForRole(ctx context.Context, roleID string) ([]*User, error)

	// User Claim operations
	AddUserClaim(ctx context.Context, claim *UserClaim) error
	DeleteUserClaim(ctx context.Context, id int) error
	GetClaimsForUser(ctx context.Context, userID string) ([]*UserClaim, error)

	// Role Claim operations
	AddRoleClaim(ctx context.Context, claim *RoleClaim) error
	DeleteRoleClaim(ctx context.Context, id int) error
	GetClaimsForRole(ctx context.Context, roleID string) ([]*RoleClaim, error)

	// User Login operations
	AddUserLogin(ctx context.Context, login *UserLogin) error
	GetUserByLogin(ctx context.Context, loginProvider, providerKey string) (*User, error)
	RemoveUserLogin(ctx context.Context, loginProvider, providerKey string) error
	GetUserLogins(ctx context.Context, userID string) ([]*UserLogin, error)

	// User Token operations
	SetUserToken(ctx context.Context, token *UserToken) error
	GetUserToken(ctx context.Context, userID, loginProvider, name string) (*UserToken, error)
	RemoveUserToken(ctx context.Context, userID, loginProvider, name string) error

	// Passkey operations
	AddPasskey(ctx context.Context, passkey *UserPasskey) error
	GetPasskeysForUser(ctx context.Context, userID string) ([]*UserPasskey, error)
	GetPasskeyByCredentialID(ctx context.Context, credentialID []byte) (*UserPasskey, error)
	UpdatePasskeyCounter(ctx context.Context, id string, counter int) error
	UpdatePasskeyLastUsed(ctx context.Context, id string) error
	DeletePasskey(ctx context.Context, id string) error
}

// repository implements the Repository interface
type repository struct {
	db bun.IDB
}

// NewRepository creates a new identity repository
func NewRepository(db bun.IDB) Repository {
	return &repository{db: db}
}

// CreateUser persists a new user
func (r *repository) CreateUser(ctx context.Context, user *User) error {
	_, err := r.db.NewInsert().Model(user).Exec(ctx)
	return err
}

// GetUserByID retrieves a user by ID
func (r *repository) GetUserByID(ctx context.Context, id string) (*User, error) {
	var user User
	err := r.db.NewSelect().Model(&user).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetUserByUserName retrieves a user by username
func (r *repository) GetUserByUserName(ctx context.Context, userName string) (*User, error) {
	var user User
	err := r.db.NewSelect().Model(&user).Where("user_name = ?", userName).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetUserByEmail retrieves a user by email
func (r *repository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	var user User
	err := r.db.NewSelect().Model(&user).Where("email = ?", email).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetUserByNormalizedEmail retrieves a user by normalized email
func (r *repository) GetUserByNormalizedEmail(ctx context.Context, normalizedEmail string) (*User, error) {
	var user User
	err := r.db.NewSelect().Model(&user).Where("normalized_email = ?", normalizedEmail).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// UpdateUser updates an existing user
func (r *repository) UpdateUser(ctx context.Context, user *User) error {
	_, err := r.db.NewUpdate().Model(user).WherePK().Exec(ctx)
	return err
}

// DeleteUser removes a user by ID
func (r *repository) DeleteUser(ctx context.Context, id string) error {
	_, err := r.db.NewDelete().Model((*User)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}

// CreateRole persists a new role
func (r *repository) CreateRole(ctx context.Context, role *Role) error {
	_, err := r.db.NewInsert().Model(role).Exec(ctx)
	return err
}

// GetRoleByID retrieves a role by ID
func (r *repository) GetRoleByID(ctx context.Context, id string) (*Role, error) {
	var role Role
	err := r.db.NewSelect().Model(&role).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return &role, nil
}

// GetRoleByName retrieves a role by name
func (r *repository) GetRoleByName(ctx context.Context, name string) (*Role, error) {
	var role Role
	err := r.db.NewSelect().Model(&role).Where("name = ?", name).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return &role, nil
}

// GetRoleByNormalizedName retrieves a role by normalized name
func (r *repository) GetRoleByNormalizedName(ctx context.Context, normalizedName string) (*Role, error) {
	var role Role
	err := r.db.NewSelect().Model(&role).Where("normalized_name = ?", normalizedName).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return &role, nil
}

// UpdateRole updates an existing role
func (r *repository) UpdateRole(ctx context.Context, role *Role) error {
	_, err := r.db.NewUpdate().Model(role).WherePK().Exec(ctx)
	return err
}

// DeleteRole removes a role by ID
func (r *repository) DeleteRole(ctx context.Context, id string) error {
	_, err := r.db.NewDelete().Model((*Role)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}

// AddUserToRole adds a user to a role
func (r *repository) AddUserToRole(ctx context.Context, userID, roleID string) error {
	userRole := &UserRole{UserID: userID, RoleID: roleID}
	_, err := r.db.NewInsert().Model(userRole).Exec(ctx)
	return err
}

// RemoveUserFromRole removes a user from a role
func (r *repository) RemoveUserFromRole(ctx context.Context, userID, roleID string) error {
	_, err := r.db.NewDelete().Model((*UserRole)(nil)).Where("sec_user_id = ? AND sec_role_id = ?", userID, roleID).Exec(ctx)
	return err
}

// GetRolesForUser retrieves all roles for a user
func (r *repository) GetRolesForUser(ctx context.Context, userID string) ([]*Role, error) {
	var roles []*Role
	err := r.db.NewSelect().
		Model(&roles).
		Join("JOIN sec_user_roles AS ur ON ur.sec_role_id = roles.id").
		Where("ur.sec_user_id = ?", userID).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return roles, nil
}

// GetUsersForRole retrieves all users for a role
func (r *repository) GetUsersForRole(ctx context.Context, roleID string) ([]*User, error) {
	var users []*User
	err := r.db.NewSelect().
		Model(&users).
		Join("JOIN sec_user_roles AS ur ON ur.sec_user_id = users.id").
		Where("ur.sec_role_id = ?", roleID).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return users, nil
}

// AddUserClaim adds a claim for a user
func (r *repository) AddUserClaim(ctx context.Context, claim *UserClaim) error {
	_, err := r.db.NewInsert().Model(claim).Exec(ctx)
	return err
}

// DeleteUserClaim deletes a user claim by ID
func (r *repository) DeleteUserClaim(ctx context.Context, id int) error {
	_, err := r.db.NewDelete().Model((*UserClaim)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}

// GetClaimsForUser retrieves all claims for a user
func (r *repository) GetClaimsForUser(ctx context.Context, userID string) ([]*UserClaim, error) {
	var claims []*UserClaim
	err := r.db.NewSelect().Model(&claims).Where("sec_user_id = ?", userID).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return claims, nil
}

// AddRoleClaim adds a claim for a role
func (r *repository) AddRoleClaim(ctx context.Context, claim *RoleClaim) error {
	_, err := r.db.NewInsert().Model(claim).Exec(ctx)
	return err
}

// DeleteRoleClaim deletes a role claim by ID
func (r *repository) DeleteRoleClaim(ctx context.Context, id int) error {
	_, err := r.db.NewDelete().Model((*RoleClaim)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}

// GetClaimsForRole retrieves all claims for a role
func (r *repository) GetClaimsForRole(ctx context.Context, roleID string) ([]*RoleClaim, error) {
	var claims []*RoleClaim
	err := r.db.NewSelect().Model(&claims).Where("sec_role_id = ?", roleID).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return claims, nil
}

// AddUserLogin adds an external login for a user
func (r *repository) AddUserLogin(ctx context.Context, login *UserLogin) error {
	_, err := r.db.NewInsert().Model(login).Exec(ctx)
	return err
}

// GetUserByLogin retrieves a user by external login provider and key
func (r *repository) GetUserByLogin(ctx context.Context, loginProvider, providerKey string) (*User, error) {
	var userLogin UserLogin
	err := r.db.NewSelect().Model(&userLogin).
		Where("login_provider = ? AND provider_key = ?", loginProvider, providerKey).
		Scan(ctx)
	if err != nil {
		return nil, err
	}

	var user User
	err = r.db.NewSelect().Model(&user).Where("id = ?", userLogin.UserID).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// RemoveUserLogin removes an external login
func (r *repository) RemoveUserLogin(ctx context.Context, loginProvider, providerKey string) error {
	_, err := r.db.NewDelete().Model((*UserLogin)(nil)).
		Where("login_provider = ? AND provider_key = ?", loginProvider, providerKey).
		Exec(ctx)
	return err
}

// GetUserLogins retrieves all external logins for a user
func (r *repository) GetUserLogins(ctx context.Context, userID string) ([]*UserLogin, error) {
	var logins []*UserLogin
	err := r.db.NewSelect().Model(&logins).Where("sec_user_id = ?", userID).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return logins, nil
}

// SetUserToken sets or updates a user token
func (r *repository) SetUserToken(ctx context.Context, token *UserToken) error {
	// Try to update first, if no rows affected then insert
	res, err := r.db.NewUpdate().Model(token).WherePK().Exec(ctx)
	if err != nil {
		return err
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		_, err = r.db.NewInsert().Model(token).Exec(ctx)
		return err
	}
	return nil
}

// GetUserToken retrieves a user token
func (r *repository) GetUserToken(ctx context.Context, userID, loginProvider, name string) (*UserToken, error) {
	var token UserToken
	err := r.db.NewSelect().Model(&token).
		Where("sec_user_id = ? AND login_provider = ? AND name = ?", userID, loginProvider, name).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return &token, nil
}

// RemoveUserToken removes a user token
func (r *repository) RemoveUserToken(ctx context.Context, userID, loginProvider, name string) error {
	_, err := r.db.NewDelete().Model((*UserToken)(nil)).
		Where("sec_user_id = ? AND login_provider = ? AND name = ?", userID, loginProvider, name).
		Exec(ctx)
	return err
}

// AddPasskey adds a new passkey for a user
func (r *repository) AddPasskey(ctx context.Context, passkey *UserPasskey) error {
	_, err := r.db.NewInsert().Model(passkey).Exec(ctx)
	return err
}

// GetPasskeysForUser retrieves all passkeys for a user
func (r *repository) GetPasskeysForUser(ctx context.Context, userID string) ([]*UserPasskey, error) {
	var passkeys []*UserPasskey
	err := r.db.NewSelect().Model(&passkeys).Where("sec_user_id = ?", userID).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return passkeys, nil
}

// GetPasskeyByCredentialID retrieves a passkey by credential ID
func (r *repository) GetPasskeyByCredentialID(ctx context.Context, credentialID []byte) (*UserPasskey, error) {
	var passkey UserPasskey
	err := r.db.NewSelect().Model(&passkey).Where("credential_id = ?", credentialID).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return &passkey, nil
}

// UpdatePasskeyCounter updates the counter for a passkey
func (r *repository) UpdatePasskeyCounter(ctx context.Context, id string, counter int) error {
	passkey := &UserPasskey{ID: id, Counter: counter}
	_, err := r.db.NewUpdate().Model(passkey).Column("counter").WherePK().Exec(ctx)
	return err
}

// UpdatePasskeyLastUsed updates the last used timestamp for a passkey
func (r *repository) UpdatePasskeyLastUsed(ctx context.Context, id string) error {
	_, err := r.db.NewUpdate().Model((*UserPasskey)(nil)).
		Set("last_used_at = NOW()").
		Where("id = ?", id).
		Exec(ctx)
	return err
}

// DeletePasskey removes a passkey
func (r *repository) DeletePasskey(ctx context.Context, id string) error {
	_, err := r.db.NewDelete().Model((*UserPasskey)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}
