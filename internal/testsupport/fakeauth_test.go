package testsupport_test

import (
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sujanto-gaws/kopiochi/internal/authn"
	"github.com/sujanto-gaws/kopiochi/internal/testsupport"
)

// These helpers are about to be load-bearing for every transport test in the
// tree, so they get real tests: a fake that silently injects the wrong subject
// — or no subject at all — turns every ownership check downstream green
// without proving anything.
//
// The test package is external (testsupport_test) on purpose. These assertions
// are written from the position of a consumer, against the exported surface
// only, and the panic message is asserted as a literal rather than through the
// package's own constant so that rewording it is a visible change here.

// capture is the downstream handler in most of these tests: it records what,
// if anything, reached it.
type capture struct {
	called    bool
	principal authn.Principal
	present   bool
}

func (c *capture) handler() http.Handler {
	return http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		c.called = true
		c.principal, c.present = authn.FromContext(r.Context())
	})
}

// serve runs one request through mw and h.
//
// mw is typed as authn.Middleware rather than as a bare
// func(http.Handler) http.Handler so that every call below doubles as an
// assignment check against the contract's own name. A separate "satisfies the
// interface" test would prove nothing on top of this: authn.Middleware is a
// type *alias*, so the two spellings are one type and the compiler cannot tell
// them apart.
func serve(t *testing.T, mw authn.Middleware, h http.Handler) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	mw(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	return rec
}

func TestFakeAuthInjectsTheSubject(t *testing.T) {
	var got capture
	serve(t, testsupport.FakeAuth("u-123"), got.handler())

	require.True(t, got.called, "the downstream handler was never reached")
	require.True(t, got.present, "no Principal reached the handler")
	require.Equal(t, "u-123", got.principal.Subject)
	require.Nil(t, got.principal.Scopes, "FakeAuth should not invent scopes")
	require.Nil(t, got.principal.Extra, "FakeAuth should not invent claims")
}

// The point of the fake is that a handler calling MustFromContext — which
// panics on an absent or unauthenticated Principal — runs clean behind it.
// FromContext passing is not enough: MustFromContext is what the handlers
// under test actually call.
func TestFakeAuthSatisfiesMustFromContext(t *testing.T) {
	var subject string
	h := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		subject = authn.MustFromContext(r.Context()).Subject
	})

	require.NotPanics(t, func() { serve(t, testsupport.FakeAuth("u-123"), h) })
	require.Equal(t, "u-123", subject)
}

// An empty subject is a mistake at the call site, and without this guard the
// consequence is a panic from authn deep inside a handler on a later line.
// The guard moves the failure to the FakeAuth("") call itself.
func TestFakeAuthRejectsAnEmptySubject(t *testing.T) {
	require.PanicsWithValue(t,
		"testsupport: FakeAuth called with an empty subject; a Principal with an empty Subject is not authenticated — use FakeAuthPrincipal if you mean to inject one",
		func() { testsupport.FakeAuth("") })
}

func TestFakeAuthPrincipalCarriesScopesAndExtra(t *testing.T) {
	want := authn.Principal{
		Subject: "u-456",
		Scopes:  []string{"user:read", "user:write"},
		Extra:   map[string]string{"tenant": "acme"},
	}

	var got capture
	serve(t, testsupport.FakeAuthPrincipal(want), got.handler())

	require.True(t, got.present)
	require.Equal(t, want.Subject, got.principal.Subject)
	require.Equal(t, want.Scopes, got.principal.Scopes)
	require.Equal(t, want.Extra, got.principal.Extra)
}

// FakeAuthPrincipal is the literal injector and deliberately carries no
// guard: it is the escape hatch a test uses to reproduce what a *broken*
// middleware does. Injecting a zero Principal must therefore succeed at
// construction and panic downstream, in authn, with authn's own message.
func TestFakeAuthPrincipalInjectsVerbatimIncludingAnEmptySubject(t *testing.T) {
	var got capture
	require.NotPanics(t, func() {
		serve(t, testsupport.FakeAuthPrincipal(authn.Principal{}), got.handler())
	})
	require.True(t, got.present, "a zero Principal is still a Principal in context")
	require.Equal(t, "", got.principal.Subject)

	h := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_ = authn.MustFromContext(r.Context())
	})
	require.PanicsWithValue(t,
		"authn: Principal in context has an empty Subject",
		func() { serve(t, testsupport.FakeAuthPrincipal(authn.Principal{}), h) })
}

// One middleware value is mounted once and serves every request in a table
// test. If the Scopes slice were shared across requests, a handler that sorted
// or appended to it would corrupt the next request's principal — and the test
// that caught it would look flaky. authn.WithPrincipal copies per request;
// this asserts the fake actually benefits from that.
func TestFakeAuthPrincipalIsolatesScopesPerRequest(t *testing.T) {
	mw := testsupport.FakeAuthPrincipal(authn.Principal{
		Subject: "u-789",
		Scopes:  []string{"original"},
		Extra:   map[string]string{"k": "original"},
	})

	first := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		p := authn.MustFromContext(r.Context())
		p.Scopes[0] = "mutated"
		p.Extra["k"] = "mutated"
	})
	serve(t, mw, first)

	var got capture
	serve(t, mw, got.handler())
	require.Equal(t, []string{"original"}, got.principal.Scopes,
		"a previous request mutated the scopes the next one sees")
	require.Equal(t, map[string]string{"k": "original"}, got.principal.Extra,
		"a previous request mutated the claims the next one sees")
}

// The fake authenticates; it does not respond. A middleware that wrote a
// header or a status would change what every handler test observes.
func TestFakeAuthWritesNothingToTheResponse(t *testing.T) {
	rec := serve(t, testsupport.FakeAuth("u-123"),
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		}))

	require.Equal(t, http.StatusTeapot, rec.Code)
	require.Empty(t, rec.Body.String())
	require.Empty(t, rec.Header().Get("WWW-Authenticate"))
	require.Empty(t, rec.Header().Values("Content-Type"))
}

// A fake that recovered a handler panic would hide the failures these tests
// exist to surface. This is also the property B3's conformance suite asserts
// of real middleware, so the fake is held to it too.
func TestFakeAuthPropagatesAHandlerPanic(t *testing.T) {
	h := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	})
	require.PanicsWithValue(t, "boom", func() { serve(t, testsupport.FakeAuth("u-123"), h) })
}

// The middleware must derive a new request rather than mutate the caller's —
// otherwise a test that reuses a *http.Request across cases leaks a principal
// into the case that is supposed to have none.
func TestFakeAuthDoesNotMutateTheIncomingRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	var got capture
	testsupport.FakeAuth("u-123")(got.handler()).ServeHTTP(httptest.NewRecorder(), req)

	require.True(t, got.present)
	_, present := authn.FromContext(req.Context())
	require.False(t, present, "the incoming request's own context was mutated")
}

// "No JWT/keypair imports in this file" is task B1's acceptance criterion, and
// a criterion checked once by hand is a criterion that decays. This checks it
// on every run.
//
// It has to read the file rather than inspect the package: auth.go is in the
// same package and legitimately imports golang-jwt and crypto/*, so any
// package-level check — go list, a dependency graph tool — reports those
// imports and cannot say which file pulled them in. The import set is asserted
// exactly, not merely filtered for "jwt", because the value at stake is that a
// handler test needs no cryptography at all: crypto/rsa, internal/config or
// the identity module would each break that promise in a different way, and an
// allowlist catches all of them including the ones nobody thought to name.
func TestFakeAuthImportsNothingButNetHTTPAndAuthn(t *testing.T) {
	path := filepath.Join(testsupport.RepoRoot(t), "internal", "testsupport", "fakeauth.go")

	parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	require.NoError(t, err)

	got := make([]string, 0, len(parsed.Imports))
	for _, spec := range parsed.Imports {
		unquoted, err := strconv.Unquote(spec.Path.Value)
		require.NoError(t, err)
		got = append(got, unquoted)
	}
	sort.Strings(got)

	require.Equal(t, []string{
		"github.com/sujanto-gaws/kopiochi/internal/authn",
		"net/http",
	}, got, "fakeauth.go must stay free of JWT, keypair and config imports; "+
		"a fake that needs cryptography is not a fake")
}
