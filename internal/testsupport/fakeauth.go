package testsupport

import (
	"net/http"

	"github.com/sujanto-gaws/kopiochi/internal/authn"
)

// This file is the reason a handler test no longer needs cryptography.
//
// Before it, proving that a CRUD handler rejects a caller who does not own the
// row meant generating an RSA keypair, building a config around it, minting a
// signed token and mounting the real identity middleware — four moving parts,
// none of which the test was about, and all of which could fail for reasons
// having nothing to do with ownership. What the handler actually depends on is
// an authn.Principal in the request context. So that is what these helpers
// put there.
//
// Nothing here imports golang-jwt, crypto or the identity module, and nothing
// here should ever start to. A test that needs a real signed token is testing
// the token verifier, and belongs behind MintToken in auth.go — see the
// package doc in db.go for why internal/** may not reach into modules/**.

// panicEmptySubject is what FakeAuth panics with. See FakeAuth for why it
// panics at all.
const panicEmptySubject = "testsupport: FakeAuth called with an empty subject; " +
	"a Principal with an empty Subject is not authenticated — " +
	"use FakeAuthPrincipal if you mean to inject one"

// FakeAuth returns a middleware that authenticates every request as subject.
//
// It is the common case: a handler test that needs a caller, not a token.
//
//	h := chi.NewRouter()
//	h.With(testsupport.FakeAuth("u-123")).Get("/me", handler)
//
// The return type is authn.Middleware, which is a type alias for
// func(http.Handler) http.Handler, so the result assigns directly to a
// module's middleware config field under either spelling and needs no
// conversion.
//
// # Why an empty subject panics
//
// FakeAuth("") is a mistake in essentially every case — a typo, or a subject
// variable that was never assigned. Passing it through would be quietly
// destructive: the fake would inject a Principal that authn itself considers
// unauthenticated, and the failure would arrive later, from a different
// package, as a panic inside whatever handler first called
// authn.MustFromContext. The test author would be reading a stack trace about
// authn to learn about a typo on their own line 3.
//
// So the check happens here, when the middleware is constructed, and reports
// the mistake in the caller's own frame. Panicking rather than taking a
// *testing.T and calling t.Fatal is deliberate on two counts: it keeps the
// signature the one the plan published and the rest of the tree is written
// against, and it applies exactly the policy authn already chose for this
// same condition one step downstream — an empty Subject is a programming
// error, and programming errors panic.
//
// A test that genuinely wants an empty Subject — to reproduce what a broken
// middleware does — uses FakeAuthPrincipal, which carries no such guard.
func FakeAuth(subject string) authn.Middleware {
	if subject == "" {
		panic(panicEmptySubject)
	}
	return FakeAuthPrincipal(authn.Principal{Subject: subject})
}

// FakeAuthPrincipal returns a middleware that authenticates every request as
// p. It is for tests that need scopes or provider claims, which FakeAuth
// deliberately leaves nil:
//
//	testsupport.FakeAuthPrincipal(authn.Principal{
//	    Subject: "u-123",
//	    Scopes:  []string{"user:write"},
//	})
//
// p is injected verbatim. There is no validation and no defaulting, including
// none for an empty Subject: this is the literal injector, and a helper that
// second-guessed the struct it was handed would be no use for reproducing what
// a misbehaving middleware produces. FakeAuth is the variant with an opinion.
//
// Every request gets its own copy of p's Scopes and Extra, because
// authn.WithPrincipal copies them on the way in. That matters here more than
// in production: one middleware value typically serves an entire table test,
// so without the copy a handler that appended to Scopes would change what the
// next case sees, and the resulting failure would look like flakiness.
//
// The middleware writes nothing to the response and recovers nothing: a
// handler panic propagates to the test, which is where it is useful.
func FakeAuthPrincipal(p authn.Principal) authn.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(authn.WithPrincipal(r.Context(), p)))
		})
	}
}
