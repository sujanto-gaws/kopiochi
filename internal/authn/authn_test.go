package authn

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

// Key types belonging to some other package, used to prove that only this
// package's unexported struct key can retrieve a Principal. They are defined
// types rather than bare string/int because context.WithValue with a built-in
// key type is itself a lint finding (staticcheck SA1029) — which is the same
// point this test is making.
type (
	foreignEmptyKey  struct{}
	foreignNamedKey  struct{ name string }
	foreignStringKey string
)

// isZero reports whether p is the zero Principal. Principal carries a slice
// and a map, so it is not a comparable type and == cannot be used on it.
func isZero(p Principal) bool {
	return p.Subject == "" && p.Scopes == nil && p.Extra == nil
}

// panicMessage runs fn and returns the string it panicked with. It fails the
// test if fn returns normally or panics with a non-string, because both
// panic messages are part of this package's contract and must be assertable.
func panicMessage(t *testing.T, fn func()) string {
	t.Helper()

	var got any
	returned := false
	func() {
		defer func() { got = recover() }()
		fn()
		returned = true
	}()

	if returned {
		t.Fatal("expected a panic, got a normal return")
	}
	s, ok := got.(string)
	if !ok {
		t.Fatalf("panic value %#v is not a string, so callers cannot assert on it", got)
	}
	return s
}

func TestWithPrincipal_RoundTrips(t *testing.T) {
	t.Parallel()

	want := Principal{
		Subject: "acct_123",
		Scopes:  []string{"read", "write"},
		Extra:   map[string]string{"tenant": "acme"},
	}

	got, ok := FromContext(WithPrincipal(context.Background(), want))
	if !ok {
		t.Fatal("FromContext reported no Principal right after WithPrincipal stored one")
	}
	if got.Subject != want.Subject {
		t.Errorf("Subject = %q, want %q", got.Subject, want.Subject)
	}
	if len(got.Scopes) != len(want.Scopes) {
		t.Fatalf("Scopes = %v, want %v", got.Scopes, want.Scopes)
	}
	for i := range want.Scopes {
		if got.Scopes[i] != want.Scopes[i] {
			t.Errorf("Scopes[%d] = %q, want %q", i, got.Scopes[i], want.Scopes[i])
		}
	}
	if len(got.Extra) != len(want.Extra) {
		t.Fatalf("Extra = %v, want %v", got.Extra, want.Extra)
	}
	for k, v := range want.Extra {
		if got.Extra[k] != v {
			t.Errorf("Extra[%q] = %q, want %q", k, got.Extra[k], v)
		}
	}
}

func TestFromContext_AbsentReportsFalse(t *testing.T) {
	t.Parallel()

	got, ok := FromContext(context.Background())
	if ok {
		t.Fatalf("FromContext reported a Principal on a bare context: %#v", got)
	}
	if !isZero(got) {
		t.Errorf("Principal = %#v, want the zero value when absent", got)
	}
}

// The bool must separate "no middleware ran" from "a middleware stored a zero
// Principal". Without that, a caller cannot tell an unwired route from an
// unauthenticated one, and MustFromContext could not report the two
// differently.
func TestFromContext_ZeroPrincipalIsPresentNotAbsent(t *testing.T) {
	t.Parallel()

	got, ok := FromContext(WithPrincipal(context.Background(), Principal{}))
	if !ok {
		t.Fatal("a deliberately stored zero Principal was reported as absent")
	}
	if !isZero(got) {
		t.Errorf("Principal = %#v, want the zero value", got)
	}
}

func TestFromContext_ForeignKeysDoNotCollide(t *testing.T) {
	t.Parallel()

	planted := Principal{Subject: "attacker"}
	cases := []struct {
		name string
		key  any
	}{
		{"empty struct from another package", foreignEmptyKey{}},
		{"named struct from another package", foreignNamedKey{name: "authn"}},
		{"string-backed key", foreignStringKey("authn")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.WithValue(context.Background(), tc.key, planted)
			if got, ok := FromContext(ctx); ok {
				t.Fatalf("FromContext read a Principal planted under a foreign key: %#v", got)
			}
		})
	}
}

// The producer may hold the slice and map it passed in — a pooled or cached
// claims value, for instance. Mutating them afterwards must not reach a
// request that already went through WithPrincipal.
func TestWithPrincipal_CopiesScopesAndExtra(t *testing.T) {
	t.Parallel()

	scopes := []string{"read"}
	extra := map[string]string{"tenant": "acme"}
	ctx := WithPrincipal(context.Background(), Principal{
		Subject: "acct_123",
		Scopes:  scopes,
		Extra:   extra,
	})

	scopes[0] = "admin"
	extra["tenant"] = "evil-corp"
	delete(extra, "missing")
	extra["added"] = "later"

	got, ok := FromContext(ctx)
	if !ok {
		t.Fatal("FromContext reported no Principal")
	}
	if got.Scopes[0] != "read" {
		t.Errorf("Scopes[0] = %q, want %q: the caller's slice was not copied", got.Scopes[0], "read")
	}
	if got.Extra["tenant"] != "acme" {
		t.Errorf("Extra[tenant] = %q, want %q: the caller's map was not copied", got.Extra["tenant"], "acme")
	}
	if _, present := got.Extra["added"]; present {
		t.Error("a key added to the caller's map after WithPrincipal appeared in the stored Principal")
	}
}

// Nil and empty must survive as themselves so a round trip is lossless.
func TestWithPrincipal_PreservesNilAndEmptyScopesAndExtra(t *testing.T) {
	t.Parallel()

	t.Run("nil stays nil", func(t *testing.T) {
		t.Parallel()

		got, _ := FromContext(WithPrincipal(context.Background(), Principal{Subject: "acct_123"}))
		if got.Scopes != nil {
			t.Errorf("Scopes = %#v, want nil", got.Scopes)
		}
		if got.Extra != nil {
			t.Errorf("Extra = %#v, want nil", got.Extra)
		}
	})

	t.Run("empty stays empty and non-nil", func(t *testing.T) {
		t.Parallel()

		got, _ := FromContext(WithPrincipal(context.Background(), Principal{
			Subject: "acct_123",
			Scopes:  []string{},
			Extra:   map[string]string{},
		}))
		if got.Scopes == nil || len(got.Scopes) != 0 {
			t.Errorf("Scopes = %#v, want an empty non-nil slice", got.Scopes)
		}
		if got.Extra == nil || len(got.Extra) != 0 {
			t.Errorf("Extra = %#v, want an empty non-nil map", got.Extra)
		}
	})
}

// Documents the deliberate other half of the copy decision: reads share. If
// someone adds copy-on-read, this test fails, and that is the intended
// friction — the cost is paid by every handler on every request, so the
// change should be a conscious one.
func TestFromContext_ReadsShareScopesAndExtra(t *testing.T) {
	t.Parallel()

	ctx := WithPrincipal(context.Background(), Principal{
		Subject: "acct_123",
		Scopes:  []string{"read"},
		Extra:   map[string]string{"tenant": "acme"},
	})

	first, _ := FromContext(ctx)
	first.Scopes[0] = "mutated"
	first.Extra["tenant"] = "mutated"

	second, _ := FromContext(ctx)
	if second.Scopes[0] != "mutated" || second.Extra["tenant"] != "mutated" {
		t.Fatalf("reads no longer share Scopes/Extra (got %#v); FromContext now copies, "+
			"which contradicts the documented contract — update the doc comment deliberately", second)
	}
}

// A wrapper middleware is the documented extension path for anything that
// does not belong in Principal, and wrapping means storing a second
// Principal over the first.
func TestWithPrincipal_LastWriteWins(t *testing.T) {
	t.Parallel()

	ctx := WithPrincipal(context.Background(), Principal{Subject: "first"})
	ctx = WithPrincipal(ctx, Principal{Subject: "second"})

	got, ok := FromContext(ctx)
	if !ok {
		t.Fatal("FromContext reported no Principal")
	}
	if got.Subject != "second" {
		t.Errorf("Subject = %q, want %q", got.Subject, "second")
	}
}

func TestMustFromContext_ReturnsThePrincipal(t *testing.T) {
	t.Parallel()

	got := MustFromContext(WithPrincipal(context.Background(), Principal{Subject: "acct_123"}))
	if got.Subject != "acct_123" {
		t.Errorf("Subject = %q, want %q", got.Subject, "acct_123")
	}
}

func TestMustFromContext_PanicsWhenAbsent(t *testing.T) {
	t.Parallel()

	got := panicMessage(t, func() { MustFromContext(context.Background()) })
	want := "authn: no Principal in context: this handler is not behind an authn.Middleware"
	if got != want {
		t.Errorf("panic message = %q, want %q", got, want)
	}
}

func TestMustFromContext_PanicsOnEmptySubject(t *testing.T) {
	t.Parallel()

	ctx := WithPrincipal(context.Background(), Principal{Scopes: []string{"read"}})
	got := panicMessage(t, func() { MustFromContext(ctx) })
	want := "authn: Principal in context has an empty Subject"
	if got != want {
		t.Errorf("panic message = %q, want %q", got, want)
	}
}

// The two failures have different causes — an unwired route versus a
// middleware that produced an unauthenticated Principal — so they must not
// share a message.
func TestMustFromContext_PanicMessagesAreDistinct(t *testing.T) {
	t.Parallel()

	if panicNoPrincipal == panicEmptySubject {
		t.Fatalf("both panic messages are %q; they must name their distinct causes", panicNoPrincipal)
	}
}

// Middleware must stay a type alias (=) and not become a defined type, so
// that plain func(http.Handler) http.Handler values assign in both
// directions with no conversion at any call site. An alias has no type name;
// a defined type would report "Middleware" here.
func TestMiddleware_IsATypeAliasNotADefinedType(t *testing.T) {
	t.Parallel()

	if got := reflect.TypeOf((*Middleware)(nil)).Elem().Name(); got != "" {
		t.Errorf("Middleware reports type name %q; it must remain a type alias, not a defined type", got)
	}
}

// wrap accepts a Middleware, so passing a plain func(http.Handler)
// http.Handler to it is the assignability assertion, checked by the compiler
// at the call site below.
func wrap(mw Middleware, next http.Handler) http.Handler { return mw(next) }

func TestMiddleware_CarriesAPrincipalToTheHandler(t *testing.T) {
	t.Parallel()

	plain := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := WithPrincipal(r.Context(), Principal{Subject: "acct_123"})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	seen := ""
	handler := wrap(plain, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = MustFromContext(r.Context()).Subject
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/protected", nil))

	if seen != "acct_123" {
		t.Errorf("handler saw Subject %q, want %q", seen, "acct_123")
	}
}
