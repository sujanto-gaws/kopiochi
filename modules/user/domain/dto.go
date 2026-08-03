package user

import (
	"time"
)

// CreateUserRequest represents the request body for creating a user
type CreateUserRequest struct {
	Name  string `json:"name" validate:"required"`
	Email string `json:"email" validate:"required,email"`
}

// UpdateUserRequest represents the request body for updating a user
type UpdateUserRequest struct {
	Name  string `json:"name" validate:"required"`
	Email string `json:"email" validate:"required,email"`
}

// UserResponse represents the response body for user operations
type UserResponse struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ToUserResponse converts a domain User entity to UserResponse DTO
func ToUserResponse(u *User) *UserResponse {
	if u == nil {
		return nil
	}
	return &UserResponse{
		ID:        u.ID,
		Name:      u.Name,
		Email:     u.Email,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
}

// ToUserResponses converts a slice of domain User entities to UserResponse DTOs
func ToUserResponses(users []*User) []*UserResponse {
	responses := make([]*UserResponse, len(users))
	for i, u := range users {
		responses[i] = ToUserResponse(u)
	}
	return responses
}

// ToDomain converts CreateUserRequest DTO to domain User entity
func (r *CreateUserRequest) ToDomain() *User {
	return &User{
		Name:  r.Name,
		Email: r.Email,
	}
}

// ToDomain converts UpdateUserRequest DTO to domain User entity
func (r *UpdateUserRequest) ToDomain() *User {
	return &User{
		Name:  r.Name,
		Email: r.Email,
	}
}
