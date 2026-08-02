package main

import (
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

// TestBuildApp_RegistersModules is task 1.1(a) from the remediation plan
// (docs/architectures/06-quality/testing-strategy.md:71-76): the guard
// against an empty container that would start, log "modules registered",
// answer the health check, and serve nothing.
//
// It reuses testConfig(t) from routes_test.go rather than building a second
// way to construct a valid *config.Config.
func TestBuildApp_RegistersModules(t *testing.T) {
	t.Parallel()

	cfg := testConfig(t)

	app, err := BuildApp(cfg, nil, zerolog.Nop())
	require.NoError(t, err)
	require.NotEmpty(t, app.Modules)
}

// TestBuildApp_FailsOnInvalidConfig asserts that BuildApp surfaces a module's
// config validation failure as a wrapped error instead of returning a
// partially-built or empty App. An empty Issuer fails identity.Config.
// Validate (modules/identity/module.go), which is the first module BuildApp
// constructs.
//
// This test does NOT exercise the "no modules registered: refusing to start
// an empty application" guard at cmd/api/container.go:66-68 — see the
// package-level finding below. Writing a test that claims to cover that
// guard when it cannot fail would be worse than not testing it at all, so
// this test is scoped to what BuildApp actually does today: propagate a
// module build error.
func TestBuildApp_FailsOnInvalidConfig(t *testing.T) {
	t.Parallel()

	cfg := testConfig(t)
	cfg.Auth.Issuer = "" // identity.Config.Validate rejects an empty issuer

	app, err := BuildApp(cfg, nil, zerolog.Nop())
	require.Error(t, err)
	require.Nil(t, app)
	require.Contains(t, err.Error(), "issuer is required")
}

// FINDING (not fixed here — tests only, per task scope): the zero-module
// guard at cmd/api/container.go:66-68
//
//	if len(mods) == 0 {
//	    return nil, errors.New("no modules registered: refusing to start an empty application")
//	}
//
// is currently unreachable. BuildApp appends identityMod and userMod to mods
// immediately after each builds successfully, and returns early (before
// reaching the guard) on any build error. So by the time execution reaches
// `len(mods) == 0`, either the function has already returned via one of the
// two `return nil, fmt.Errorf(...)` paths, or both appends have already
// happened and len(mods) == 2. There is no config or code path in the
// current source that reaches the guard with an empty mods slice — it would
// only become reachable if a module were conditionally added (e.g. behind a
// feature flag) without an early return on the "not added" branch. No test
// is written against this guard directly because none can be made to fail
// against the current implementation; a test asserting NoError still passes
// through the (correct) module-building logic, not through the guard being
// meaningfully exercised.
