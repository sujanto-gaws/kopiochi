package transport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/sujanto-gaws/kopiochi/internal/authn"
	"github.com/sujanto-gaws/kopiochi/internal/testsupport"
	domain "github.com/sujanto-gaws/kopiochi/modules/user/domain"
)

// This package had no test file at all before task B2, which is why the
// "transport tests drop MintToken/keypair setup" clause found nothing to drop:
// there was no keypair here to remove, only the absence of coverage. What these
// tests pin is the property the module's constructor test can only assert half
// of — module_test.go proves user.New refuses to build without a middleware,
// and the tests below prove that the middleware it was given is actually on the
// path of every route Routes mounts, rather than on some of them.
//
// Nothing here mints a token or generates a key. testsupport.FakeAuth supplies
// the caller directly, which is the whole point of it: the thing under test is
// the route table, not RS256.

const testSubject = "3f1b8a54-2c9e-4d77-9a1e-2b6c0d5e8f41"

// fakeService is a UserService whose every method is a swappable func, so a
// test names only the behavior it cares about. calls counts every method entry
// across the interface — that single counter is what proves a rejected request
// never reached the application layer, which is a stronger statement than
// "the status code was 401".
type fakeService struct {
	calls  int
	create func(context.Context, *domain.CreateUserRequest) (*domain.UserResponse, error)
	get    func(context.Context, int64) (*domain.UserResponse, error)
	update func(context.Context, int64, *domain.UpdateUserRequest) (*domain.UserResponse, error)
	del    func(context.Context, int64) error
}

func newFakeService() *fakeService {
	ok := &domain.UserResponse{ID: 42, Name: "Ada", Email: "ada@example.com"}
	return &fakeService{
		create: func(context.Context, *domain.CreateUserRequest) (*domain.UserResponse, error) { return ok, nil },
		get:    func(context.Context, int64) (*domain.UserResponse, error) { return ok, nil },
		update: func(context.Context, int64, *domain.UpdateUserRequest) (*domain.UserResponse, error) { return ok, nil },
		del:    func(context.Context, int64) error { return nil },
	}
}

func (f *fakeService) CreateUser(ctx context.Context, req *domain.CreateUserRequest) (*domain.UserResponse, error) {
	f.calls++
	return f.create(ctx, req)
}

func (f *fakeService) GetUserByID(ctx context.Context, id int64) (*domain.UserResponse, error) {
	f.calls++
	return f.get(ctx, id)
}

func (f *fakeService) UpdateUser(ctx context.Context, id int64, req *domain.UpdateUserRequest) (*domain.UserResponse, error) {
	f.calls++
	return f.update(ctx, id, req)
}

func (f *fakeService) DeleteUser(ctx context.Context, id int64) error {
	f.calls++
	return f.del(ctx, id)
}

// mount builds the real route table the module mounts in production: a chi
// router with Routes applied to it, so URL params resolve through chi exactly
// as they do at runtime rather than through a hand-built RouteContext.
func mount(svc UserService, mw authn.Middleware) http.Handler {
	r := chi.NewRouter()
	NewUserHandler(svc, mw).Routes(r)
	return r
}

func request(method, path, body string) *http.Request {
	if body == "" {
		return httptest.NewRequest(method, path, nil)
	}
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	return r
}

const validBody = `{"name":"Ada","email":"ada@example.com"}`

// routeTable is every route Routes mounts. The tests below iterate it rather
// than naming four cases each, so a fifth route added to Routes without a line
// here is caught by TestRoutes_TableMatchesTheMountedRoutes.
var routeTable = []struct {
	method string
	path   string
	body   string
	want   int
}{
	{http.MethodPost, "/users", validBody, http.StatusCreated},
	{http.MethodGet, "/users/42", "", http.StatusOK},
	{http.MethodPut, "/users/42", validBody, http.StatusOK},
	{http.MethodDelete, "/users/42", "", http.StatusNoContent},
}

// TestRoutes_TableMatchesTheMountedRoutes walks the chi tree the handler built
// and compares it to routeTable. Without it, a route added to Routes and not to
// the table would silently escape every protection assertion in this file — the
// tests would keep passing while a new endpoint sat unexercised.
func TestRoutes_TableMatchesTheMountedRoutes(t *testing.T) {
	r := chi.NewRouter()
	NewUserHandler(newFakeService(), testsupport.FakeAuth(testSubject)).Routes(r)

	mounted := map[string]bool{}
	err := chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		mounted[method+" "+route] = true
		return nil
	})
	if err != nil {
		t.Fatalf("chi.Walk: %v", err)
	}

	if len(mounted) != len(routeTable) {
		t.Errorf("chi tree has %d routes, routeTable has %d: %v", len(mounted), len(routeTable), mounted)
	}
	for _, rt := range routeTable {
		key := rt.method + " " + rt.path
		// chi reports the pattern, not a concrete path; normalize the two
		// id-bearing paths back to their pattern.
		key = strings.Replace(key, "/users/42", "/users/{id}", 1)
		if !mounted[key] {
			t.Errorf("route %q is in routeTable but not in the mounted chi tree", key)
		}
	}
}

// TestRoutes_EveryRouteIsBehindTheInjectedMiddleware is the security property.
// Routes wraps its group in h.authMW; if a future edit moved one route outside
// that group, the route table would look identical and nothing else in the
// suite would notice. A middleware that rejects everything makes the difference
// observable: a route inside the group answers 401 and never touches the
// service, a route outside it answers 200 and does.
func TestRoutes_EveryRouteIsBehindTheInjectedMiddleware(t *testing.T) {
	deny := func(http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		})
	}

	for _, rt := range routeTable {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			svc := newFakeService()
			rec := httptest.NewRecorder()
			mount(svc, deny).ServeHTTP(rec, request(rt.method, rt.path, rt.body))

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d: this route is not behind the auth middleware",
					rec.Code, http.StatusUnauthorized)
			}
			if svc.calls != 0 {
				t.Errorf("the application service was called %d time(s) on a rejected request; "+
					"the handler ran despite the middleware refusing the caller", svc.calls)
			}
		})
	}
}

// TestRoutes_AuthenticatedRequestsReachTheHandlers is the control for the test
// above: the 401s there must come from the middleware refusing, not from the
// routes being broken. It also checks what the middleware left behind — a
// Principal carrying the subject FakeAuth was given — observed from inside the
// chain, between the middleware and the handler.
//
// That observation is deliberately made by a probe rather than by a handler
// assertion, because no handler in this package reads the Principal: these
// routes take their target id from the URL and have no ownership check. Task
// B2 intended to add one, and could not — Principal.Subject is the identity
// module's uuid and this module's rows are keyed by an int64 with no mapping
// between them. Until that is resolved, what is verifiable is that the
// middleware runs and what it produces is well-formed, which is what this test
// pins; it must not be read as evidence that a caller can only reach their own
// record, because nothing here establishes that.
func TestRoutes_AuthenticatedRequestsReachTheHandlers(t *testing.T) {
	for _, rt := range routeTable {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			var (
				seen  authn.Principal
				found bool
			)
			fake := testsupport.FakeAuth(testSubject)
			probe := func(next http.Handler) http.Handler {
				return fake(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					seen, found = authn.FromContext(r.Context())
					next.ServeHTTP(w, r)
				}))
			}

			svc := newFakeService()
			rec := httptest.NewRecorder()
			mount(svc, probe).ServeHTTP(rec, request(rt.method, rt.path, rt.body))

			if rec.Code != rt.want {
				t.Errorf("status = %d, want %d (body %q)", rec.Code, rt.want, rec.Body.String())
			}
			if svc.calls != 1 {
				t.Errorf("service calls = %d, want 1", svc.calls)
			}
			if !found {
				t.Fatal("no authn.Principal in the request context; the injected middleware did not run on this route")
			}
			if seen.Subject != testSubject {
				t.Errorf("Principal.Subject = %q, want %q", seen.Subject, testSubject)
			}
		})
	}
}

// TestCreateUser_Success checks the one response body worth asserting: the
// created record is echoed back, so a handler that returned 201 with an empty
// body would not pass.
func TestCreateUser_Success(t *testing.T) {
	svc := newFakeService()
	rec := httptest.NewRecorder()
	mount(svc, testsupport.FakeAuth(testSubject)).ServeHTTP(rec, request(http.MethodPost, "/users", validBody))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	var got domain.UserResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v (body %q)", err, rec.Body.String())
	}
	if got.ID != 42 || got.Email != "ada@example.com" {
		t.Errorf("response = %+v, want the created user echoed back", got)
	}
}

// TestHandlers_ErrorMapping pins the status code each failure produces. The
// mapping is the handler's entire job on the sad path, and the distinction that
// matters most here is domain.ErrUserNotFound → 404 versus any other service
// error → 500: collapsing those would either leak that a row exists or hide a
// real fault as a 404.
func TestHandlers_ErrorMapping(t *testing.T) {
	boom := errors.New("repository exploded")

	tests := []struct {
		name    string
		method  string
		path    string
		body    string
		arrange func(*fakeService)
		want    int
		wantMsg string
	}{
		{
			name: "create rejects a body that is not JSON", method: http.MethodPost, path: "/users",
			body: "{not json", want: http.StatusBadRequest, wantMsg: "invalid request body",
		},
		{
			name: "create surfaces a domain validation error", method: http.MethodPost, path: "/users", body: validBody,
			arrange: func(f *fakeService) {
				f.create = func(context.Context, *domain.CreateUserRequest) (*domain.UserResponse, error) {
					return nil, domain.ErrInvalidEmail
				}
			},
			want: http.StatusBadRequest, wantMsg: domain.ErrInvalidEmail.Error(),
		},
		{
			name: "create hides an unexpected error", method: http.MethodPost, path: "/users", body: validBody,
			arrange: func(f *fakeService) {
				f.create = func(context.Context, *domain.CreateUserRequest) (*domain.UserResponse, error) {
					return nil, boom
				}
			},
			want: http.StatusInternalServerError, wantMsg: "failed to create user",
		},
		{
			name: "get rejects a non-numeric id", method: http.MethodGet, path: "/users/abc",
			want: http.StatusBadRequest, wantMsg: "invalid user ID",
		},
		{
			name: "get maps not-found to 404", method: http.MethodGet, path: "/users/42",
			arrange: func(f *fakeService) {
				f.get = func(context.Context, int64) (*domain.UserResponse, error) { return nil, domain.ErrUserNotFound }
			},
			want: http.StatusNotFound, wantMsg: "user not found",
		},
		{
			name: "get hides an unexpected error", method: http.MethodGet, path: "/users/42",
			arrange: func(f *fakeService) {
				f.get = func(context.Context, int64) (*domain.UserResponse, error) { return nil, boom }
			},
			want: http.StatusInternalServerError, wantMsg: "failed to fetch user",
		},
		{
			name: "update rejects a non-numeric id", method: http.MethodPut, path: "/users/abc", body: validBody,
			want: http.StatusBadRequest, wantMsg: "invalid user ID",
		},
		{
			name: "update rejects a body that is not JSON", method: http.MethodPut, path: "/users/42",
			body: "{not json", want: http.StatusBadRequest, wantMsg: "invalid request body",
		},
		{
			name: "update maps not-found to 404", method: http.MethodPut, path: "/users/42", body: validBody,
			arrange: func(f *fakeService) {
				f.update = func(context.Context, int64, *domain.UpdateUserRequest) (*domain.UserResponse, error) {
					return nil, domain.ErrUserNotFound
				}
			},
			want: http.StatusNotFound, wantMsg: "user not found",
		},
		{
			name: "update surfaces a domain validation error", method: http.MethodPut, path: "/users/42", body: validBody,
			arrange: func(f *fakeService) {
				f.update = func(context.Context, int64, *domain.UpdateUserRequest) (*domain.UserResponse, error) {
					return nil, domain.ErrInvalidName
				}
			},
			want: http.StatusBadRequest, wantMsg: domain.ErrInvalidName.Error(),
		},
		{
			name: "update hides an unexpected error", method: http.MethodPut, path: "/users/42", body: validBody,
			arrange: func(f *fakeService) {
				f.update = func(context.Context, int64, *domain.UpdateUserRequest) (*domain.UserResponse, error) {
					return nil, boom
				}
			},
			want: http.StatusInternalServerError, wantMsg: "failed to update user",
		},
		{
			name: "delete rejects a non-numeric id", method: http.MethodDelete, path: "/users/abc",
			want: http.StatusBadRequest, wantMsg: "invalid user ID",
		},
		{
			name: "delete maps not-found to 404", method: http.MethodDelete, path: "/users/42",
			arrange: func(f *fakeService) {
				f.del = func(context.Context, int64) error { return domain.ErrUserNotFound }
			},
			want: http.StatusNotFound, wantMsg: "user not found",
		},
		{
			name: "delete hides an unexpected error", method: http.MethodDelete, path: "/users/42",
			arrange: func(f *fakeService) {
				f.del = func(context.Context, int64) error { return boom }
			},
			want: http.StatusInternalServerError, wantMsg: "failed to delete user",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newFakeService()
			if tc.arrange != nil {
				tc.arrange(svc)
			}

			rec := httptest.NewRecorder()
			mount(svc, testsupport.FakeAuth(testSubject)).ServeHTTP(rec, request(tc.method, tc.path, tc.body))

			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d (body %q)", rec.Code, tc.want, rec.Body.String())
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}

			var body map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode error body: %v (body %q)", err, rec.Body.String())
			}
			if body["error"] != tc.wantMsg {
				t.Errorf("error = %q, want %q", body["error"], tc.wantMsg)
			}
		})
	}
}

// TestDeleteUser_SuccessHasNoBody guards the one writeJSON call that passes a
// nil value: 204 with a body is a protocol error, and the nil branch in
// writeJSON is the only thing preventing it.
func TestDeleteUser_SuccessHasNoBody(t *testing.T) {
	rec := httptest.NewRecorder()
	mount(newFakeService(), testsupport.FakeAuth(testSubject)).
		ServeHTTP(rec, request(http.MethodDelete, "/users/42", ""))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("204 response carried a body %q; it must be empty", rec.Body.String())
	}
}
