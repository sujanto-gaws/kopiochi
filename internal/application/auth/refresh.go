package auth

import (
	"context"
	"fmt"
	"time"
	domain "github.com/sujanto-gaws/kopiochi/internal/domain/auth"
)

func (s *Service) Refresh(ctx context.Context, refreshToken string) (*TokenResponse, error) {
	if refreshToken == "" {
		return nil, ErrRefreshTokenInvalid
	}
	hash := domain.HashToken(refreshToken)
	stored, err := s.tokenStore.FindValid(ctx, hash)
	if err != nil || stored.ExpiresAt.Before(time.Now()) {
		return nil, ErrRefreshTokenInvalid
	}
	// Revoke old token
	_ = s.tokenStore.RevokeAllForUser(ctx, stored.UserID)

	user, err := s.userRepo.FindByID(ctx, stored.UserID)
	if err != nil {
		return nil, ErrRefreshTokenInvalid
	}

	access, err := s.tokenIssuer.IssueAccessToken(*user, s.cfg.AccessTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("access token: %w", err)
	}
	idToken, _ := s.tokenIssuer.IssueIDToken(*user, s.cfg.ClientID)

	newRefreshPlain, err := generateRandomToken()
	if err != nil {
		return nil, fmt.Errorf("random: %w", err)
	}
	newHash := domain.HashToken(newRefreshPlain)
	newEntity := domain.RefreshToken{
		UserID:    user.ID.String(),
		TokenHash: newHash,
		ExpiresAt: time.Now().Add(s.cfg.RefreshTokenTTL),
	}
	if err := s.tokenStore.Store(ctx, newEntity); err != nil {
		return nil, fmt.Errorf("store new refresh: %w", err)
	}

	return &TokenResponse{
		AccessToken:  access,
		TokenType:    "Bearer",
		ExpiresIn:    int(s.cfg.AccessTokenTTL.Seconds()),
		RefreshToken: newRefreshPlain,
		IDToken:      idToken,
	}, nil
}