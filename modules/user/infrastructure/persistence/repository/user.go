// Package repository implements the user domain's persistence port over
// bun.
package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/uptrace/bun"

	domain "github.com/sujanto-gaws/kopiochi/modules/user/domain"
	"github.com/sujanto-gaws/kopiochi/modules/user/infrastructure/persistence/models"
)

// userRepository implements the domain.Repository interface
type userRepository struct {
	db bun.IDB
}

// NewUserRepository creates a new user repository
func NewUserRepository(db bun.IDB) domain.Repository {
	return &userRepository{db: db}
}

// Create persists a new user
func (r *userRepository) Create(ctx context.Context, u *domain.User) error {
	dbModel := toDBModel(u)
	_, err := r.db.NewInsert().Model(dbModel).Exec(ctx)
	if err == nil {
		// Update domain entity with generated ID and timestamps
		u.ID = dbModel.ID
		u.CreatedAt = dbModel.CreatedAt
		u.UpdatedAt = dbModel.UpdatedAt
	}
	return err
}

// GetByID retrieves a user by ID
func (r *userRepository) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	var dbModel models.UserDBModel
	err := r.db.NewSelect().
		Model(&dbModel).
		Where("id = ?", id).
		Scan(ctx)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return toDomainEntity(&dbModel), nil
}

// GetByEmail retrieves a user by email, ignoring case: an address is the same
// address whatever case it is typed in. The predicate matches the
// idx_users_email_lower expression index (migration 00007) exactly, so it
// stays an index scan.
func (r *userRepository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	var dbModel models.UserDBModel
	err := r.db.NewSelect().
		Model(&dbModel).
		Where("lower(email) = lower(?)", email).
		Scan(ctx)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return toDomainEntity(&dbModel), nil
}

// Update updates an existing user
func (r *userRepository) Update(ctx context.Context, u *domain.User) error {
	dbModel := toDBModel(u)
	_, err := r.db.NewUpdate().Model(dbModel).WherePK().Exec(ctx)
	if err == nil {
		u.UpdatedAt = dbModel.UpdatedAt
	}
	return err
}

// Delete removes a user by ID
func (r *userRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.NewDelete().Model((*models.UserDBModel)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}

// toDomainEntity converts database model to domain entity
func toDomainEntity(dbModel *models.UserDBModel) *domain.User {
	if dbModel == nil {
		return nil
	}
	return &domain.User{
		ID:        dbModel.ID,
		Name:      dbModel.Name,
		Email:     dbModel.Email,
		CreatedAt: dbModel.CreatedAt,
		UpdatedAt: dbModel.UpdatedAt,
	}
}

// toDBModel converts domain entity to database model
func toDBModel(u *domain.User) *models.UserDBModel {
	if u == nil {
		return nil
	}
	return &models.UserDBModel{
		ID:        u.ID,
		Name:      u.Name,
		Email:     u.Email,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
}
