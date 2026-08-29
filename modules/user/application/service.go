// Package application holds the user module's use cases: bringing the caller's
// own profile into existence, and reading it.
//
// Every use case takes the caller's identity as its first argument and none
// takes anybody else's. That is deliberate and is the layer at which E16 is
// closed: a handler has only the Principal it was given to pass, so a
// cross-user request is not something this API can express.
package application

import (
	"context"

	"github.com/google/uuid"

	domain "github.com/sujanto-gaws/kopiochi/modules/user/domain"
)

// Service implements the user application service.
type Service struct {
	repo domain.Repository
}

// NewService creates a new user service.
func NewService(repo domain.Repository) *Service {
	return &Service{repo: repo}
}

// EnsureOwnProfile creates the caller's profile if it does not exist and
// returns it either way.
//
// Idempotent on purpose. A caller asking twice has not made a mistake, and the
// second request is indistinguishable from a retry of the first — so this
// answers both the same way rather than turning a lost response into a 409 the
// client cannot act on.
//
// It cannot create anybody else's: caller is the authenticated subject, and
// there is no other id in scope.
func (s *Service) EnsureOwnProfile(ctx context.Context, caller uuid.UUID) (*domain.UserResponse, error) {
	if err := s.repo.EnsureExists(ctx, caller); err != nil {
		return nil, err
	}
	return s.GetOwnProfile(ctx, caller)
}

// GetOwnProfile returns the caller's profile.
//
// The `id <= 0` guard the old GetUserByID carried is gone with the int64 it
// guarded (E16-2). A uuid has no invalid-but-parseable range: either it parsed
// at the edge or the request never reached here.
func (s *Service) GetOwnProfile(ctx context.Context, caller uuid.UUID) (*domain.UserResponse, error) {
	u, err := s.repo.GetByID(ctx, caller)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, domain.ErrUserNotFound
	}
	return domain.ToUserResponse(u), nil
}
