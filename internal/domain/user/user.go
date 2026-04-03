package user

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidName  = errors.New("invalid user name")
	ErrInvalidEmail = errors.New("invalid email")
	ErrUserNotFound = errors.New("user not found")
)

// User is the domain entity representing a user in the system
// This is a pure domain entity without infrastructure concerns
type User struct {
	ID        int64
	Name      string
	Email     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Validate validates the user entity against business rules
func (u *User) Validate() error {
	if strings.TrimSpace(u.Name) == "" {
		return ErrInvalidName
	}
	if strings.TrimSpace(u.Email) == "" {
		return ErrInvalidEmail
	}
	if !isValidEmail(u.Email) {
		return ErrInvalidEmail
	}
	return nil
}

// isValidEmail performs basic email format validation
func isValidEmail(email string) bool {
	if len(email) < 5 {
		return false
	}
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false
	}
	return parts[0] != "" && strings.Contains(parts[1], ".") && parts[1] != ""
}
