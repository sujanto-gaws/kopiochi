package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	domain "github.com/sujanto-gaws/kopiochi/internal/domain/auth"
	"github.com/sujanto-gaws/kopiochi/internal/infrastructure/persistence/auth/models"
	"github.com/uptrace/bun"
)

type UserRepo struct {
	db bun.IDB
}

func NewUserRepo(db bun.IDB) *UserRepo {
	return &UserRepo{db: db}
}

func (r *UserRepo) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	row := new(models.BunUser)
	err := r.db.NewSelect().
		Model(row).
		Where("email = ?", email).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("not found")
		}
		return nil, err
	}
	return toDomainUser(row), nil
}

func (r *UserRepo) FindByID(ctx context.Context, id string) (*domain.User, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, errors.New("invalid id")
	}
	row := new(models.BunUser)
	if err := r.db.NewSelect().Model(row).Where("id = ?", uid).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("not found")
		}
		return nil, err
	}
	return toDomainUser(row), nil
}

func (r *UserRepo) FindByUsername(ctx context.Context, username string) (*domain.User, error) {
	row := new(models.BunUser)
	err := r.db.NewSelect().
		Model(row).
		Where("email = ?", username). // BunUser has no username column; email is the login identifier
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("not found")
		}
		return nil, err
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
	return err
}

// Map domain User to BunUser
func fromDomainUser(u *domain.User) *models.BunUser {
	return &models.BunUser{
		ID:                  u.ID,
		Email:               u.Email,
		Name:                u.Name,
		Roles:               u.Roles,
		Permissions:         u.Permissions,
		PasswordHash:        u.PasswordHash,
		MFAEnabled:          u.MFAEnabled,
		MFASecret:           u.MFASecret,
		FailedLoginAttempts: u.FailedLoginAttempts,
		LockedUntil:         u.LockedUntil,
	}
}

func toDomainUser(row *models.BunUser) *domain.User {
	return &domain.User{
		ID:                  row.ID,
		Email:               row.Email,
		Name:                row.Name,
		Roles:               row.Roles,
		Permissions:         row.Permissions,
		PasswordHash:        row.PasswordHash,
		MFAEnabled:          row.MFAEnabled,
		MFASecret:           row.MFASecret,
		FailedLoginAttempts: row.FailedLoginAttempts,
		LockedUntil:         row.LockedUntil,
	}
}
