package httpx

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	"github.com/sujanto-gaws/kopiochi/internal/config"
)

func testServerConfig(t *testing.T, port int) config.Server {
	t.Helper()
	return config.Server{
		Host:              "127.0.0.1",
		Port:              port,
		ReadTimeout:       5 * time.Second,
		ReadHeaderTimeout: 2 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       10 * time.Second,
		ShutdownTimeout:   5 * time.Second,
		RequestTimeout:    3 * time.Second,
	}
}

// TestServe_ReturnsListenErrorInsteadOfExiting is the regression test for
// problems 2 and 3 in lifecycle-and-shutdown.md. The implementation this
// replaces called log.Fatal inside the serving goroutine, so a port already
// in use called os.Exit(1): no deferred function ran, the pool was never
// drained, no shutdown func fired, and main never learned why. Run also
// returned nothing, so RunE returned nil and the process exited 0.
//
// A test cannot observe os.Exit from inside the process — which is precisely
// what made the old behaviour untestable, and is half the reason it was
// wrong. What this asserts is the property that replaced it: the error
// reaches the caller, and it does so promptly rather than blocking until a
// signal arrives.
func TestServe_ReturnsListenErrorInsteadOfExiting(t *testing.T) {
	// Occupy a port, then ask the server to bind the same one.
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer func() { _ = occupied.Close() }()

	_, portStr, err := net.SplitHostPort(occupied.Addr().String())
	if err != nil {
		t.Fatalf("split addr: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}

	srv := NewServer(testServerConfig(t, port), chi.NewRouter(), zerolog.Nop())

	// A context that is NOT cancelled: if Serve returned nil here it would
	// mean the failure was swallowed and the caller told everything is fine.
	done := make(chan error, 1)
	go func() { done <- srv.Serve(context.Background()) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Serve() = nil on a port already in use; the failure must reach the caller")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve() did not return on a bind failure")
	}
}

// TestServe_ReturnsNilOnContextCancellation is the control: a signal-driven
// stop is a clean stop, and must not be reported as a failure — otherwise
// every normal shutdown would give the process a non-zero exit code.
func TestServe_ReturnsNilOnContextCancellation(t *testing.T) {
	srv := NewServer(testServerConfig(t, 0), chi.NewRouter(), zerolog.Nop())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx) }()

	// Give the listener a moment to come up before cancelling, so this
	// exercises the ctx.Done() branch rather than racing the bind.
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve() = %v on a cancelled context, want nil (a signal stop is not a failure)", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve() did not return after its context was cancelled")
	}

	if err := srv.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown() = %v", err)
	}
}

// TestReadyz_ReportsNotReadyWhileDraining is the readiness half of the
// lifecycle work. Once shutdown begins, the process must stop advertising
// itself immediately, so the load balancer removes it *while* in-flight
// requests finish rather than after. Reporting ready throughout the drain
// means new requests keep arriving at a server that is closing its listener,
// and they fail.
func TestReadyz_ReportsNotReadyWhileDraining(t *testing.T) {
	srv := NewServer(testServerConfig(t, 0), chi.NewRouter(), zerolog.Nop())

	r := chi.NewRouter()
	r.Get("/readyz", readyzHandler(okPinger{}, srv.Draining))

	// Before shutdown: ready.
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/readyz before shutdown = %d, want %d", rec.Code, http.StatusOK)
	}

	if err := srv.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() = %v", err)
	}

	// After shutdown has begun: not ready, even though the database is still
	// perfectly reachable. Draining is checked first for exactly this reason.
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("/readyz while draining = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if body := rec.Body.String(); !strings.Contains(body, "draining") {
		t.Errorf("/readyz body = %s, want it to report the draining state", body)
	}
}

// TestReadyz_NilDrainingIsIgnored keeps the field optional: callers with no
// server to ask (tests, tools) pass nil and get plain dependency-based
// readiness.
func TestReadyz_NilDrainingIsIgnored(t *testing.T) {
	rec := httptest.NewRecorder()
	readyzHandler(okPinger{}, nil)(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("/readyz with a nil draining func = %d, want %d", rec.Code, http.StatusOK)
	}
}

type okPinger struct{}

func (okPinger) Ping(context.Context) error { return nil }
