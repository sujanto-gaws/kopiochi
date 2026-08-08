package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	domain "github.com/sujanto-gaws/kopiochi/modules/identity/domain"
	"time"
)

func (s *Service) Login(ctx context.Context, req LoginRequest) (*TokenResponse, error) {
	user, err := s.userRepo.FindByUsername(ctx, req.Username)
	if err != nil {
		// The attempted username, not a user id: nobody authenticated, and a
		// reader must not mistake an attempt for a confirmed identity.
		s.audit.LoginFailed(ctx, req.Username, ReasonUnknownUser)
		return nil, ErrInvalidCredentials
	}
	if user.IsLocked() {
		s.audit.LoginFailed(ctx, req.Username, ReasonAccountLocked)
		return nil, ErrAccountLocked
	}
	if !s.passwordHasher.Verify(req.Password, user.PasswordHash) {
		wasLocked := user.IsLocked()
		user.RecordFailedLogin(s.cfg.MaxFailedAttempts, s.cfg.LockDuration)
		_ = s.userRepo.Save(ctx, user) // best effort

		s.audit.LoginFailed(ctx, req.Username, ReasonBadPassword)
		// Emitted only on the transition, so the event means "this account
		// just locked" rather than repeating once per subsequent attempt —
		// which is what makes it alertable.
		if !wasLocked && user.IsLocked() {
			s.audit.AccountLocked(ctx, user.ID.String())
		}
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
		// Deliberately not a login success: the password was right but the
		// second factor is outstanding, and recording success here would make
		// the audit trail claim an authentication that has not happened.
		return nil, &MFAError{
			Token: mfaToken,
			User:  toUserDTO(*user),
		}
	}

	resp, err := s.issueFullTokens(ctx, *user)
	if err != nil {
		return nil, err
	}
	s.audit.LoginSucceeded(ctx, user.ID.String())
	return resp, nil
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
