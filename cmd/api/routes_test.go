package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/sujanto-gaws/kopiochi/internal/config"
	"github.com/sujanto-gaws/kopiochi/internal/httpx"
)

// testConfig builds a *config.Config that satisfies identity.Config.Validate,
// backed by a freshly generated (test-only) RSA key pair — it does not depend
// on any key files checked into the repo.
func testConfig(t *testing.T) *config.Config {
	t.Helper()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	dir := t.TempDir()
	privPath := filepath.Join(dir, "private.pem")
	pubPath := filepath.Join(dir, "public.pem")

	privBytes := x509.MarshalPKCS1PrivateKey(priv)
	writePEM(t, privPath, "RSA PRIVATE KEY", privBytes)

	pubBytes, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	require.NoError(t, err)
	writePEM(t, pubPath, "PUBLIC KEY", pubBytes)

	cfg := &config.Config{}
	cfg.Auth = config.Auth{
		PrivateKeyPath:    privPath,
		PublicKeyPath:     pubPath,
		Issuer:            "kopiochi-test",
		ClientID:          "kopiochi-test",
		AccessTokenTTL:    15 * time.Minute,
		RefreshTokenTTL:   168 * time.Hour,
		MFATemporaryTTL:   5 * time.Minute,
		MaxFailedAttempts: 5,
		LockDuration:      15 * time.Minute,
	}
	cfg.Server = config.Server{
		RequestTimeout: 5 * time.Second,
	}
	return cfg
}

func writePEM(t *testing.T, path, typ string, bytes []byte) {
	t.Helper()
	encoded := pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: bytes})
	require.NoError(t, os.WriteFile(path, encoded, 0o600))
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

	r, closeRouter, err := httpx.NewRouter(cfg.Server, cfg.Security, zerolog.Nop())
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

	require.Contains(t, got, "POST /api/v1/users")
	require.Contains(t, got, "GET /api/v1/users/{id}")
	require.Contains(t, got, "PUT /api/v1/users/{id}")
	require.Contains(t, got, "DELETE /api/v1/users/{id}")
}

func routeTableString(routes []string) string {
	out := ""
	for _, r := range routes {
		out += r + "\n"
	}
	return out
}
