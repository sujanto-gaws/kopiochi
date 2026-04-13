package ofbizuser

import (
	"context"
)

// Repository defines the domain interface for UserLogin persistence
type Repository interface {
	Create(ctx context.Context, userLogin *UserLogin) error
	GetByID(ctx context.Context, userLoginID string) (*UserLogin, error)
	GetByPartyID(ctx context.Context, partyID string) ([]*UserLogin, error)
	Update(ctx context.Context, userLogin *UserLogin) error
	Delete(ctx context.Context, userLoginID string) error
	UpdatePassword(ctx context.Context, userLoginID string, hashedPassword string) error
}
