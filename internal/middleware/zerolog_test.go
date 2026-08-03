package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
)

// lastLine decodes the final JSON log record written to buf.
func lastLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()

	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	if len(lines) == 0 || len(lines[0]) == 0 {
		t.Fatalf("no log output was produced")
	}

	var rec map[string]any
	if err := json.Unmarshal(lines[len(lines)-1], &rec); err != nil {
		t.Fatalf("log line is not JSON: %v (line=%q)", err, lines[len(lines)-1])
	}
	return rec
}

func serve(h http.Handler, req *http.Request) {
	h.ServeHTTP(httptest.NewRecorder(), req)
}

// TestRequestLogger_PutsALoggerInTheContext is the change that makes
// correlation possible at all: before it, a repository error and the request
// that caused it were two unrelated log lines.
func TestRequestLogger_PutsALoggerInTheContext(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	base := zerolog.New(&buf)

	h := chimw.RequestID(RequestLogger(base)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This is what any layer below the handler would do.
		zerolog.Ctx(r.Context()).Info().Msg("from deep inside")
	})))

	serve(h, httptest.NewRequest(http.MethodGet, "/deep", nil))

	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	if len(lines) != 2 {
		t.Fatalf("got %d log lines, want 2 (the handler's and the access log)", len(lines))
	}

	var inner map[string]any
	if err := json.Unmarshal(lines[0], &inner); err != nil {
		t.Fatalf("inner line is not JSON: %v", err)
	}
	if inner["message"] != "from deep inside" {
		t.Fatalf("first line = %v, want the handler's own message", inner)
	}
	if inner["request_id"] == nil || inner["request_id"] == "" {
		t.Error("a line emitted from inside the handler carries no request_id")
	}
}

func TestRequestLogger_LogsTheAccessFields(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	h := chimw.RequestID(RequestLogger(zerolog.New(&buf))(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte("hello"))
		})))

	serve(h, httptest.NewRequest(http.MethodPost, "/api/v1/things", nil))

	rec := lastLine(t, &buf)
	for _, field := range []string{"method", "path", "status", "bytes_written", "duration_ms", "request_id"} {
		if _, ok := rec[field]; !ok {
			t.Errorf("access log is missing %q: %v", field, rec)
		}
	}
	if rec["method"] != http.MethodPost {
		t.Errorf("method = %v, want POST", rec["method"])
	}
	if rec["path"] != "/api/v1/things" {
		t.Errorf("path = %v, want /api/v1/things", rec["path"])
	}
	if rec["status"] != float64(http.StatusCreated) {
		t.Errorf("status = %v, want 201", rec["status"])
	}
	if rec["bytes_written"] != float64(len("hello")) {
		t.Errorf("bytes_written = %v, want 5", rec["bytes_written"])
	}
}

// TestRequestLogger_LevelFollowsStatus is what makes "alert on error-level
// logs" a usable rule: a client sending a malformed body must not page anyone,
// and a server fault must.
func TestRequestLogger_LevelFollowsStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		status    int
		wantLevel string
	}{
		{"success is info", http.StatusOK, "info"},
		{"redirect is info", http.StatusMovedPermanently, "info"},
		{"client error is warn", http.StatusBadRequest, "warn"},
		{"not found is warn", http.StatusNotFound, "warn"},
		{"server error is error", http.StatusInternalServerError, "error"},
		{"bad gateway is error", http.StatusBadGateway, "error"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			h := RequestLogger(zerolog.New(&buf))(http.HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(tc.status)
				}))

			serve(h, httptest.NewRequest(http.MethodGet, "/", nil))

			if got := lastLine(t, &buf)["level"]; got != tc.wantLevel {
				t.Errorf("status %d logged at level %v, want %q", tc.status, got, tc.wantLevel)
			}
		})
	}
}

// TestRequestLogger_ReportsTheStatusThatWentOnTheWire: a handler that returns
// without writing sends 200, but chi's wrapper reports 0. Logging 0 makes the
// success case look like a broken response in every dashboard.
func TestRequestLogger_ReportsImplicit200(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	h := RequestLogger(zerolog.New(&buf))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	serve(h, httptest.NewRequest(http.MethodGet, "/", nil))

	if got := lastLine(t, &buf)["status"]; got != float64(http.StatusOK) {
		t.Errorf("status = %v, want 200", got)
	}
}

// TestRequestLogger_BindsTheResolvedClientIP: it must be the address RealIP
// resolved, never a raw header, or the access log becomes attacker-controlled.
func TestRequestLogger_BindsTheResolvedClientIP(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	h := RealIP(nil)(RequestLogger(zerolog.New(&buf))(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.9:44444"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	serve(h, req)

	got := lastLine(t, &buf)["client_ip"]
	if got != "203.0.113.9" {
		t.Errorf("client_ip = %v, want the socket address 203.0.113.9 (no trusted proxies configured)", got)
	}
}
