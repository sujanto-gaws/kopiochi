//go:build !swagger

package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestSwaggerDisabled_ExplainsItself covers the choice to register a handler
// on /swagger/* even when the UI is not built in.
//
// Falling through to the 404 handler would answer "No route matches this
// path", and an operator who knows the endpoint used to exist reads that as a
// routing bug — then goes looking for a mounting mistake that is not there.
// Saying "this binary was built without it" turns a debugging session into a
// build flag.
func TestSwaggerDisabled_ExplainsItself(t *testing.T) {
	t.Parallel()

	r := newTestMux()
	Mount(r, nil, Deps{})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/swagger/index.html", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != ProblemContentType {
		t.Errorf("Content-Type = %q, want %q", ct, ProblemContentType)
	}

	var p Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("body is not JSON: %v (%q)", err, rec.Body.String())
	}
	if p.Type != "swagger_not_built" {
		t.Errorf("problem.type = %q, want swagger_not_built — a bare 404 reads as a routing bug", p.Type)
	}
	if p.Detail == "" {
		t.Error("the problem carries no detail, so it does not say how to get the UI back")
	}
}
