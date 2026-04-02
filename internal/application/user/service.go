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
func (s *Service) CreateUser(ctx context.Context, name, email string) (*user.User, error) {
	u := &user.User{
		Name:  name,
		Email: email,
	}

	if err := u.Validate(); err != nil {
		return nil, err
	}

	if err := s.repo.Create(ctx, u); err != nil {
		return nil, err
	}

	return u, nil
}

// GetUserByID retrieves a user by their ID
func (s *Service) GetUserByID(ctx context.Context, id int64) (*user.User, error) {
	if id <= 0 {
		return nil, user.ErrInvalidName
	}

	return s.repo.GetByID(ctx, id)
}

// GetUserByEmail retrieves a user by their email
func (s *Service) GetUserByEmail(ctx context.Context, email string) (*user.User, error) {
	return s.repo.GetByEmail(ctx, email)
}

// UpdateUser updates an existing user
func (s *Service) UpdateUser(ctx context.Context, id int64, name, email string) (*user.User, error) {
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	u.Name = name
	u.Email = email

	if err := u.Validate(); err != nil {
		return nil, err
	}

	if err := s.repo.Update(ctx, u); err != nil {
		return nil, err
	}

	return u, nil
}

// DeleteUser deletes a user by their ID
func (s *Service) DeleteUser(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}
