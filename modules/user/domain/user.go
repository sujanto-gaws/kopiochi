// Package domain defines the profile entity: the profile OF an authenticated
// identity (table: users, PK: auth_users.id).
//
// It is still distinct from the identity itself (modules/identity, table:
// auth_users), but it is no longer independent of it. Before E16 the two were
// unrelated — a BIGSERIAL profile with its own name and email, and no value
// tying it to whoever was logged in — which is why any valid token could read,
// overwrite or delete any other user's row: there was nothing to compare.
// The profile is now keyed by the identity's uuid, so ownership is expressible.
//
// It holds no name and no email (E20). Those live on the identity, which is
// their single source of truth; a second copy is a staleness hazard, and the
// one E15 refused for the same reason — a stale address means "your password
// was changed" is mailed to the address the attacker just set.
package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrUserNotFound is returned when no profile exists for an id.
//
// ErrInvalidName and ErrInvalidEmail are gone with the columns they guarded.
// A profile has no field a caller supplies, so there is nothing left to
// validate — which is also why the write API shrank to a single idempotent
// create (E24).
var ErrUserNotFound = errors.New("user not found")

// User is the profile of one identity.
//
// ID is the identity's uuid — auth_users.id — and not a key of this entity's
// own. A profile without an identity is not a thing this module can represent.
type User struct {
	ID        uuid.UUID
	CreatedAt time.Time
	UpdatedAt time.Time
}
