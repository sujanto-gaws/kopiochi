package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
	domain "github.com/sujanto-gaws/kopiochi/internal/domain/auth"
)

func (s *Service) Login(ctx context.Context, req LoginRequest) (*TokenResponse, error) {
	user, err := s.userRepo.FindByUsername(ctx, req.Username)
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	if user.IsLocked() {
		return nil, ErrAccountLocked
	}
	if !s.passwordHasher.Verify(req.Password, user.PasswordHash) {
		user.RecordFailedLogin(s.cfg.MaxFailedAttempts, s.cfg.LockDuration)
		_ = s.userRepo.Save(ctx, user) // best effort
		return nil, ErrInvalidCredentials
	}
	user.ResetFailedLogins()
	if err := s.userRepo.Save(ctx, user); err != nil {
		return nil, fmt.Errorf("user save: %w", err)
	}

	if user.MFAEnabled {
		mfaToken, err := s.tokenIssuer.IssueMFAToken(*user)
		if err != nil {
			return nil, fmt.Errorf("mfa token: %w", err)
		}
		return nil, &MFAError{
			Token: mfaToken,
			User:  toUserDTO(*user),
		}
	}
	return s.issueFullTokens(ctx, *user)
}

func (s *Service) issueFullTokens(ctx context.Context, user domain.User) (*TokenResponse, error) {
	access, err := s.tokenIssuer.IssueAccessToken(user, s.cfg.AccessTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("access token: %w", err)
	}
	idToken, _ := s.tokenIssuer.IssueIDToken(user, s.cfg.ClientID)
	refreshPlain, err := generateRandomToken()
	if err != nil {
		return nil, fmt.Errorf("random: %w", err)
	}
	hash := domain.HashToken(refreshPlain)
	entity := domain.RefreshToken{
		UserID:    user.ID.String(),
		TokenHash: hash,
		ExpiresAt: time.Now().Add(s.cfg.RefreshTokenTTL),
	}
	if err := s.tokenStore.Store(ctx, entity); err != nil {
		return nil, fmt.Errorf("store refresh: %w", err)
	}
	return &TokenResponse{
		AccessToken:  access,
		TokenType:    "Bearer",
		ExpiresIn:    int(s.cfg.AccessTokenTTL.Seconds()),
		RefreshToken: refreshPlain,
		IDToken:      idToken,
	}, nil
}

func toUserDTO(u domain.User) UserDTO {
	return UserDTO{
		ID:          u.ID.String(),
		Email:       u.Email,
		Name:        u.Name,
		Roles:       u.Roles,
		Permissions: u.Permissions,
	}
}

func generateRandomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}