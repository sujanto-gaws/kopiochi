package user

import (
	"net/http"
	"testing"

	"github.com/sujanto-gaws/kopiochi/internal/module"
)

// TestNew_FailsClosedWithoutAuthMiddleware is the security property that
// justified giving this module a constructor at all. Every route it exposes
// serves user records, and all of them are meant to require authentication.
// A module that constructs successfully with no auth middleware would mount
// those routes wide open, and nothing downstream would notice — the route
// table would look identical.
//
// So the absence of a middleware must be a construction error, not a
// zero-value that quietly degrades to "no protection".
func TestNew_FailsClosedWithoutAuthMiddleware(t *testing.T) {
	m, err := New(module.Deps{}, Config{})
	if err == nil {
		t.Fatalf("New succeeded with no auth middleware, returning module %q; it must fail closed", m.Name)
	}
}

// TestNew_BuildsWithAuthMiddleware is the control: the failure above must be
// specifically about the missing middleware, not about New rejecting a nil
// DB or anything else incidental. A nil bun.IDB is fine at construction time
// — the repository only dereferences it when a request actually runs a
// query, which is the same thing cmd/api's route-table test relies on.
func TestNew_BuildsWithAuthMiddleware(t *testing.T) {
	noop := func(next http.Handler) http.Handler { return next }

	m, err := New(module.Deps{}, Config{AuthMiddleware: noop})
	if err != nil {
		t.Fatalf("New failed with a valid auth middleware: %v", err)
	}
	if m.Name != "user" {
		t.Errorf("module name = %q, want %q", m.Name, "user")
	}
	if m.Routes == nil {
		t.Error("module has no Routes func; it would mount nothing")
	}
}
