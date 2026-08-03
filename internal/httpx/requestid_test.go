package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"

	"github.com/sujanto-gaws/kopiochi/internal/config"
)

// TestEchoRequestID_ReturnsTheID exists because chi's RequestID only puts the
// id in the request *context*. Without echoing it, the id reaches our logs and
// our problem+json bodies, but a client that got a successful response has
// nothing to quote when they later report that it was wrong.
func TestEchoRequestID_ReturnsTheID(t *testing.T) {
	t.Parallel()

	var inContext string
	h := chimw.RequestID(EchoRequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inContext = chimw.GetReqID(r.Context())
	})))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	got := rec.Header().Get(RequestIDHeader)
	if got == "" {
		t.Fatal("no X-Request-Id on the response")
	}
	if got != inContext {
		t.Errorf("header %q does not match the context id %q", got, inContext)
	}
}

// TestEchoRequestID_WithoutAGeneratorSetsNothing: an empty header is worse
// than none, because a client would quote an empty string as its correlation
// id.
func TestEchoRequestID_WithoutAGeneratorSetsNothing(t *testing.T) {
	t.Parallel()

	h := EchoRequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if _, ok := rec.Header()[RequestIDHeader]; ok {
		t.Errorf("%s was set with no id available", RequestIDHeader)
	}
}

// TestEchoRequestID_OnEveryResponse: the header must be present on errors too,
// which is where a user is most likely to want it.
func TestEchoRequestID_OnEveryResponse(t *testing.T) {
	t.Parallel()

	r, closeRouter, err := NewRouter(routerTestConfig(), config.Security{}, zerolog.Nop(), nil)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}
	t.Cleanup(func() { _ = closeRouter() })
	r.Get("/ok", func(w http.ResponseWriter, req *http.Request) {})
	r.Get("/boom", func(w http.ResponseWriter, req *http.Request) { panic("boom") })

	for _, path := range []string{"/ok", "/boom", "/no-such-route"} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

			if rec.Header().Get(RequestIDHeader) == "" {
				t.Errorf("no %s on the response to %s (status %d)", RequestIDHeader, path, rec.Code)
			}
		})
	}
}

// TestNewAdminServer_UsesItsOwnAddressAndTimeouts: the metrics listener must
// bind what config.Metrics says, not the API's address — Config.Validate
// rejects them being equal, and this is the other half of that guarantee.
func TestNewAdminServer_UsesItsOwnAddressAndTimeouts(t *testing.T) {
	t.Parallel()

	srv := NewAdminServer(config.Metrics{Addr: "127.0.0.1:19090", Path: "/metrics"},
		http.NewServeMux(), zerolog.Nop())

	if srv.httpServer.Addr != "127.0.0.1:19090" {
		t.Errorf("Addr = %q, want the configured metrics address", srv.httpServer.Addr)
	}
	// Fixed rather than configurable: the only client is a scraper, and a
	// value tuned for API traffic is not the right value here.
	if srv.httpServer.ReadHeaderTimeout <= 0 {
		t.Error("ReadHeaderTimeout is unset; the admin listener is slowloris-exposed")
	}
	if srv.httpServer.WriteTimeout < 10*time.Second {
		t.Errorf("WriteTimeout = %v, too short for a large scrape", srv.httpServer.WriteTimeout)
	}
}

// TestNewAdminServer_DrainsLikeAnyOtherServer: it goes on the same lifecycle
// stack, so Shutdown must work and flip Draining.
func TestNewAdminServer_DrainsLikeAnyOtherServer(t *testing.T) {
	t.Parallel()

	srv := NewAdminServer(config.Metrics{Addr: "127.0.0.1:0"}, http.NewServeMux(), zerolog.Nop())

	if srv.Draining() {
		t.Error("Draining() is true before shutdown")
	}
	if err := srv.Shutdown(t.Context()); err != nil {
		t.Errorf("Shutdown() = %v, want nil", err)
	}
	if !srv.Draining() {
		t.Error("Draining() is false after Shutdown")
	}
}
