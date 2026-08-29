package domain

import (
	"time"

	"github.com/google/uuid"
)

// CreateUserRequest and UpdateUserRequest are deleted, not emptied (E24).
//
// Both were {Name, Email}, and both fields are columns E20 removed. An empty
// request struct would have been a parameter that cannot carry anything,
// documenting a body no caller should send. The routes that took them are gone
// with them: a PUT with no writable field is a 200 that lies, and the POST that
// accepted an arbitrary body is the "unrestricted creation behind mere
// authentication" leg of E16.

// UserResponse is the profile as the API returns it.
//
// The id is the caller's own identity uuid, so this response tells a client
// nothing it did not already know about itself — which is the point. It carries
// no name and no email; those belong to the identity.
type UserResponse struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ToUserResponse converts a profile to its API representation.
func ToUserResponse(u *User) *UserResponse {
	if u == nil {
		return nil
	}
	return &UserResponse{
		ID:        u.ID,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
}
