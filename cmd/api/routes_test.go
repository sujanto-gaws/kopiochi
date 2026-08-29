package main

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/sujanto-gaws/kopiochi/internal/config"
	"github.com/sujanto-gaws/kopiochi/internal/httpx"
	"github.com/sujanto-gaws/kopiochi/internal/testsupport"
)

// testConfig builds a *config.Config that satisfies identity.Config.Validate,
// backed by a freshly generated (test-only) RSA key pair — it does not depend
// on any key files checked into the repo.
//
// The fixture itself moved to internal/testsupport in Phase 4.1 so that module
// tests can build a valid config without importing package main. This wrapper
// stays because the callers in this package do not need the keypair.
func testConfig(t *testing.T) *config.Config {
	t.Helper()

	cfg, _ := testsupport.Config(t)
	return cfg
}

// TestRouteTable builds the real application (BuildApp) and the real router
// (httpx.Mount), then walks the resulting chi tree. This is the guard against
// both defects fixed in this change: an empty container serving no routes,
// and modules mounting at the root instead of under /api/v1 (see
// docs/architectures/02-composition/routing-and-versioning.md).
//
// This test doubles as task 1.1(b) TestRouteTable from the remediation plan.
func TestRouteTable(t *testing.T) {
	cfg := testConfig(t)

	app, err := BuildApp(cfg, nil, zerolog.Nop())
	require.NoError(t, err)
	require.NotEmpty(t, app.Modules, "application must register at least one module")

	r, closeRouter, err := httpx.NewRouter(cfg.Server, cfg.Security, zerolog.Nop(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = closeRouter() })
	httpx.Mount(r, app.Modules, httpx.Deps{Pinger: nil})

	var got []string
	err = chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		got = append(got, method+" "+route)
		return nil
	})
	require.NoError(t, err)

	t.Logf("route table:\n%s", routeTableString(got))

	require.Contains(t, got, "POST /api/v1/auth/login")
	require.Contains(t, got, "POST /api/v1/auth/refresh")
	require.Contains(t, got, "POST /api/v1/auth/mfa/verify")
	require.Contains(t, got, "POST /api/v1/auth/logout")
	require.Contains(t, got, "POST /api/v1/auth/mfa/setup")
	require.Contains(t, got, "POST /api/v1/auth/mfa/setup/verify")

	require.Contains(t, got, "GET /healthz")
	require.Contains(t, got, "GET /readyz")
	require.Contains(t, got, "GET /health") // deprecated alias for /healthz

	// Two routes, both /users/me, and none carrying an id (E16, E24). The
	// absences are asserted rather than left implicit: an id-bearing profile
	// route reappearing is the IDOR reappearing, and a route table that only
	// checks for what should be present would not notice.
	require.Contains(t, got, "POST /api/v1/users/me")
	require.Contains(t, got, "GET /api/v1/users/me")
	require.NotContains(t, got, "/api/v1/users/{id}",
		"a profile route addressed by id is back; that is E16")
	require.NotContains(t, got, "PUT /api/v1/users",
		"the profile has no writable field, so a PUT can only lie (E24)")
	require.NotContains(t, got, "DELETE /api/v1/users",
		"deleting a profile without its identity leaves a logged-in caller with none")
}

func routeTableString(routes []string) string {
	out := ""
	for _, r := range routes {
		out += r + "\n"
	}
	return out
}
