package main

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	identityrepo "github.com/sujanto-gaws/kopiochi/modules/identity/infrastructure/persistence/repository"
	notifdomain "github.com/sujanto-gaws/kopiochi/modules/notification/domain"
	notifsender "github.com/sujanto-gaws/kopiochi/modules/notification/infrastructure/sender"
)

// The adapter is one lookup and one error translation, and the translation is
// the whole point: it decides whether a notification is retried or destroyed.
// Backwards one way, a deleted user is retried until its budget is gone;
// backwards the other, a security mail dies on a network blip. Neither is
// visible in review, so both are asserted here.
//
// The fixtures are the ones the login end-to-end test already uses
// (scratchIdentityDB, seedAuthUser), so the seed goes through identity's own
// repository and mapping rather than a second description of its table.

func TestIdentityEmailResolver_ReturnsTheRecipientsAddress(t *testing.T) {
	db := scratchIdentityDB(t)

	const address = "ada@example.test"
	user := seedAuthUser(t, db, "ada", address)

	resolver := identityEmailResolver{users: identityrepo.NewUserRepo(db)}

	got, err := resolver.ResolveEmail(context.Background(), user.ID)
	require.NoError(t, err)
	require.Equal(t, address, got)
}

// A recipient who does not exist can never be delivered to, so the row dies on
// its first attempt instead of spending its whole budget rediscovering that.
func TestIdentityEmailResolver_ReportsAMissingUserAsPermanent(t *testing.T) {
	db := scratchIdentityDB(t)

	resolver := identityEmailResolver{users: identityrepo.NewUserRepo(db)}

	_, err := resolver.ResolveEmail(context.Background(), uuid.New())
	require.Error(t, err)
	require.ErrorIs(t, err, notifsender.ErrNoAddress)
	require.ErrorIs(t, err, notifdomain.ErrNonRetryable)
}

// An account with no address on file is the same answer as no account: there is
// nowhere to deliver, and no retry creates one.
func TestIdentityEmailResolver_ReportsAUserWithNoEmailAsPermanent(t *testing.T) {
	db := scratchIdentityDB(t)

	user := seedAuthUser(t, db, "no-address", "")

	resolver := identityEmailResolver{users: identityrepo.NewUserRepo(db)}

	_, err := resolver.ResolveEmail(context.Background(), user.ID)
	require.Error(t, err)
	require.ErrorIs(t, err, notifsender.ErrNoAddress)
}

// The other half of the contract, and the half that is easy to get wrong: a
// lookup that failed is not an answer about the recipient, and must be retried.
//
// No database needed — the handle here refuses to connect, which is exactly the
// transient failure being modelled.
func TestIdentityEmailResolver_RetriesAFailedLookup(t *testing.T) {
	t.Parallel()

	resolver := identityEmailResolver{users: identityrepo.NewUserRepo(lazyDB(t))}

	_, err := resolver.ResolveEmail(context.Background(), uuid.New())
	require.Error(t, err)
	require.NotErrorIs(t, err, notifsender.ErrNoAddress)
	require.NotErrorIs(t, err, notifdomain.ErrNonRetryable)
}
