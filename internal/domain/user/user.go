package user

import (
	"errors"
	"time"

	"github.com/uptrace/bun"
)

var (
	ErrInvalidName  = errors.New("invalid user name")
	ErrInvalidEmail = errors.New("invalid email")
)

// User is the domain entity representing a user in the system
type User struct {
	bun.BaseModel `bun:"table:users,alias:u"`
	ID            int64     `bun:"id,pk,autoincrement"`
	Name          string    `bun:"name,notnull"`
	Email         string    `bun:"email,notnull,unique"`
	CreatedAt     time.Time `bun:"created_at,notnull,default:now()"`
	UpdatedAt     time.Time `bun:"updated_at,notnull,default:now()"`
}

// Validate validates the user entity business rules
func (u *User) Validate() error {
	if u.Name == "" {
		return ErrInvalidName
	}
	if u.Email == "" {
		return ErrInvalidEmail
	}
	return nil
}
