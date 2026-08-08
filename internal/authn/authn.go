// Package authn defines the authentication contract for this repository: who
// the caller is (Principal), the shape every authentication middleware has
// (Middleware), and the context plumbing that carries one to the other.
//
// It is deliberately the smallest package in the tree. It imports context and
// net/http and nothing else — no ORM, no router, no business module — so any
// module can depend on it without inheriting a dependency graph, and so
// replacing the authentication provider means writing a new Middleware
// instead of rewriting every consumer. Before this package existed the
// contract was implicit: a bare func(http.Handler) http.Handler in one
// module's config, plus an unexported context key owned by another module —
// an import edge that was not an import, and therefore invisible to depguard
// and to tools/archtest.
//
// The admission rule for Principal, which is the thing that keeps this
// package small:
//
// A field enters Principal only when two or more consumer modules need it; everything else rides in Extra or a wrapper middleware.
//
// What is absent is absent on purpose, and stays absent until a second
// consumer asks for it:
//
//   - No Validate(token). Token format is an implementation detail of the
//     provider, not part of the contract.
//   - No Authorizer, no roles, no permissions. No consumer needs one today.
//   - Extra is map[string]string rather than map[string]any, which forces
//     custom claims to stay serializable and comparable. A consumer that
//     needs richer data writes a wrapper middleware — the intended extension
//     path anyway.
package authn

import (
	"context"
	"net/http"
)

// Principal is the authenticated caller of the current request. It is a value
// type: FromContext hands back a copy of the struct, so assigning to a field
// of the result affects nobody else. The slice and map it carries are a
// different matter — see WithPrincipal.
type Principal struct {
	// Subject is the stable account id of the caller. It is required: a
	// Principal with an empty Subject is not authenticated, and
	// MustFromContext panics rather than let one reach a handler.
	Subject string

	// Scopes is optional. Nil and empty mean the same thing to this
	// package, which never interprets the contents.
	Scopes []string

	// Extra carries provider-specific claims. It is opaque here: this
	// package neither reads nor validates any key.
	Extra map[string]string
}

// Middleware is the shape every authentication middleware has.
//
// It is a type alias (=) and not a defined type on purpose. Existing plain
// func(http.Handler) http.Handler values — chi middleware, the middleware
// fields already on module configs — assign to it and from it with no
// conversion, so adopting the name costs nothing at any call site. A defined
// type would have made the contract slightly more self-documenting and every
// adoption a code change; the trade was made in favor of adoption.
type Middleware = func(http.Handler) http.Handler

// ctxKey is the key a Principal is stored under. It is an unexported struct
// type, so no other package can name it, construct it, or produce a key that
// compares equal to it. Collision is impossible rather than merely unlikely,
// which is not true of a string or int key.
type ctxKey struct{}

// The two ways MustFromContext can fail, kept distinct so a production panic
// says which one happened. "No Principal" means the route was not wired
// behind a Middleware; "empty Subject" means a Middleware ran and produced an
// unauthenticated Principal, which is a bug in that Middleware. Both name the
// package so the panic is traceable without a stack trace.
const (
	panicNoPrincipal  = "authn: no Principal in context: this handler is not behind an authn.Middleware"
	panicEmptySubject = "authn: Principal in context has an empty Subject"
)

// WithPrincipal returns a copy of ctx carrying p.
//
// Scopes and Extra are copied. The hazard being closed is a producer that
// reuses a slice or map across requests — a pooled or cached claims value,
// say: without the copy, mutating it for request two would race with request
// one still reading it on another goroutine. That is a data race across a
// trust boundary, and it is worth an allocation to make it unreachable. The
// allocation is also noise next to the signature verification the caller just
// performed to obtain p.
//
// Reads deliberately do NOT copy: FromContext and MustFromContext return the
// same slice and map every caller sees, so a handler that mutates them
// mutates what the rest of the chain sees. That is a same-request,
// same-goroutine bug in a single trust domain rather than a boundary
// crossing, and copying on every read would tax every handler on every
// request to prevent it. Treat a Principal obtained from the context as
// read-only.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	p.Scopes = cloneScopes(p.Scopes)
	p.Extra = cloneExtra(p.Extra)
	return context.WithValue(ctx, ctxKey{}, p)
}

// FromContext returns the Principal carried by ctx, and whether there was
// one.
//
// The bool distinguishes "no Middleware ran" from "a Middleware ran and
// stored a zero Principal": the first returns (Principal{}, false), the
// second (Principal{}, true). The comma-ok type assertion is what separates
// them — an absent key yields an untyped nil that fails the assertion, while
// a stored zero value is a non-nil any holding a Principal that passes it.
func FromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(ctxKey{}).(Principal)
	return p, ok
}

// MustFromContext returns the Principal carried by ctx and panics if there is
// none, or if the one it finds has an empty Subject.
//
// It is for handlers that are only ever routed behind a Middleware, where an
// absent or unauthenticated Principal is a wiring mistake and not a runtime
// condition. Panicking is the point: the alternative is a handler that
// silently treats "" as an account id. Handlers on routes that are reachable
// both with and without authentication should call FromContext instead.
func MustFromContext(ctx context.Context) Principal {
	p, ok := FromContext(ctx)
	if !ok {
		panic(panicNoPrincipal)
	}
	if p.Subject == "" {
		panic(panicEmptySubject)
	}
	return p
}

// cloneScopes copies s, preserving nil. Hand-rolled rather than slices.Clone
// to keep this package's imports at context and net/http.
func cloneScopes(s []string) []string {
	if s == nil {
		return nil
	}
	out := make([]string, len(s))
	copy(out, s)
	return out
}

// cloneExtra copies m, preserving nil. Hand-rolled rather than maps.Clone for
// the same reason as cloneScopes.
func cloneExtra(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
