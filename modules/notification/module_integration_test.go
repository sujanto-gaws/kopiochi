package notification_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/sujanto-gaws/kopiochi/internal/module"
	"github.com/sujanto-gaws/kopiochi/internal/testsupport"
	"github.com/sujanto-gaws/kopiochi/modules/notification"
	domain "github.com/sujanto-gaws/kopiochi/modules/notification/domain"
	"github.com/sujanto-gaws/kopiochi/modules/notification/infrastructure/persistence/repository"
)

// The merge gate for "the module exists": a row committed to the outbox is
// delivered by the module's own dispatcher, through its own renderer and its
// own sender, with nothing in this test standing in for any of them.
//
// Everything above this line is fakes and construction checks, which prove that
// the pieces fit and not that they work. This is the one test where the wiring
// is the subject: notification.New builds the graph and starts the worker, and
// the only thing the test does afterwards is watch the row.
//
// It skips cleanly with no database, like every other integration test here,
// and does not call t.Parallel: it truncates the shared test database.
func TestModuleDeliversAQueuedNotification(t *testing.T) {
	bunDB := testsupport.MigratedDB(t)
	testsupport.TruncateAll(t, bunDB)

	ctx := context.Background()
	recipient := uuid.New()

	// In-app, because its sender is the one wired unconditionally — and
	// because the row *is* the notification, so "delivered" is observable in
	// the same table the test queued it in.
	//
	// The template key is a real one, shipped in this binary, and the payload
	// is exactly what it names: the renderer runs with missingkey=error, so a
	// row whose payload is wrong dead-letters instead of being delivered. That
	// makes this test a check on the shipped template family too.
	queued, err := domain.NewNotification(domain.NewNotificationParams{
		ID:          uuid.New(),
		RecipientID: recipient,
		Channel:     domain.ChannelInApp,
		Category:    domain.CategorySecurity,
		TemplateKey: "security.password_changed",
		Payload:     map[string]any{"ChangedAt": "30 August 2026 at 09:14 UTC"},
	}, time.Now())
	require.NoError(t, err)

	repo := repository.NewNotificationRepo(bunDB)
	require.NoError(t, repo.Enqueue(ctx, queued))

	cfg := validConfig()
	// Fast enough that the test does not wait on the shipped five-second
	// interval, and still a real ticker.
	cfg.Dispatcher.PollInterval = 10 * time.Millisecond
	cfg.Dispatcher.StalledAfter = time.Minute
	cfg.Dispatcher.DrainTimeout = 5 * time.Second

	m, err := notification.New(module.Deps{DB: bunDB, Logger: zerolog.Nop()}, cfg)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, m.Close()) })

	// Polled until the row reaches a state the dispatcher settles it in.
	// "not pending" is not that state: sending is the row mid-flight, and a
	// test that stopped there would be asserting on a claim rather than on a
	// delivery.
	var settled *domain.Notification
	deadline := time.Now().Add(10 * time.Second)
	for settled == nil || !isSettled(settled.Status) {
		if time.Now().After(deadline) {
			t.Fatalf("the dispatcher never settled the row; last seen: %+v", settled)
		}
		time.Sleep(10 * time.Millisecond)

		rows, err := repo.ListForRecipient(ctx, recipient, domain.ListFilter{})
		require.NoError(t, err)
		require.Len(t, rows, 1)
		settled = rows[0]
	}

	require.Equal(t, domain.StatusSent, settled.Status, "last error: %q", settled.LastError)
	require.NotNil(t, settled.SentAt)
	require.Zero(t, settled.Attempts, "a first-attempt success must not spend an attempt")
	require.Empty(t, settled.LastError)
	require.Nil(t, settled.ReadAt, "delivery is not reading; ReadAt belongs to the recipient")
}

// isSettled reports whether a delivery attempt has finished, whatever its
// outcome. Failed is settled too: the row is back in the queue with a reason,
// which is a result the test can read rather than a moment it can miss.
func isSettled(s domain.Status) bool {
	switch s {
	case domain.StatusSent, domain.StatusFailed, domain.StatusDead:
		return true
	default:
		return false
	}
}
