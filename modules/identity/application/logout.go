package auth

import (
	"context"
)

func (s *Service) Logout(ctx context.Context, userID string) error {
	if err := s.tokenStore.RevokeAllForUser(ctx, userID); err != nil {
		return err
	}
	s.audit.LogoutSucceeded(ctx, userID)
	return nil
}
