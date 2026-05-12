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

type RefreshTokenStore struct {
	db bun.IDB
}

func NewRefreshTokenStore(db bun.IDB) *RefreshTokenStore {
	return &RefreshTokenStore{db: db}
}

func (s *RefreshTokenStore) Store(ctx context.Context, token domain.RefreshToken) error {
	row := &models.RefreshTokenRow{
		UserID:    uuid.MustParse(token.UserID),
		TokenHash: token.TokenHash,
		ExpiresAt: token.ExpiresAt,
	}
	_, err := s.db.NewInsert().Model(row).Exec(ctx)
	return err
}

func (s *RefreshTokenStore) FindValid(ctx context.Context, tokenHash string) (*domain.RefreshToken, error) {
	row := new(models.RefreshTokenRow)
	err := s.db.NewSelect().
		Model(row).
		Where("token_hash = ?", tokenHash).
		Where("revoked = false").
		Where("expires_at > now()").
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("invalid token")
		}
		return nil, err
	}
	return &domain.RefreshToken{
		UserID:    row.UserID.String(),
		TokenHash: row.TokenHash,
		ExpiresAt: row.ExpiresAt,
	}, nil
}

func (s *RefreshTokenStore) RevokeAllForUser(ctx context.Context, userID string) error {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return err
	}
	_, err = s.db.NewUpdate().
		Model((*models.RefreshTokenRow)(nil)).
		Set("revoked = true").
		Where("user_id = ?", uid).
		Exec(ctx)
	return err
}
