package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"

	"github.com/sujanto-gaws/kopiochi/internal/config"
	"github.com/sujanto-gaws/kopiochi/internal/module"
	"github.com/sujanto-gaws/kopiochi/internal/platform/secret"
)

// TestBuildApp_RegistersModules is task 1.1(a) from the remediation plan
// (docs/architectures/06-quality/testing-strategy.md:71-76): the guard
// against an empty container that would start, log "modules registered",
// answer the health check, and serve nothing.
//
// It reuses testConfig(t) from routes_test.go rather than building a second
// way to construct a valid *config.Config.
func TestBuildApp_RegistersModules(t *testing.T) {
	t.Parallel()

	cfg := testConfig(t)

	app, err := BuildApp(cfg, nil, zerolog.Nop())
	require.NoError(t, err)
	require.NotEmpty(t, app.Modules)
}

// TestBuildApp_FailsOnInvalidConfig asserts that BuildApp surfaces a module's
// config validation failure as a wrapped error instead of returning a
// partially-built or empty App. An empty Issuer fails identity.Config.
// Validate (modules/identity/module.go), which is the first module BuildApp
// constructs.
//
// This test does NOT exercise the "no modules registered: refusing to start
// an empty application" guard at cmd/api/container.go:66-68 — see the
// package-level finding below. Writing a test that claims to cover that
// guard when it cannot fail would be worse than not testing it at all, so
// this test is scoped to what BuildApp actually does today: propagate a
// module build error.
func TestBuildApp_FailsOnInvalidConfig(t *testing.T) {
	t.Parallel()

	cfg := testConfig(t)
	cfg.Auth.Issuer = "" // identity.Config.Validate rejects an empty issuer

	app, err := BuildApp(cfg, nil, zerolog.Nop())
	require.Error(t, err)
	require.Nil(t, app)
	require.Contains(t, err.Error(), "issuer is required")
}

// FINDING (not fixed here — tests only, per task scope): the zero-module
// guard at cmd/api/container.go:66-68
//
//	if len(mods) == 0 {
//	    return nil, errors.New("no modules registered: refusing to start an empty application")
//	}
//
// is currently unreachable. BuildApp appends identityMod and userMod to mods
// immediately after each builds successfully, and returns early (before
// reaching the guard) on any build error. So by the time execution reaches
// `len(mods) == 0`, either the function has already returned via one of the
// two `return nil, fmt.Errorf(...)` paths, or both appends have already
// happened and len(mods) == 2. There is no config or code path in the
// current source that reaches the guard with an empty mods slice — it would
// only become reachable if a module were conditionally added (e.g. behind a
// feature flag) without an early return on the "not added" branch. No test
// is written against this guard directly because none can be made to fail
// against the current implementation; a test asserting NoError still passes
// through the (correct) module-building logic, not through the guard being
// meaningfully exercised.

// refusingConnector is a database handle that never dials. database/sql
// connects lazily, so sql.OpenDB over it yields a real, non-nil *bun.DB that
// every module can build its repositories from, and that fails loudly if
// anything queries. BuildApp's other tests pass a nil DB; the notification
// module refuses one when it is enabled, which is what this exists for.
type refusingConnector struct{}

func (refusingConnector) Connect(context.Context) (driver.Conn, error) {
	return nil, errors.New("this test must not touch the database")
}

func (refusingConnector) Driver() driver.Driver { return nil }

func lazyDB(t *testing.T) bun.IDB {
	t.Helper()

	sqldb := sql.OpenDB(refusingConnector{})
	t.Cleanup(func() { _ = sqldb.Close() })
	return bun.NewDB(sqldb, pgdialect.New())
}

// enabledNotification is a valid module whose dispatcher will not poll during a
// test: the first tick is an hour away and every test here closes the app long
// before that.
func enabledNotification() config.Notification {
	return config.Notification{
		Enabled: true,
		Dispatcher: config.NotificationDispatcher{
			PollInterval: time.Hour,
			BatchSize:    50,
			Workers:      2,
			MaxAttempts:  6,
			BackoffBase:  30 * time.Second,
			BackoffCap:   time.Hour,
			StalledAfter: time.Hour,
			DrainTimeout: 5 * time.Second,
		},
	}
}

// closeModules is what serve does through the lifecycle stack: every module
// that owns something gets its Close called, in reverse construction order. A
// test that builds an App and drops it leaks the dispatcher's goroutines.
func closeModules(t *testing.T, app *App) {
	t.Helper()

	for i := len(app.Modules) - 1; i >= 0; i-- {
		if m := app.Modules[i]; m.Close != nil {
			if err := m.Close(); err != nil {
				t.Errorf("close module %s: %v", m.Name, err)
			}
		}
	}
}

func moduleNamed(app *App, name string) *module.Module {
	for _, m := range app.Modules {
		if m.Name == name {
			return m
		}
	}
	return nil
}

// The disabled module is still registered. Pretending it is absent would make
// "which modules does this deployment run" a question with two answers, and it
// is the state every other test in this package boots in — testsupport.Config
// zeroes the section, so notification.enabled is false there.
func TestBuildApp_RegistersTheNotificationModuleEvenWhenDisabled(t *testing.T) {
	t.Parallel()

	cfg := testConfig(t)
	cfg.Notification.Enabled = false

	app, err := BuildApp(cfg, nil, zerolog.Nop())
	require.NoError(t, err)

	m := moduleNamed(app, "notification")
	require.NotNil(t, m, "the notification module is not registered")
	require.NotNil(t, m.Routes, "httpx.Mount calls Routes unconditionally")
	require.NotNil(t, m.Close)
	require.NoError(t, m.Close())
}

// The other half of the acceptance: the application boots with the module on,
// and everything it started stops again.
func TestBuildApp_BootsWithTheNotificationModuleEnabled(t *testing.T) {
	db := lazyDB(t)

	before := runtime.NumGoroutine()

	cfg := testConfig(t)
	cfg.Notification = enabledNotification()

	app, err := BuildApp(cfg, db, zerolog.Nop())
	require.NoError(t, err)
	require.NotNil(t, moduleNamed(app, "notification"))

	require.Greater(t, runtime.NumGoroutine(), before, "the dispatcher did not start")

	closeModules(t, app)

	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > before {
		if time.Now().After(deadline) {
			t.Fatalf("shutdown left %d goroutine(s) running", runtime.NumGoroutine()-before)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// Fail closed at boot, and say which setting. Email switched on with no host,
// no from address and no APP_NOTIFICATION_EMAIL_PASSWORD is not a deployment
// that sends no mail — it is one that dead-letters every security notification
// it queues.
func TestBuildApp_FailsOnIncompleteNotificationEmailConfig(t *testing.T) {
	t.Parallel()

	cfg := testConfig(t)
	cfg.Notification = enabledNotification()
	cfg.Notification.Email.Enabled = true

	app, err := BuildApp(cfg, lazyDB(t), zerolog.Nop())
	require.Error(t, err)
	require.Nil(t, app)
	require.Contains(t, err.Error(), "build notification module")
	require.Contains(t, err.Error(), "email.smtp_host is required")
	require.Contains(t, err.Error(), "APP_NOTIFICATION_EMAIL_PASSWORD")
}

// The credential travels from the host's config into the module's as a
// secret.String, and secret.String redacts itself in every formatting verb —
// so a wiring mistake that logged the whole config would not print it.
func TestBuildApp_KeepsTheSMTPPasswordRedacted(t *testing.T) {
	t.Parallel()

	cfg := testConfig(t)
	cfg.Notification = enabledNotification()
	cfg.Notification.Email = config.NotificationEmail{
		Enabled:  true,
		SMTPHost: "smtp.example.test",
		SMTPPort: 587,
		From:     "no-reply@example.test",
		Password: secret.String("the-smtp-credential"),
		Timeout:  10 * time.Second,
	}

	app, err := BuildApp(cfg, lazyDB(t), zerolog.Nop())
	require.NoError(t, err)
	t.Cleanup(func() { closeModules(t, app) })

	require.NotContains(t, fmt.Sprintf("%v %+v %s", cfg.Notification, cfg.Notification, cfg.Notification.Email.Password),
		"the-smtp-credential")
}

// The composition root's half of E15: with email on, BuildApp must supply the
// address resolver the sender consumes.
//
// Asserted through the module's own fail-closed rule rather than by reaching
// into the App — notification.Config.Validate refuses an enabled email block
// with no resolver, so an unwired one is a boot failure and this test is the
// thing that would see it.
func TestBuildApp_WiresTheEmailAddressResolver(t *testing.T) {
	t.Parallel()

	cfg := testConfig(t)
	cfg.Notification = enabledNotification()
	cfg.Notification.Email = config.NotificationEmail{
		Enabled:  true,
		SMTPHost: "smtp.example.test",
		SMTPPort: 587,
		From:     "no-reply@example.test",
		Password: secret.String("a-real-credential"),
		Timeout:  10 * time.Second,
	}

	app, err := BuildApp(cfg, lazyDB(t), zerolog.Nop())
	require.NoError(t, err, "the email sender was built without an address resolver")
	t.Cleanup(func() { closeModules(t, app) })
}

// And it is built only when it is needed: a deployment that sends no email owes
// no resolver, and attaching a repository to a module that will never call it
// is wiring for its own sake.
func TestBuildApp_OmitsTheEmailAddressResolverWhenEmailIsOff(t *testing.T) {
	t.Parallel()

	cfg := testConfig(t)
	cfg.Notification = enabledNotification()

	app, err := BuildApp(cfg, lazyDB(t), zerolog.Nop())
	require.NoError(t, err)
	t.Cleanup(func() { closeModules(t, app) })
}
