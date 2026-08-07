package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"

	"github.com/sujanto-gaws/kopiochi/internal/testsupport"
	domain "github.com/sujanto-gaws/kopiochi/modules/identity/domain"
	"github.com/sujanto-gaws/kopiochi/modules/identity/infrastructure/hasher"
)

// Integration tests for the identity module's persistence, against a real
// Postgres. They skip cleanly when none is reachable — see
// testsupport.ScratchPostgres.
//
// The refresh-token store is the highest-value target here: its FindValid
// carries three conditions (hash match, not revoked, not expired) that exist
// entirely in SQL, so no unit test can reach them. Each is the difference
// between a revoked session staying dead and coming back to life.

func newDB(t *testing.T) *bun.DB {
	t.Helper()

	db := testsupport.MigratedDB(t)
	testsupport.TruncateAll(t, db)
	return db
}

// seedUser inserts an auth user and returns it.
func seedUser(t *testing.T, db *bun.DB, username string) *domain.User {
	t.Helper()

	hash, err := (hasher.BcryptHasher{}).Hash("correct horse battery staple")
	require.NoError(t, err)

	u := &domain.User{
		ID:           uuid.New(),
		Username:     username,
		Email:        username + "@example.com",
		Name:         "Test " + username,
		Roles:        []string{"user"},
		Permissions:  []string{},
		PasswordHash: hash,
	}
	require.NoError(t, NewUserRepo(db).Save(context.Background(), u))
	return u
}

func TestUserRepo_SaveThenFind(t *testing.T) {
	db := newDB(t)
	repo := NewUserRepo(db)
	ctx := context.Background()

	u := seedUser(t, db, "alice")

	byID, err := repo.FindByID(ctx, u.ID.String())
	require.NoError(t, err)
	require.Equal(t, u.Username, byID.Username)

	byName, err := repo.FindByUsername(ctx, "alice")
	require.NoError(t, err)
	require.Equal(t, u.ID, byName.ID)

	byEmail, err := repo.FindByEmail(ctx, "alice@example.com")
	require.NoError(t, err)
	require.Equal(t, u.ID, byEmail.ID)
}

// TestUserRepo_FindIsCaseInsensitive: the identifier someone types at the
// login form is not required to match the capitalisation they registered with.
// Before migration 00007 both lookups used a plain "=", so "Alice" failed to
// find the account "alice" owns and the correct password was rejected.
func TestUserRepo_FindIsCaseInsensitive(t *testing.T) {
	db := newDB(t)
	repo := NewUserRepo(db)
	ctx := context.Background()

	u := seedUser(t, db, "alice") // email: alice@example.com

	for _, username := range []string{"alice", "Alice", "ALICE", "aLiCe"} {
		got, err := repo.FindByUsername(ctx, username)
		require.NoError(t, err, "FindByUsername(%q) did not find the account", username)
		require.Equal(t, u.ID, got.ID)
	}

	for _, email := range []string{"alice@example.com", "Alice@Example.com", "ALICE@EXAMPLE.COM"} {
		got, err := repo.FindByEmail(ctx, email)
		require.NoError(t, err, "FindByEmail(%q) did not find the account", email)
		require.Equal(t, u.ID, got.ID)
	}
}

// TestUserRepo_PreservesStoredCasing: matching case-insensitively must not
// mean storing case-insensitively. What the user typed is what comes back —
// the normalisation lives in the index and the predicate, not in the data.
func TestUserRepo_PreservesStoredCasing(t *testing.T) {
	db := newDB(t)
	repo := NewUserRepo(db)
	ctx := context.Background()

	u := &domain.User{
		ID:          uuid.New(),
		Username:    "McDonald",
		Email:       "McDonald@Example.com",
		Roles:       []string{"user"},
		Permissions: []string{},
	}
	require.NoError(t, repo.Save(ctx, u))

	got, err := repo.FindByUsername(ctx, "mcdonald")
	require.NoError(t, err)
	require.Equal(t, "McDonald", got.Username, "the stored username was lowercased")
	require.Equal(t, "McDonald@Example.com", got.Email, "the stored email was lowercased")
}

// TestUserRepo_RejectsCaseVariantDuplicates is the other half of the same
// rule. A lookup that ignores case is only single-valued if the write path
// refuses to create the second row — otherwise FindByUsername picks an
// arbitrary one of two accounts, which decides whose password is checked.
func TestUserRepo_RejectsCaseVariantDuplicates(t *testing.T) {
	db := newDB(t)
	repo := NewUserRepo(db)
	ctx := context.Background()

	seedUser(t, db, "alice")

	err := repo.Save(ctx, &domain.User{
		ID:          uuid.New(),
		Username:    "ALICE",
		Email:       "someone-else@example.com",
		Roles:       []string{"user"},
		Permissions: []string{},
	})
	require.Error(t, err, "a second account was created for username ALICE")

	err = repo.Save(ctx, &domain.User{
		ID:          uuid.New(),
		Username:    "somebody-else",
		Email:       "ALICE@EXAMPLE.COM",
		Roles:       []string{"user"},
		Permissions: []string{},
	})
	require.Error(t, err, "a second account was created for email ALICE@EXAMPLE.COM")
}

// TestUserRepo_FindMissingIsAnError: unlike the user module's repository,
// these return an error for a missing row, and Login relies on that to map to
// ErrInvalidCredentials. The two modules differ here; both are recorded so
// neither is "fixed" into the other by accident.
func TestUserRepo_FindMissingIsAnError(t *testing.T) {
	db := newDB(t)
	repo := NewUserRepo(db)
	ctx := context.Background()

	_, err := repo.FindByUsername(ctx, "nobody")
	require.Error(t, err)

	_, err = repo.FindByID(ctx, uuid.New().String())
	require.Error(t, err)
}

// TestUserRepo_SaveRoundTripsLockoutState is the persistence half of the
// lockout rules. The domain increments the counter and sets the deadline; if
// Save dropped either column the account would never stay locked across
// requests, and the unit tests would still pass.
func TestUserRepo_SaveRoundTripsLockoutState(t *testing.T) {
	db := newDB(t)
	repo := NewUserRepo(db)
	ctx := context.Background()

	u := seedUser(t, db, "alice")
	u.RecordFailedLogin(3, time.Hour)
	u.RecordFailedLogin(3, time.Hour)
	u.RecordFailedLogin(3, time.Hour)
	require.True(t, u.IsLocked(), "precondition: the user should be locked")
	require.NoError(t, repo.Save(ctx, u))

	got, err := repo.FindByID(ctx, u.ID.String())
	require.NoError(t, err)
	require.Equal(t, 3, got.FailedLoginAttempts, "the failure count was not persisted")
	require.True(t, got.IsLocked(), "the lock deadline was not persisted; the account unlocks itself on reload")
}

func TestUserRepo_SaveRoundTripsRolesAndPermissions(t *testing.T) {
	db := newDB(t)
	repo := NewUserRepo(db)
	ctx := context.Background()

	u := seedUser(t, db, "alice")
	u.Roles = []string{"admin", "user"}
	u.Permissions = []string{"users:read", "users:write"}
	require.NoError(t, repo.Save(ctx, u))

	got, err := repo.FindByID(ctx, u.ID.String())
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"admin", "user"}, got.Roles)
	require.ElementsMatch(t, []string{"users:read", "users:write"}, got.Permissions)
}

func TestRefreshTokenStore_StoreThenFindValid(t *testing.T) {
	db := newDB(t)
	u := seedUser(t, db, "alice")
	store := NewRefreshTokenStore(db)
	ctx := context.Background()

	tok := domain.RefreshToken{
		UserID:    u.ID.String(),
		TokenHash: domain.HashToken("a-refresh-token"),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	require.NoError(t, store.Store(ctx, tok))

	got, err := store.FindValid(ctx, tok.TokenHash)
	require.NoError(t, err)
	require.Equal(t, u.ID.String(), got.UserID)
}

// TestRefreshTokenStore_ExpiredIsNotValid exercises the `expires_at > now()`
// clause. now() is the database's clock, not Go's, so this is the only place
// the condition can be checked at all.
func TestRefreshTokenStore_ExpiredIsNotValid(t *testing.T) {
	db := newDB(t)
	u := seedUser(t, db, "alice")
	store := NewRefreshTokenStore(db)
	ctx := context.Background()

	hash := domain.HashToken("an-expired-token")
	require.NoError(t, store.Store(ctx, domain.RefreshToken{
		UserID:    u.ID.String(),
		TokenHash: hash,
		ExpiresAt: time.Now().Add(-time.Minute),
	}))

	_, err := store.FindValid(ctx, hash)
	require.Error(t, err, "an expired refresh token was returned as valid")
}

// TestRefreshTokenStore_RevokedIsNotValid exercises the `revoked = false`
// clause — the mechanism behind logout and behind rotation. If the flag were
// ignored, logging out would not end the session.
func TestRefreshTokenStore_RevokedIsNotValid(t *testing.T) {
	db := newDB(t)
	u := seedUser(t, db, "alice")
	store := NewRefreshTokenStore(db)
	ctx := context.Background()

	hash := domain.HashToken("a-token-to-revoke")
	require.NoError(t, store.Store(ctx, domain.RefreshToken{
		UserID:    u.ID.String(),
		TokenHash: hash,
		ExpiresAt: time.Now().Add(time.Hour),
	}))

	require.NoError(t, store.RevokeAllForUser(ctx, u.ID.String()))

	_, err := store.FindValid(ctx, hash)
	require.Error(t, err, "a revoked refresh token was returned as valid")
}

// TestRefreshTokenStore_RevokeAffectsOnlyThatUser: a revoke that hit every row
// would log out the entire service whenever one person logged out.
func TestRefreshTokenStore_RevokeAffectsOnlyThatUser(t *testing.T) {
	db := newDB(t)
	alice := seedUser(t, db, "alice")
	bob := seedUser(t, db, "bob")
	store := NewRefreshTokenStore(db)
	ctx := context.Background()

	bobHash := domain.HashToken("bobs-token")
	require.NoError(t, store.Store(ctx, domain.RefreshToken{
		UserID: bob.ID.String(), TokenHash: bobHash, ExpiresAt: time.Now().Add(time.Hour),
	}))

	require.NoError(t, store.RevokeAllForUser(ctx, alice.ID.String()))

	got, err := store.FindValid(ctx, bobHash)
	require.NoError(t, err, "revoking alice's tokens also revoked bob's")
	require.Equal(t, bob.ID.String(), got.UserID)
}

func TestRefreshTokenStore_UnknownHashIsAnError(t *testing.T) {
	db := newDB(t)
	store := NewRefreshTokenStore(db)

	_, err := store.FindValid(context.Background(), domain.HashToken("never-issued"))
	require.Error(t, err)
}

func TestMFAStore_BackupCodeRoundTrip(t *testing.T) {
	db := newDB(t)
	u := seedUser(t, db, "alice")
	h := hasher.BcryptHasher{}
	store := NewMFAStore(db, h)
	ctx := context.Background()

	const plain = "backup-code-1"
	hash, err := h.Hash(plain)
	require.NoError(t, err)
	require.NoError(t, store.StoreBackupCodes(ctx, u.ID.String(), []string{hash}))

	found, err := store.FindAndUseBackupCode(ctx, u.ID.String(), plain)
	require.NoError(t, err)
	require.True(t, found)
}

// TestMFAStore_BackupCodeIsSingleUse exercises the `used = true` update. A
// backup code that keeps working is a static password that bypasses the second
// factor permanently, and the flag only exists in the database.
func TestMFAStore_BackupCodeIsSingleUse(t *testing.T) {
	db := newDB(t)
	u := seedUser(t, db, "alice")
	h := hasher.BcryptHasher{}
	store := NewMFAStore(db, h)
	ctx := context.Background()

	const plain = "backup-code-1"
	hash, err := h.Hash(plain)
	require.NoError(t, err)
	require.NoError(t, store.StoreBackupCodes(ctx, u.ID.String(), []string{hash}))

	found, err := store.FindAndUseBackupCode(ctx, u.ID.String(), plain)
	require.NoError(t, err)
	require.True(t, found, "precondition: the first use should succeed")

	found, err = store.FindAndUseBackupCode(ctx, u.ID.String(), plain)
	require.NoError(t, err)
	require.False(t, found, "the backup code was accepted twice")
}

// TestMFAStore_BackupCodesAreScopedToTheUser: one user's backup code must
// never authenticate another's account.
func TestMFAStore_BackupCodesAreScopedToTheUser(t *testing.T) {
	db := newDB(t)
	alice := seedUser(t, db, "alice")
	bob := seedUser(t, db, "bob")
	h := hasher.BcryptHasher{}
	store := NewMFAStore(db, h)
	ctx := context.Background()

	const plain = "alices-backup-code"
	hash, err := h.Hash(plain)
	require.NoError(t, err)
	require.NoError(t, store.StoreBackupCodes(ctx, alice.ID.String(), []string{hash}))

	found, err := store.FindAndUseBackupCode(ctx, bob.ID.String(), plain)
	require.NoError(t, err)
	require.False(t, found, "alice's backup code authenticated bob")
}

func TestMFAStore_WrongCodeIsNotFound(t *testing.T) {
	db := newDB(t)
	u := seedUser(t, db, "alice")
	h := hasher.BcryptHasher{}
	store := NewMFAStore(db, h)
	ctx := context.Background()

	hash, err := h.Hash("the-real-code")
	require.NoError(t, err)
	require.NoError(t, store.StoreBackupCodes(ctx, u.ID.String(), []string{hash}))

	found, err := store.FindAndUseBackupCode(ctx, u.ID.String(), "not-the-real-code")
	require.NoError(t, err)
	require.False(t, found)
}

// TestMFAStore_RejectsAMalformedUserID: the id is parsed as a UUID before it
// reaches SQL, so a malformed one is an error rather than a query.
func TestMFAStore_RejectsAMalformedUserID(t *testing.T) {
	db := newDB(t)
	store := NewMFAStore(db, hasher.BcryptHasher{})

	_, err := store.FindAndUseBackupCode(context.Background(), "not-a-uuid", "x")
	require.Error(t, err)
}
