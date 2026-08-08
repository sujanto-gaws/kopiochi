package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMatchPattern is here because the first version of matchPattern silently
// matched nothing for "modules/*/infrastructure/...": it trimmed the "/..."
// and then prefix-compared the literal text, star included. A pattern that
// matches nothing is indistinguishable from no policy at all, so the floor
// looked configured and enforced zero packages.
func TestMatchPattern(t *testing.T) {
	t.Parallel()

	tests := []struct {
		pattern string
		pkg     string
		want    bool
	}{
		{"internal/config", "internal/config", true},
		{"internal/config", "internal/configx", false},
		{"internal/config", "internal/config/sub", false},

		{"modules/*/domain", "modules/user/domain", true},
		{"modules/*/domain", "modules/identity/domain", true},
		{"modules/*/domain", "modules/user/application", false},
		// A single * must not span a separator.
		{"modules/*/domain", "modules/user/sub/domain", false},

		{"cmd/...", "cmd", true},
		{"cmd/...", "cmd/api", true},
		{"cmd/...", "cmd/api/deep/nested", true},
		{"cmd/...", "cmdx", false},
		{"cmd/...", "internal/cmd", false},

		// The combination that was broken.
		{"modules/*/infrastructure/...", "modules/identity/infrastructure/persistence/repository", true},
		{"modules/*/infrastructure/...", "modules/user/infrastructure/persistence/repository", true},
		{"modules/*/infrastructure/...", "modules/identity/infrastructure", true},
		{"modules/*/infrastructure/...", "modules/identity/application", false},
		{"modules/*/infrastructure/...", "internal/infrastructure/x", false},
	}

	for _, tc := range tests {
		t.Run(tc.pattern+" vs "+tc.pkg, func(t *testing.T) {
			t.Parallel()

			if got := matchPattern(tc.pattern, tc.pkg); got != tc.want {
				t.Errorf("matchPattern(%q, %q) = %v, want %v", tc.pattern, tc.pkg, got, tc.want)
			}
		})
	}
}

func writeProfile(t *testing.T, body string) string {
	t.Helper()

	p := filepath.Join(t.TempDir(), "coverage.out")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	return p
}

func TestParseProfile(t *testing.T) {
	t.Parallel()

	// Two statements covered out of four in pkg/a, none of two in pkg/b.
	profile := `mode: atomic
github.com/sujanto-gaws/kopiochi/pkg/a/one.go:1.1,2.2 2 1
github.com/sujanto-gaws/kopiochi/pkg/a/two.go:1.1,2.2 2 0
github.com/sujanto-gaws/kopiochi/pkg/b/one.go:1.1,2.2 2 0
`
	got, err := parseProfile(writeProfile(t, profile))
	if err != nil {
		t.Fatalf("parseProfile() error = %v", err)
	}

	if got["pkg/a"] != 50 {
		t.Errorf("pkg/a = %v, want 50", got["pkg/a"])
	}
	if got["pkg/b"] != 0 {
		t.Errorf("pkg/b = %v, want 0", got["pkg/b"])
	}
}

// TestParseProfile_RejectsANonProfile: reading an arbitrary file and reporting
// "0 packages, all floors met" would be the worst possible outcome — a green
// build from a file that is not coverage data.
func TestParseProfile_RejectsANonProfile(t *testing.T) {
	t.Parallel()

	if _, err := parseProfile(writeProfile(t, "this is not a coverage profile\n")); err == nil {
		t.Error("parseProfile() accepted a file with no mode line")
	}
}

func TestParseProfile_RejectsAMalformedLine(t *testing.T) {
	t.Parallel()

	profile := "mode: atomic\ngithub.com/x/y/z.go:1.1,2.2 not-a-number 1\n"
	if _, err := parseProfile(writeProfile(t, profile)); err == nil {
		t.Error("parseProfile() accepted a malformed statement count")
	}
}

func TestCheck_FailsBelowAFloor(t *testing.T) {
	t.Parallel()

	policy := &Policy{Floors: []Floor{{Pattern: "modules/*/domain", Min: 90}}}
	err := check(policy, map[string]float64{"modules/user/domain": 50}, true)

	if err == nil {
		t.Fatal("check() = nil, want a floor failure")
	}
}

func TestCheck_PassesAtTheFloor(t *testing.T) {
	t.Parallel()

	policy := &Policy{Floors: []Floor{{Pattern: "modules/*/domain", Min: 90}}}
	if err := check(policy, map[string]float64{"modules/user/domain": 90}, true); err != nil {
		t.Errorf("check() = %v, want nil at exactly the floor", err)
	}
}

// TestCheck_FailsOnARegression is the ratchet.
func TestCheck_FailsOnARegression(t *testing.T) {
	t.Parallel()

	policy := &Policy{Baseline: map[string]float64{"internal/thing": 80}}
	if err := check(policy, map[string]float64{"internal/thing": 70}, true); err == nil {
		t.Error("check() = nil, want a regression failure")
	}
}

func TestCheck_AllowsAnImprovement(t *testing.T) {
	t.Parallel()

	policy := &Policy{Baseline: map[string]float64{"internal/thing": 80}}
	if err := check(policy, map[string]float64{"internal/thing": 95}, true); err != nil {
		t.Errorf("check() = %v, want nil when coverage improves", err)
	}
}

// TestCheck_FailsWhenABaselinedPackageDisappears is the case a naive ratchet
// misses. If a package's tests stop building or start skipping, it vanishes
// from the profile — and comparing only the packages that reported makes that
// look identical to "no regression".
func TestCheck_FailsWhenABaselinedPackageDisappears(t *testing.T) {
	t.Parallel()

	policy := &Policy{Baseline: map[string]float64{"internal/gone": 80, "internal/here": 80}}
	err := check(policy, map[string]float64{"internal/here": 80}, true)

	if err == nil {
		t.Fatal("check() = nil, want a failure for the package missing from the profile")
	}
}

func TestCheck_ExemptPackagesAreNeitherFlooredNorRatcheted(t *testing.T) {
	t.Parallel()

	policy := &Policy{
		Floors:   []Floor{{Pattern: "cmd/api", Min: 90}},
		Exempt:   []string{"cmd/..."},
		Baseline: map[string]float64{"cmd/api": 90},
	}
	if err := check(policy, map[string]float64{"cmd/api": 10}, true); err != nil {
		t.Errorf("check() = %v, want nil for an exempt package", err)
	}
}

// TestFloorFor_FirstMatchWins pins the documented precedence, so a specific
// pattern placed above a general one actually takes effect.
func TestFloorFor_FirstMatchWins(t *testing.T) {
	t.Parallel()

	policy := &Policy{Floors: []Floor{
		{Pattern: "modules/user/domain", Min: 95},
		{Pattern: "modules/*/domain", Min: 90},
	}}

	got, ok := policy.floorFor("modules/user/domain")
	if !ok || got.Min != 95 {
		t.Errorf("floorFor() = %+v (ok=%v), want the more specific 95%% floor", got, ok)
	}
}

// TestUpdateBaseline_RefusesToLower: a ratchet you can silently release is not
// a ratchet.
func TestUpdateBaseline_RefusesToLower(t *testing.T) {
	t.Parallel()

	policy := &Policy{Baseline: map[string]float64{"internal/thing": 80}}
	path := filepath.Join(t.TempDir(), "policy.json")

	err := updateBaseline(policy, path, map[string]float64{"internal/thing": 60}, false, true)
	if err == nil {
		t.Fatal("updateBaseline() = nil, want a refusal to lower the recorded value")
	}
	if policy.Baseline["internal/thing"] != 80 {
		t.Errorf("baseline was lowered to %v despite the refusal", policy.Baseline["internal/thing"])
	}
}

func TestUpdateBaseline_LowersWithTheExplicitFlag(t *testing.T) {
	t.Parallel()

	policy := &Policy{Baseline: map[string]float64{"internal/thing": 80}}
	path := filepath.Join(t.TempDir(), "policy.json")

	if err := updateBaseline(policy, path, map[string]float64{"internal/thing": 60}, true, true); err != nil {
		t.Fatalf("updateBaseline() error = %v", err)
	}
	if policy.Baseline["internal/thing"] != 60 {
		t.Errorf("baseline = %v, want 60", policy.Baseline["internal/thing"])
	}
}

func TestUpdateBaseline_RaisesAndWrites(t *testing.T) {
	t.Parallel()

	policy := &Policy{Baseline: map[string]float64{"internal/thing": 80}}
	path := filepath.Join(t.TempDir(), "policy.json")

	if err := updateBaseline(policy, path, map[string]float64{"internal/thing": 91.25}, false, true); err != nil {
		t.Fatalf("updateBaseline() error = %v", err)
	}
	if policy.Baseline["internal/thing"] != 91.3 {
		t.Errorf("baseline = %v, want 91.3 (rounded to one decimal)", policy.Baseline["internal/thing"])
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("policy file was not written: %v", err)
	}
}

// TestRun_EmptyProfileIsAFailureByDefault: an empty profile means the tests did
// not run. Treating that as "nothing to check, therefore fine" is how a
// coverage gate ends up passing a build with no tests at all.
func TestRun_EmptyProfileIsAFailureByDefault(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.json")
	if err := os.WriteFile(policyPath, []byte(`{"floors":[],"exempt":[],"baseline":{}}`), 0o600); err != nil {
		t.Fatalf("write policy: %v", err)
	}

	profilePath := writeProfile(t, "mode: atomic\n")

	if err := run(profilePath, policyPath, false, false, true, true); err == nil {
		t.Error("run() = nil for an empty profile, want a failure")
	}
}

// TestCheck_DatabasePackagesAreNotCheckedWithoutOne: an integration-covered
// package reports 0% when the tests skipped, which is indistinguishable from
// "the tests were deleted". Quietly passing that as measured is the one
// outcome a coverage gate must never produce, so it is reported as NOT CHECKED
// instead — and enforced for real where a database exists.
func TestCheck_DatabasePackagesAreNotCheckedWithoutOne(t *testing.T) {
	t.Parallel()

	policy := &Policy{
		Floors:           []Floor{{Pattern: "modules/*/infrastructure/...", Min: 60}},
		RequiresDatabase: []string{"modules/*/infrastructure/persistence/repository"},
	}
	coverage := map[string]float64{"modules/user/infrastructure/persistence/repository": 0}

	if err := check(policy, coverage, false); err != nil {
		t.Errorf("check(withDatabase=false) = %v, want the package skipped rather than failed", err)
	}
	if err := check(policy, coverage, true); err == nil {
		t.Error("check(withDatabase=true) = nil, want the floor enforced when a database was available")
	}
}

// TestCheck_DatabasePackagesAreNotRatchetedWithoutOne: the same package must
// also be exempt from the "in the baseline but absent" check, or a local run
// fails for a package that simply did not report.
func TestCheck_DatabasePackagesAreNotRatchetedWithoutOne(t *testing.T) {
	t.Parallel()

	policy := &Policy{
		RequiresDatabase: []string{"modules/*/infrastructure/persistence/repository"},
		Baseline: map[string]float64{
			"modules/user/infrastructure/persistence/repository": 75,
		},
	}

	if err := check(policy, map[string]float64{"internal/other": 90}, false); err != nil {
		t.Errorf("check() = %v, want the database-backed package skipped", err)
	}
	if err := check(policy, map[string]float64{"internal/other": 90}, true); err == nil {
		t.Error("check(withDatabase=true) = nil, want a failure for the baselined package missing from the profile")
	}
}
