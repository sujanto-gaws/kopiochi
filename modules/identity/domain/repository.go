package auth

import "context"

type UserRepository interface {
	FindByEmail(ctx context.Context, email string) (*User, error)
	FindByID(ctx context.Context, id string) (*User, error)
	FindByUsername(ctx context.Context, username string) (*User, error)
	Save(ctx context.Context, user *User) error
}

type RefreshTokenStore interface {
	Store(ctx context.Context, token RefreshToken) error
	FindValid(ctx context.Context, tokenHash string) (*RefreshToken, error)
	RevokeAllForUser(ctx context.Context, userID string) error
}

type MFAStore interface {
	StoreBackupCodes(ctx context.Context, userID string, codeHashes []string) error
	FindAndUseBackupCode(ctx context.Context, userID string, plainCode string) (found bool, err error)
}
