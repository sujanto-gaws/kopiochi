package main

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/sujanto-gaws/kopiochi/internal/config"
	"github.com/sujanto-gaws/kopiochi/internal/httpx"
	"github.com/sujanto-gaws/kopiochi/internal/testsupport"
)

// These are the two guards named in
// docs/architectures/06-quality/testing-strategy.md — "guards fail-open" and
// "guards escalation" — driven through the real composition root and the real
// mounted router, so the middleware chain is what actually rejects them.
//
// They need no database. Every case here is refused by AuthRequired before a
// handler runs, which is precisely the property under test: an unauthenticated
// request must never reach the point of touching data. BuildApp accepts a nil
// bun.IDB, so the router is real while the repositories behind it are never
// called.

// protectedRoutes are the mounted routes that require a valid access token.
// Listed explicitly rather than discovered, so that adding a protected route
// without adding it here is a visible omission rather than silent coverage
// loss — and so that accidentally *un*protecting one fails these tests.
var protectedRoutes = []struct{ method, path string }{
	{http.MethodPost, "/api/v1/auth/logout"},
	{http.MethodPost, "/api/v1/auth/mfa/setup"},
	{http.MethodPost, "/api/v1/auth/mfa/setup/verify"},
	{http.MethodPost, "/api/v1/users/me"},
	{http.MethodGet, "/api/v1/users/me"},

	// The notification module's whole surface. Its own tests use a fake
	// middleware — what they are about is the mailbox, not RS256 — so these
	// entries are the only place the real AuthRequired is proven to be on
	// those routes, and modules/notification cannot prove it itself: it may
	// not import modules/identity (R2), and cmd/api is the package that sees
	// both.
	{http.MethodGet, "/api/v1/notifications"},
	{http.MethodPost, "/api/v1/notifications/00000000-0000-0000-0000-000000000000/read"},
	{http.MethodPost, "/api/v1/notifications/read-all"},
	{http.MethodGet, "/api/v1/notifications/preferences"},
	{http.MethodPut, "/api/v1/notifications/preferences"},
}

// buildAuthTestRouter builds the real application and route tree over a nil
// database, returning the router and the keypair its token verification uses.
func buildAuthTestRouter(t *testing.T) (http.Handler, testsupport.Keypair) {
	t.Helper()

	cfg, kp := testsupport.Config(t)

	// Enabled, because a disabled notification module mounts no routes and
	// half the table below would then be asserted against a 404 that has
	// nothing to do with authentication. lazyDB never dials and the
	// dispatcher's first tick is an hour away; nothing here gets past
	// AuthRequired, which is the property under test.
	cfg.Notification = enabledNotification()

	app, err := BuildApp(cfg, lazyDB(t), zerolog.Nop())
	require.NoError(t, err)
	t.Cleanup(func() { closeModules(t, app) })

	r, closeRouter, err := httpx.NewRouter(cfg.Server, cfg.Security, zerolog.Nop(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = closeRouter() })
	httpx.Mount(r, app.Modules, httpx.Deps{Pinger: nil})

	return r, kp
}

// TestProtectedRoute_WithoutToken_Returns401 is the fail-open guard: a route
// that forgets its middleware answers 200 (or 500) here instead of 401, and
// this is the only test that would notice.
func TestProtectedRoute_WithoutToken_Returns401(t *testing.T) {
	t.Parallel()

	r, _ := buildAuthTestRouter(t)

	for _, route := range protectedRoutes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			t.Parallel()

			rec := testsupport.Do(t, r, testsupport.JSONRequest(t, route.method, route.path, nil))
			testsupport.RequireStatus(t, rec, http.StatusUnauthorized)
		})
	}
}

// TestProtectedRoute_WithMFAToken_Returns401 is the escalation guard. The MFA
// token is issued after a password check but before the second factor; if it
// were accepted as an access token, enabling MFA would weaken the account
// rather than strengthen it — an attacker with only the password would hold a
// usable API credential.
func TestProtectedRoute_WithMFAToken_Returns401(t *testing.T) {
	t.Parallel()

	r, kp := buildAuthTestRouter(t)
	mfaToken := testsupport.MintToken(t, kp, testsupport.Claims{
		Subject: uuid.New().String(),
		Class:   testsupport.ClassMFA,
	})

	for _, route := range protectedRoutes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			t.Parallel()

			req := testsupport.WithBearer(testsupport.JSONRequest(t, route.method, route.path, nil), mfaToken)
			rec := testsupport.Do(t, r, req)
			testsupport.RequireStatus(t, rec, http.StatusUnauthorized)
		})
	}
}

// TestProtectedRoute_RejectsMalformedAndForgedTokens sweeps the rest of the
// rejection surface in one place. Each case is a different way a caller might
// try to get past AuthRequired.
func TestProtectedRoute_RejectsMalformedAndForgedTokens(t *testing.T) {
	t.Parallel()

	r, kp := buildAuthTestRouter(t)
	subject := uuid.New().String()
	other := testsupport.NewKeypair(t)

	tests := []struct {
		name   string
		header string
	}{
		{"empty Authorization", ""},
		{"no Bearer prefix", testsupport.AccessToken(t, kp, subject)},
		{"wrong scheme", "Basic " + testsupport.AccessToken(t, kp, subject)},
		{"lowercase bearer", "bearer " + testsupport.AccessToken(t, kp, subject)},
		{"Bearer with nothing after it", "Bearer "},
		{"not a JWT at all", "Bearer not-a-jwt"},
		{"expired", "Bearer " + testsupport.ExpiredToken(t, kp, subject)},
		{"signed by an unrelated key", "Bearer " + testsupport.AccessToken(t, other, subject)},
		{"wrong issuer", "Bearer " + testsupport.MintToken(t, kp, testsupport.Claims{
			Subject: subject, Issuer: "https://attacker.example",
		})},
		{"wrong audience", "Bearer " + testsupport.MintToken(t, kp, testsupport.Claims{
			Subject: subject, Audience: "some-other-client",
		})},
		{"id token used as an access token", "Bearer " + testsupport.MintToken(t, kp, testsupport.Claims{
			Subject: subject, Class: testsupport.ClassID,
		})},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := testsupport.JSONRequest(t, http.MethodPost, "/api/v1/auth/logout", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}

			rec := testsupport.Do(t, r, req)
			testsupport.RequireStatus(t, rec, http.StatusUnauthorized)
		})
	}
}

// TestProtectedRoute_ValidTokenPassesTheGuard is the control. Without it every
// test above would still pass if AuthRequired rejected everything
// unconditionally, which would prove nothing about the guard and everything
// about a broken endpoint.
//
// It asserts only that the request got *past* authentication — the database is
// nil, so what happens next is not this test's business.
func TestProtectedRoute_ValidTokenPassesTheGuard(t *testing.T) {
	t.Parallel()

	r, kp := buildAuthTestRouter(t)
	token := testsupport.AccessToken(t, kp, uuid.New().String())

	req := testsupport.WithBearer(testsupport.JSONRequest(t, http.MethodPost, "/api/v1/auth/logout", nil), token)
	rec := testsupport.Do(t, r, req)

	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("a valid access token was rejected (%d); the rejection tests above prove nothing\nbody: %s",
			rec.Code, rec.Body.String())
	}
}

// TestPublicRoutes_NeedNoToken: the login and refresh endpoints must stay
// reachable without credentials. Putting them behind the auth middleware would
// make it impossible to obtain credentials in the first place.
func TestPublicRoutes_NeedNoToken(t *testing.T) {
	t.Parallel()

	r, _ := buildAuthTestRouter(t)

	// /auth/mfa/verify is deliberately absent. It carries no auth middleware,
	// but its handler reads a short-lived MFA token from the Authorization
	// header itself and answers 401 without one — correctly. It gets its own
	// test below.
	public := []struct{ method, path string }{
		{http.MethodPost, "/api/v1/auth/login"},
		{http.MethodPost, "/api/v1/auth/refresh"},
		{http.MethodGet, "/healthz"},
	}

	for _, route := range public {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			t.Parallel()

			rec := testsupport.Do(t, r, testsupport.JSONRequest(t, route.method, route.path, map[string]string{}))
			if rec.Code == http.StatusUnauthorized {
				t.Errorf("%s %s requires a token; credentials could never be obtained", route.method, route.path)
			}
		})
	}
}

// TestMFAVerify_RejectsAnAccessToken is the class check in the direction the
// other tests do not cover.
//
// /auth/mfa/verify sits outside the access-token middleware by design — a
// caller mid-login does not have an access token yet. That makes it the one
// endpoint where a *fully* authenticated token could plausibly be accepted by
// accident, and the service asks Validate for domain.ClassMFA specifically to
// stop it. This drives that through the assembled router.
//
// It needs no database: the class mismatch is refused during token validation,
// before any repository is consulted.
func TestMFAVerify_RejectsAnAccessToken(t *testing.T) {
	t.Parallel()

	r, kp := buildAuthTestRouter(t)
	accessToken := testsupport.AccessToken(t, kp, uuid.New().String())

	req := testsupport.WithBearer(
		testsupport.JSONRequest(t, http.MethodPost, "/api/v1/auth/mfa/verify",
			map[string]string{"code": "123456"}),
		accessToken)

	rec := testsupport.Do(t, r, req)
	testsupport.RequireStatus(t, rec, http.StatusUnauthorized)
}

// TestMFAVerify_RequiresAToken records that the endpoint's own header check
// answers 401, rather than reaching the service with an empty token.
func TestMFAVerify_RequiresAToken(t *testing.T) {
	t.Parallel()

	r, _ := buildAuthTestRouter(t)

	rec := testsupport.Do(t, r, testsupport.JSONRequest(t, http.MethodPost,
		"/api/v1/auth/mfa/verify", map[string]string{"code": "123456"}))
	testsupport.RequireStatus(t, rec, http.StatusUnauthorized)
}

// TestUnknownRoute_IsProblemJSON checks the Phase 4.7 handlers through the
// fully assembled router rather than a bare chi.Mux, which is the only way to
// know they are actually registered by NewRouter.
func TestUnknownRoute_IsProblemJSON(t *testing.T) {
	t.Parallel()

	r, _ := buildAuthTestRouter(t)

	rec := testsupport.Do(t, r, testsupport.JSONRequest(t, http.MethodGet, "/api/v1/does-not-exist", nil))
	testsupport.RequireStatus(t, rec, http.StatusNotFound)

	if ct := rec.Header().Get("Content-Type"); ct != httpx.ProblemContentType {
		t.Errorf("Content-Type = %q, want %q", ct, httpx.ProblemContentType)
	}

	var p httpx.Problem
	testsupport.DecodeJSON(t, rec, &p)
	if p.Status != http.StatusNotFound {
		t.Errorf("problem.status = %d, want 404", p.Status)
	}
}

// TestWrongMethod_IsProblemJSONWithAllow likewise proves the 405 handler and
// its recomputed Allow header survive assembly into the real route tree.
func TestWrongMethod_IsProblemJSONWithAllow(t *testing.T) {
	t.Parallel()

	r, _ := buildAuthTestRouter(t)

	// /api/v1/auth/login exists for POST only.
	rec := testsupport.Do(t, r, testsupport.JSONRequest(t, http.MethodGet, "/api/v1/auth/login", nil))
	testsupport.RequireStatus(t, rec, http.StatusMethodNotAllowed)

	if got := rec.Header().Get("Allow"); got != http.MethodPost {
		t.Errorf("Allow = %q, want %q", got, http.MethodPost)
	}
	if ct := rec.Header().Get("Content-Type"); ct != httpx.ProblemContentType {
		t.Errorf("Content-Type = %q, want %q", ct, httpx.ProblemContentType)
	}
}

// compile-time assurance that the config fixture still satisfies what BuildApp
// needs; a signature change here should fail at build, not at run time.
var _ = func(cfg *config.Config) *config.Config { return cfg }
