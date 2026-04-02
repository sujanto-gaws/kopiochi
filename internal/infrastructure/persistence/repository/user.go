package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/uptrace/bun"

	"github.com/sujanto-gaws/kopiochi/internal/domain/user"
)

// userRepository implements the user.Repository interface
type userRepository struct {
	db bun.IDB
}

// NewUserRepository creates a new user repository
func NewUserRepository(db bun.IDB) user.Repository {
	return &userRepository{db: db}
}

// Create persists a new user
func (r *userRepository) Create(ctx context.Context, u *user.User) error {
	_, err := r.db.NewInsert().Model(u).Exec(ctx)
	return err
}

// GetByID retrieves a user by ID
func (r *userRepository) GetByID(ctx context.Context, id int64) (*user.User, error) {
	var u user.User
	err := r.db.NewSelect().
		Model(&u).
		Where("id = ?", id).
		Scan(ctx)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

// GetByEmail retrieves a user by email
func (r *userRepository) GetByEmail(ctx context.Context, email string) (*user.User, error) {
	var u user.User
	err := r.db.NewSelect().
		Model(&u).
		Where("email = ?", email).
		Scan(ctx)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

// Update updates an existing user
func (r *userRepository) Update(ctx context.Context, u *user.User) error {
	_, err := r.db.NewUpdate().Model(u).WherePK().Exec(ctx)
	return err
}

// Delete removes a user by ID
func (r *userRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.NewDelete().Model((*user.User)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}
