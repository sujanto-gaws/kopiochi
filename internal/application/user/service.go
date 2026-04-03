package user

import (
	"context"

	"github.com/sujanto-gaws/kopiochi/internal/domain/user"
)

// Service implements the user application service
type Service struct {
	repo user.Repository
}

// NewService creates a new user service
func NewService(repo user.Repository) *Service {
	return &Service{repo: repo}
}

// CreateUser creates a new user with the given name and email
func (s *Service) CreateUser(ctx context.Context, req *user.CreateUserRequest) (*user.UserResponse, error) {
	u := req.ToDomain()

	if err := u.Validate(); err != nil {
		return nil, err
	}

	if err := s.repo.Create(ctx, u); err != nil {
		return nil, err
	}

	return user.ToUserResponse(u), nil
}

// GetUserByID retrieves a user by their ID
func (s *Service) GetUserByID(ctx context.Context, id int64) (*user.UserResponse, error) {
	if id <= 0 {
		return nil, user.ErrUserNotFound
	}

	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, user.ErrUserNotFound
	}

	return user.ToUserResponse(u), nil
}

// GetUserByEmail retrieves a user by their email
func (s *Service) GetUserByEmail(ctx context.Context, email string) (*user.UserResponse, error) {
	u, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, user.ErrUserNotFound
	}

	return user.ToUserResponse(u), nil
}

// UpdateUser updates an existing user
func (s *Service) UpdateUser(ctx context.Context, id int64, req *user.UpdateUserRequest) (*user.UserResponse, error) {
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, user.ErrUserNotFound
	}

	u.Name = req.Name
	u.Email = req.Email

	if err := u.Validate(); err != nil {
		return nil, err
	}

	if err := s.repo.Update(ctx, u); err != nil {
		return nil, err
	}

	return user.ToUserResponse(u), nil
}

// DeleteUser deletes a user by their ID
func (s *Service) DeleteUser(ctx context.Context, id int64) error {
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if u == nil {
		return user.ErrUserNotFound
	}

	return s.repo.Delete(ctx, id)
}
