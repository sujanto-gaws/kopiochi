package auth

import (
	"context"
)

func (s *Service) Logout(ctx context.Context, userID string) error {
	return s.tokenStore.RevokeAllForUser(ctx, userID)
}