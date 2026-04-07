package models

import (
	"time"

	"github.com/uptrace/bun"
)

// UserDBModel is the database model for the users table
// This is an infrastructure concern - maps directly to DB schema
type UserDBModel struct {
	bun.BaseModel `bun:"table:users,alias:u"`
	ID            int64     `bun:"id,pk,autoincrement"`
	Name          string    `bun:"name,notnull"`
	Email         string    `bun:"email,notnull,unique"`
	CreatedAt     time.Time `bun:"created_at,notnull,default:now()"`
	UpdatedAt     time.Time `bun:"updated_at,notnull,default:now()"`
}
