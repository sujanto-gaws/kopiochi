package testsupport

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/sujanto-gaws/kopiochi/internal/config"
)

// Keypair is a freshly generated test RSA keypair written to disk, because the
// identity module loads its keys by path.
type Keypair struct {
	Private     *rsa.PrivateKey
	PrivatePath string
	PublicPath  string
}

// NewKeypair generates a 2048-bit RSA keypair into a per-test temp directory
// and returns the paths.
//
// Generated per test rather than read from a fixture on purpose: a committed
// test key is a key that eventually gets copied into a deployment, and the
// repository has already had one credential-in-history incident (see Phase
// 0.5). 2048 bits keeps generation cheap enough to do per test.
func NewKeypair(t *testing.T) Keypair {
	t.Helper()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	dir := t.TempDir()
	kp := Keypair{
		Private:     priv,
		PrivatePath: filepath.Join(dir, "private.pem"),
		PublicPath:  filepath.Join(dir, "public.pem"),
	}

	// PKCS1 ("BEGIN RSA PRIVATE KEY") is what the loader parses; modern
	// OpenSSL defaults to PKCS8, which it cannot read. See the `keys` target
	// in the Makefile.
	writePEM(t, kp.PrivatePath, "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(priv))

	pubBytes, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	require.NoError(t, err)
	writePEM(t, kp.PublicPath, "PUBLIC KEY", pubBytes)

	return kp
}

// TestIssuer and TestAudience are the iss/aud every helper here uses. They are
// exported so a test asserting rejection of a *wrong* issuer or audience can
// name the right one without hardcoding a string that later drifts.
const (
	TestIssuer   = "kopiochi-test"
	TestAudience = "kopiochi-test"
)

// Config returns a *config.Config that passes identity.Config.Validate,
// backed by a fresh keypair. The keypair is returned alongside so a test can
// mint its own tokens against the same key the application verifies with.
func Config(t *testing.T) (*config.Config, Keypair) {
	t.Helper()

	kp := NewKeypair(t)

	cfg := &config.Config{}
	cfg.Auth = config.Auth{
		PrivateKeyPath:    kp.PrivatePath,
		PublicKeyPath:     kp.PublicPath,
		Issuer:            TestIssuer,
		ClientID:          TestAudience,
		AccessTokenTTL:    15 * time.Minute,
		RefreshTokenTTL:   168 * time.Hour,
		MFATemporaryTTL:   5 * time.Minute,
		MaxFailedAttempts: 5,
		LockDuration:      15 * time.Minute,
	}
	cfg.Server = config.Server{
		RequestTimeout: 5 * time.Second,
	}

	return cfg, kp
}

func writePEM(t *testing.T, path, typ string, der []byte) {
	t.Helper()
	encoded := pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der})
	require.NoError(t, os.WriteFile(path, encoded, 0o600))
}
