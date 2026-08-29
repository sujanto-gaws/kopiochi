// Package models holds the bun row types for the user module.
package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// UserDBModel is the users row: a profile keyed by the identity it belongs to.
//
// ID is auth_users.id, not a key of this table's own (E16-ARCH). That is the
// whole point of the reshape: before it there was no value a handler could
// compare a caller against, so the IDOR was a missing column rather than a
// missing check. `autoincrement` is absent because it is meaningless for a
// uuid supplied by the caller's identity.
//
// There is no Name and no Email (E20). Every column this table used to carry
// already existed on auth_users, and the profile's copies had exactly one
// consumer — the CRUD JSON echoing back what had just been posted.
type UserDBModel struct {
	bun.BaseModel `bun:"table:users,alias:u"`

	ID        uuid.UUID `bun:"id,pk,type:uuid"`
	CreatedAt time.Time `bun:"created_at,notnull,default:now()"`
	UpdatedAt time.Time `bun:"updated_at,notnull,default:now()"`
}
