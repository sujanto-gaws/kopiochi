package notification_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"net/http"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"

	"github.com/sujanto-gaws/kopiochi/internal/module"
	"github.com/sujanto-gaws/kopiochi/internal/platform/secret"
	"github.com/sujanto-gaws/kopiochi/modules/notification"
)

// refusingConnector is a database handle that never dials.
//
// database/sql connects lazily, so sql.OpenDB over this yields a real, non-nil
// *bun.DB that satisfies every construction path without a server — and fails
// loudly if anything actually queries. That is exactly the shape these tests
// want: New must build its repositories, and nothing must touch the database
// before the first poll, which none of these tests waits for.
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

// enabledConfig is a valid enabled module whose dispatcher will not poll during
// a test: the first tick is a whole PollInterval away, and every test here
// closes the module long before that.
func enabledConfig() notification.Config {
	cfg := validConfig()
	cfg.Dispatcher.PollInterval = time.Hour
	cfg.Dispatcher.StalledAfter = time.Hour
	cfg.Dispatcher.DrainTimeout = 5 * time.Second
	return cfg
}

func deps(t *testing.T, db bun.IDB) module.Deps {
	t.Helper()
	return module.Deps{DB: db, Logger: zerolog.Nop()}
}

func TestNewDisabledBuildsARoutelessModuleThatStartsNothing(t *testing.T) {
	before := runtime.NumGoroutine()

	cfg := validConfig()
	cfg.Enabled = false

	// No database at all: a disabled module must not need one, which is what
	// makes "switch it off" a usable answer for a deployment that has none.
	m, err := notification.New(deps(t, nil), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if m.Name != notification.Name {
		t.Errorf("Name = %q, want %q", m.Name, notification.Name)
	}

	// Routes is non-nil and mounts nothing. httpx.Mount calls it
	// unconditionally, so a nil here would panic the router at boot.
	if m.Routes == nil {
		t.Fatal("Routes is nil; httpx.Mount does not guard it")
	}
	r := chi.NewRouter()
	m.Routes(r)
	if len(r.Routes()) != 0 {
		t.Errorf("a disabled module mounted routes: %v", r.Routes())
	}

	if m.Close == nil {
		t.Fatal("Close is nil; it must be safe to call whatever the configuration")
	}
	if err := m.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if err := m.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}

	if got := runtime.NumGoroutine(); got > before {
		t.Errorf("a disabled module started %d goroutine(s)", got-before)
	}
}

func TestNewEnabledStartsADispatcherThatCloseStops(t *testing.T) {
	// Built before the baseline is taken: database/sql starts its own
	// connection-opener goroutine inside OpenDB, and counting it as the
	// dispatcher's would make this test lie in both directions.
	db := lazyDB(t)

	before := runtime.NumGoroutine()

	m, err := notification.New(deps(t, db), enabledConfig())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if m.Close == nil {
		t.Fatal("an enabled module must expose the Close that stops its dispatcher")
	}

	// Started: workers + sweeper + the goroutine that waits on them.
	if got := runtime.NumGoroutine(); got <= before {
		t.Fatalf("no dispatcher goroutines started: %d, was %d", got, before)
	}

	if err := m.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > before {
		if time.Now().After(deadline) {
			t.Fatalf("Close left %d goroutine(s) running", runtime.NumGoroutine()-before)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// An enabled module with no database is a wiring mistake, and the honest place
// to say so is boot. The alternative builds cleanly and panics inside a
// background goroutine at the first tick, minutes later, in a log nobody is
// reading.
func TestNewEnabledRefusesToBuildWithoutADatabase(t *testing.T) {
	t.Parallel()

	m, err := notification.New(deps(t, nil), enabledConfig())
	if err == nil {
		t.Fatalf("New built an enabled module with no database: %+v", m)
	}
	if !strings.Contains(err.Error(), "database is required") {
		t.Errorf("unhelpful error: %v", err)
	}
}

// An enabled module mounts the read model, and it mounts it behind the
// middleware the composition root supplied. modules/notification/transport
// tests the handlers; this is the seam — that New wires them at all, onto the
// router the host hands over.
func TestNewEnabledMountsTheReadModelRoutes(t *testing.T) {
	m, err := notification.New(deps(t, lazyDB(t)), enabledConfig())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = m.Close() })

	r := chi.NewRouter()
	r.Route("/api/v1", m.Routes)

	var got []string
	if err := chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		got = append(got, method+" "+route)
		return nil
	}); err != nil {
		t.Fatalf("walk: %v", err)
	}
	sort.Strings(got)

	want := []string{
		"GET /api/v1/notifications",
		"GET /api/v1/notifications/preferences",
		"POST /api/v1/notifications/read-all",
		"POST /api/v1/notifications/{id}/read",
		"PUT /api/v1/notifications/preferences",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("routes are\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// Fail closed, the way user.New does. A module that builds without a middleware
// is a module that serves one user's mailbox to whoever asks — and it would
// build cleanly, boot cleanly, and be discovered by a caller rather than by a
// test.
func TestNewEnabledRefusesToBuildWithoutAnAuthMiddleware(t *testing.T) {
	t.Parallel()

	cfg := enabledConfig()
	cfg.Auth = nil

	m, err := notification.New(deps(t, lazyDB(t)), cfg)
	if err == nil {
		t.Fatalf("New built an enabled module with no auth middleware: %+v", m)
	}
	if !strings.Contains(err.Error(), "auth middleware is required") {
		t.Errorf("error does not name the missing dependency: %v", err)
	}
}

func TestNewRejectsAnInvalidConfig(t *testing.T) {
	t.Parallel()

	cfg := enabledConfig()
	cfg.Dispatcher.Workers = 0

	m, err := notification.New(deps(t, lazyDB(t)), cfg)
	if err == nil {
		t.Fatalf("New accepted a config with no workers: %+v", m)
	}
	if !strings.Contains(err.Error(), "workers must be positive") {
		t.Errorf("error does not name the setting: %v", err)
	}
}

// The log sender is wired from configuration, and enabling it must not change
// anything else about how the module builds.
func TestNewWiresTheLogSenderWhenItIsAskedFor(t *testing.T) {
	cfg := enabledConfig()
	cfg.LogSender = notification.LogSenderConfig{Enabled: true, Channel: "email"}

	m, err := notification.New(deps(t, lazyDB(t)), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = m.Close() })

	// Two senders for one channel is refused by application.NewService, so a
	// module that builds at all is a module whose routing table is coherent.
	// The collision cases are config_test's; this asserts the wiring runs.
	if m.Name != notification.Name {
		t.Errorf("Name = %q, want %q", m.Name, notification.Name)
	}
}

// enabledEmailConfig points at a port nothing listens on, deliberately. See
// TestModuleRegistersTheEmailSender.
func enabledEmailConfig(t *testing.T, port int) notification.Config {
	t.Helper()

	cfg := enabledConfig()
	cfg.Email = notification.EmailConfig{
		Enabled:  true,
		SMTPHost: "127.0.0.1",
		SMTPPort: port,
		From:     "no-reply@example.test",
		Password: secret.String("a-real-credential"),
		Timeout:  time.Second,
	}
	cfg.EmailAddressResolver = stubResolver{}
	return cfg
}

func TestNewBuildsTheEmailSenderWhenEmailIsConfigured(t *testing.T) {
	t.Parallel()

	m, err := notification.New(deps(t, lazyDB(t)), enabledEmailConfig(t, 587))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = m.Close() })
}

// The SMTP sender parses the From address at construction, so an address that
// no mail server will accept fails the boot rather than every delivery.
//
// The value here passes Config.Validate's "it contains an @" check on purpose:
// that check exists to catch a display name pasted into the wrong field, and
// this asserts the stricter parse behind it rather than re-asserting the same
// rule twice.
func TestNewRefusesAnUnparsableFromAddress(t *testing.T) {
	t.Parallel()

	cfg := enabledEmailConfig(t, 587)
	cfg.Email.From = "Ada <ada@example.test"

	m, err := notification.New(deps(t, lazyDB(t)), cfg)
	if err == nil {
		t.Fatalf("New accepted an unparsable from address: %+v", m)
	}
	if !strings.Contains(err.Error(), "from address") {
		t.Errorf("error does not name the setting: %v", err)
	}
}

func TestNewRefusesEmailWithoutAnAddressResolver(t *testing.T) {
	t.Parallel()

	cfg := enabledEmailConfig(t, 587)
	cfg.EmailAddressResolver = nil

	m, err := notification.New(deps(t, lazyDB(t)), cfg)
	if err == nil {
		t.Fatalf("New accepted email with no resolver: %+v", m)
	}
	if !strings.Contains(err.Error(), "address resolver is required") {
		t.Errorf("error does not name the missing dependency: %v", err)
	}
}
