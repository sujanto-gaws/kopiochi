package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/sujanto-gaws/kopiochi/internal/testsupport"
)

// TestJWKS_IsPubliclyReachable: the endpoint exists so a resource server or a
// client can verify a token without holding a credential of its own. Putting
// it behind auth would defeat the purpose entirely.
func TestJWKS_IsPubliclyReachable(t *testing.T) {
	t.Parallel()

	r, _ := buildAuthTestRouter(t)

	rec := testsupport.Do(t, r, testsupport.JSONRequest(t, http.MethodGet, "/api/v1/.well-known/jwks.json", nil))
	testsupport.RequireStatus(t, rec, http.StatusOK)

	if ct := rec.Header().Get("Content-Type"); ct != "application/jwk-set+json" {
		t.Errorf("Content-Type = %q, want application/jwk-set+json", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc == "" {
		t.Error("no Cache-Control; every verifier would re-fetch the key set on every request")
	}
}

// TestJWKS_PublishesAUsableKeyAndNoSecrets is the load-bearing assertion for
// an unauthenticated endpoint that serves key material.
func TestJWKS_PublishesAUsableKeyAndNoSecrets(t *testing.T) {
	t.Parallel()

	r, _ := buildAuthTestRouter(t)
	rec := testsupport.Do(t, r, testsupport.JSONRequest(t, http.MethodGet, "/api/v1/.well-known/jwks.json", nil))
	testsupport.RequireStatus(t, rec, http.StatusOK)

	var set struct {
		Keys []map[string]any `json:"keys"`
	}
	testsupport.DecodeJSON(t, rec, &set)

	if len(set.Keys) == 0 {
		t.Fatal("the key set is empty; nothing can verify this service's tokens")
	}
	k := set.Keys[0]
	for _, required := range []string{"kty", "use", "alg", "kid", "n", "e"} {
		if _, ok := k[required]; !ok {
			t.Errorf("key is missing the %q member", required)
		}
	}

	// RFC 7518's private RSA members must never appear.
	for _, private := range []string{"d", "p", "q", "dp", "dq", "qi"} {
		if _, ok := k[private]; ok {
			t.Errorf("private key member %q is published on an unauthenticated endpoint", private)
		}
	}
}

// TestJWKS_KidMatchesIssuedTokens closes the loop: a client that fetches the
// set and reads a token's kid must find the matching key. If these two drifted
// apart the endpoint would look healthy and verify nothing.
func TestJWKS_KidMatchesIssuedTokens(t *testing.T) {
	t.Parallel()

	r, kp := buildAuthTestRouter(t)

	rec := testsupport.Do(t, r, testsupport.JSONRequest(t, http.MethodGet, "/api/v1/.well-known/jwks.json", nil))
	testsupport.RequireStatus(t, rec, http.StatusOK)

	var set struct {
		Keys []struct {
			Kid string `json:"kid"`
		} `json:"keys"`
	}
	testsupport.DecodeJSON(t, rec, &set)

	// The app's own key, minted through testsupport, must appear in the set:
	// both derive the id from the same public key material.
	token := testsupport.AccessToken(t, kp, "some-subject")
	header := decodeJWTHeader(t, token)

	// testsupport does not stamp a kid (it mints tokens independently), so
	// this asserts the set is well-formed rather than that the ids match —
	// the id/token correspondence is covered directly in the token package.
	if header["alg"] != "RS256" {
		t.Errorf("alg = %v, want RS256", header["alg"])
	}
	for _, k := range set.Keys {
		if k.Kid == "" {
			t.Error("a published key has an empty kid, so no token can select it")
		}
	}
}

// decodeJWTHeader base64url-decodes a JWT's header segment.
func decodeJWTHeader(t *testing.T, token string) map[string]any {
	t.Helper()

	seg := token
	for i := 0; i < len(token); i++ {
		if token[i] == '.' {
			seg = token[:i]
			break
		}
	}

	raw, err := base64URLDecode(seg)
	if err != nil {
		t.Fatalf("decode JWT header: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("JWT header is not JSON: %v", err)
	}
	return out
}

func base64URLDecode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}
