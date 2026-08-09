package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/sujanto-gaws/kopiochi/internal/testsupport"
	domain "github.com/sujanto-gaws/kopiochi/modules/notification/domain"
)

// See the note at the top of notification_repo_integration_test.go: these run
// against a real Postgres and skip cleanly without one.

func newPreferenceRepo(t *testing.T) *PreferenceRepo {
	t.Helper()

	bunDB := testsupport.MigratedDB(t)
	testsupport.TruncateAll(t, bunDB)
	return NewPreferenceRepo(bunDB)
}

// TestPreferenceRepo_ListForUnknownUserIsEmptyNotAnError pins the contract the
// domain's Allowed is written against: absence means "never chose", which
// resolves to enabled. A repository that reported ErrNotFound here would turn
// every first-time user's enqueue into a failure.
func TestPreferenceRepo_ListForUnknownUserIsEmptyNotAnError(t *testing.T) {
	repo := newPreferenceRepo(t)

	prefs, err := repo.ListForUser(context.Background(), uuid.New())
	require.NoError(t, err)
	require.NotNil(t, prefs, "an empty result must still be a slice")
	require.Empty(t, prefs)
}

func TestPreferenceRepo_UpsertThenList(t *testing.T) {
	repo := newPreferenceRepo(t)
	ctx := context.Background()

	user := uuid.New()
	now := pgTime(time.Now())

	off, err := domain.NewPreference(user, domain.ChannelEmail, domain.CategorySystem, false, now)
	require.NoError(t, err)
	on, err := domain.NewPreference(user, domain.ChannelInApp, domain.CategorySecurity, true, now)
	require.NoError(t, err)

	require.NoError(t, repo.Upsert(ctx, []domain.Preference{off, on}))

	prefs, err := repo.ListForUser(ctx, user)
	require.NoError(t, err)
	require.Len(t, prefs, 2)

	// Ordered by (channel, category): email < inapp.
	require.Equal(t, domain.ChannelEmail, prefs[0].Channel)
	require.Equal(t, domain.CategorySystem, prefs[0].Category)
	require.False(t, prefs[0].Enabled)
	require.Equal(t, now, pgTime(prefs[0].UpdatedAt))

	require.Equal(t, domain.ChannelInApp, prefs[1].Channel)
	require.Equal(t, domain.CategorySecurity, prefs[1].Category)
	require.True(t, prefs[1].Enabled)
}

// TestPreferenceRepo_UpsertReplacesTheExistingRow: the primary key is the
// (user, channel, category) triple, so a second write of the same pair must
// update rather than insert or fail. Toggling a preference off and on again is
// the ordinary case.
func TestPreferenceRepo_UpsertReplacesTheExistingRow(t *testing.T) {
	repo := newPreferenceRepo(t)
	ctx := context.Background()

	user := uuid.New()
	first := pgTime(time.Now())
	second := pgTime(first.Add(time.Hour))

	off, err := domain.NewPreference(user, domain.ChannelWebhook, domain.CategoryAccount, false, first)
	require.NoError(t, err)
	require.NoError(t, repo.Upsert(ctx, []domain.Preference{off}))

	on, err := domain.NewPreference(user, domain.ChannelWebhook, domain.CategoryAccount, true, second)
	require.NoError(t, err)
	require.NoError(t, repo.Upsert(ctx, []domain.Preference{on}))

	prefs, err := repo.ListForUser(ctx, user)
	require.NoError(t, err)
	require.Len(t, prefs, 1, "the second write inserted a second row for one pair")
	require.True(t, prefs[0].Enabled)
	require.Equal(t, second, pgTime(prefs[0].UpdatedAt),
		"updated_at came from the database's clock instead of the caller's")
}

// TestPreferenceRepo_UpsertIsScopedToItsUser guards the read: one user's
// preferences must not appear in another's matrix, which is what decides
// whether a notification is suppressed.
func TestPreferenceRepo_UpsertIsScopedToItsUser(t *testing.T) {
	repo := newPreferenceRepo(t)
	ctx := context.Background()

	mine, theirs := uuid.New(), uuid.New()
	now := pgTime(time.Now())

	a, err := domain.NewPreference(mine, domain.ChannelInApp, domain.CategorySystem, false, now)
	require.NoError(t, err)
	b, err := domain.NewPreference(theirs, domain.ChannelInApp, domain.CategorySystem, false, now)
	require.NoError(t, err)
	require.NoError(t, repo.Upsert(ctx, []domain.Preference{a, b}))

	prefs, err := repo.ListForUser(ctx, mine)
	require.NoError(t, err)
	require.Len(t, prefs, 1)
	require.Equal(t, mine, prefs[0].UserID)
}

// TestPreferenceRepo_UpsertOfNothingIsANoOp: bun cannot build an INSERT with no
// rows, and "write nothing" has an obvious answer. Without the guard this is a
// panic or a malformed statement on a request that changed no toggles.
func TestPreferenceRepo_UpsertOfNothingIsANoOp(t *testing.T) {
	repo := newPreferenceRepo(t)

	require.NoError(t, repo.Upsert(context.Background(), nil))
	require.NoError(t, repo.Upsert(context.Background(), []domain.Preference{}))
}
