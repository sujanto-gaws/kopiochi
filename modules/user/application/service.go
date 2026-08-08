// Package application holds the user module's application layer: the use cases
// that orchestrate user CRUD over the domain's repository interfaces.
package application

import (
	"context"

	domain "github.com/sujanto-gaws/kopiochi/modules/user/domain"
)

// Service implements the user application service
type Service struct {
	repo domain.Repository
}

// NewService creates a new user service
func NewService(repo domain.Repository) *Service {
	return &Service{repo: repo}
}

// CreateUser creates a new user with the given name and email
func (s *Service) CreateUser(ctx context.Context, req *domain.CreateUserRequest) (*domain.UserResponse, error) {
	u := req.ToDomain()

	if err := u.Validate(); err != nil {
		return nil, err
	}

	if err := s.repo.Create(ctx, u); err != nil {
		return nil, err
	}

	return domain.ToUserResponse(u), nil
}

// GetUserByID retrieves a user by their ID
func (s *Service) GetUserByID(ctx context.Context, id int64) (*domain.UserResponse, error) {
	if id <= 0 {
		return nil, domain.ErrUserNotFound
	}

	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, domain.ErrUserNotFound
	}

	return domain.ToUserResponse(u), nil
}

// GetUserByEmail retrieves a user by their email
func (s *Service) GetUserByEmail(ctx context.Context, email string) (*domain.UserResponse, error) {
	u, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, domain.ErrUserNotFound
	}

	return domain.ToUserResponse(u), nil
}

// UpdateUser updates an existing user
func (s *Service) UpdateUser(ctx context.Context, id int64, req *domain.UpdateUserRequest) (*domain.UserResponse, error) {
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, domain.ErrUserNotFound
	}

	u.Name = req.Name
	u.Email = req.Email

	if err := u.Validate(); err != nil {
		return nil, err
	}

	if err := s.repo.Update(ctx, u); err != nil {
		return nil, err
	}

	return domain.ToUserResponse(u), nil
}

// DeleteUser deletes a user by their ID
func (s *Service) DeleteUser(ctx context.Context, id int64) error {
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if u == nil {
		return domain.ErrUserNotFound
	}

	return s.repo.Delete(ctx, id)
}
