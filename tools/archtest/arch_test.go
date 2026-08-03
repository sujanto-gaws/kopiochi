// Package archtest enforces the dependency rules in
// docs/architectures/01-modularity/dependency-rules.md as ordinary Go tests.
//
// It lives here, rather than in any of the packages it inspects, because it is
// about the shape of the whole import graph and belongs to no single package.
// Running as a plain `go test ./tools/archtest/...` means it gates CI with no
// extra tooling, and — unlike the depguard rules in .golangci.yml, which match
// on file globs — it can express "module A must not import module B"
// generically, for modules that do not exist yet.
//
// # Always run this with -count=1
//
// These tests read the whole repository through go/packages, but Go's test
// cache keys only on this package's own files and its declared dependencies.
// Introducing a violation anywhere else in the tree does not invalidate that
// cache, so a cached PASS is reported for a tree that now fails. Measured:
// adding a modules/user/domain -> modules/identity/domain import and rerunning
// `go test ./tools/archtest/...` prints "ok (cached)"; the same run with
// -count=1 fails with three violations.
//
// The Makefile `arch` target and the CI workflow both pass -count=1. Anything
// else that runs these tests must too, or the enforcement is theatre.
package archtest

import (
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

const (
	modulePrefix   = "github.com/sujanto-gaws/kopiochi/modules/"
	internalPrefix = "github.com/sujanto-gaws/kopiochi/internal/"
)

// loadPackages loads every package under the given patterns with its import
// list. Failing to load is a hard failure: a silently empty result would make
// every assertion below vacuously pass, which is the one way an architecture
// test can be worse than no test at all.
//
// Tests: true includes each package's _test.go files. This is not optional
// coverage — the first violation these rules found in anger was
// internal/db/schema_test.go importing two modules' persistence models, which
// a production-files-only load does not see at all. Test code participates in
// the import graph and can invert a dependency just as thoroughly.
func loadPackages(t *testing.T, patterns ...string) []*packages.Package {
	t.Helper()

	pkgs, err := packages.Load(&packages.Config{
		Mode:  packages.NeedName | packages.NeedImports,
		Dir:   "../..",
		Tests: true,
	}, patterns...)
	if err != nil {
		t.Fatalf("load packages: %v", err)
	}
	if len(pkgs) == 0 {
		t.Fatalf("loaded 0 packages for %v; the patterns are wrong and every assertion would pass vacuously", patterns)
	}
	for _, p := range pkgs {
		if len(p.Errors) > 0 {
			t.Fatalf("package %s failed to load: %v", p.PkgPath, p.Errors[0])
		}
	}
	return pkgs
}

// normalize maps the synthetic package paths that Tests: true produces back
// onto the real package they belong to.
//
// go/packages reports four entries per package that has tests:
//
//	example.com/p               the production package
//	example.com/p [p.test]      the same package, recompiled with its _test.go files
//	example.com/p_test [p.test] the external _test package, if any
//	example.com/p.test          the synthetic main that runs them
//
// The " [...]" suffix and the "_test" package-name suffix are compilation
// artefacts, not import paths anyone wrote. The ".test" main imports the
// package under test by definition, so counting it as a real edge reports
// every tested module as importing itself.
//
// normalize returns "" for the synthetic main, meaning "skip this entirely".
func normalize(pkgPath string) string {
	if i := strings.Index(pkgPath, " ["); i >= 0 {
		pkgPath = pkgPath[:i]
	}
	if strings.HasSuffix(pkgPath, ".test") {
		return ""
	}
	return strings.TrimSuffix(pkgPath, "_test")
}

// moduleOf returns the module a package belongs to — "identity" for
// .../modules/identity/domain — or "" if the path is not under modules/.
func moduleOf(pkgPath string) string {
	pkgPath = normalize(pkgPath)
	if pkgPath == "" {
		return ""
	}
	rest, ok := strings.CutPrefix(pkgPath, modulePrefix)
	if !ok {
		return ""
	}
	name, _, _ := strings.Cut(rest, "/")
	return name
}

// TestModulesDoNotImportEachOther enforces R2. Cross-module needs are
// expressed as an interface declared by the consumer and satisfied at the
// composition root — the way modules/user takes an http middleware in its
// Config rather than importing modules/identity to build one.
//
// Without this, the first cross-module import would compile fine and only
// reveal itself later, as a pair of modules that can no longer be changed or
// deployed independently.
func TestModulesDoNotImportEachOther(t *testing.T) {
	pkgs := loadPackages(t, modulePrefix+"...")

	for _, p := range pkgs {
		owner := moduleOf(p.PkgPath)
		if owner == "" {
			continue
		}
		for imp := range p.Imports {
			other := moduleOf(imp)
			if other != "" && other != owner {
				t.Errorf("%s imports %s: modules must not depend on each other (R2). "+
					"Declare the interface you need in %s and satisfy it in cmd/api/container.go.",
					p.PkgPath, imp, owner)
			}
		}
	}
}

// TestPlatformDoesNotImportModules enforces R3's second and third clauses:
// internal/platform, internal/httpx, internal/db and internal/config are the
// shared kernel every module is allowed to depend on, so none of them may
// depend on a module in return. Only cmd/** imports both.
//
// A single violation here inverts the dependency for the whole tree: the
// shared kernel stops being shared the moment it knows about one particular
// business capability.
func TestPlatformDoesNotImportModules(t *testing.T) {
	pkgs := loadPackages(t, internalPrefix+"...")

	for _, p := range pkgs {
		if normalize(p.PkgPath) == "" {
			continue
		}
		for imp := range p.Imports {
			if moduleOf(imp) != "" {
				t.Errorf("%s imports %s: internal/** is the shared kernel and must not depend on a business module (R3)",
					p.PkgPath, imp)
			}
		}
	}
}

// TestDomainLayerStaysPure enforces R1 for the innermost layer: a module's
// domain package may use the standard library and internal/platform, and
// nothing else. The ORM, the router, the config loader and the logger are all
// infrastructure concerns; a domain that imports any of them cannot be tested
// or reasoned about without standing up that infrastructure.
//
// depguard covers this too, but only for files matching its globs. This
// applies to every modules/*/domain package that exists, including ones added
// after the .golangci.yml globs were last thought about.
func TestDomainLayerStaysPure(t *testing.T) {
	forbidden := map[string]string{
		"github.com/uptrace/bun":   "the ORM",
		"github.com/go-chi/chi/v5": "the HTTP router",
		"github.com/spf13/viper":   "configuration loading",
		"github.com/rs/zerolog":    "the logger",
		"github.com/jackc/pgx/v5":  "the database driver",
		"net/http":                 "the HTTP transport",
		internalPrefix + "config":  "the application config",
		internalPrefix + "db":      "the database package",
		internalPrefix + "httpx":   "the HTTP layer",
		internalPrefix + "module":  "the module host contract",
	}

	pkgs := loadPackages(t, modulePrefix+"...")

	var checked int
	for _, p := range pkgs {
		if !isLayer(p.PkgPath, "domain") {
			continue
		}
		checked++

		for imp := range p.Imports {
			for bad, why := range forbidden {
				if imp == bad || strings.HasPrefix(imp, bad+"/") {
					t.Errorf("%s imports %s: the domain layer must not depend on %s (R1)", p.PkgPath, imp, why)
				}
			}
		}
	}

	if checked == 0 {
		t.Fatal("no modules/*/domain package was inspected; the layer detection is broken and this test proves nothing")
	}
}

// TestApplicationLayerDoesNotTouchInfrastructure enforces the application row
// of R1: application services talk to the domain interfaces, and the
// composition root decides which implementations satisfy them. An application
// package that imports the ORM or the router has bypassed that inversion.
func TestApplicationLayerDoesNotTouchInfrastructure(t *testing.T) {
	forbidden := map[string]string{
		"github.com/uptrace/bun":   "the ORM",
		"github.com/go-chi/chi/v5": "the HTTP router",
		"github.com/jackc/pgx/v5":  "the database driver",
	}

	pkgs := loadPackages(t, modulePrefix+"...")

	var checked int
	for _, p := range pkgs {
		if !isLayer(p.PkgPath, "application") {
			continue
		}
		checked++

		for imp := range p.Imports {
			for bad, why := range forbidden {
				if imp == bad || strings.HasPrefix(imp, bad+"/") {
					t.Errorf("%s imports %s: the application layer talks to domain interfaces, not %s (R1)", p.PkgPath, imp, why)
				}
			}
		}
		// The application layer sits above infrastructure, never beside it.
		if owner := moduleOf(p.PkgPath); owner != "" {
			infraPrefix := modulePrefix + owner + "/infrastructure"
			for imp := range p.Imports {
				if strings.HasPrefix(imp, infraPrefix) {
					t.Errorf("%s imports %s: application must not depend on its own infrastructure (R1)", p.PkgPath, imp)
				}
			}
		}
	}

	if checked == 0 {
		t.Fatal("no modules/*/application package was inspected; the layer detection is broken and this test proves nothing")
	}
}

// TestNoUtilsPackages enforces R4. `utils`, `common`, `shared`, `helpers` and
// `misc` are not boundaries; they are buckets named for their lack of a
// concept, and they accumulate unrelated code until the domain layer imports
// one of them for a single function and inherits all the rest.
//
// This is forward-looking: no such package exists today, and the point is that
// none ever appears.
func TestNoUtilsPackages(t *testing.T) {
	banned := []string{"utils", "util", "common", "shared", "helpers", "misc"}

	pkgs := loadPackages(t, "github.com/sujanto-gaws/kopiochi/...")

	for _, p := range pkgs {
		last := normalize(p.PkgPath)
		if last == "" {
			continue
		}
		if i := strings.LastIndex(last, "/"); i >= 0 {
			last = last[i+1:]
		}
		for _, b := range banned {
			if last == b {
				t.Errorf("package %s: %q is not a boundary, it is a bucket (R4). "+
					"Name the package for what it is — internal/platform/paging, internal/platform/crypto, internal/httpx.",
					p.PkgPath, b)
			}
		}
	}
}

// isLayer reports whether pkgPath is the named layer of some module, or a
// package nested inside it (modules/x/domain and modules/x/domain/whatever).
func isLayer(pkgPath, layer string) bool {
	pkgPath = normalize(pkgPath)
	if pkgPath == "" {
		return false
	}
	rest, ok := strings.CutPrefix(pkgPath, modulePrefix)
	if !ok {
		return false
	}
	_, after, ok := strings.Cut(rest, "/")
	if !ok {
		return false
	}
	return after == layer || strings.HasPrefix(after, layer+"/")
}
