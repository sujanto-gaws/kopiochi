// Package authntest holds the conformance suite every authn.Middleware must
// pass.
//
// The canonical 401 that identity emits today is a property of one
// implementation. This package turns it into a contract: RunMiddlewareSuite
// takes any authn.Middleware and asserts the behavior every consumer in the
// tree is entitled to assume, so that replacing identity with an OIDC
// middleware — or adding a second one beside it — cannot silently reintroduce
// per-reason error bodies, a missing challenge header, or a principal that
// survives a rejection.
//
// It knows nothing about tokens. The caller supplies *requests*: one minter for
// the success path and a named map of minters for the ways a credential can
// fail. That is what keeps this package on the correct side of the dependency
// rules — internal/** must not import modules/**, so the suite cannot know how
// identity signs a JWT, and identity wires itself in from its own test file.
//
// The imports are deliberately stdlib plus internal/authn. A conformance suite
// that imported internal/httpx would be asserting that a middleware calls one
// particular writer, which is an implementation detail; what it asserts instead
// is the bytes on the wire.
package authntest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/sujanto-gaws/kopiochi/internal/authn"
)

// suiteSubject is the account id the success path is minted for and asserted
// on. It is deliberately not a plausible production id: a middleware that
// hardcodes a subject, or that echoes a default instead of the credential's
// own claim, fails on this value where it would pass on "user-1".
const suiteSubject = "authntest-subject-4f21c9"

// problemMediaType is the media type RFC 9457 defines for error responses.
//
// Spelled as a literal rather than imported from internal/httpx on purpose,
// and for two reasons. It keeps this package's dependency footprint at
// internal/authn. And it makes the suite a second, independent statement of the
// contract: asserting a value against the constant that produced it proves only
// that the constant equals itself.
const problemMediaType = "application/problem+json"

// reporter is the slice of *testing.T the suite reports through.
//
// It exists so the suite can be run against a recorder in this package's own
// tests — "the suite fails a broken middleware" is an assertion, and an
// assertion that can only be checked by eyeballing a red build is not tested.
// RunMiddlewareSuite keeps the published *testing.T signature; the seam is one
// level down.
//
// It has no Fatalf and no FailNow, which is a constraint on the suite as much
// as a convenience for the recorder. A real Fatalf calls runtime.Goexit and a
// recorded one returns normally, so a suite written against Fatalf would take a
// different path under test than in production. Every check below reports with
// Errorf and returns explicitly, so the recorder observes exactly what a
// *testing.T would.
type reporter interface {
	Helper()
	Errorf(format string, args ...any)
}

// RunMiddlewareSuite asserts that mw honors the authentication contract.
//
// mintValid must return a request carrying a credential that authenticates as
// subject. mintInvalid maps a case name — "expired_token", "wrong_algorithm" —
// to a minter for a request that must be rejected; the name appears in every
// failure the case produces, so a red build says which credential broke the
// contract rather than that one did.
//
// What is asserted, and why each one is worth a test:
//
//   - Valid credential: the downstream handler is reached, an authn.Principal
//     is present in its context, and Principal.Subject is the subject that was
//     minted. Asserting "not a 401" is not enough — a middleware that panics,
//     or one that authenticates the wrong account, passes that.
//
//   - Every invalid credential: HTTP 401, a problem+json Content-Type, and a
//     non-empty WWW-Authenticate (RFC 9110 requires a 401 to carry a
//     challenge).
//
//   - EVERY rejection is byte-identical to every other: the whole body, and
//     every response header except Date. This is the security property: a
//     rejection that says "expired" for one credential and "bad signature" for
//     another is an oracle telling an attacker which tokens are structurally
//     valid. Asserted by comparing the responses to each other, which is why at
//     least two invalid cases are required.
//
//     The whole response, and not just the "detail" member, because a middleware
//     that leaks through "title", through the problem "type" URI, or through
//     WWW-Authenticate — RFC 6750's error_description, which is the default
//     shape of most OAuth middleware — was passing this suite with zero
//     findings (E17).
//
//     "instance" is normalised before comparing, and it is the ONLY exception.
//     RFC 7807 defines it as a URI reference for the specific occurrence, and
//     this repository's writer sets it to the request path; a caller whose
//     invalid cases hit different paths would otherwise get a false failure
//     from a perfectly uniform middleware. Everything the SERVER chooses is
//     compared; the one member that echoes the REQUEST is not.
//
//     An absent "detail" is NOT a finding. A middleware that answers every 401
//     with {} is uniform — it leaks nothing, and RFC 7807 makes "detail"
//     optional. Requiring it would fail a conformant replacement for a reason
//     unrelated to leaking.
//
//   - A rejected request leaves no principal downstream.
//
//   - A panicking handler's panic propagates out of mw. Middleware that
//     recovers turns a handler bug into a 500 with no stack, and hides it from
//     every test that only checks the status.
//
// The suite reports every violation it finds rather than stopping at the first,
// so one run tells the whole story.
func RunMiddlewareSuite(t *testing.T, mw authn.Middleware,
	mintValid func(t *testing.T, subject string) *http.Request,
	mintInvalid map[string]func(t *testing.T) *http.Request,
) {
	t.Helper()
	runSuite(t, t, mw, mintValid, mintInvalid)
}

// runSuite is RunMiddlewareSuite with the reporting separated from the
// *testing.T the minters are handed.
//
// report receives the conformance findings. mint is passed to the caller's
// minters, which need a real *testing.T because they are ordinary test helpers
// that call t.Helper and t.Fatal. Splitting them is what lets the package's own
// tests capture a failing run without failing.
func runSuite(report reporter, mint *testing.T, mw authn.Middleware,
	mintValid func(t *testing.T, subject string) *http.Request,
	mintInvalid map[string]func(t *testing.T) *http.Request,
) {
	report.Helper()

	if mw == nil {
		report.Errorf("authntest: mw is nil; there is no middleware to run the suite against")
		return
	}

	if mintValid == nil {
		report.Errorf("authntest: mintValid is nil; the suite cannot check the success path, " +
			"which is where a middleware authenticating the wrong account shows up")
	} else {
		checkValidCredentialAuthenticates(report, mint, mw, mintValid)
		checkHandlerPanicPropagates(report, mint, mw, mintValid)
	}

	checkInvalidCredentialsAreRejected(report, mint, mw, mintInvalid)
}

// checkValidCredentialAuthenticates covers the success path: reached, present,
// and the right subject.
//
// The three are reported separately and in order, each returning rather than
// cascading, because they fail for different reasons: not reached means the
// middleware rejects a credential it should accept, absent means it accepts
// without publishing a principal, and a mismatched subject means it
// authenticates somebody else — the worst of the three and the one an
// end-to-end "status is not 401" assertion cannot see.
func checkValidCredentialAuthenticates(report reporter, mint *testing.T, mw authn.Middleware,
	mintValid func(t *testing.T, subject string) *http.Request,
) {
	report.Helper()

	req := mintValid(mint, suiteSubject)
	if req == nil {
		report.Errorf("valid: mintValid returned a nil request")
		return
	}

	var (
		reached   bool
		principal authn.Principal
		present   bool
	)
	rec := httptest.NewRecorder()
	mw(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		reached = true
		principal, present = authn.FromContext(r.Context())
	})).ServeHTTP(rec, req)

	if !reached {
		report.Errorf("valid: the handler was never reached — the middleware rejected a credential "+
			"it must accept (status %d, body %q)", rec.Code, rec.Body.String())
		return
	}
	if !present {
		report.Errorf("valid: the handler ran with no authn.Principal in its context; " +
			"a middleware that authenticates must publish who the caller is, or every " +
			"downstream authn.MustFromContext panics")
		return
	}
	// Subject alone, compared as a string. authn.Principal holds a []string and
	// is deliberately not comparable, and the rest of it is provider-specific:
	// scopes and extra claims are exactly what one middleware fills and another
	// leaves nil, so a contract that pinned them would fail every replacement
	// for reasons that are not contract violations.
	if principal.Subject != suiteSubject {
		report.Errorf("valid: Principal.Subject = %q, want %q — the middleware authenticated a "+
			"different account than the credential names", principal.Subject, suiteSubject)
	}
}

// checkInvalidCredentialsAreRejected runs every named failure case and then
// compares their bodies to each other.
func checkInvalidCredentialsAreRejected(report reporter, mint *testing.T, mw authn.Middleware,
	mintInvalid map[string]func(t *testing.T) *http.Request,
) {
	report.Helper()

	// Two, not one. "detail is identical across all invalid cases" is a
	// statement about a set, and with a single case it is vacuously true — the
	// suite would report success while checking nothing about the one property
	// that keeps the error body from being an oracle.
	if len(mintInvalid) < 2 {
		report.Errorf("authntest: mintInvalid has %d case(s); the suite needs at least two, "+
			"because the indistinguishability check compares the rejections to each other",
			len(mintInvalid))
	}

	// Sorted, so a failure names the cases in the same order every run and a
	// diff between two runs is a real change.
	names := make([]string, 0, len(mintInvalid))
	for name := range mintInvalid {
		names = append(names, name)
	}
	sort.Strings(names)

	rejections := make(map[string]rejection, len(names))
	for _, name := range names {
		if fp, ok := checkOneRejection(report, mint, mw, name, mintInvalid[name]); ok {
			rejections[name] = fp
		}
	}

	checkRejectionsAreIndistinguishable(report, names, rejections)
}

// checkOneRejection asserts the canonical 401 for a single case and returns the
// "detail" member for the cross-case comparison.
//
// The bool result distinguishes "no detail" from "the body could not be read",
// so an unparseable body is excluded from the invariance check rather than
// silently compared as the empty string.
func checkOneRejection(report reporter, mint *testing.T, mw authn.Middleware,
	name string, minter func(t *testing.T) *http.Request,
) (rejection, bool) {
	report.Helper()

	if minter == nil {
		report.Errorf("invalid[%s]: the minter is nil", name)
		return rejection{}, false
	}
	req := minter(mint)
	if req == nil {
		report.Errorf("invalid[%s]: the minter returned a nil request", name)
		return rejection{}, false
	}

	var (
		reached bool
		present bool
		subject string
	)
	rec := httptest.NewRecorder()
	mw(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		reached = true
		p, ok := authn.FromContext(r.Context())
		present, subject = ok, p.Subject
	})).ServeHTTP(rec, req)

	// Two independent findings, both reported when both apply. They are
	// different defects: reaching the handler at all means the middleware let an
	// invalid credential through, while leaving a principal behind means every
	// handler downstream will treat the request as authenticated — the shape a
	// missing `return` after writing the 401 produces, and the more dangerous of
	// the two. Neither message assumes the other, so the pair reads correctly
	// for a middleware that rejects-then-continues and for one that never
	// rejects at all.
	if reached {
		report.Errorf("invalid[%s]: the handler ran (response status %d); an invalid credential "+
			"must not reach the handler", name, rec.Code)
	}
	if reached && present {
		report.Errorf("invalid[%s]: the handler ran with an authn.Principal (Subject %q) in its "+
			"context; a request that fails authentication must leave no principal downstream",
			name, subject)
	}

	if rec.Code != http.StatusUnauthorized {
		report.Errorf("invalid[%s]: status = %d, want %d", name, rec.Code, http.StatusUnauthorized)
	}

	// Parsed rather than string-compared so that a charset parameter is not a
	// failure — "application/problem+json; charset=utf-8" is the same media
	// type, and a conformance suite should reject the wrong type, not the wrong
	// spelling.
	contentType := rec.Header().Get("Content-Type")
	if mediaType, _, err := mime.ParseMediaType(contentType); err != nil || mediaType != problemMediaType {
		report.Errorf("invalid[%s]: Content-Type = %q, want %q — a client cannot distinguish an "+
			"authentication failure from an application error without it",
			name, contentType, problemMediaType)
	}

	// Presence only. The scheme and realm are the middleware's to choose (a
	// Bearer challenge for this service, something else for a replacement);
	// what RFC 9110 requires, and what a client needs in order to know how to
	// re-authenticate, is that a challenge is there at all.
	if rec.Header().Get("WWW-Authenticate") == "" {
		report.Errorf("invalid[%s]: no WWW-Authenticate header; RFC 9110 requires a 401 to carry "+
			"a challenge", name)
	}

	fp, err := fingerprint(rec)
	if err != nil {
		report.Errorf("invalid[%s]: the response body is not JSON (%v): %q",
			name, err, rec.Body.String())
		return rejection{}, false
	}
	return fp, true
}

// rejection is everything about a 401 that the middleware chose, in a form two
// of them can be compared by.
type rejection struct {
	// headers is every response header except Date, canonicalised and sorted so
	// two equal rejections produce equal strings.
	headers string
	// body is the parsed body re-encoded with "instance" removed, so member
	// order and whitespace do not count as a difference and the one
	// request-dependent member does not.
	body string
}

// fingerprint reduces a recorded rejection to its comparable form.
func fingerprint(rec *httptest.ResponseRecorder) (rejection, error) {
	var headers []string
	for k, v := range rec.Header() {
		// Date is a clock reading, not a choice the middleware made. It is the
		// only header excluded: a leak can hide in any of the others, including
		// ones invented by the middleware, so the comparison is a denylist of
		// exactly one rather than an allowlist of the ones thought of here.
		if http.CanonicalHeaderKey(k) == "Date" {
			continue
		}
		headers = append(headers, fmt.Sprintf("%s: %s", http.CanonicalHeaderKey(k), strings.Join(v, ", ")))
	}
	sort.Strings(headers)

	body := rec.Body.Bytes()
	if len(bytes.TrimSpace(body)) == 0 {
		return rejection{headers: strings.Join(headers, "\n"), body: ""}, nil
	}

	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return rejection{}, err
	}
	delete(parsed, "instance")

	// Rendered as sorted key=value pairs rather than re-marshalled, so member
	// order and whitespace do not count as a difference — and so there is no
	// second error path. Marshalling a map that came from json.Unmarshal cannot
	// fail, which would leave a branch no test can reach.
	keys := make([]string, 0, len(parsed))
	for k := range parsed {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, fmt.Sprintf("%s=%v", k, parsed[k]))
	}
	return rejection{headers: strings.Join(headers, "\n"), body: strings.Join(pairs, " ")}, nil
}

// checkRejectionsAreIndistinguishable asserts every rejection is the same
// response: same headers, same body.
//
// Compared against the first case in sorted order rather than pairwise, so N
// divergent cases produce N-1 failures naming a fixed reference instead of
// N*(N-1)/2 naming each other.
//
// Headers and body are reported separately because they are different mistakes
// with different fixes. A body difference is usually a message built from the
// validation error; a header difference is usually RFC 6750's
// error_description, which a middleware author may not think of as a body at
// all — and which this suite used to accept without looking (E17).
func checkRejectionsAreIndistinguishable(report reporter, names []string, rejections map[string]rejection) {
	report.Helper()

	var reference rejection
	var referenceName string
	for _, name := range names {
		fp, ok := rejections[name]
		if !ok {
			continue
		}
		if referenceName == "" {
			reference, referenceName = fp, name
			continue
		}

		if fp.headers != reference.headers {
			report.Errorf("invalid[%s]: response headers differ from invalid[%s].\n"+
				"  %s:\n%s\n  %s:\n%s\n"+
				"every rejection must be indistinguishable, and a challenge that names the "+
				"reason — RFC 6750's error_description is the usual one — is an oracle "+
				"telling an attacker which credentials are structurally valid",
				name, referenceName, name, indent(fp.headers), referenceName, indent(reference.headers))
		}
		if fp.body != reference.body {
			report.Errorf("invalid[%s]: body = %s, but invalid[%s] answered %s — every "+
				"rejection must emit the same body, or it tells an attacker which "+
				"credentials are structurally valid and which are merely stale. "+
				"(\"instance\" is excluded from this comparison; every other member counts.)",
				name, fp.body, referenceName, reference.body)
		}
	}
}

// indent prefixes each line, so a multi-line header block in a failure message
// is readable next to the case names.
//
// No empty case: this is only reached from a headers-differ failure, and a
// rejection that reached that point has at least a Content-Type. A branch for
// a state the caller cannot produce is a branch no test can reach.
func indent(s string) string {
	return "    " + strings.ReplaceAll(s, "\n", "\n    ")
}

// panicValue is what the panicking handler panics with. A sentinel rather than
// a bare string literal so the suite can tell "the handler's panic came back"
// from "the middleware panicked on its own".
const panicValue = "authntest: deliberate handler panic"

// checkHandlerPanicPropagates asserts mw does not recover.
//
// Middleware that swallows a handler panic converts a nil-map write into a
// silent empty 200, or — with a recovery middleware further out — into a 500
// that no auth test looks at. The request is a valid one on purpose: the
// handler has to actually run for there to be a panic to swallow.
func checkHandlerPanicPropagates(report reporter, mint *testing.T, mw authn.Middleware,
	mintValid func(t *testing.T, subject string) *http.Request,
) {
	report.Helper()

	req := mintValid(mint, suiteSubject)
	if req == nil {
		report.Errorf("panic: mintValid returned a nil request")
		return
	}

	recovered := func() (v any) {
		defer func() { v = recover() }()
		mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			panic(panicValue)
		})).ServeHTTP(httptest.NewRecorder(), req)
		return nil
	}()

	switch got := recovered.(type) {
	case nil:
		report.Errorf("panic: the handler panicked and nothing came back out of the middleware. " +
			"Either it recovered the panic — which hides handler bugs from every test that only " +
			"checks the status — or it never called the handler at all")
	case string:
		if got != panicValue {
			report.Errorf("panic: recovered %q, want the handler's own panic value %q",
				got, panicValue)
		}
	default:
		report.Errorf("panic: recovered %#v of type %T, want the handler's own panic value %q",
			recovered, recovered, panicValue)
	}
}
