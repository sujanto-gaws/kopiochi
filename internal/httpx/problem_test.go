package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

func TestWriteProblem_ShapeAndHeaders(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/widgets/7", nil)

	WriteProblem(rec, r, http.StatusConflict, "already_exists", "Conflict", "A widget with that name exists.")

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
	if ct := rec.Header().Get("Content-Type"); ct != ProblemContentType {
		t.Errorf("Content-Type = %q, want %q", ct, ProblemContentType)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want %q", got, "nosniff")
	}

	var p Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if p.Type != "already_exists" || p.Title != "Conflict" || p.Status != http.StatusConflict {
		t.Errorf("problem = %+v, want type/title/status to round-trip", p)
	}
	if p.Instance != "/api/v1/widgets/7" {
		t.Errorf("instance = %q, want the request path", p.Instance)
	}
}

// TestWriteProblem_CarriesTheRequestID is the whole point of the RequestID
// extension member: a user quoting the id from a failed response must be able
// to find the exact log line, without anyone guessing from timestamps.
func TestWriteProblem_CarriesTheRequestID(t *testing.T) {
	t.Parallel()

	var body []byte
	h := chimw.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := httptest.NewRecorder()
		WriteProblem(rec, r, http.StatusBadRequest, "bad_request", "Bad Request", "")
		body = rec.Body.Bytes()
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	var p Problem
	if err := json.Unmarshal(body, &p); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if p.RequestID == "" {
		t.Error("request_id is empty; a reported error cannot be correlated with its log line")
	}
}

// TestRouterErrors_AreProblemJSON exercises 404 and 405 through a real chi
// router, because chi calls these handlers itself — a unit test of the
// handlers alone would not prove they are actually wired.
func TestRouterErrors_AreProblemJSON(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	r.NotFound(NotFound())
	r.MethodNotAllowed(MethodNotAllowed(r))
	r.Get("/exists", func(w http.ResponseWriter, r *http.Request) {})

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantType   string
	}{
		{"no route", http.MethodGet, "/nope", http.StatusNotFound, "not_found"},
		{"wrong method", http.MethodPost, "/exists", http.StatusMethodNotAllowed, "method_not_allowed"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if ct := rec.Header().Get("Content-Type"); ct != ProblemContentType {
				t.Fatalf("Content-Type = %q, want %q (chi's default is text/plain)", ct, ProblemContentType)
			}

			var p Problem
			if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
				t.Fatalf("body is not JSON: %v (body=%q)", err, rec.Body.String())
			}
			if p.Type != tc.wantType {
				t.Errorf("problem.type = %q, want %q", p.Type, tc.wantType)
			}
		})
	}
}

// TestMethodNotAllowed_SetsAllow covers a trap this test found the hard way.
//
// chi's Mux.MethodNotAllowedHandler returns a registered custom handler
// *instead of* chi's default, and only that default sets Allow. So simply
// registering a problem+json 405 handler silently dropped the header RFC 9110
// requires a 405 to carry — the one thing telling a client what to try
// instead. MethodNotAllowed therefore recomputes it from the routing tree.
func TestMethodNotAllowed_SetsAllow(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	r.MethodNotAllowed(MethodNotAllowed(r))
	r.Get("/exists", func(w http.ResponseWriter, r *http.Request) {})
	r.Put("/exists", func(w http.ResponseWriter, r *http.Request) {})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/exists", nil))

	got := rec.Header().Get("Allow")
	if got == "" {
		t.Fatal("Allow header was dropped; a 405 without it tells the client nothing")
	}
	for _, want := range []string{http.MethodGet, http.MethodPut} {
		if !strings.Contains(got, want) {
			t.Errorf("Allow = %q, want it to list %s", got, want)
		}
	}
	if strings.Contains(got, http.MethodDelete) {
		t.Errorf("Allow = %q, must not list the method that was just rejected", got)
	}
}

// TestAllowedMethods_UnknownPathIsEmpty: a path with no routes at all must
// produce no Allow header rather than an empty one, which would claim the
// resource accepts nothing.
func TestAllowedMethods_UnknownPathIsEmpty(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	r.Get("/exists", func(w http.ResponseWriter, r *http.Request) {})

	if got := allowedMethods(r, "/nothing-here"); len(got) != 0 {
		t.Errorf("allowedMethods(unknown path) = %v, want empty", got)
	}
}
