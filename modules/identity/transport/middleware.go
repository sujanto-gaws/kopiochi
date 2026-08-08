package transport

import (
	"net/http"
	"strings"

	"github.com/sujanto-gaws/kopiochi/internal/authn"
	"github.com/sujanto-gaws/kopiochi/internal/httpx"
	domain "github.com/sujanto-gaws/kopiochi/modules/identity/domain"
)

// AuthRequired returns the middleware that turns a verified access token into
// an authn.Principal on the request context.
//
// The return type is authn.Middleware, which is a type alias for
// func(http.Handler) http.Handler and therefore assignable anywhere the bare
// func type was accepted before. Naming it is the point: this module no longer
// publishes "a middleware plus whatever context key identity happens to use",
// it publishes an implementation of a contract that lives in internal/authn,
// where every consumer can see it.
//
// Every rejection goes through httpx.Unauthorized, which emits one canonical
// problem+json body regardless of *why* the credential failed. That uniformity
// is the security property: a per-reason body tells an attacker which tokens
// are structurally valid and which are merely stale.
func AuthRequired(tokenIssuer domain.TokenIssuer) authn.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") {
				httpx.Unauthorized(w, r)
				return
			}
			tokenStr := strings.TrimPrefix(header, "Bearer ")
			// want=domain.ClassAccess makes it structurally impossible for a
			// token minted for any other purpose (e.g. the short-lived MFA
			// token) to be accepted here -- Validate rejects a class
			// mismatch itself, rather than this middleware inspecting a
			// convention-only field.
			claims, err := tokenIssuer.Validate(tokenStr, domain.ClassAccess)
			if err != nil {
				httpx.Unauthorized(w, r)
				return
			}
			// A token that verifies but carries no "sub" is not an
			// authenticated caller, and this is the only place that can say
			// so with a 401. Letting it through would store a zero-Subject
			// Principal, which authn.MustFromContext panics on -- turning a
			// bad credential into a 500. The check used to live in
			// AuthHandler.Logout; it belongs here, ahead of the context
			// write, so that every consumer of this middleware inherits it
			// rather than re-implementing it.
			if claims.Subject == "" {
				httpx.Unauthorized(w, r)
				return
			}
			// Scopes is deliberately left nil. The "scope" claim on an access
			// token this service issues is the constant "access" (see
			// infrastructure/token.IssueAccessToken) -- a duplicate of the
			// token class, not an authorization grant. domain.Class's own
			// documentation names "scope" as the convention-only field that
			// must not be used to decide what a token is for, so copying it
			// into an authorization-shaped contract field would rebuild the
			// hazard the class check exists to remove. Extra is left nil too:
			// nothing consumes it, and authn.WithPrincipal's clone is
			// allocation-free only while both are nil.
			p := authn.Principal{Subject: claims.Subject}
			next.ServeHTTP(w, r.WithContext(authn.WithPrincipal(r.Context(), p)))
		})
	}
}
