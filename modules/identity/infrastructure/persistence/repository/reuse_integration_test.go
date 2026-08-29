package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/sujanto-gaws/kopiochi/internal/db"
	domain "github.com/sujanto-gaws/kopiochi/modules/identity/domain"
)

// Integration tests for the refresh-token reuse-detection path: FindAny,
// Rotate and RevokeFamily.
//
// These three carried NO coverage at all, which is worse here than the number
// suggests. Rotation alone does not survive theft — after a stolen token is
// spent, both parties hold something that looks valid — so the family sweep is
// what actually ends the attacker's session. Every property below is enforced
// in SQL (a rowsAffected check, a family predicate), so none of it is reachable
// from a unit test.

// liveToken stores a valid token for u in the given family (empty starts a new
// one) and returns the stored token as the database sees it.
func liveToken(t *testing.T, store *RefreshTokenStore, userID, familyID, secret string) *domain.RefreshToken {
	t.Helper()

	ctx := context.Background()
	hash := domain.HashToken(secret)
	require.NoError(t, store.Store(ctx, domain.RefreshToken{
		UserID:    userID,
		TokenHash: hash,
		FamilyID:  familyID,
		ExpiresAt: time.Now().Add(time.Hour),
	}))

	stored, err := store.FindValid(ctx, hash)
	require.NoError(t, err, "seeded token is not valid")
	return stored
}

// TestRefreshTokenStore_FindAnyDistinguishesSpentFromUnknown is the whole
// reason FindAny exists. FindValid answers "can this be used", and folds "never
// existed" and "already spent" into one error. Reuse detection turns on exactly
// that difference: the first is a credential that never was, the second is
// evidence that a real one was captured.
func TestRefreshTokenStore_FindAnyDistinguishesSpentFromUnknown(t *testing.T) {
	dbc := newDB(t)
	u := seedUser(t, dbc, "alice")
	store := NewRefreshTokenStore(dbc)
	ctx := context.Background()

	parent := liveToken(t, store, u.ID.String(), "", "the-original")
	require.NoError(t, store.Rotate(ctx, parent.TokenHash, domain.RefreshToken{
		UserID:    u.ID.String(),
		TokenHash: domain.HashToken("the-successor"),
		FamilyID:  parent.FamilyID,
		ExpiresAt: time.Now().Add(time.Hour),
	}))

	_, err := store.FindValid(ctx, parent.TokenHash)
	require.Error(t, err, "a spent token must not be valid")

	spent, err := store.FindAny(ctx, parent.TokenHash)
	require.NoError(t, err, "a spent token must still be findable, or reuse is undetectable")
	require.True(t, spent.Used, "the spent token does not report itself as used")
	require.True(t, spent.Revoked)
	require.Equal(t, parent.FamilyID, spent.FamilyID, "the family link was lost")

	_, err = store.FindAny(ctx, domain.HashToken("never-issued"))
	require.ErrorIs(t, err, db.ErrNotFound,
		"an unknown token must be distinguishable from a spent one")
}

// TestRefreshTokenStore_RotateRetiresTheOldAndIssuesTheNew: both halves must
// land, and they must land together.
func TestRefreshTokenStore_RotateRetiresTheOldAndIssuesTheNew(t *testing.T) {
	dbc := newDB(t)
	u := seedUser(t, dbc, "alice")
	store := NewRefreshTokenStore(dbc)
	ctx := context.Background()

	parent := liveToken(t, store, u.ID.String(), "", "parent")
	childHash := domain.HashToken("child")

	require.NoError(t, store.Rotate(ctx, parent.TokenHash, domain.RefreshToken{
		UserID:    u.ID.String(),
		TokenHash: childHash,
		FamilyID:  parent.FamilyID,
		ExpiresAt: time.Now().Add(time.Hour),
	}))

	child, err := store.FindValid(ctx, childHash)
	require.NoError(t, err, "the successor was not issued")
	require.Equal(t, parent.FamilyID, child.FamilyID,
		"the successor started a new family, so a later reuse sweep would miss the parent")

	_, err = store.FindValid(ctx, parent.TokenHash)
	require.Error(t, err, "the rotated token is still usable")
}

// TestRefreshTokenStore_RotateIsSingleUse covers the rowsAffected check, which
// is the store's entire concurrency story: two refreshes presenting the same
// token both pass the earlier read, and only one UPDATE may match. Asserted
// sequentially — the property is "the second one loses", and forcing genuine
// goroutine overlap to observe that would only make the test flaky (BL36).
func TestRefreshTokenStore_RotateIsSingleUse(t *testing.T) {
	dbc := newDB(t)
	u := seedUser(t, dbc, "alice")
	store := NewRefreshTokenStore(dbc)
	ctx := context.Background()

	parent := liveToken(t, store, u.ID.String(), "", "parent")
	rotateTo := func(secret string) error {
		return store.Rotate(ctx, parent.TokenHash, domain.RefreshToken{
			UserID:    u.ID.String(),
			TokenHash: domain.HashToken(secret),
			FamilyID:  parent.FamilyID,
			ExpiresAt: time.Now().Add(time.Hour),
		})
	}

	require.NoError(t, rotateTo("first-child"))
	require.ErrorIs(t, rotateTo("second-child"), domain.ErrRefreshTokenAlreadyUsed)

	// The loser's child must not exist. If the UPDATE fails but the INSERT has
	// already run, the attacker is handed a working token by the very call that
	// was supposed to refuse them.
	_, err := store.FindAny(ctx, domain.HashToken("second-child"))
	require.ErrorIs(t, err, db.ErrNotFound, "the refused rotation still issued a token")
}

// TestRefreshTokenStore_RotateUnknownTokenIsRefused: nothing matches, so
// rowsAffected is zero and the caller is told the token is spent rather than
// being silently issued a successor to a token that never existed.
func TestRefreshTokenStore_RotateUnknownTokenIsRefused(t *testing.T) {
	dbc := newDB(t)
	u := seedUser(t, dbc, "alice")
	store := NewRefreshTokenStore(dbc)
	ctx := context.Background()

	err := store.Rotate(ctx, domain.HashToken("never-issued"), domain.RefreshToken{
		UserID:    u.ID.String(),
		TokenHash: domain.HashToken("child-of-nothing"),
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.ErrorIs(t, err, domain.ErrRefreshTokenAlreadyUsed)

	_, err = store.FindAny(ctx, domain.HashToken("child-of-nothing"))
	require.ErrorIs(t, err, db.ErrNotFound, "a successor was issued for a token that never existed")
}

// TestRefreshTokenStore_RevokeFamilyKillsTheWholeChain: the count is what the
// audit event reports, and "revoked 3" versus "revoked 0" is the difference
// between catching a theft early and catching it late.
func TestRefreshTokenStore_RevokeFamilyKillsTheWholeChain(t *testing.T) {
	dbc := newDB(t)
	u := seedUser(t, dbc, "alice")
	store := NewRefreshTokenStore(dbc)
	ctx := context.Background()

	first := liveToken(t, store, u.ID.String(), "", "sibling-one")
	family := first.FamilyID
	liveToken(t, store, u.ID.String(), family, "sibling-two")
	liveToken(t, store, u.ID.String(), family, "sibling-three")

	revoked, err := store.RevokeFamily(ctx, family)
	require.NoError(t, err)
	require.Equal(t, 3, revoked, "the reported count is what the audit record carries")

	for _, secret := range []string{"sibling-one", "sibling-two", "sibling-three"} {
		_, err := store.FindValid(ctx, domain.HashToken(secret))
		require.Error(t, err, "%q survived the family revocation", secret)
	}

	// Already-revoked rows are excluded, so a second sweep reports nothing left
	// to kill rather than re-counting the same tokens.
	again, err := store.RevokeFamily(ctx, family)
	require.NoError(t, err)
	require.Zero(t, again)
}

// TestRefreshTokenStore_RevokeFamilyLeavesOtherFamiliesAlone: a reuse detection
// on one login must not log the account out everywhere. That is what separates
// this from RevokeAllForUser.
func TestRefreshTokenStore_RevokeFamilyLeavesOtherFamiliesAlone(t *testing.T) {
	dbc := newDB(t)
	u := seedUser(t, dbc, "alice")
	store := NewRefreshTokenStore(dbc)
	ctx := context.Background()

	compromised := liveToken(t, store, u.ID.String(), "", "laptop")
	untouched := liveToken(t, store, u.ID.String(), "", "phone")
	require.NotEqual(t, compromised.FamilyID, untouched.FamilyID,
		"an empty FamilyID must start a NEW family, or these are the same login")

	revoked, err := store.RevokeFamily(ctx, compromised.FamilyID)
	require.NoError(t, err)
	require.Equal(t, 1, revoked)

	_, err = store.FindValid(ctx, untouched.TokenHash)
	require.NoError(t, err, "revoking one family logged the user out of the other")
}

// TestRefreshTokenStore_RevokeFamilyRejectsAMalformedID: the id reaches here
// from a token row, but a parse that silently produced the zero uuid would
// revoke whatever happened to carry it.
func TestRefreshTokenStore_RevokeFamilyRejectsAMalformedID(t *testing.T) {
	dbc := newDB(t)
	store := NewRefreshTokenStore(dbc)

	revoked, err := store.RevokeFamily(context.Background(), "not-a-uuid")
	require.Error(t, err)
	require.Zero(t, revoked)
	require.Contains(t, err.Error(), "parse family id")
}

// TestRefreshTokenStore_StoreRejectsAMalformedFamilyID: toRefreshRow's parse
// branch. A token stored under the zero uuid would be swept by any unrelated
// revocation that also failed to parse.
func TestRefreshTokenStore_StoreRejectsAMalformedFamilyID(t *testing.T) {
	dbc := newDB(t)
	u := seedUser(t, dbc, "alice")
	store := NewRefreshTokenStore(dbc)

	err := store.Store(context.Background(), domain.RefreshToken{
		UserID:    u.ID.String(),
		TokenHash: domain.HashToken("orphan"),
		FamilyID:  "not-a-uuid",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse family id")
}

// TestRefreshTokenStore_EmptyFamilyIDStartsAFreshFamily documents the one case
// where the store generates an id rather than storing what it was given.
func TestRefreshTokenStore_EmptyFamilyIDStartsAFreshFamily(t *testing.T) {
	dbc := newDB(t)
	u := seedUser(t, dbc, "alice")
	store := NewRefreshTokenStore(dbc)

	tok := liveToken(t, store, u.ID.String(), "", "fresh-login")
	parsed, err := uuid.Parse(tok.FamilyID)
	require.NoError(t, err, "the generated family id is not a uuid")
	require.NotEqual(t, uuid.Nil, parsed, "a fresh login was filed under the zero uuid")
}
