package domain

import (
	"context"

	"github.com/google/uuid"
)

// Repository is the profile store.
//
// It has exactly two operations because the profile has exactly two: a caller
// may bring their own into existence, and may read it. There is no Update — a
// profile has no field to change (E20) — no Delete, because a profile cannot
// outlive the identity it belongs to and account deletion is identity's, and no
// GetByEmail, because this module no longer knows what an email is.
type Repository interface {
	// EnsureExists creates id's profile if it does not already exist, and is a
	// no-op if it does.
	//
	// Idempotent by contract rather than by luck: creating your own profile
	// twice is not an error, and two concurrent first requests from the same
	// caller must not turn one of them into a 500.
	EnsureExists(ctx context.Context, id uuid.UUID) error

	// GetByID returns id's profile, or ErrUserNotFound.
	GetByID(ctx context.Context, id uuid.UUID) (*User, error)
}
