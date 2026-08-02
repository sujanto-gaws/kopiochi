package token

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	domain "github.com/sujanto-gaws/kopiochi/modules/identity/domain"
)

const (
	testIssuer   = "kopiochi-test"
	testAudience = "kopiochi-test-client"
)

// testUser returns a minimal domain.User sufficient to mint any token class.
func testUser() domain.User {
	return domain.User{
		ID:    uuid.New(),
		Email: "alice@example.com",
		Name:  "Alice",
	}
}

// newTestService builds a *JWTService backed by a freshly generated (test-
// only) RSA key pair written to PEM files in a temp dir, wired with a fixed
// issuer/audience and a small leeway.
func newTestService(t *testing.T) *JWTService {
	t.Helper()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	dir := t.TempDir()
	privPath := filepath.Join(dir, "private.pem")
	pubPath := filepath.Join(dir, "public.pem")

	require.NoError(t, os.WriteFile(privPath, pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	}), 0o600))

	pubBytes, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(pubPath, pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubBytes,
	}), 0o600))

	svc, err := NewJWTService(privPath, pubPath, testIssuer, testAudience, time.Second)
	require.NoError(t, err)
	return svc
}

// signRaw signs claims with svc's own private key using RS256, bypassing the
// Issue* helpers so tests can construct tokens with deliberately wrong
// iss/aud/exp values.
func signRaw(t *testing.T, svc *JWTService, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signed, err := tok.SignedString(svc.privateKey)
	require.NoError(t, err)
	return signed
}

func validClaims(svc *JWTService) jwt.MapClaims {
	now := time.Now()
	return jwt.MapClaims{
		"sub":   uuid.NewString(),
		"iss":   svc.issuer,
		"aud":   svc.audience,
		"scope": "access",
		"iat":   now.Unix(),
		"exp":   now.Add(15 * time.Minute).Unix(),
	}
}

// TestValidate_RejectsWrongAlgorithm proves the classic "confusion attack" is
// structurally closed: an attacker who forges an HS256 token, signed with the
// RSA *public* key (which is, by definition, public) treated as an HMAC
// secret, must be rejected -- not merely a config assertion that RS256 is
// configured.
func TestValidate_RejectsWrongAlgorithm(t *testing.T) {
	svc := newTestService(t)

	pubDER, err := x509.MarshalPKIXPublicKey(svc.publicKey)
	require.NoError(t, err)

	claims := validClaims(svc)
	forged := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	forgedToken, err := forged.SignedString(pubDER)
	require.NoError(t, err)

	_, err = svc.Validate(forgedToken)
	require.Error(t, err, "an HS256 token signed with the RSA public key must never verify")
}

// TestValidate_RejectsNoneAlgorithm is the companion case to the HS256
// confusion attack: the "none" algorithm (no signature at all) must also be
// rejected outright.
func TestValidate_RejectsNoneAlgorithm(t *testing.T) {
	svc := newTestService(t)

	claims := validClaims(svc)
	unsigned := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	unsignedToken, err := unsigned.SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	_, err = svc.Validate(unsignedToken)
	require.Error(t, err, "an unsigned \"none\"-alg token must never verify")
}

// TestValidate_RejectsWrongIssuer proves iss is actually checked, not merely
// parsed and ignored.
func TestValidate_RejectsWrongIssuer(t *testing.T) {
	svc := newTestService(t)

	claims := validClaims(svc)
	claims["iss"] = "some-other-service"
	tokenStr := signRaw(t, svc, claims)

	_, err := svc.Validate(tokenStr)
	require.Error(t, err, "a token issued by a different iss must be rejected")
}

// TestValidate_RejectsWrongAudience proves aud is actually checked. If this
// service's secret/keypair were ever shared with another service, tokens
// minted for that other service's audience must not verify here.
func TestValidate_RejectsWrongAudience(t *testing.T) {
	svc := newTestService(t)

	claims := validClaims(svc)
	claims["aud"] = "some-other-audience"
	tokenStr := signRaw(t, svc, claims)

	_, err := svc.Validate(tokenStr)
	require.Error(t, err, "a token minted for a different aud must be rejected")
}

// TestValidate_RejectsExpired proves exp is enforced beyond the configured
// leeway -- a token that has been expired well past any reasonable clock
// skew must never verify.
func TestValidate_RejectsExpired(t *testing.T) {
	svc := newTestService(t)

	claims := validClaims(svc)
	claims["iat"] = time.Now().Add(-time.Hour).Unix()
	claims["exp"] = time.Now().Add(-time.Minute).Unix()
	tokenStr := signRaw(t, svc, claims)

	_, err := svc.Validate(tokenStr)
	require.Error(t, err, "an expired token must be rejected")
}

// TestValidate_AcceptsGenuineAccessToken is the control proving Validate
// isn't rejecting everything: a genuinely issued access token must verify.
func TestValidate_AcceptsGenuineAccessToken(t *testing.T) {
	svc := newTestService(t)
	user := testUser()

	tokenStr, err := svc.IssueAccessToken(user, 15*time.Minute)
	require.NoError(t, err)

	claims, err := svc.Validate(tokenStr)
	require.NoError(t, err)
	require.Equal(t, "access", claims.Scope)
	require.Equal(t, user.ID.String(), claims.Subject)
}
