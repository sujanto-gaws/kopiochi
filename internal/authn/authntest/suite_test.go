package authntest

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sujanto-gaws/kopiochi/internal/authn"
	"github.com/sujanto-gaws/kopiochi/internal/testsupport"
)

// A conformance suite that passes everything is worse than no suite: it turns
// an unchecked property into a falsely checked one, and the next person reads
// the green build as evidence. So every assertion the suite claims to make is
// tested here, by running the suite against a middleware that violates that one
// property and requiring the suite to say so.
//
// # How a failing run is captured without failing this test
//
// A *testing.T cannot be used: Fail propagates to the parent, so a subtest that
// fails on purpose fails the run whatever t.Run returns, and there is no
// supported way to unfail it. The seam is one level down instead —
// RunMiddlewareSuite is a thin wrapper over runSuite, which reports through the
// unexported `reporter` interface, and recordingT below implements that
// interface by appending to a slice.
//
// Two things keep the seam honest. TestRunMiddlewareSuiteAcceptsTheConforming-
// Fixture calls the *exported* entry point against a real *testing.T, so the
// wrapper cannot rot into a function nobody calls. And
// TestSuiteReportsNothingForTheConformingFixture runs the conforming middleware
// through recordingT and requires zero findings, so every failure recorded
// below is attributable to the fixture's one deliberate defect rather than to
// the harness.
//
// This file is in package authntest, not authntest_test, because the seam is
// deliberately unexported: a public hook for "run the suite and collect the
// failures" would be new API surface that only this file needs.

// recordingT captures what the suite reports instead of failing the test.
//
// Helper is a no-op: line attribution is meaningless when the failures are
// being collected rather than printed.
type recordingT struct {
	failures []string
}

func (r *recordingT) Helper() {}

func (r *recordingT) Errorf(format string, args ...any) {
	r.failures = append(r.failures, fmt.Sprintf(format, args...))
}

// mentioning returns the recorded failures containing substr.
func (r *recordingT) mentioning(substr string) []string {
	var hits []string
	for _, f := range r.failures {
		if strings.Contains(f, substr) {
			hits = append(hits, f)
		}
	}
	return hits
}

// requireFinding asserts the suite reported a failure containing every one of
// substrs, in the same message.
//
// Matching on the message rather than merely on "something failed" is what
// makes each test below prove *its own* assertion bit: a fixture that broke the
// content type must be caught by the content-type check, not by some unrelated
// check that happens to trip on the same response.
func (r *recordingT) requireFinding(t *testing.T, substrs ...string) {
	t.Helper()

	for _, f := range r.failures {
		matched := true
		for _, s := range substrs {
			if !strings.Contains(f, s) {
				matched = false
				break
			}
		}
		if matched {
			return
		}
	}
	t.Fatalf("the suite reported no failure mentioning all of %q.\nrecorded findings (%d):\n%s",
		substrs, len(r.failures), strings.Join(r.failures, "\n---\n"))
}

// runAgainst drives the suite over mw and returns what it reported.
//
// mint is the real *testing.T: the fixture minters below are trivial and must
// not fail, so a failure from one of them should fail this test rather than be
// swallowed as a conformance finding.
func runAgainst(t *testing.T, mw authn.Middleware) *recordingT {
	t.Helper()

	rec := &recordingT{}
	runSuite(rec, t, mw, fixtureMintValid, fixtureMintInvalid())
	return rec
}

// ---------------------------------------------------------------------------
// The fixture middleware, and the single-defect variants of it.
// ---------------------------------------------------------------------------

// brokenness selects exactly one deviation from the canonical behavior.
//
// One knob per property the suite claims to enforce, and the tests below set
// one at a time. A fixture that broke two properties at once would pass its
// test even if only one of the two checks worked.
type brokenness struct {
	// status replaces 401 on the rejection path.
	status int
	// contentType replaces application/problem+json.
	contentType string
	// omitChallenge drops the WWW-Authenticate header.
	omitChallenge bool
	// detailPerReason writes why the credential failed instead of the one
	// invariant detail — the oracle the contract exists to deny.
	detailPerReason bool
	// leakPrincipal calls the handler with a principal after writing the 401,
	// the shape a missing `return` produces.
	leakPrincipal bool
	// recoverPanics swallows a handler panic.
	recoverPanics bool
	// wrongSubject authenticates somebody other than the credential's subject.
	wrongSubject bool
}

// fixtureMiddleware is a hand-written authn.Middleware, canonical when handed a
// zero brokenness.
//
// Hand-written rather than assembled from internal/httpx: the point of a
// fixture is to be wrong in a controlled way, and a fixture that delegated to
// the production writer could not be. It also keeps this package's dependencies
// where the package doc says they are.
func fixtureMiddleware(b brokenness) authn.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if b.recoverPanics {
				defer func() { _ = recover() }()
			}

			subject, reason := fixtureCredential(r)
			if reason != "" {
				fixtureReject(w, b, reason)
				if !b.leakPrincipal {
					return
				}
			}
			if b.wrongSubject {
				subject += "-somebody-else"
			}
			next.ServeHTTP(w, r.WithContext(
				authn.WithPrincipal(r.Context(), authn.Principal{Subject: subject})))
		})
	}
}

// fixtureCredential is the fixture's toy verifier. A bearer token is the
// subject it authenticates, except for two reserved values that stand in for
// the structural failures a real verifier reports.
func fixtureCredential(r *http.Request) (subject, reason string) {
	header := r.Header.Get("Authorization")
	switch {
	case header == "":
		return "", "missing"
	case !strings.HasPrefix(header, "Bearer "):
		return "", "malformed"
	}

	switch token := strings.TrimPrefix(header, "Bearer "); token {
	case "":
		return "", "empty"
	case "expired":
		return "", "expired"
	default:
		return token, ""
	}
}

// fixtureReject writes the canonical 401, bent by b.
func fixtureReject(w http.ResponseWriter, b brokenness, reason string) {
	detail := "authentication required"
	if b.detailPerReason {
		detail = "the credential is " + reason
	}

	contentType := problemMediaType
	if b.contentType != "" {
		contentType = b.contentType
	}

	status := http.StatusUnauthorized
	if b.status != 0 {
		status = b.status
	}

	if !b.omitChallenge {
		w.Header().Set("WWW-Authenticate", `Bearer realm="fixture"`)
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":   "about:blank",
		"title":  "Unauthorized",
		"status": status,
		"detail": detail,
	})
}

// fixtureMintValid builds the request fixtureMiddleware accepts as subject.
func fixtureMintValid(t *testing.T, subject string) *http.Request {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+subject)
	return req
}

// fixtureMintInvalid is three failure cases with three distinct reasons — the
// minimum that makes the detail-invariance check able to fail.
func fixtureMintInvalid() map[string]func(t *testing.T) *http.Request {
	bearer := func(header string) func(t *testing.T) *http.Request {
		return func(t *testing.T) *http.Request {
			t.Helper()

			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			if header != "" {
				req.Header.Set("Authorization", header)
			}
			return req
		}
	}
	return map[string]func(t *testing.T) *http.Request{
		"missing_header":   bearer(""),
		"malformed_scheme": bearer("Basic abc"),
		"expired_token":    bearer("Bearer expired"),
	}
}

// ---------------------------------------------------------------------------
// The controls: the suite must accept a conforming middleware, through both
// the exported entry point and the recorded seam.
// ---------------------------------------------------------------------------

// TestRunMiddlewareSuiteAcceptsTheConformingFixture is the only test here that
// calls the exported function. Everything else goes through runSuite, so
// without this one the published API could stop calling the suite entirely and
// this file would stay green.
func TestRunMiddlewareSuiteAcceptsTheConformingFixture(t *testing.T) {
	RunMiddlewareSuite(t, fixtureMiddleware(brokenness{}), fixtureMintValid, fixtureMintInvalid())
}

// TestSuiteReportsNothingForTheConformingFixture is the control for every
// rejection test below: it establishes that the recorder path reports zero
// findings when nothing is wrong, so a finding recorded later is caused by the
// fixture's defect and not by the harness.
func TestSuiteReportsNothingForTheConformingFixture(t *testing.T) {
	rec := runAgainst(t, fixtureMiddleware(brokenness{}))
	require.Empty(t, rec.failures,
		"the suite reported a conforming middleware as broken; every rejection test below is "+
			"meaningless until this passes")
}

// ---------------------------------------------------------------------------
// One test per property, each against a fixture broken in exactly that way.
// ---------------------------------------------------------------------------

func TestSuiteRejectsAWrongStatus(t *testing.T) {
	rec := runAgainst(t, fixtureMiddleware(brokenness{status: http.StatusForbidden}))
	rec.requireFinding(t, "invalid[expired_token]", "status = 403, want 401")
}

func TestSuiteRejectsAWrongContentType(t *testing.T) {
	rec := runAgainst(t, fixtureMiddleware(brokenness{contentType: "application/json"}))
	rec.requireFinding(t, "invalid[missing_header]", "Content-Type = \"application/json\"")
}

// A charset parameter is not a defect: the media type is what a client keys
// off. This pins the tolerance so nobody later "fixes" the suite into a string
// comparison and breaks a conforming middleware.
func TestSuiteAcceptsAProblemContentTypeWithParameters(t *testing.T) {
	rec := runAgainst(t, fixtureMiddleware(brokenness{
		contentType: "application/problem+json; charset=utf-8",
	}))
	require.Empty(t, rec.mentioning("Content-Type"),
		"a charset parameter is the same media type and must not be a conformance failure")
}

func TestSuiteRejectsAMissingChallenge(t *testing.T) {
	rec := runAgainst(t, fixtureMiddleware(brokenness{omitChallenge: true}))
	rec.requireFinding(t, "invalid[expired_token]", "no WWW-Authenticate header")
}

// The security property. A per-reason detail is the one defect here that a
// human reviewer is most likely to call an improvement.
func TestSuiteRejectsADetailThatVariesByReason(t *testing.T) {
	rec := runAgainst(t, fixtureMiddleware(brokenness{detailPerReason: true}))
	rec.requireFinding(t, "detail = ", "every rejection must emit the same detail")
}

func TestSuiteRejectsAPrincipalLeftBehindAfterRejection(t *testing.T) {
	rec := runAgainst(t, fixtureMiddleware(brokenness{leakPrincipal: true}))
	rec.requireFinding(t, "invalid[missing_header]", "must leave no principal downstream")
}

func TestSuiteRejectsAMiddlewareThatSwallowsAHandlerPanic(t *testing.T) {
	rec := runAgainst(t, fixtureMiddleware(brokenness{recoverPanics: true}))
	rec.requireFinding(t, "panic:", "nothing came back out of the middleware")
}

func TestSuiteRejectsTheWrongSubject(t *testing.T) {
	rec := runAgainst(t, fixtureMiddleware(brokenness{wrongSubject: true}))
	rec.requireFinding(t, "Principal.Subject =", "authenticated a different account")
}

// testsupport.FakeAuthPrincipal with a zero Principal is the ready-made broken
// middleware: it authenticates every request as nobody and rejects nothing. It
// is checked here rather than only through the knobs above because it is what a
// consumer will actually reach for by accident — a fake left mounted where the
// real middleware belongs — and because it violates three properties at once,
// which proves the suite reports all of them in one run instead of stopping at
// the first.
func TestSuiteRejectsFakeAuthPrincipalWithAnEmptySubject(t *testing.T) {
	rec := runAgainst(t, testsupport.FakeAuthPrincipal(authn.Principal{}))

	rec.requireFinding(t, "Principal.Subject = \"\"", "authenticated a different account")
	rec.requireFinding(t, "invalid[missing_header]", "an invalid credential must not reach the handler")
	rec.requireFinding(t, "invalid[missing_header]", "must leave no principal downstream")
	rec.requireFinding(t, "invalid[expired_token]", "status = 200, want 401")
}

// A middleware that authenticates nobody — no principal in the context at all —
// is not the same defect as one that authenticates the wrong subject, and the
// suite must name them differently. Without this, "present" could be dropped
// from the success check and only the subject test would notice.
func TestSuiteRejectsAMiddlewareThatPublishesNoPrincipal(t *testing.T) {
	passthrough := authn.Middleware(func(next http.Handler) http.Handler { return next })

	rec := runAgainst(t, passthrough)
	rec.requireFinding(t, "valid:", "no authn.Principal in its context")
	require.Empty(t, rec.mentioning("must leave no principal downstream"),
		"no principal was published, so the downstream-leak finding must not fire")
}

// A middleware that refuses a credential it must accept is a denial of service
// against every legitimate caller, and it is the failure the success path
// exists to catch. Driven here by minting a credential the fixture rejects,
// which is indistinguishable from the middleware's side.
func TestSuiteRejectsAMiddlewareThatRefusesAValidCredential(t *testing.T) {
	rec := &recordingT{}
	runSuite(rec, t, fixtureMiddleware(brokenness{}),
		func(t *testing.T, _ string) *http.Request { return fixtureMintValid(t, "expired") },
		fixtureMintInvalid())

	rec.requireFinding(t, "valid: the handler was never reached")
	// The panic check reads the same evidence and says so: no panic came back
	// because the handler never ran. Pinned so the second half of that message
	// cannot be dropped as redundant.
	rec.requireFinding(t, "panic:", "it never called the handler at all")
}

// The panic check must distinguish "the handler's panic came back" from "some
// other panic came back". Without that, a middleware that recovered the
// handler's panic and raised its own — a wrapper turning every panic into a
// sentinel error, say — would pass a check that only asked whether *something*
// panicked.
func TestSuiteRejectsAReplacedPanicValue(t *testing.T) {
	repanic := func(v any) authn.Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer func() {
					if recover() != nil {
						panic(v)
					}
				}()
				next.ServeHTTP(w, r.WithContext(
					authn.WithPrincipal(r.Context(), authn.Principal{Subject: suiteSubject})))
			})
		}
	}

	rec := &recordingT{}
	checkHandlerPanicPropagates(rec, t, repanic("something else entirely"), fixtureMintValid)
	rec.requireFinding(t, `recovered "something else entirely"`, "want the handler's own panic value")

	rec = &recordingT{}
	checkHandlerPanicPropagates(rec, t, repanic(errors.New("wrapped")), fixtureMintValid)
	rec.requireFinding(t, "of type *errors.errorString", "want the handler's own panic value")
}

// ---------------------------------------------------------------------------
// Argument guards. A suite that silently checks nothing is the failure mode
// this whole file exists to prevent, so being called wrongly is itself a
// finding.
// ---------------------------------------------------------------------------

// A minter returning nil must be a finding, not a nil-pointer panic from inside
// the suite: the second reports a crash in authntest for a mistake in the
// caller's own minter.
func TestSuiteReportsANilRequestFromMintValid(t *testing.T) {
	rec := &recordingT{}
	runSuite(rec, t, fixtureMiddleware(brokenness{}),
		func(*testing.T, string) *http.Request { return nil },
		fixtureMintInvalid())

	rec.requireFinding(t, "valid: mintValid returned a nil request")
	rec.requireFinding(t, "panic: mintValid returned a nil request")
}

func TestSuiteReportsANilRequestFromAnInvalidMinter(t *testing.T) {
	cases := fixtureMintInvalid()
	cases["nil_request"] = func(*testing.T) *http.Request { return nil }

	rec := &recordingT{}
	runSuite(rec, t, fixtureMiddleware(brokenness{}), fixtureMintValid, cases)
	rec.requireFinding(t, "invalid[nil_request]", "the minter returned a nil request")
}

func TestSuiteReportsANilMiddleware(t *testing.T) {
	rec := &recordingT{}
	runSuite(rec, t, nil, fixtureMintValid, fixtureMintInvalid())
	rec.requireFinding(t, "mw is nil")
}

func TestSuiteReportsANilValidMinter(t *testing.T) {
	rec := &recordingT{}
	runSuite(rec, t, fixtureMiddleware(brokenness{}), nil, fixtureMintInvalid())
	rec.requireFinding(t, "mintValid is nil")
}

// One invalid case makes "detail is identical across all invalid cases"
// vacuously true. The suite says so rather than passing.
func TestSuiteReportsTooFewInvalidCases(t *testing.T) {
	rec := &recordingT{}
	runSuite(rec, t, fixtureMiddleware(brokenness{}), fixtureMintValid,
		map[string]func(t *testing.T) *http.Request{
			"missing_header": fixtureMintInvalid()["missing_header"],
		})
	rec.requireFinding(t, "mintInvalid has 1 case(s)")
}

func TestSuiteReportsANilInvalidMinter(t *testing.T) {
	cases := fixtureMintInvalid()
	cases["nil_minter"] = nil

	rec := &recordingT{}
	runSuite(rec, t, fixtureMiddleware(brokenness{}), fixtureMintValid, cases)
	rec.requireFinding(t, "invalid[nil_minter]", "the minter is nil")
}

// A body that is not JSON cannot contribute a detail, and must be reported
// rather than compared as the empty string — otherwise a middleware emitting
// plain text for every rejection would pass the invariance check.
func TestSuiteReportsANonJSONBody(t *testing.T) {
	textOnly := authn.Middleware(func(http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="fixture"`)
			w.Header().Set("Content-Type", problemMediaType)
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("unauthorized"))
		})
	})

	rec := &recordingT{}
	// mintValid is nil: this middleware rejects everything, so the success-path
	// checks would only add noise about a defect this test is not about.
	runSuite(rec, t, textOnly, nil, fixtureMintInvalid())
	rec.requireFinding(t, "invalid[expired_token]", "is not JSON")
	require.Empty(t, rec.mentioning("every rejection must emit the same detail"),
		"an unreadable body must be excluded from the invariance check, not compared as \"\"")
}
