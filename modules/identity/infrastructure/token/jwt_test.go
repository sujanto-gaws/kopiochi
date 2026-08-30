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

func validClaims(svc *JWTService, cls domain.Class) jwt.MapClaims {
	now := time.Now()
	return jwt.MapClaims{
		"sub": uuid.NewString(),
		"iss": svc.issuer,
		"aud": svc.audience,
		"cls": string(cls),
		"iat": now.Unix(),
		"exp": now.Add(15 * time.Minute).Unix(),
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

	claims := validClaims(svc, domain.ClassAccess)
	forged := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	forgedToken, err := forged.SignedString(pubDER)
	require.NoError(t, err)

	_, err = svc.Validate(forgedToken, domain.ClassAccess)
	require.Error(t, err, "an HS256 token signed with the RSA public key must never verify")
}

// TestValidate_RejectsNoneAlgorithm is the companion case to the HS256
// confusion attack: the "none" algorithm (no signature at all) must also be
// rejected outright.
func TestValidate_RejectsNoneAlgorithm(t *testing.T) {
	svc := newTestService(t)

	claims := validClaims(svc, domain.ClassAccess)
	unsigned := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	unsignedToken, err := unsigned.SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	_, err = svc.Validate(unsignedToken, domain.ClassAccess)
	require.Error(t, err, "an unsigned \"none\"-alg token must never verify")
}

// TestValidate_RejectsWrongIssuer proves iss is actually checked, not merely
// parsed and ignored.
func TestValidate_RejectsWrongIssuer(t *testing.T) {
	svc := newTestService(t)

	claims := validClaims(svc, domain.ClassAccess)
	claims["iss"] = "some-other-service"
	tokenStr := signRaw(t, svc, claims)

	_, err := svc.Validate(tokenStr, domain.ClassAccess)
	require.Error(t, err, "a token issued by a different iss must be rejected")
}

// TestValidate_RejectsWrongAudience proves aud is actually checked. If this
// service's secret/keypair were ever shared with another service, tokens
// minted for that other service's audience must not verify here.
func TestValidate_RejectsWrongAudience(t *testing.T) {
	svc := newTestService(t)

	claims := validClaims(svc, domain.ClassAccess)
	claims["aud"] = "some-other-audience"
	tokenStr := signRaw(t, svc, claims)

	_, err := svc.Validate(tokenStr, domain.ClassAccess)
	require.Error(t, err, "a token minted for a different aud must be rejected")
}

// TestValidate_RejectsExpired proves exp is enforced beyond the configured
// leeway -- a token that has been expired well past any reasonable clock
// skew must never verify.
func TestValidate_RejectsExpired(t *testing.T) {
	svc := newTestService(t)

	claims := validClaims(svc, domain.ClassAccess)
	claims["iat"] = time.Now().Add(-time.Hour).Unix()
	claims["exp"] = time.Now().Add(-time.Minute).Unix()
	tokenStr := signRaw(t, svc, claims)

	_, err := svc.Validate(tokenStr, domain.ClassAccess)
	require.Error(t, err, "an expired token must be rejected")
}

// TestValidate_RejectsMFATokenAsAccessToken is the Phase 2 exit criterion:
// a short-lived, half-authenticated MFA token must never be accepted where
// an access token is expected. This must fail structurally (the Validate API
// requires the caller to state the expected class), not by an incidental
// string comparison the caller could forget.
func TestValidate_RejectsMFATokenAsAccessToken(t *testing.T) {
	svc := newTestService(t)
	user := testUser()

	mfaToken, err := svc.IssueMFAToken(user)
	require.NoError(t, err)

	claims, err := svc.Validate(mfaToken, domain.ClassAccess)
	require.Error(t, err, "an MFA token must never validate as an access token")
	require.Nil(t, claims)
	require.ErrorIs(t, err, domain.ErrWrongTokenClass)

	// Control: the same token must validate successfully as what it
	// actually is.
	claims, err = svc.Validate(mfaToken, domain.ClassMFA)
	require.NoError(t, err, "the MFA token must still validate as an MFA token")
	require.Equal(t, domain.ClassMFA, claims.Class)
	require.Equal(t, user.ID.String(), claims.Subject)
}

// TestValidate_AcceptsGenuineAccessToken is the control proving Validate
// isn't rejecting everything: a genuinely issued access token must verify.
func TestValidate_AcceptsGenuineAccessToken(t *testing.T) {
	svc := newTestService(t)
	user := testUser()

	tokenStr, err := svc.IssueAccessToken(user, 15*time.Minute)
	require.NoError(t, err)

	claims, err := svc.Validate(tokenStr, domain.ClassAccess)
	require.NoError(t, err)
	require.Equal(t, domain.ClassAccess, claims.Class)
	require.Equal(t, user.ID.String(), claims.Subject)
}

// TestAccessTokenCarriesNoUnenforcedPrivilegeClaims — E26.
//
// This is the assertion whose absence let the claims sit there. Every existing
// test in this package passed unchanged when `roles` and `permissions` were
// removed, because nothing had ever asserted they were minted. A claim nobody
// tests and nobody enforces is a claim nobody has decided to have.
//
// The property is not "these two strings are gone". It is that this service
// does not sign a privilege assertion it will not act on. A signed claim
// travels: a downstream reader can trust it without ever asking this service,
// which turns it into an authorization decision made on data nobody checks.
//
// When an authorization primitive lands (E26), these come back — alongside the
// code that enforces them, and with this test inverted deliberately rather than
// deleted quietly.
func TestAccessTokenCarriesNoUnenforcedPrivilegeClaims(t *testing.T) {
	svc := newTestService(t)

	u := testUser()
	u.Roles = []string{"admin", "operator"}
	u.Permissions = []string{"users:delete", "billing:write"}

	tokenStr, err := svc.IssueAccessToken(u, time.Minute)
	require.NoError(t, err)

	// Read the raw claims rather than the parsed domain.Claims: the point is
	// what was SIGNED, not what the validator chooses to surface.
	parsed, _, err := jwt.NewParser().ParseUnverified(tokenStr, jwt.MapClaims{})
	require.NoError(t, err)
	claims, ok := parsed.Claims.(jwt.MapClaims)
	require.True(t, ok)

	for _, name := range []string{"roles", "permissions"} {
		if v, present := claims[name]; present {
			t.Errorf("the access token signs a %q claim (%v) that nothing in this service "+
				"enforces; a downstream reader would trust it (E26)", name, v)
		}
	}

	// The control: the token is still a usable access token. This must fail
	// because a privilege claim is absent, never because minting broke.
	require.Equal(t, u.ID.String(), claims["sub"])
	require.Equal(t, string(domain.ClassAccess), claims["cls"])

	got, err := svc.Validate(tokenStr, domain.ClassAccess)
	require.NoError(t, err, "the token no longer validates; this test is about claims, not minting")
	require.Equal(t, u.ID.String(), got.Subject)
	require.Empty(t, got.Roles, "roles surfaced from a token that never carried them")
	require.Empty(t, got.Permissions)
}

// TestValidateStillReadsPrivilegeClaimsFromOlderTokens: the parse side keeps
// reading `roles`/`permissions` on purpose.
//
// Tokens minted before E26 stay valid until they expire. A validator that
// dropped the fields would return a different Claims for the same token
// depending on when it was issued — a difference no caller could see coming.
// Reading a claim commits to nothing; minting one does.
func TestValidateStillReadsPrivilegeClaimsFromOlderTokens(t *testing.T) {
	svc := newTestService(t)
	u := testUser()

	// Mint by hand, the way this service did before E26.
	now := time.Now()
	legacy, err := svc.sign(jwt.MapClaims{
		"sub":         u.ID.String(),
		"email":       u.Email,
		"name":        u.Name,
		"roles":       []string{"admin"},
		"permissions": []string{"users:delete"},
		"scope":       "access",
		"cls":         string(domain.ClassAccess),
		"iss":         testIssuer,
		"aud":         testAudience,
		"iat":         now.Unix(),
		"exp":         now.Add(time.Minute).Unix(),
	})
	require.NoError(t, err)

	got, err := svc.Validate(legacy, domain.ClassAccess)
	require.NoError(t, err, "a token issued before E26 stopped validating")
	require.Equal(t, []string{"admin"}, got.Roles)
	require.Equal(t, []string{"users:delete"}, got.Permissions)
}
