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

// authnPkg is the authentication contract: who the caller is, expressed as a
// Principal and a Middleware, with no opinion about how the caller proved it.
const authnPkg = internalPrefix + "authn"

// authnAreas lists the only parts of the tree allowed to import authnPkg.
//
// Each entry names an *area*, and an area covers the package itself plus
// everything beneath it. "modules/*" expands the star across exactly one
// segment — the module name — so it covers modules/user and
// modules/user/transport alike. See underArea.
//
// Why these four and nothing else:
//
//   - modules/* — the consumers. A module's transport layer depends on a
//     Principal instead of on whichever context key identity happens to use
//     today, which is the whole reason the contract exists.
//   - internal/httpx — owns the canonical 401 the contract's rejection path
//     produces. It does not import authn today (Unauthorized deliberately
//     takes only w and r), and is listed so that the day it needs to, the
//     fence permits it rather than being widened under time pressure.
//   - internal/testsupport — FakeAuth returns an authn.Middleware, so it
//     cannot be written without this import.
//   - internal/authn/authntest — the conformance suite takes an
//     authn.Middleware as its subject.
//
// Note the list does not include cmd/**. The composition root wires identity's
// middleware into a module's config today by inference, never naming the type,
// so it needs no entry; if that changes, widening the fence is a decision to
// take deliberately rather than a diff to wave through.
var authnAreas = []string{
	modulePrefix + "*",
	internalPrefix + "httpx",
	internalPrefix + "testsupport",
	authnPkg + "/authntest",
}

// TestOnlyDesignatedPackagesImportAuthn fences the authentication contract.
//
// internal/authn is the kernel every module's transport rests on, and a
// contract is only replaceable while the set of things that know about it
// stays small and named. An import from anywhere else is how the SPI stops
// being an SPI: the infrastructure layer starts reading Principals, the
// domain grows an opinion about bearer tokens, and swapping the identity
// implementation for another one stops being a composition-root decision.
//
// Unlike the depguard rules in .golangci.yml, which match file globs and so
// only police the layers someone thought to list, this walks the real import
// graph and therefore covers packages that do not exist yet.
func TestOnlyDesignatedPackagesImportAuthn(t *testing.T) {
	pkgs := loadPackages(t, "github.com/sujanto-gaws/kopiochi/...")

	var edges int
	for _, p := range pkgs {
		importer := normalize(p.PkgPath)
		// "" is the synthetic test main. A package importing itself is what
		// an external _test package looks like after normalization, and
		// internal/authn is trivially allowed to be internal/authn.
		if importer == "" || importer == authnPkg {
			continue
		}

		for imp := range p.Imports {
			if normalize(imp) != authnPkg {
				continue
			}
			edges++
			if mayImportAuthn(importer) {
				continue
			}
			t.Errorf("%s imports %s: only modules/*, internal/httpx, internal/testsupport and "+
				"internal/authn/authntest may depend on the authentication contract (R1/R3). "+
				"Accept an authn.Middleware or an authn.Principal as a parameter and let "+
				"cmd/api/container.go supply it, rather than importing the contract here.",
				p.PkgPath, imp)
		}
	}

	// No importer at all means the rule inspected nothing and would keep
	// passing however the fence were broken — the same vacuous green the
	// layer tests above guard against.
	if edges == 0 {
		t.Fatal("no package imports " + authnPkg + "; this rule proves nothing and the contract may have moved")
	}
}

// mayImportAuthn reports whether pkgPath falls inside one of authnAreas.
func mayImportAuthn(pkgPath string) bool {
	for _, area := range authnAreas {
		if underArea(area, pkgPath) {
			return true
		}
	}
	return false
}

// underArea reports whether pkgPath is area or a package beneath it, with "*"
// matching exactly one path segment.
//
// Recursion is deliberate and uniform: an area is a region of the tree, and a
// subpackage of a permitted region is part of that region — modules/user and
// modules/user/transport are both "a module", internal/testsupport/sub is
// still test support. The alternative, exact matching, fences on a package
// list rather than on a boundary, so the first subpackage anyone adds gets
// flagged for being new rather than for being wrong.
//
// Matching is segment-by-segment because a string prefix would let
// internal/httpxfoo pass as internal/httpx.
func underArea(area, pkgPath string) bool {
	areaSegs := strings.Split(area, "/")
	pkgSegs := strings.Split(pkgPath, "/")
	if len(pkgSegs) < len(areaSegs) {
		return false
	}
	for i, seg := range areaSegs {
		if seg != "*" && seg != pkgSegs[i] {
			return false
		}
	}
	return true
}

// TestMayImportAuthnSemantics pins what the fence actually permits, because
// the rule above only fails when the tree is wrong: a matcher that quietly
// permitted everything, or nothing beyond the exact four strings, would look
// identical in a green run. The prefix cases are the ones worth having —
// "internal/httpxfoo" is a real way for a string-prefix matcher to hand out
// access to a package nobody listed.
func TestMayImportAuthnSemantics(t *testing.T) {
	const repo = "github.com/sujanto-gaws/kopiochi/"

	cases := []struct {
		pkg  string
		want bool
	}{
		// modules/* is recursive: the module root and every layer under it.
		{"modules/user", true},
		{"modules/user/transport", true},
		{"modules/identity/transport/handlers", true},
		{"modules/identity/infrastructure/persistence/repository", true},
		{"modules", false},

		// The three named internal areas, each recursive.
		{"internal/httpx", true},
		{"internal/httpx/middleware", true},
		{"internal/testsupport", true},
		{"internal/testsupport/sub", true},
		{"internal/authn/authntest", true},
		{"internal/authn/authntest/fixture", true},

		// Segment matching, not string prefixes.
		{"internal/httpxfoo", false},
		{"internal/testsupportive", false},
		{"modulesfoo/bar", false},

		// internal/authn itself is not an area; the rule skips it as a
		// self-import rather than matching it here.
		{"internal/authn", false},

		// Everything else, including the composition root.
		{"internal/db", false},
		{"internal/config", false},
		{"internal/platform/secret", false},
		{"cmd/api", false},
		{"tools/archtest", false},
		{"internal", false},
	}

	for _, tc := range cases {
		if got := mayImportAuthn(repo + tc.pkg); got != tc.want {
			t.Errorf("mayImportAuthn(%q) = %v, want %v", tc.pkg, got, tc.want)
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
//
// internal/authn is on the list because TestOnlyDesignatedPackagesImportAuthn
// cannot put it there. That rule permits modules/* recursively — it has to,
// since a module types its middleware at the module root — so it says who may
// import the contract, not which layer may. Without this entry a
// modules/*/domain package could import authn with nothing objecting: it
// compiles, depguard's domain-purity rule denies only bun/chi/viper/zerolog,
// and the fence sees a permitted area. "The domain grows an opinion about
// bearer tokens" is what that fence's doc claims to prevent; this is the half
// of it that actually does.
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
		authnPkg:                   "the authentication contract",
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

// TestInfrastructureLayerDoesNotTouchApplication enforces the infrastructure row
// of R1, which until now was written down and checked by nothing —
// TestApplicationLayerDoesNotTouchInfrastructure guards the OPPOSITE direction,
// and E11 found the gap.
//
// The rule is not symmetry for its own sake. Infrastructure holds the adapters:
// repositories, senders, external clients. Each one implements a port declared
// by an inner layer, and the whole point of that inversion is that the adapter
// knows nothing about the use cases driving it. An adapter that can import
// application can call a use case, which turns the dependency ring into a cycle
// at runtime even while every file still compiles — a sender that reacts to a
// failure by re-entering the dispatch cycle is the shape to fear.
//
// This is checked at the import level, so it cannot distinguish "names a port
// type" from "calls a service". That is precisely why the ports an adapter must
// name live in domain rather than application: see domain.RenderedMessage and
// domain.ErrNonRetryable, and E11 for the decision. If this rule is ever
// relaxed, nothing mechanical replaces it.
func TestInfrastructureLayerDoesNotTouchApplication(t *testing.T) {
	pkgs := loadPackages(t, modulePrefix+"...")

	var checked int
	for _, p := range pkgs {
		if !isLayer(p.PkgPath, "infrastructure") {
			continue
		}
		checked++

		owner := moduleOf(p.PkgPath)
		if owner == "" {
			continue
		}
		appPrefix := modulePrefix + owner + "/application"
		for imp := range p.Imports {
			if imp == appPrefix || strings.HasPrefix(imp, appPrefix+"/") {
				t.Errorf("%s imports %s: infrastructure implements ports declared in domain and must not depend on application (R1, E11)", p.PkgPath, imp)
			}
		}
	}

	if checked == 0 {
		t.Fatal("no modules/*/infrastructure package was inspected; the layer detection is broken and this test proves nothing")
	}
}

// TestApplicationLayerDoesNotTouchInfrastructure enforces the application row
// of R1: application services talk to the domain interfaces, and the
// composition root decides which implementations satisfy them. An application
// package that imports the ORM or the router has bypassed that inversion.
//
// internal/authn is denied here for the same reason as in the domain rule, and
// with an extra one specific to this layer: a use case that reads a Principal
// is taking its caller identity from the HTTP request rather than from its own
// arguments, which makes it untestable without a request and unusable from any
// other entry point. Transport resolves the Principal and passes the subject
// down as a parameter. See TestDomainLayerStaysPure for why
// TestOnlyDesignatedPackagesImportAuthn cannot express this.
func TestApplicationLayerDoesNotTouchInfrastructure(t *testing.T) {
	forbidden := map[string]string{
		"github.com/uptrace/bun":   "the ORM",
		"github.com/go-chi/chi/v5": "the HTTP router",
		"github.com/jackc/pgx/v5":  "the database driver",
		authnPkg:                   "the authentication contract",
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
