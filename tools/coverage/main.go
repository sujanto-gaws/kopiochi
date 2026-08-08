// Command coverage enforces the per-package coverage policy in
// docs/architectures/06-quality/testing-strategy.md.
//
// It does two separate jobs against one `go test -coverprofile` file:
//
//   - Floors. Packages matching a pattern must meet a minimum. Deliberately
//     per-package rather than one global number: a global target is met most
//     cheaply by testing trivial getters in whatever package is largest, which
//     moves the number without covering anything that matters.
//
//   - A ratchet. Every package's coverage is recorded in a committed baseline,
//     and a drop below it fails. This is what stops the usual outcome — a
//     policy set at today's number, then quietly eroded by changes that each
//     lower it by half a point.
//
// Usage:
//
//	go run ./tools/coverage -profile coverage.out            # check
//	go run ./tools/coverage -profile coverage.out -update    # rewrite baseline
//
// The ratchet only ever moves up on its own: -update refuses to lower a
// recorded value unless -allow-decrease is given, so "just re-baseline it"
// is a visible, reviewable act rather than a reflex.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// modulePrefix is stripped from profile package paths to get repo-relative
// ones, so the policy file reads like the directory tree.
const modulePrefix = "github.com/sujanto-gaws/kopiochi/"

// tolerance absorbs floating-point noise when comparing against the baseline.
// Without it a package can "regress" by 0.0000001% and fail the build for
// nothing.
const tolerance = 0.05

// Policy is the committed coverage contract.
type Policy struct {
	// Floors are minimums by glob, most specific first. The first match wins.
	Floors []Floor `json:"floors"`
	// Exempt packages are neither floored nor ratcheted.
	Exempt []string `json:"exempt"`
	// RequiresDatabase names packages whose coverage comes from integration
	// tests. Those tests skip when no Postgres is reachable, which reports 0%
	// — indistinguishable from "the tests were deleted".
	//
	// Rather than guess, the caller states whether a database was available:
	// without -with-database these packages are reported as NOT CHECKED and
	// excluded from both the floors and the ratchet. CI passes the flag, so
	// the floors are real there. The one thing this must never do is quietly
	// pass 0% as if it had been measured.
	RequiresDatabase []string `json:"requires_database"`
	// Baseline is the ratchet: package path to the percentage last recorded.
	Baseline map[string]float64 `json:"baseline"`
}

// Floor is a minimum coverage percentage for packages matching Pattern.
type Floor struct {
	Pattern string  `json:"pattern"`
	Min     float64 `json:"min"`
	Why     string  `json:"why,omitempty"`
}

func main() {
	var (
		profilePath    = flag.String("profile", "coverage.out", "coverage profile to read")
		policyPath     = flag.String("policy", "tools/coverage/policy.json", "coverage policy file")
		update         = flag.Bool("update", false, "rewrite the baseline from this profile")
		allowDecrease  = flag.Bool("allow-decrease", false, "with -update, permit lowering a recorded baseline")
		requireProfile = flag.Bool("require-profile", true, "fail if the profile is missing or empty")
		withDatabase   = flag.Bool("with-database", false, "a Postgres was reachable, so integration-covered packages are real numbers")
	)
	flag.Parse()

	if err := run(*profilePath, *policyPath, *update, *allowDecrease, *requireProfile, *withDatabase); err != nil {
		fmt.Fprintf(os.Stderr, "coverage: %v\n", err)
		os.Exit(1)
	}
}

func run(profilePath, policyPath string, update, allowDecrease, requireProfile, withDatabase bool) error {
	policy, err := loadPolicy(policyPath)
	if err != nil {
		return err
	}

	coverage, err := parseProfile(profilePath)
	if err != nil {
		return err
	}
	if len(coverage) == 0 {
		if requireProfile {
			return fmt.Errorf("%s contains no packages; did `go test -coverprofile` run?", profilePath)
		}
		fmt.Println("coverage: profile is empty, nothing to check")
		return nil
	}

	if update {
		return updateBaseline(policy, policyPath, coverage, allowDecrease, withDatabase)
	}
	return check(policy, coverage, withDatabase)
}

// check reports every violation rather than the first. A tool that stops at
// one failure turns a single fix-and-rerun cycle into as many cycles as there
// are problems.
func check(policy *Policy, coverage map[string]float64, withDatabase bool) error {
	var floorFailures, ratchetFailures, unchecked []string

	for _, pkg := range sortedKeys(coverage) {
		if policy.isExempt(pkg) {
			continue
		}
		if !withDatabase && policy.requiresDatabase(pkg) {
			unchecked = append(unchecked, "  "+pkg)
			continue
		}
		pct := coverage[pkg]

		if floor, ok := policy.floorFor(pkg); ok && pct+tolerance < floor.Min {
			msg := fmt.Sprintf("  %s: %.1f%% < floor %.1f%% (pattern %q)", pkg, pct, floor.Min, floor.Pattern)
			if floor.Why != "" {
				msg += "\n      " + floor.Why
			}
			floorFailures = append(floorFailures, msg)
		}

		if base, ok := policy.Baseline[pkg]; ok && pct+tolerance < base {
			ratchetFailures = append(ratchetFailures, fmt.Sprintf(
				"  %s: %.1f%% < baseline %.1f%% (down %.1f)", pkg, pct, base, base-pct))
		}
	}

	// A package in the baseline that has vanished from the profile is not a
	// pass. It usually means its tests stopped building or were skipped, which
	// looks identical to "no regression" if you only compare the packages that
	// did report.
	var missing []string
	for pkg := range policy.Baseline {
		_, reported := coverage[pkg]
		skipped := !withDatabase && policy.requiresDatabase(pkg)
		if !reported && !policy.isExempt(pkg) && !skipped {
			missing = append(missing, "  "+pkg)
		}
	}
	sort.Strings(missing)

	report(coverage, policy, withDatabase)

	if len(unchecked) > 0 {
		sort.Strings(unchecked)
		fmt.Printf("\nNOT CHECKED — integration tests need a database, and none was reported.\n"+
			"Pass -with-database where one is available (CI does):\n%s\n",
			strings.Join(unchecked, "\n"))
	}

	var problems []string
	if len(floorFailures) > 0 {
		problems = append(problems, "below their floor:\n"+strings.Join(floorFailures, "\n"))
	}
	if len(ratchetFailures) > 0 {
		problems = append(problems, "regressed against the baseline:\n"+strings.Join(ratchetFailures, "\n"))
	}
	if len(missing) > 0 {
		problems = append(problems, "in the baseline but absent from this profile (tests removed, skipped, or not building):\n"+
			strings.Join(missing, "\n"))
	}

	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "\n\n"))
	}

	fmt.Println("\ncoverage: all floors met, no regressions")
	return nil
}

func report(coverage map[string]float64, policy *Policy, withDatabase bool) {
	fmt.Println("package coverage:")
	for _, pkg := range sortedKeys(coverage) {
		note := ""
		switch {
		case policy.isExempt(pkg):
			note = "  (exempt)"
		case !withDatabase && policy.requiresDatabase(pkg):
			note = "  (not checked: needs a database)"
		default:
			if floor, ok := policy.floorFor(pkg); ok {
				note = fmt.Sprintf("  (floor %.0f%%)", floor.Min)
			}
		}
		fmt.Printf("  %-70s %5.1f%%%s\n", pkg, coverage[pkg], note)
	}
}

func updateBaseline(policy *Policy, policyPath string, coverage map[string]float64, allowDecrease, withDatabase bool) error {
	if policy.Baseline == nil {
		policy.Baseline = map[string]float64{}
	}

	var lowered []string
	for pkg, pct := range coverage {
		if policy.isExempt(pkg) {
			continue
		}
		// Recording 0% for a package whose integration tests skipped would
		// write down a number that was never measured, and the ratchet would
		// then treat that fiction as the thing to defend.
		if !withDatabase && policy.requiresDatabase(pkg) {
			continue
		}
		if old, ok := policy.Baseline[pkg]; ok && pct+tolerance < old && !allowDecrease {
			lowered = append(lowered, fmt.Sprintf("  %s: %.1f%% < recorded %.1f%%", pkg, pct, old))
			continue
		}
		policy.Baseline[pkg] = round1(pct)
	}

	if len(lowered) > 0 {
		sort.Strings(lowered)
		return fmt.Errorf(
			"refusing to lower the baseline for:\n%s\n\nFix the coverage, or re-run with -allow-decrease and say why in the commit message",
			strings.Join(lowered, "\n"))
	}

	encoded, err := json.MarshalIndent(policy, "", "  ")
	if err != nil {
		return fmt.Errorf("encode policy: %w", err)
	}
	if err := os.WriteFile(policyPath, append(encoded, '\n'), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", policyPath, err)
	}

	fmt.Printf("coverage: baseline updated in %s (%d packages)\n", policyPath, len(policy.Baseline))
	return nil
}

func loadPolicy(policyPath string) (*Policy, error) {
	raw, err := os.ReadFile(policyPath)
	if err != nil {
		return nil, fmt.Errorf("read policy: %w", err)
	}

	var p Policy
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("parse %s: %w", policyPath, err)
	}
	return &p, nil
}

func (p *Policy) isExempt(pkg string) bool {
	for _, pattern := range p.Exempt {
		if matchPattern(pattern, pkg) {
			return true
		}
	}
	return false
}

func (p *Policy) requiresDatabase(pkg string) bool {
	for _, pattern := range p.RequiresDatabase {
		if matchPattern(pattern, pkg) {
			return true
		}
	}
	return false
}

// floorFor returns the first matching floor. Order in the file is significant
// and documented there: the most specific pattern must come first.
func (p *Policy) floorFor(pkg string) (Floor, bool) {
	for _, f := range p.Floors {
		if matchPattern(f.Pattern, pkg) {
			return f, true
		}
	}
	return Floor{}, false
}

// matchPattern supports path.Match globs per segment, plus a trailing "/..."
// meaning "this package and everything under it".
//
// Matching is segment-by-segment rather than by prefix string because the two
// features combine: "modules/*/infrastructure/..." has to expand the * and
// then accept any depth below. A prefix comparison against the literal text
// silently matches nothing, which looks exactly like "no floor configured".
func matchPattern(pattern, pkg string) bool {
	prefix := strings.TrimSuffix(pattern, "/...")
	recursive := prefix != pattern

	patSegs := strings.Split(prefix, "/")
	pkgSegs := strings.Split(pkg, "/")

	if recursive {
		if len(pkgSegs) < len(patSegs) {
			return false
		}
		pkgSegs = pkgSegs[:len(patSegs)]
	} else if len(patSegs) != len(pkgSegs) {
		return false
	}

	for i := range patSegs {
		ok, err := path.Match(patSegs[i], pkgSegs[i])
		if err != nil || !ok {
			return false
		}
	}
	return true
}

// parseProfile reads a Go coverage profile and returns statement coverage per
// package.
//
// Coverage is computed over statements, not lines, because that is what the
// profile records: each block carries a statement count and an execution
// count. `go tool cover -func` reports the same numbers.
func parseProfile(profilePath string) (map[string]float64, error) {
	f, err := os.Open(profilePath)
	if err != nil {
		return nil, fmt.Errorf("open profile: %w", err)
	}
	defer func() { _ = f.Close() }()

	type counts struct{ total, covered int }
	perPkg := map[string]*counts{}

	scanner := bufio.NewScanner(f)
	// Profiles for a large tree can carry long lines; the default 64K token
	// limit is not guaranteed to be enough.
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	first := true
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if first {
			first = false
			if strings.HasPrefix(line, "mode:") {
				continue
			}
			return nil, fmt.Errorf("%s does not start with a mode line; is it a coverage profile?", profilePath)
		}

		pkg, stmts, execCount, err := parseProfileLine(line)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", profilePath, err)
		}

		c := perPkg[pkg]
		if c == nil {
			c = &counts{}
			perPkg[pkg] = c
		}
		c.total += stmts
		if execCount > 0 {
			c.covered += stmts
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read profile: %w", err)
	}

	out := make(map[string]float64, len(perPkg))
	for pkg, c := range perPkg {
		if c.total == 0 {
			// A package with no statements is 100% covered by definition, and
			// reporting 0% would fail a floor for a file of nothing but type
			// declarations.
			out[pkg] = 100
			continue
		}
		out[pkg] = float64(c.covered) / float64(c.total) * 100
	}
	return out, nil
}

// parseProfileLine parses "path/file.go:1.2,3.4 5 6" into the package path,
// the statement count and the execution count.
func parseProfileLine(line string) (pkg string, stmts, execCount int, err error) {
	fields := strings.Fields(line)
	if len(fields) != 3 {
		return "", 0, 0, fmt.Errorf("malformed profile line %q", line)
	}

	colon := strings.LastIndex(fields[0], ":")
	if colon < 0 {
		return "", 0, 0, fmt.Errorf("malformed profile line %q: no file:range separator", line)
	}
	file := fields[0][:colon]

	if stmts, err = strconv.Atoi(fields[1]); err != nil {
		return "", 0, 0, fmt.Errorf("malformed statement count in %q: %w", line, err)
	}
	if execCount, err = strconv.Atoi(fields[2]); err != nil {
		return "", 0, 0, fmt.Errorf("malformed execution count in %q: %w", line, err)
	}

	// Profiles use forward slashes on every platform, so path (not
	// filepath) is correct here — filepath.Dir on Windows would not split
	// these at all.
	pkg = strings.TrimPrefix(path.Dir(filepath.ToSlash(file)), modulePrefix)
	return pkg, stmts, execCount, nil
}

func sortedKeys(m map[string]float64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func round1(v float64) float64 {
	return float64(int(v*10+0.5)) / 10
}
