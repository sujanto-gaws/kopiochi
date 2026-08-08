package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// callUnauthorized invokes Unauthorized for a request at path and returns the
// recorded response.
//
// withRequestID decides whether chi's RequestID middleware runs first, which
// is the difference between the two harnesses this package is used from: a
// bare chi.NewRouter() has no request id, NewRouter mounts chimw.RequestID and
// does. request_id is omitempty, so the body genuinely differs between them
// and both are part of the contract.
func callUnauthorized(t *testing.T, method, path string, withRequestID bool) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)

	var h http.Handler = http.HandlerFunc(Unauthorized)
	if withRequestID {
		h = chimw.RequestID(h)
	}
	h.ServeHTTP(rec, req)

	return rec
}

// TestUnauthorized_CanonicalShape pins every header and every body member of
// the canonical 401, in both harnesses.
func TestUnauthorized_CanonicalShape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		method        string
		path          string
		withRequestID bool
		wantMembers   []string
	}{
		{
			name:          "without a request id",
			method:        http.MethodGet,
			path:          "/api/v1/users/42",
			withRequestID: false,
			wantMembers:   []string{"detail", "instance", "status", "title", "type"},
		},
		{
			name:          "with a request id",
			method:        http.MethodPost,
			path:          "/api/v1/auth/logout",
			withRequestID: true,
			wantMembers:   []string{"detail", "instance", "request_id", "status", "title", "type"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := callUnauthorized(t, tc.method, tc.path, tc.withRequestID)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
			}
			if got := rec.Header().Get("Content-Type"); got != ProblemContentType {
				t.Errorf("Content-Type = %q, want %q", got, ProblemContentType)
			}
			// RFC 9110 requires a 401 to carry a challenge; WriteProblem does
			// not set one, so Unauthorized must, before the body is written.
			if got := rec.Header().Get("WWW-Authenticate"); got != `Bearer realm="api"` {
				t.Errorf("WWW-Authenticate = %q, want %q", got, `Bearer realm="api"`)
			}
			if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
				t.Errorf("X-Content-Type-Options = %q, want %q", got, "nosniff")
			}

			var p Problem
			if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
				t.Fatalf("body is not JSON: %v (body=%q)", err, rec.Body.String())
			}
			if p.Type != "about:blank" {
				t.Errorf("type = %q, want %q", p.Type, "about:blank")
			}
			if p.Title != "Unauthorized" {
				t.Errorf("title = %q, want %q", p.Title, "Unauthorized")
			}
			if p.Status != http.StatusUnauthorized {
				t.Errorf("body status = %d, want it to match the HTTP status %d", p.Status, http.StatusUnauthorized)
			}
			if p.Detail != "authentication required" {
				t.Errorf("detail = %q, want %q", p.Detail, "authentication required")
			}
			if p.Instance != tc.path {
				t.Errorf("instance = %q, want the request path %q", p.Instance, tc.path)
			}

			// The member set is part of the contract: request_id is omitempty,
			// so it must appear under the real router and vanish without it,
			// and nothing else may creep in.
			var raw map[string]json.RawMessage
			if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
				t.Fatalf("body is not a JSON object: %v", err)
			}
			got := make([]string, 0, len(raw))
			for k := range raw {
				got = append(got, k)
			}
			sort.Strings(got)

			if len(got) != len(tc.wantMembers) {
				t.Fatalf("body members = %v, want %v", got, tc.wantMembers)
			}
			for i, want := range tc.wantMembers {
				if got[i] != want {
					t.Fatalf("body members = %v, want %v", got, tc.wantMembers)
				}
			}
			if tc.withRequestID && p.RequestID == "" {
				t.Error("request_id is present but empty; a rejection could not be correlated with its log line")
			}
		})
	}
}

// TestUnauthorizedDetail_IsTheConstantVerbatim is the drift guard.
//
// Two assertions, and both are needed. The first pins the literal, so nobody
// can reword the constant and keep the tests green. The second pins that the
// response actually carries that constant rather than a copy that could be
// edited independently.
func TestUnauthorizedDetail_IsTheConstantVerbatim(t *testing.T) {
	t.Parallel()

	if unauthorizedDetail != "authentication required" {
		t.Fatalf("unauthorizedDetail = %q, want %q — the canonical detail is a documented contract, not an implementation detail",
			unauthorizedDetail, "authentication required")
	}

	rec := callUnauthorized(t, http.MethodGet, "/api/v1/users/42", false)

	var p Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if p.Detail != unauthorizedDetail {
		t.Errorf("detail = %q, want the package constant %q verbatim", p.Detail, unauthorizedDetail)
	}
}

// TestUnauthorized_DetailIsReasonInvariant guards the security property behind
// the constant.
//
// Unauthorized takes no reason argument, so today invariance holds by
// construction. This test makes it hold by contract instead: if someone later
// threads the rejection reason through — via a parameter, the request context,
// an Authorization header sniff, or the route — this fails. Distinguishing
// expired from malformed from wrong-class in the body tells an attacker which
// tokens are structurally valid; the reason belongs in the log, keyed by
// request id.
func TestUnauthorized_DetailIsReasonInvariant(t *testing.T) {
	t.Parallel()

	// One request per rejection reason the identity middleware distinguishes
	// internally (§8.1's five golden cases), phrased as the request that
	// triggers it.
	reasons := []struct {
		name   string
		mutate func(*http.Request)
		method string
		path   string
	}{
		{name: "missing header", method: http.MethodPost, path: "/api/v1/auth/logout", mutate: func(*http.Request) {}},
		{name: "malformed bearer", method: http.MethodPost, path: "/api/v1/auth/logout", mutate: func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer not-a-jwt")
		}},
		{name: "expired token", method: http.MethodPost, path: "/api/v1/auth/logout", mutate: func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer expired.looking.token")
		}},
		{name: "wrong algorithm", method: http.MethodPost, path: "/api/v1/auth/logout", mutate: func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer alg.none.token")
		}},
		{name: "wrong token class", method: http.MethodPost, path: "/api/v1/auth/logout", mutate: func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer refresh.class.token")
		}},
		{name: "empty authorization", method: http.MethodGet, path: "/api/v1/auth/logout", mutate: func(r *http.Request) {
			r.Header.Set("Authorization", "")
		}},
	}

	for _, tc := range reasons {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, nil)
		tc.mutate(req)

		Unauthorized(rec, req)

		var p Problem
		if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
			t.Fatalf("%s: body is not JSON: %v", tc.name, err)
		}
		if p.Detail != unauthorizedDetail {
			t.Errorf("%s: detail = %q, want %q — every rejection reason must be indistinguishable in the body",
				tc.name, p.Detail, unauthorizedDetail)
		}
		if got := rec.Header().Get("WWW-Authenticate"); got != unauthorizedChallenge {
			t.Errorf("%s: WWW-Authenticate = %q, want %q", tc.name, got, unauthorizedChallenge)
		}
	}
}

// TestUnauthorized_WritesTheStatusOnce catches a double WriteHeader, which the
// net/http server logs as a warning and which would mean Unauthorized and
// WriteProblem are both committing the status line.
func TestUnauthorized_WritesTheStatusOnce(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	counter := &countingWriter{ResponseWriter: rec}

	Unauthorized(counter, httptest.NewRequest(http.MethodGet, "/api/v1/users/42", nil))

	if counter.writeHeaderCalls != 1 {
		t.Errorf("WriteHeader called %d times, want exactly 1", counter.writeHeaderCalls)
	}
}

type countingWriter struct {
	http.ResponseWriter
	writeHeaderCalls int
}

func (c *countingWriter) WriteHeader(status int) {
	c.writeHeaderCalls++
	c.ResponseWriter.WriteHeader(status)
}
