package httpx

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

// panicking returns a handler that panics with v.
func panicking(v any) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic(v)
	})
}

func TestRecovery_PanicBecomesProblemJSON500(t *testing.T) {
	t.Parallel()

	h := Recovery(zerolog.Nop())(panicking("boom"))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/things", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if ct := rec.Header().Get("Content-Type"); ct != ProblemContentType {
		t.Errorf("Content-Type = %q, want %q", ct, ProblemContentType)
	}

	var p Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("body is not JSON: %v (body=%q)", err, rec.Body.String())
	}
	if p.Status != http.StatusInternalServerError {
		t.Errorf("problem.status = %d, want 500", p.Status)
	}
	if p.Instance != "/api/v1/things" {
		t.Errorf("problem.instance = %q, want %q", p.Instance, "/api/v1/things")
	}
}

// TestRecovery_DoesNotLeakThePanicToTheClient is the reason this middleware
// exists rather than a bare 500: a panic message routinely contains a query, a
// file path, or a dumped struct, and chi's Recoverer would have printed it to
// stderr where an operator might paste it anywhere. It must not reach the
// response under any circumstances.
func TestRecovery_DoesNotLeakThePanicToTheClient(t *testing.T) {
	t.Parallel()

	const sensitive = "dsn=postgres://admin:hunter2@db.internal/prod"

	var logged bytes.Buffer
	h := Recovery(zerolog.New(&logged))(panicking(sensitive))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if strings.Contains(rec.Body.String(), sensitive) {
		t.Errorf("panic value leaked into the response body: %q", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "hunter2") {
		t.Errorf("panic value leaked into the response body: %q", rec.Body.String())
	}
	// The operator still needs it, so it must be in the log.
	if !strings.Contains(logged.String(), sensitive) {
		t.Errorf("panic value missing from the log; operator has nothing to debug with:\n%s", logged.String())
	}
	// And the log must carry a stack, otherwise the line says a panic
	// happened without saying where.
	if !strings.Contains(logged.String(), "stack") {
		t.Errorf("log line carries no stack trace:\n%s", logged.String())
	}
}

// TestRecovery_LeavesAlreadyCommittedResponsesAlone guards the case that makes
// naive recovery middleware corrupt output: the handler streamed part of a
// response and then panicked. The status is already on the wire, so writing a
// 500 on top produces "superfluous WriteHeader" and a body that is half one
// response and half another.
func TestRecovery_LeavesAlreadyCommittedResponsesAlone(t *testing.T) {
	t.Parallel()

	h := Recovery(zerolog.Nop())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"partial":`))
		panic("failed halfway through")
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (already committed; must not be rewritten)", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != `{"partial":` {
		t.Errorf("body = %q, want the partial write untouched", got)
	}
}

// TestRecovery_ImplicitWriteCountsAsCommitted covers the same rule via the
// other path: net/http commits a 200 on the first Write, with no WriteHeader
// call for the wrapper to observe.
func TestRecovery_ImplicitWriteCountsAsCommitted(t *testing.T) {
	t.Parallel()

	h := Recovery(zerolog.Nop())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("partial"))
		panic("after an implicit 200")
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if got := rec.Body.String(); got != "partial" {
		t.Errorf("body = %q, want %q — a problem document was appended to a committed response", got, "partial")
	}
}

// TestRecovery_RepanicsOnErrAbortHandler: http.ErrAbortHandler is the
// documented way for a handler to abandon a response deliberately
// (httputil.ReverseProxy uses it). net/http suppresses its own logging for it;
// swallowing it here would convert an intentional abort into a spurious 500
// and an error-level log line.
func TestRecovery_RepanicsOnErrAbortHandler(t *testing.T) {
	t.Parallel()

	defer func() {
		rec := recover()
		if rec != http.ErrAbortHandler {
			t.Errorf("recovered %v, want it to propagate http.ErrAbortHandler", rec)
		}
	}()

	h := Recovery(zerolog.Nop())(panicking(http.ErrAbortHandler))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	t.Fatal("ServeHTTP returned; ErrAbortHandler was swallowed")
}

func TestRecovery_PassesNonPanickingResponsesThrough(t *testing.T) {
	t.Parallel()

	h := Recovery(zerolog.Nop())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("fine"))
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusTeapot || rec.Body.String() != "fine" {
		t.Errorf("got %d %q, want 418 %q", rec.Code, rec.Body.String(), "fine")
	}
}
