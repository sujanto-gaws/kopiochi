package token

import (
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
)

// JWK is a public RSA key in the JSON Web Key form of RFC 7517.
type JWK struct {
	Kty string `json:"kty"` // key type: RSA
	Use string `json:"use"` // "sig" — signature verification, not encryption
	Alg string `json:"alg"` // RS256
	Kid string `json:"kid"` // matches the "kid" header on tokens signed with it
	N   string `json:"n"`   // modulus, base64url, unpadded
	E   string `json:"e"`   // exponent, base64url, unpadded
}

// JWKS is a JSON Web Key Set.
type JWKS struct {
	Keys []JWK `json:"keys"`
}

// Kid computes the RFC 7638 thumbprint of an RSA public key.
//
// Derived from the key rather than assigned, which is what makes rotation
// work without coordination: the same key always produces the same kid in
// every process that loads it, so a verifier and a signer agree without
// sharing configuration. An assigned identifier has to be kept in sync by
// hand, and a mismatch there fails every token with no obvious cause.
//
// The construction is fixed by the RFC: the JSON object of the *required*
// members only ("e", "kty", "n"), lexicographically ordered, no whitespace,
// SHA-256, base64url without padding. Adding a member or reordering changes
// the result, so this is built literally rather than through a struct.
func Kid(pub *rsa.PublicKey) string {
	n := base64url(pub.N.Bytes())
	e := base64url(exponentBytes(pub.E))

	canonical := fmt.Sprintf(`{"e":%q,"kty":"RSA","n":%q}`, e, n)
	sum := sha256.Sum256([]byte(canonical))
	return base64url(sum[:])
}

// toJWK renders a public key as a JWK.
func toJWK(pub *rsa.PublicKey) JWK {
	return JWK{
		Kty: "RSA",
		Use: "sig",
		Alg: rs256Alg,
		Kid: Kid(pub),
		N:   base64url(pub.N.Bytes()),
		E:   base64url(exponentBytes(pub.E)),
	}
}

// JWKS returns the key set a client needs to verify this service's tokens.
//
// It contains every key the service will *accept*, not only the one it signs
// with — during a rotation both are live, and a client that fetched the set
// once must be able to verify tokens issued on either side of the switch.
//
// Only public material appears here. There is no code path in this package
// that can put a private key into a JWK, which is deliberate: the endpoint is
// unauthenticated by design.
func (s *JWTService) JWKS() JWKS {
	keys := make([]JWK, 0, len(s.verificationKeys))

	// Signing key first. Clients that ignore kid and try keys in order then
	// succeed on the first attempt for the overwhelming majority of tokens.
	if s.publicKey != nil {
		keys = append(keys, toJWK(s.publicKey))
	}
	activeKid := s.signingKid

	for kid, pub := range s.verificationKeys {
		if kid == activeKid {
			continue
		}
		keys = append(keys, toJWK(pub))
	}
	return JWKS{Keys: keys}
}

// MarshalJSON is defined so an empty set serialises as {"keys":[]} rather than
// {"keys":null}. A client iterating the array breaks on null.
func (s JWKS) MarshalJSON() ([]byte, error) {
	type alias JWKS
	if s.Keys == nil {
		s.Keys = []JWK{}
	}
	return json.Marshal(alias(s))
}

func base64url(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

// exponentBytes renders the public exponent as the shortest big-endian byte
// string, which is what RFC 7518 requires — 65537 is "AQAB", three bytes, not
// a zero-padded eight.
func exponentBytes(e int) []byte {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(e))

	i := 0
	for i < len(buf)-1 && buf[i] == 0 {
		i++
	}
	return buf[i:]
}
