package domain

import (
	"context"

	"github.com/google/uuid"
)

// Service is the profile use cases, stated in terms of the caller.
//
// Every method takes the caller's own id and there is no parameter naming
// somebody else's, which is what makes the E16 IDOR unexpressible here rather
// than merely unperformed: a handler has nothing to pass but the Principal it
// was given.
type Service interface {
	EnsureOwnProfile(ctx context.Context, caller uuid.UUID) (*UserResponse, error)
	GetOwnProfile(ctx context.Context, caller uuid.UUID) (*UserResponse, error)
}
