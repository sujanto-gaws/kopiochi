package user

import (
	"context"
)

// Service defines the domain interface for user business logic
type Service interface {
	CreateUser(ctx context.Context, name, email string) (*User, error)
	GetUserByID(ctx context.Context, id int64) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	UpdateUser(ctx context.Context, id int64, name, email string) (*User, error)
	DeleteUser(ctx context.Context, id int64) error
}
