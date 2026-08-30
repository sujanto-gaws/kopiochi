// This file is D9's composition-level acceptance test: it drives a real
// identity use case through the real HTTP application (BuildApp +
// httpx.Mount, the same composition path production uses) against a migrated
// database, and asserts a pending row lands in the notification module's own
// outbox — with nothing in this test standing in for the identity module, the
// cmd/api adapter, or the notification module's Enqueue.
package main

import (
	"context"
	"net/http"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/sujanto-gaws/kopiochi/internal/httpx"
	notifdomain "github.com/sujanto-gaws/kopiochi/modules/notification/domain"
	notifrepo "github.com/sujanto-gaws/kopiochi/modules/notification/infrastructure/persistence/repository"
	notiftemplate "github.com/sujanto-gaws/kopiochi/modules/notification/infrastructure/template"
)

// TestLogin_AccountLockedEnqueuesAnInAppSecurityNotification is the D9 accept
// criterion: "one composition-level test drives an identity use case and
// asserts a pending row lands (DB-backed, CI-arbitrated)".
//
// The dispatcher's PollInterval is an hour (enabledNotification()), so this
// test observes the row before the dispatcher's first tick — Enqueue commits
// synchronously inside Login's own request, and ListForRecipient reads
// whatever is there regardless of delivery status. Email is left disabled by
// enabledNotification(), so only the in-app enqueue is expected to have
// landed; the adapter's own unit tests (notifier_adapter_test.go) cover the
// email attempt and its idempotency key independently of a database.
func TestLogin_AccountLockedEnqueuesAnInAppSecurityNotification(t *testing.T) {
	bunDB := scratchIdentityDB(t)

	const username = "carol"
	seeded := seedAuthUser(t, bunDB, username, "carol@example.com")

	cfg := testConfig(t)
	cfg.Notification = enabledNotification()

	app, err := BuildApp(cfg, bunDB, zerolog.Nop())
	require.NoError(t, err)
	t.Cleanup(func() { closeModules(t, app) })

	r, closeRouter, err := httpx.NewRouter(cfg.Server, cfg.Security, zerolog.Nop(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = closeRouter() })
	httpx.Mount(r, app.Modules, httpx.Deps{Pinger: nil})

	// Enough wrong passwords to cross the lockout threshold
	// (testsupport.Config: MaxFailedAttempts=5) and trip login.go's
	// !wasLocked && user.IsLocked() transition guard exactly once.
	for i := 0; i < cfg.Auth.MaxFailedAttempts; i++ {
		rec := doLogin(t, r, username, "definitely-the-wrong-password")
		require.NotEqual(t, http.StatusOK, rec.Code, "attempt %d unexpectedly succeeded", i+1)
	}

	repo := notifrepo.NewNotificationRepo(bunDB)
	rows, err := repo.ListForRecipient(context.Background(), seeded.ID, notifdomain.ListFilter{})
	require.NoError(t, err)
	require.Len(t, rows, 1, "exactly one in-app row for the one lockout episode")

	row := rows[0]
	require.Equal(t, notifdomain.CategorySecurity, row.Category)
	require.Equal(t, "security.account_locked", row.TemplateKey)
	require.Equal(t, notifdomain.StatusPending, row.Status, "the dispatcher has not polled yet")
	require.Contains(t, row.IdempotencyKey, "account_locked:"+seeded.ID.String(),
		"idempotency key must name the event and the user")
	require.Contains(t, row.IdempotencyKey, string(notifdomain.ChannelInApp),
		"idempotency key must be channel-scoped, or a second channel's enqueue would collide on the same key")
	require.NotEmpty(t, row.Payload["LockedUntil"])

	// The row renders: a payload the template cannot execute would only
	// surface as a dead letter hours later. Exercised directly, against the
	// module's own shipped renderer, rather than by waiting out the
	// dispatcher's poll interval, which this test does not.
	renderer, err := notiftemplate.New()
	require.NoError(t, err)
	_, err = renderer.Render(row.TemplateKey, notifdomain.ChannelInApp, row.Payload)
	require.NoError(t, err, "the enqueued payload does not satisfy the security.account_locked.inapp template")
}
