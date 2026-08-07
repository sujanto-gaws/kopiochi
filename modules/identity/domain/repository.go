package domain

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

	// FindAny returns a token by hash whatever its state — revoked, used or
	// expired.
	//
	// FindValid deliberately filters those out, which makes a stolen token and
	// a token that never existed indistinguishable. Reuse detection needs to
	// tell them apart: one is a nonexistent credential, the other is evidence
	// that a real token was captured.
	FindAny(ctx context.Context, tokenHash string) (*RefreshToken, error)

	// Rotate atomically marks oldHash used and stores next in the same family.
	//
	// Atomic because the two halves fail differently and both failures are
	// bad: marking the old token used without storing the new one logs the
	// user out mid-session, and storing the new one without retiring the old
	// leaves two live tokens — which is the state rotation exists to prevent
	// and which the reuse check would later report as theft.
	Rotate(ctx context.Context, oldHash string, next RefreshToken) error

	// RevokeFamily revokes every token in familyID and records that it was a
	// reuse detection rather than a logout. Returns how many rows it
	// invalidated, which is what the audit event reports.
	RevokeFamily(ctx context.Context, familyID string) (int, error)
}

type MFAStore interface {
	StoreBackupCodes(ctx context.Context, userID string, codeHashes []string) error
	FindAndUseBackupCode(ctx context.Context, userID string, plainCode string) (found bool, err error)
}
