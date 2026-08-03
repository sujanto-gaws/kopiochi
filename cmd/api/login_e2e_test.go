// This file proves the Phase 1 exit criterion (docs/architectures/07-roadmap/
// remediation-plan.md:191): "POST /api/v1/auth/login returns a token against
// a migrated database." It lives in cmd/api (not modules/identity) because
// proving the criterion end to end requires the real composition root
// (BuildApp) and the real mounted router (httpx.Mount) — exactly what
// routes_test.go in this same package already builds — plus a real migrated
// Postgres database, which only cmd/api's tests currently combine with
// BuildApp. Anything less (e.g. calling application.Service.Login directly)
// would prove the service works, not that the wired HTTP application does.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/pressly/goose/v3"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"

	"github.com/sujanto-gaws/kopiochi/internal/httpx"
	"github.com/sujanto-gaws/kopiochi/internal/testutil"
	identityapp "github.com/sujanto-gaws/kopiochi/modules/identity/application"
	domain "github.com/sujanto-gaws/kopiochi/modules/identity/domain"
	"github.com/sujanto-gaws/kopiochi/modules/identity/infrastructure/hasher"
	identityrepo "github.com/sujanto-gaws/kopiochi/modules/identity/infrastructure/persistence/repository"
	"github.com/sujanto-gaws/kopiochi/modules/identity/infrastructure/token"
)

// testSeededPassword is the known plaintext behind every seeded user's
// PasswordHash in this file. It is hashed for real via hasher.BcryptHasher —
// the same hasher identity.New wires into the login path — so cost and
// format match production exactly; this test never fabricates a hash string.
const testSeededPassword = "correct horse battery staple"

// scratchIdentityDB returns a *bun.DB backed by a disposable Postgres
// container with every migration in migrations/ applied (goose.Up), reusing
// internal/testutil.ScratchPostgres — the same helper internal/db/
// schema_test.go uses — rather than inventing a second way to stand up a
// throwaway database.
func scratchIdentityDB(t *testing.T) *bun.DB {
	t.Helper()

	sqlDB, cleanup := testutil.ScratchPostgres(t)
	t.Cleanup(cleanup)

	migrationsDir, err := filepath.Abs(filepath.Join("..", "..", "migrations"))
	require.NoError(t, err)

	require.NoError(t, goose.SetDialect("postgres"))
	require.NoError(t, goose.Up(sqlDB, migrationsDir))

	return bun.NewDB(sqlDB, pgdialect.New())
}

// seedAuthUser inserts one row into auth_users directly through the identity
// module's own repository (identityrepo.UserRepo), so the seed goes through
// exactly the same Bun model/mapping the application uses to read it back.
// Login authenticates via FindByUsername, not FindByEmail, so username is the
// field that must be set correctly for login to find this row at all.
func seedAuthUser(t *testing.T, bunDB *bun.DB, username, email string) *domain.User {
	t.Helper()

	hash, err := (hasher.BcryptHasher{}).Hash(testSeededPassword)
	require.NoError(t, err)

	user := &domain.User{
		ID:           uuid.New(),
		Username:     username,
		Email:        email,
		Name:         "Test User",
		Roles:        []string{"user"},
		Permissions:  []string{},
		PasswordHash: hash,
	}

	repo := identityrepo.NewUserRepo(bunDB)
	require.NoError(t, repo.Save(context.Background(), user))
	return user
}

// buildE2ERouter builds the real application (BuildApp) against the given
// database and mounts the real route tree (httpx.Mount) — the same
// composition path production uses — and returns it alongside the config
// used to build it, so the caller can independently validate tokens the app
// issues via the app's own JWTService/public key.
func buildE2ERouter(t *testing.T, bunDB *bun.DB) (http.Handler, *token.JWTService) {
	t.Helper()

	cfg := testConfig(t)

	app, err := BuildApp(cfg, bunDB, zerolog.Nop())
	require.NoError(t, err)

	r, closeRouter, err := httpx.NewRouter(cfg.Server, cfg.Security, zerolog.Nop())
	require.NoError(t, err)
	t.Cleanup(func() { _ = closeRouter() })
	httpx.Mount(r, app.Modules, httpx.Deps{Pinger: nil})

	// A second JWTService instance, built from the same key paths/issuer, so
	// that validating the token below exercises the app's real verification
	// logic (RS256 + public key) rather than merely re-parsing the JWT.
	jwtSvc, err := token.NewJWTService(cfg.Auth.PrivateKeyPath, cfg.Auth.PublicKeyPath, cfg.Auth.Issuer, cfg.Auth.ClientID, cfg.Auth.TokenLeeway)
	require.NoError(t, err)

	return r, jwtSvc
}

func doLogin(t *testing.T, r http.Handler, username, password string) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(identityapp.LoginRequest{Username: username, Password: password})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// TestLogin_EndToEnd_ReturnsToken is the Phase 1 exit criterion itself:
// POST /api/v1/auth/login, against a migrated database, returns 200 and a
// token that verifies under the app's own JWTService as a genuine
// access-scoped token — not merely a non-empty string in a JSON field.
func TestLogin_EndToEnd_ReturnsToken(t *testing.T) {
	bunDB := scratchIdentityDB(t)

	const username = "alice"
	seeded := seedAuthUser(t, bunDB, username, "alice@example.com")

	r, jwtSvc := buildE2ERouter(t, bunDB)

	rec := doLogin(t, r, username, testSeededPassword)

	require.Equal(t, http.StatusOK, rec.Code, "login response body: %s", rec.Body.String())

	var tokenResp identityapp.TokenResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &tokenResp), "response body: %s", rec.Body.String())
	require.NotEmpty(t, tokenResp.AccessToken, "expected a non-empty access token in the login response")

	// The load-bearing assertion: the token must actually verify against the
	// app's own public key and carry the access-token class/subject — a
	// non-empty string alone proves nothing.
	claims, err := jwtSvc.Validate(tokenResp.AccessToken, domain.ClassAccess)
	require.NoError(t, err, "access token returned by /api/v1/auth/login must verify via the app's own JWTService")
	require.Equal(t, domain.ClassAccess, claims.Class, "token returned by a successful login must carry the access token class")
	require.Equal(t, seeded.ID.String(), claims.Subject, "token subject must identify the user who logged in")
}

// TestLogin_WrongPassword_ReturnsInvalidGrant guards fail-open: a wrong
// password must never yield a 200 or a usable token.
//
// The application returns HTTP 400 with an OAuth2 "invalid_grant" error body
// for bad credentials (modules/identity/transport/auth.go's Login handler),
// not 401 — that is the RFC 6749 convention for a rejected grant, and matches
// what the handler actually does. This test asserts that real, observed
// behavior rather than a differently-numbered status code.
func TestLogin_WrongPassword_ReturnsInvalidGrant(t *testing.T) {
	bunDB := scratchIdentityDB(t)

	const username = "bob"
	seedAuthUser(t, bunDB, username, "bob@example.com")

	r, _ := buildE2ERouter(t, bunDB)

	rec := doLogin(t, r, username, "definitely-the-wrong-password")

	require.Equal(t, http.StatusBadRequest, rec.Code, "response body: %s", rec.Body.String())
	require.NotEqual(t, http.StatusOK, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), "response body: %s", rec.Body.String())
	require.Equal(t, "invalid_grant", body["error"])
	require.Empty(t, body["access_token"], "no access token must be issued on failed login")
}
