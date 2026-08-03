package token

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	domain "github.com/sujanto-gaws/kopiochi/modules/identity/domain"
)

// writeKeypair generates a keypair and returns its private and public PEM
// paths.
func writeKeypair(t *testing.T) (privPath, pubPath string, priv *rsa.PrivateKey) {
	t.Helper()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	dir := t.TempDir()
	privPath = filepath.Join(dir, "private.pem")
	pubPath = filepath.Join(dir, "public.pem")

	require.NoError(t, os.WriteFile(privPath, pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv),
	}), 0o600))

	pubBytes, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(pubPath, pem.EncodeToMemory(&pem.Block{
		Type: "PUBLIC KEY", Bytes: pubBytes,
	}), 0o600))

	return privPath, pubPath, priv
}

func serviceFor(t *testing.T, privPath, pubPath string) *JWTService {
	t.Helper()

	svc, err := NewJWTService(privPath, pubPath, testIssuer, testAudience, time.Second)
	require.NoError(t, err)
	return svc
}

// TestKid_IsDeterministic is what makes rotation work without coordination:
// the same key produces the same id in every process that loads it, so a
// signer and a verifier agree with nothing shared between them.
func TestKid_IsDeterministic(t *testing.T) {
	t.Parallel()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	require.Equal(t, Kid(&priv.PublicKey), Kid(&priv.PublicKey))
}

func TestKid_DiffersPerKey(t *testing.T) {
	t.Parallel()

	a, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	b, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	require.NotEqual(t, Kid(&a.PublicKey), Kid(&b.PublicKey))
}

// TestKid_MatchesRFC7638 checks the construction against the worked example in
// the RFC's Section 3.1. The thumbprint is only interoperable if every
// implementation builds the same canonical JSON, and "it is stable" would pass
// happily with a wrong recipe.
func TestKid_MatchesRFC7638(t *testing.T) {
	t.Parallel()

	// The 2048-bit key from RFC 7638 §3.1, whose thumbprint the RFC states.
	const (
		nB64 = "0vx7agoebGcQSuuPiLJXZptN9nndrQmbXEps2aiAFbWhM78LhWx4cbbfAAt" +
			"VT86zwu1RK7aPFFxuhDR1L6tSoc_BJECPebWKRXjBZCiFV4n3oknjhMstn6" +
			"4tZ_2W-5JsGY4Hc5n9yBXArwl93lqt7_RN5w6Cf0h4QyQ5v-65YGjQR0_FD" +
			"W2QvzqY368QQMicAtaSqzs8KJZgnYb9c7d0zgdAZHzu6qMQvRL5hajrn1n9" +
			"1CbOpbISD08qNLyrdkt-bFTWhAI4vMQFh6WeZu0fM4lFd2NcRwr3XPksINH" +
			"aQ-G_xBniIqbw0Ls1jF44-csFCur-kEgU8awapJzKnqDKgw"
		eB64     = "AQAB"
		wantKid  = "NzbLsXh8uDCcd-6MNwXF4W_7noWXFZAfHkxZsRGC9Xs"
		expected = 65537
	)

	nBytes, err := base64.RawURLEncoding.DecodeString(nB64)
	require.NoError(t, err)

	pub := &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: expected}

	require.Equal(t, eB64, base64.RawURLEncoding.EncodeToString(exponentBytes(pub.E)),
		"exponent must be the shortest big-endian form, not zero-padded")
	require.Equal(t, wantKid, Kid(pub),
		"thumbprint does not match RFC 7638 §3.1; the canonical JSON is wrong")
}

// TestIssuedTokens_CarryTheKid: without it a verifier holding two keys must
// try each in turn, which makes a signature failure ambiguous — it cannot tell
// "wrong key" from "forged token".
func TestIssuedTokens_CarryTheKid(t *testing.T) {
	t.Parallel()

	privPath, pubPath, priv := writeKeypair(t)
	svc := serviceFor(t, privPath, pubPath)

	tokenStr, err := svc.IssueAccessToken(testUser(), time.Minute)
	require.NoError(t, err)

	parsed, _, err := jwt.NewParser().ParseUnverified(tokenStr, jwt.MapClaims{})
	require.NoError(t, err)

	kid, ok := parsed.Header["kid"].(string)
	require.True(t, ok, "issued token has no kid header")
	require.Equal(t, Kid(&priv.PublicKey), kid)
	require.Equal(t, svc.SigningKid(), kid)
}

func TestJWKS_PublishesTheSigningKey(t *testing.T) {
	t.Parallel()

	privPath, pubPath, priv := writeKeypair(t)
	set := serviceFor(t, privPath, pubPath).JWKS()

	require.Len(t, set.Keys, 1)
	k := set.Keys[0]
	require.Equal(t, "RSA", k.Kty)
	require.Equal(t, "sig", k.Use)
	require.Equal(t, rs256Alg, k.Alg)
	require.Equal(t, Kid(&priv.PublicKey), k.Kid)
	require.NotEmpty(t, k.N)
	require.Equal(t, "AQAB", k.E, "65537 must encode as AQAB")
}

// TestJWKS_ContainsNoPrivateMaterial is the assertion that matters for an
// unauthenticated endpoint. The JWK type has no field a private key could be
// written into, and this checks the rendered JSON rather than trusting that.
func TestJWKS_ContainsNoPrivateMaterial(t *testing.T) {
	t.Parallel()

	privPath, pubPath, priv := writeKeypair(t)

	encoded, err := json.Marshal(serviceFor(t, privPath, pubPath).JWKS())
	require.NoError(t, err)
	body := string(encoded)

	// RFC 7518 names every private RSA JWK member.
	for _, member := range []string{`"d"`, `"p"`, `"q"`, `"dp"`, `"dq"`, `"qi"`} {
		require.NotContains(t, body, member, "a private key member appears in the JWKS")
	}
	// And the actual secret, in the encoding it would take.
	require.NotContains(t, body,
		base64.RawURLEncoding.EncodeToString(priv.D.Bytes()),
		"the private exponent appears in the JWKS")
}

func TestJWKS_EmptySetMarshalsAsAnArray(t *testing.T) {
	t.Parallel()

	encoded, err := json.Marshal(JWKS{})
	require.NoError(t, err)
	require.Equal(t, `{"keys":[]}`, string(encoded),
		"a null keys array breaks any client that iterates it")
}

// TestRotation_PreviousKeyStillVerifies is the whole point of 5.5. Swapping
// the signing key without keeping the old one for verification invalidates
// every token already issued — every active session logged out at once.
func TestRotation_PreviousKeyStillVerifies(t *testing.T) {
	t.Parallel()

	oldPriv, oldPub, _ := writeKeypair(t)
	newPriv, newPub, _ := writeKeypair(t)

	// A token minted before the rotation.
	oldSvc := serviceFor(t, oldPriv, oldPub)
	legacyToken, err := oldSvc.IssueAccessToken(testUser(), time.Minute)
	require.NoError(t, err)

	// After the switch, without the old key: every existing session dies.
	rotatedWithout := serviceFor(t, newPriv, newPub)
	_, err = rotatedWithout.Validate(legacyToken, domain.ClassAccess)
	require.Error(t, err, "precondition: a bare rotation should reject the old token")

	// With the old key retained for verification, it keeps working.
	rotated := serviceFor(t, newPriv, newPub)
	require.NoError(t, rotated.WithPreviousKey(oldPub))

	claims, err := rotated.Validate(legacyToken, domain.ClassAccess)
	require.NoError(t, err, "a token issued before the rotation was rejected")
	require.Equal(t, domain.ClassAccess, claims.Class)

	// And new tokens are signed with the new key.
	fresh, err := rotated.IssueAccessToken(testUser(), time.Minute)
	require.NoError(t, err)
	_, err = rotated.Validate(fresh, domain.ClassAccess)
	require.NoError(t, err)
}

func TestRotation_JWKSPublishesBothKeys(t *testing.T) {
	t.Parallel()

	oldPriv, oldPub, _ := writeKeypair(t)
	newPriv, newPub, _ := writeKeypair(t)

	svc := serviceFor(t, newPriv, newPub)
	require.NoError(t, svc.WithPreviousKey(oldPub))

	set := svc.JWKS()
	require.Len(t, set.Keys, 2, "a client fetching the set must be able to verify both sides of the rotation")

	// The signing key comes first, so a client that ignores kid and tries keys
	// in order succeeds immediately for almost every token.
	require.Equal(t, svc.SigningKid(), set.Keys[0].Kid)

	_ = oldPriv
}

// TestRotation_RejectsTheSameKeyTwice: accepting it would make the config look
// like a rotation was configured when nothing was.
func TestRotation_RejectsTheSameKeyTwice(t *testing.T) {
	t.Parallel()

	priv, pub, _ := writeKeypair(t)
	svc := serviceFor(t, priv, pub)

	err := svc.WithPreviousKey(pub)
	require.Error(t, err)
	require.Contains(t, err.Error(), "same as the signing key")
}

func TestRotation_UnreadablePreviousKeyIsAnError(t *testing.T) {
	t.Parallel()

	priv, pub, _ := writeKeypair(t)
	svc := serviceFor(t, priv, pub)

	require.Error(t, svc.WithPreviousKey(filepath.Join(t.TempDir(), "nope.pem")))
}

// TestValidate_RejectsAnUnknownKid: falling back to trying every key would
// quietly undo a rotation, and turns key selection into an oracle.
func TestValidate_RejectsAnUnknownKid(t *testing.T) {
	t.Parallel()

	privPath, pubPath, priv := writeKeypair(t)
	svc := serviceFor(t, privPath, pubPath)

	// A token signed with the right key but claiming a kid nobody knows.
	claims := jwt.MapClaims{
		"sub": "u1", "cls": string(domain.ClassAccess), "scope": "access",
		"iss": testIssuer, "aud": testAudience,
		"iat": time.Now().Unix(), "exp": time.Now().Add(time.Minute).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = "not-a-key-we-have"
	signed, err := tok.SignedString(priv)
	require.NoError(t, err)

	_, err = svc.Validate(signed, domain.ClassAccess)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "unknown key id"),
		"expected an unknown-kid error, got: %v", err)
}

// TestValidate_AcceptsATokenWithNoKid keeps tokens minted before this change
// working until they expire. The path disappears on its own.
func TestValidate_AcceptsATokenWithNoKid(t *testing.T) {
	t.Parallel()

	privPath, pubPath, priv := writeKeypair(t)
	svc := serviceFor(t, privPath, pubPath)

	claims := jwt.MapClaims{
		"sub": "u1", "cls": string(domain.ClassAccess), "scope": "access",
		"iss": testIssuer, "aud": testAudience,
		"iat": time.Now().Unix(), "exp": time.Now().Add(time.Minute).Unix(),
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(priv)
	require.NoError(t, err)

	_, err = svc.Validate(signed, domain.ClassAccess)
	require.NoError(t, err, "a legacy token without a kid header was rejected")
}
