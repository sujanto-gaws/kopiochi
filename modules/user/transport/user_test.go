package transport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/sujanto-gaws/kopiochi/internal/httpx"
	"github.com/sujanto-gaws/kopiochi/internal/testsupport"
	domain "github.com/sujanto-gaws/kopiochi/modules/user/domain"
)

// What these tests pin is the property module_test.go can only assert half of:
// it proves user.New refuses to build without a middleware, and these prove the
// middleware it was given is on the path of every route Routes mounts.
//
// Since E16 they pin a second, larger property: the handlers act on the
// Principal and on nothing else. Nothing here mints a token or generates a key
// — testsupport.FakeAuth supplies the caller directly, because the thing under
// test is the route table and the id it acts on, not RS256.

var testSubject = uuid.MustParse("3f1b8a54-2c9e-4d77-9a1e-2b6c0d5e8f41")

// fakeService records the caller it was handed. That recording is the whole
// point: the interface no longer has an argument that could name somebody
// else's profile, so what is left to verify is that the id which arrives is the
// authenticated one.
type fakeService struct {
	calls      int
	sawCaller  uuid.UUID
	ensureResp *domain.UserResponse
	ensureErr  error
	getResp    *domain.UserResponse
	getErr     error
}

func (f *fakeService) EnsureOwnProfile(_ context.Context, caller uuid.UUID) (*domain.UserResponse, error) {
	f.calls++
	f.sawCaller = caller
	return f.ensureResp, f.ensureErr
}

func (f *fakeService) GetOwnProfile(_ context.Context, caller uuid.UUID) (*domain.UserResponse, error) {
	f.calls++
	f.sawCaller = caller
	return f.getResp, f.getErr
}

// router mounts the real route table behind a middleware that authenticates
// every request as subject.
func router(svc UserService, subject string) http.Handler {
	r := chi.NewRouter()
	h := NewUserHandler(svc, testsupport.FakeAuth(subject))
	r.Route("/api/v1", h.Routes)
	return r
}

func do(t *testing.T, h http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

// TestHandlersActOnThePrincipalAndNothingElse is E16, closed.
//
// The service records the id it was given. It must be the authenticated
// subject, because that is the only id the handler has: there is no path
// parameter and no body to take one from.
func TestHandlersActOnThePrincipalAndNothingElse(t *testing.T) {
	for _, tc := range []struct {
		name   string
		method string
	}{
		{"get", http.MethodGet},
		{"ensure", http.MethodPost},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeService{
				getResp:    &domain.UserResponse{ID: testSubject},
				ensureResp: &domain.UserResponse{ID: testSubject},
			}

			rec := do(t, router(svc, testSubject.String()), tc.method, "/api/v1/users/me")
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
			}
			if svc.sawCaller != testSubject {
				t.Errorf("service was given %v, want the authenticated subject %v",
					svc.sawCaller, testSubject)
			}
		})
	}
}

// TestNoRouteTakesAnID is the structural half of the fix. E16 was not a missing
// ownership check — it was an addressable id with nothing to compare it
// against. A route table with no id in it cannot be asked for somebody else's
// row, so these 404s are the vulnerability's absence, not a guard's success.
func TestNoRouteTakesAnID(t *testing.T) {
	other := uuid.New().String()

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/users/" + other},
		{http.MethodPut, "/api/v1/users/" + other},
		{http.MethodDelete, "/api/v1/users/" + other},
		{http.MethodPost, "/api/v1/users"},
		{http.MethodPut, "/api/v1/users/me"},
		{http.MethodDelete, "/api/v1/users/me"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			svc := &fakeService{}

			rec := do(t, router(svc, testSubject.String()), tc.method, tc.path)
			if rec.Code == http.StatusOK {
				t.Fatalf("status = 200: %s %s is routed, and it should not be", tc.method, tc.path)
			}
			if svc.calls != 0 {
				t.Errorf("the application layer was reached %d times by an unrouted request", svc.calls)
			}
		})
	}
}

// TestUnauthenticatedRequestsNeverReachTheService: the counter, not the status
// code, is the assertion. A 401 with the handler having already run is a
// different and worse thing than a 401 instead of it.
func TestUnauthenticatedRequestsNeverReachTheService(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		t.Run(method, func(t *testing.T) {
			svc := &fakeService{}
			r := chi.NewRouter()
			// A middleware that authenticates nobody, standing in for a
			// request with no credentials.
			h := NewUserHandler(svc, func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusUnauthorized)
				})
			})
			r.Route("/api/v1", h.Routes)

			rec := do(t, r, method, "/api/v1/users/me")
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			if svc.calls != 0 {
				t.Errorf("the service ran %d times for an unauthenticated request", svc.calls)
			}
		})
	}
}

// TestGetReportsNotFoundForACallerWithNoProfile: a caller who has not created
// one is told so, rather than handed a fabricated empty profile.
func TestGetReportsNotFoundForACallerWithNoProfile(t *testing.T) {
	svc := &fakeService{getErr: domain.ErrUserNotFound}

	rec := do(t, router(svc, testSubject.String()), http.MethodGet, "/api/v1/users/me")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// TestAMiddlewareThatSetsNoPrincipalIs401 covers caller()'s other rejection: the
// middleware ran, allowed the request through, and put no principal in the
// context at all.
//
// This is a misconfigured or half-written middleware rather than a hostile
// client, which is exactly why it is worth pinning. The failure mode if caller()
// did not check is not a 500 — it is uuid.Nil flowing into the service as though
// it were an authenticated subject, and a profile belonging to nobody being
// created or returned. That is the E16 class of bug arriving through the door
// E16 did not close.
//
// The existing tests could not reach this branch: FakeAuth always sets a
// principal, and the middleware in TestRoutesAreUnreachableWithoutAuth rejects
// before the handler runs. Only a middleware that says yes and supplies nothing
// gets here.
func TestAMiddlewareThatSetsNoPrincipalIs401(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		t.Run(method, func(t *testing.T) {
			svc := &fakeService{}

			r := chi.NewRouter()
			h := NewUserHandler(svc, func(next http.Handler) http.Handler {
				// Says yes, supplies nothing.
				return next
			})
			r.Route("/api/v1", h.Routes)

			rec := do(t, r, method, "/api/v1/users/me")
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			if svc.calls != 0 {
				t.Errorf("the service ran %d times with no principal in the context", svc.calls)
			}
		})
	}
}

// TestEveryFailureIsProblemJSON is E32: this module answered its 404 and its
// two 500s with {"error": "..."} under application/json, the only failures in
// the tree that were not problem+json.
//
// The existing tests asserted the status code and that nothing leaked, which
// both remained true across the whole divergence — the same gap that let the
// hand-rolled 401 survive until #88. This one asserts the shape, so a future
// writeJSON(w, someStatus, ...) on a failure path cannot pass.
//
// It deliberately checks the envelope rather than the prose: type, title and
// status are the contract, detail is wording, and pinning wording would make
// this test an obstacle to improving a message rather than a guard on a shape.
func TestEveryFailureIsProblemJSON(t *testing.T) {
	boom := errors.New("dial tcp 10.0.0.5:5432: connection refused")

	for _, tc := range []struct {
		name     string
		method   string
		svc      *fakeService
		wantCode int
		wantType string
	}{
		{"get: no profile", http.MethodGet, &fakeService{getErr: domain.ErrUserNotFound}, http.StatusNotFound, "not_found"},
		{"get: outage", http.MethodGet, &fakeService{getErr: boom}, http.StatusInternalServerError, "internal_error"},
		{"ensure: outage", http.MethodPost, &fakeService{ensureErr: boom}, http.StatusInternalServerError, "internal_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, router(tc.svc, testSubject.String()), tc.method, "/api/v1/users/me")

			if rec.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantCode)
			}
			if got := rec.Header().Get("Content-Type"); got != httpx.ProblemContentType {
				t.Errorf("Content-Type = %q, want %q", got, httpx.ProblemContentType)
			}

			var p httpx.Problem
			if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
				t.Fatalf("response is not a problem document: %v (%q)", err, rec.Body)
			}
			if p.Type != tc.wantType {
				t.Errorf("type = %q, want %q", p.Type, tc.wantType)
			}
			if p.Status != tc.wantCode {
				t.Errorf("body status = %d, want %d — a problem document that disagrees with its own status line", p.Status, tc.wantCode)
			}
			if p.Title == "" {
				t.Error("title is empty; RFC 7807 makes it the human-readable summary of the type")
			}
			if p.Instance != "/api/v1/users/me" {
				t.Errorf("instance = %q, want the request path — the hand-rolled writer filled neither instance nor request_id", p.Instance)
			}

			// Unchanged from the previous shape and still the point: the
			// document must carry the status, not the cause.
			if strings.Contains(rec.Body.String(), "10.0.0.5") {
				t.Errorf("the response leaked the underlying error: %s", rec.Body)
			}
		})
	}
}

// TestResponseCarriesNoIdentityData: the profile echoes back the caller's own
// id and its timestamps. A name or an email appearing here would be a second
// copy of data the identity owns (E20).
func TestResponseCarriesNoIdentityData(t *testing.T) {
	svc := &fakeService{getResp: &domain.UserResponse{ID: testSubject}}

	rec := do(t, router(svc, testSubject.String()), http.MethodGet, "/api/v1/users/me")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v (%q)", err, rec.Body)
	}
	for _, forbidden := range []string{"name", "email"} {
		if _, ok := body[forbidden]; ok {
			t.Errorf("the profile response carries %q, which belongs to the identity", forbidden)
		}
	}
	if got := body["id"]; got != testSubject.String() {
		t.Errorf("id = %v, want the caller's own %v", got, testSubject)
	}
}

// TestAMalformedSubjectIs401NotAProfile is the branch that matters most in
// caller(): the token verified, but its subject is not an id this service can
// act for.
//
// It must be 401 and not 400, it must not reach the service, and it must be the
// SAME 401 the rest of the tree emits. A subject that
// does not parse is an authentication problem — nothing about the client's
// request is wrong — and treating it as a bad request would invite a handler to
// carry on with a zero uuid, which is a profile belonging to nobody.
func TestAMalformedSubjectIs401NotAProfile(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		t.Run(method, func(t *testing.T) {
			svc := &fakeService{}

			rec := do(t, router(svc, "not-a-uuid"), method, "/api/v1/users/me")
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			if svc.calls != 0 {
				t.Errorf("the service ran %d times for a caller with no usable id", svc.calls)
			}

			// Byte-identical to the shared writer, not merely 401. Asserting
			// the status alone is what let this module hand-roll its own body
			// for the length of a release: the test stayed green while the one
			// response A3 exists to keep uniform diverged from every other 401
			// in the tree. Comparing against httpx.Unauthorized itself means a
			// future hand-rolled body cannot pass, and means this test does not
			// restate the challenge or the media type — httpx owns those, and
			// a deliberate change there should not have to be echoed here.
			want := httptest.NewRecorder()
			httpx.Unauthorized(want, httptest.NewRequest(method, "/api/v1/users/me", nil))

			if got := rec.Header(); !reflect.DeepEqual(map[string][]string(got), map[string][]string(want.Header())) {
				t.Errorf("headers = %v, want the shared writer's %v", got, want.Header())
			}
			if got := rec.Body.String(); got != want.Body.String() {
				t.Errorf("body = %s, want the shared writer's %s", got, want.Body.String())
			}
		})
	}
}

// TestStoreFailuresAre500AndLeakNothing: an unreachable database must not be
// reported as "you have no profile" — that would tell a caller their account is
// gone during an outage — and the body must not carry the underlying error,
// which can name hosts and drivers.
func TestStoreFailuresAre500AndLeakNothing(t *testing.T) {
	boom := errors.New("dial tcp 10.0.0.5:5432: connection refused")

	for _, tc := range []struct {
		name   string
		method string
		svc    *fakeService
	}{
		{"get", http.MethodGet, &fakeService{getErr: boom}},
		{"ensure", http.MethodPost, &fakeService{ensureErr: boom}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, router(tc.svc, testSubject.String()), tc.method, "/api/v1/users/me")
			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want 500", rec.Code)
			}
			if strings.Contains(rec.Body.String(), "10.0.0.5") {
				t.Errorf("the response leaked the underlying error: %s", rec.Body)
			}
		})
	}
}
