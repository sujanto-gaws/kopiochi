// Package repository implements the user domain's persistence port over bun.
package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/sujanto-gaws/kopiochi/internal/db"
	domain "github.com/sujanto-gaws/kopiochi/modules/user/domain"
	"github.com/sujanto-gaws/kopiochi/modules/user/infrastructure/persistence/models"
)

// userRepository implements domain.Repository.
type userRepository struct {
	db bun.IDB
}

// NewUserRepository creates a new user repository.
func NewUserRepository(db bun.IDB) domain.Repository {
	return &userRepository{db: db}
}

// EnsureExists creates id's profile, or does nothing if it already has one.
//
// ON CONFLICT DO NOTHING rather than a read-then-insert: the check and the
// insert would otherwise be two statements with a gap between them, and two
// concurrent first requests from one caller would race into a duplicate-key
// error that the second caller did nothing to deserve. One statement makes the
// idempotency the database's rather than the caller's.
//
// A foreign key to auth_users(id) means an id with no identity is refused here
// rather than producing an orphan profile. It arrives from a verified token, so
// that should be unreachable — which is exactly why it is worth a constraint
// rather than a comment.
func (r *userRepository) EnsureExists(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.NewInsert().
		Model(&models.UserDBModel{ID: id}).
		On("CONFLICT (id) DO NOTHING").
		Exec(ctx)
	return db.Translate(err)
}

// GetByID returns id's profile, or domain.ErrUserNotFound.
func (r *userRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	row := new(models.UserDBModel)
	err := r.db.NewSelect().Model(row).Where("id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrUserNotFound
		}
		return nil, db.Translate(err)
	}
	return toDomain(row), nil
}

// toDomain maps a row to the entity. There is no toDBModel any more: the only
// write this repository performs supplies the id and lets the column defaults
// fill the timestamps.
func toDomain(row *models.UserDBModel) *domain.User {
	return &domain.User{
		ID:        row.ID,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}
