package transport

import (
	"encoding/json"
	"net/http"

	"github.com/sujanto-gaws/kopiochi/modules/identity/infrastructure/token"
)

// KeySetProvider exposes the public keys this service's tokens can be verified
// with. *token.JWTService satisfies it.
//
// The concrete token.JWKS type is used rather than `any` so the handler cannot
// be handed something that merely happens to have a JWKS method — this
// endpoint publishes key material, and "it compiled" is not the assurance
// wanted there.
type KeySetProvider interface {
	JWKS() token.JWKS
}

// jwksMaxAge is how long a client may cache the key set.
//
// Long enough that a busy verifier is not re-fetching constantly, short enough
// that a rotation propagates within a coffee break. It is the floor on how
// quickly a compromised key can be pulled out of circulation for clients that
// honour caching, so it is deliberately not hours.
const jwksMaxAge = "public, max-age=300"

// JWKSHandler serves RFC 7517 key set for token verification.
//
// Unauthenticated by design: the whole point is that a resource server or a
// client can verify a token without holding a credential of its own, and
// requiring one to fetch public keys would defeat that. Only public material
// is ever rendered — the JWK type in the token package has no field a private
// key could be written into.
func JWKSHandler(keys KeySetProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/jwk-set+json")
		w.Header().Set("Cache-Control", jwksMaxAge)
		w.WriteHeader(http.StatusOK)

		// Status and headers are committed; a failed encode leaves a truncated
		// body and nothing actionable.
		_ = json.NewEncoder(w).Encode(keys.JWKS())
	}
}
