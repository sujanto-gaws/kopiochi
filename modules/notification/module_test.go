package notification_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"

	"github.com/sujanto-gaws/kopiochi/internal/module"
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
