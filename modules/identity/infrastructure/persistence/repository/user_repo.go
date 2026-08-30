package repository

import (
	"context"
	"database/sql"
	"errors"

	"fmt"

	"github.com/google/uuid"

	"github.com/sujanto-gaws/kopiochi/internal/db"
	domain "github.com/sujanto-gaws/kopiochi/modules/identity/domain"
	"github.com/sujanto-gaws/kopiochi/modules/identity/infrastructure/persistence/models"
	"github.com/uptrace/bun"
)

// UserRepo is the bun-backed store for auth_users.
//
// Errors leave it as an internal/db sentinel — db.ErrNotFound,
// db.ErrConflict — via db.Translate, never as a bare errors.New or a raw
// *pgconn.PgError. That is what makes them classifiable with errors.Is by a
// caller that has to decide something on them.
//
// The lookups used to return errors.New("not found"): a distinct value on every
// call, matchable only by comparing text. Nothing in this module noticed,
// because every caller collapses any error from a lookup into the same
// response. E15 is what needed the distinction — a notification sender resolving
// a recipient's address must tell "this user is gone", which is permanent and
// must dead-letter the row, from "the database is unreachable", which is
// transient and must be retried. Guessing wrong either retries a deleted user
// until its budget is spent, or destroys a security mail on a network blip.
//
// The sibling refresh_token_store.go was already written this way; this file was
// the outlier.
type UserRepo struct {
	db bun.IDB
}

func NewUserRepo(db bun.IDB) *UserRepo {
	return &UserRepo{db: db}
}

// FindByEmail matches on lower(email), not the raw column: an address is the
// same address whatever case it is typed in. The predicate is written to match
// the idx_auth_users_email_lower expression index (migration 00007) exactly,
// so this stays an index scan rather than degrading to a sequential one.
func (r *UserRepo) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	row := new(models.BunUser)
	err := r.db.NewSelect().
		Model(row).
		Where("lower(email) = lower(?)", email).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, db.ErrNotFound
		}
		return nil, db.Translate(err)
	}
	return toDomainUser(row), nil
}

func (r *UserRepo) FindByID(ctx context.Context, id string) (*domain.User, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("parse user id: %w", err)
	}
	row := new(models.BunUser)
	if err := r.db.NewSelect().Model(row).Where("id = ?", uid).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, db.ErrNotFound
		}
		return nil, db.Translate(err)
	}
	return toDomainUser(row), nil
}

// FindByUsername is the lookup Login runs, so its case-sensitivity decided
// whether a correct password was accepted. It matches on lower(username)
// against idx_auth_users_username_lower (migration 00007), which is also what
// makes "at most one row" true here — before that index the column carried no
// uniqueness at all.
func (r *UserRepo) FindByUsername(ctx context.Context, username string) (*domain.User, error) {
	row := new(models.BunUser)
	err := r.db.NewSelect().
		Model(row).
		Where("lower(username) = lower(?)", username).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, db.ErrNotFound
		}
		return nil, db.Translate(err)
	}
	return toDomainUser(row), nil
}

func (r *UserRepo) Save(ctx context.Context, user *domain.User) error {
	bunUser := fromDomainUser(user)
	// If ID is zero, bun will generate it (but we set it in fromDomainUser if nil)
	// Use upsert to handle both insert and update safely
	_, err := r.db.NewInsert().
		Model(bunUser).
		On("CONFLICT (id) DO UPDATE").
		Set("username = EXCLUDED.username").
		Set("email = EXCLUDED.email").
		Set("name = EXCLUDED.name").
		Set("roles = EXCLUDED.roles").
		Set("permissions = EXCLUDED.permissions").
		Set("password_hash = EXCLUDED.password_hash").
		Set("mfa_enabled = EXCLUDED.mfa_enabled").
		Set("mfa_secret = EXCLUDED.mfa_secret").
		Set("failed_login_attempts = EXCLUDED.failed_login_attempts").
		Set("locked_until = EXCLUDED.locked_until").
		Set("updated_at = now()").
		Exec(ctx)
	return db.Translate(err)
}

// Map domain User to BunUser
func fromDomainUser(u *domain.User) *models.BunUser {
	return &models.BunUser{
		ID:          u.ID,
		Username:    u.Username,
		Email:       u.Email,
		Name:        u.Name,
		Roles:       u.Roles,
		Permissions: u.Permissions,
		// "" becomes NULL on the way out, the inverse of toDomainUser's collapse,
		// so the round trip is stable: a domain user with no MFA secret stores
		// NULL rather than an empty string, and the column keeps meaning "never
		// set" instead of accumulating rows that say "set to nothing". Those are
		// the rows E10 turned into a public second factor.
		PasswordHash:        nilIfEmpty(u.PasswordHash),
		MFAEnabled:          u.MFAEnabled,
		MFASecret:           nilIfEmpty(u.MFASecret),
		FailedLoginAttempts: u.FailedLoginAttempts,
		LockedUntil:         u.LockedUntil,
	}
}

// deref returns the pointed-to string, or "" when the column was NULL.
func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// nilIfEmpty returns nil for "", so an unset value stores as NULL.
func nilIfEmpty(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func toDomainUser(row *models.BunUser) *domain.User {
	return &domain.User{
		ID:          row.ID,
		Username:    row.Username,
		Email:       row.Email,
		Name:        row.Name,
		Roles:       row.Roles,
		Permissions: row.Permissions,
		// NULL becomes "" here, deliberately and in one place.
		//
		// The domain keeps plain strings because the empty string is safe for
		// both, and safe by construction rather than by luck:
		// bcrypt.CompareHashAndPassword rejects an empty hash, and since E10
		// TOTPService.ValidateCode rejects an empty secret. The pointer exists on
		// the row so the collapse is a decision made here, visible in review,
		// instead of something bun's scanner did on the way past.
		//
		// If either guard is ever removed, this line is what makes it
		// exploitable — which is the point of putting it here rather than
		// leaving the model lying about the column.
		PasswordHash:        deref(row.PasswordHash),
		MFAEnabled:          row.MFAEnabled,
		MFASecret:           deref(row.MFASecret),
		FailedLoginAttempts: row.FailedLoginAttempts,
		LockedUntil:         row.LockedUntil,
	}
}
