package transport

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/sujanto-gaws/kopiochi/internal/authn/authntest"
	"github.com/sujanto-gaws/kopiochi/internal/testsupport"
	"github.com/sujanto-gaws/kopiochi/modules/identity/infrastructure/token"
)

// This file is identity's side of the authentication contract.
//
// unauthorized_golden_test.go pins what *this* middleware does. The suite below
// pins what *any* middleware must do, and then runs identity through it — so
// the canonical 401, the challenge header, the reason-invariant detail and the
// absent principal stop being properties identity happens to have and become
// properties a replacement has to reproduce. The assertions live in
// internal/authn/authntest, which knows nothing about JWTs; everything
// token-shaped stays here, because internal/** must not import modules/**.
//
// It also closes a hole the existing tests leave open. The end-to-end control
// on the success path (cmd/api/protected_routes_test.go) asserts only that the
// status is not 401, and the global Recovery middleware turns a panic into a
// 500 — so a regression that stopped publishing the principal would still pass
// it. The suite asserts the handler ran, that a Principal reached it, and that
// its Subject is the one the token was minted for.

// TestAuthRequiredConformsToTheMiddlewareContract runs the real RS256 verifier,
// not a stub. A stub would decide for itself whether an expired or alg=none
// token is rejected, which is the question being asked.
func TestAuthRequiredConformsToTheMiddlewareContract(t *testing.T) {
	kp := testsupport.NewKeypair(t)

	// Leeway pinned small and explicit, matching protectedRouter: left to the
	// service default, the expired case would start depending on a 30-second
	// skew allowance.
	jwtSvc, err := token.NewJWTService(kp.PrivatePath, kp.PublicPath,
		testsupport.TestIssuer, testsupport.TestAudience, time.Second)
	require.NoError(t, err)

	authntest.RunMiddlewareSuite(t, AuthRequired(jwtSvc),
		func(t *testing.T, subject string) *http.Request {
			t.Helper()
			return testsupport.WithBearer(
				testsupport.JSONRequest(t, protectedMethod, protectedPath, nil),
				testsupport.AccessToken(t, kp, subject))
		},
		invalidCredentialMinters(kp, uuid.New().String()))
}

// invalidCredentialMinters adapts the shared rejection table into the map the
// suite takes.
//
// Reusing rejectionCases rather than re-deriving the four credentials the plan
// names is the point: the golden capture, the canonical-401 contract test and
// the conformance run all read one table, so a sixth way to fail authentication
// is covered by all three the moment it is added to it. The set is a superset
// of the plan's (valid, expired, malformed, wrong-alg) — it also carries the
// missing-header and refresh-class cases, which cost nothing here and are two
// more chances for the indistinguishability check to catch a leak.
func invalidCredentialMinters(kp testsupport.Keypair, subject string) map[string]func(t *testing.T) *http.Request {
	cases := rejectionCases(kp, subject)

	minters := make(map[string]func(t *testing.T) *http.Request, len(cases))
	for _, tc := range cases {
		minters[tc.name] = func(t *testing.T) *http.Request {
			t.Helper()

			req := testsupport.JSONRequest(t, protectedMethod, protectedPath, nil)
			// An empty string means the header is absent, which is not the same
			// request as one carrying an empty header — see rejectionCases.
			if h := tc.authorization(t); h != "" {
				req.Header.Set("Authorization", h)
			}
			return req
		}
	}
	return minters
}

// TestConformanceCoversEveryRejectionCase pins the wiring above to the table.
//
// Without it, a refactor that narrowed rejectionCases — or a map key colliding
// after a rename — would shrink the conformance surface silently, and the suite
// would go on passing with fewer cases. The names are spelled out rather than
// derived from the table so that removing a case has to be an edit here too.
func TestConformanceCoversEveryRejectionCase(t *testing.T) {
	kp := testsupport.NewKeypair(t)
	minters := invalidCredentialMinters(kp, uuid.New().String())

	for _, name := range []string{
		"missing_header",
		"malformed_bearer",
		"expired_token",
		"wrong_algorithm",
		"refresh_class",
	} {
		require.Contains(t, minters, name,
			"the conformance suite no longer exercises the %s credential", name)
		require.NotNil(t, minters[name])
	}
	require.Len(t, minters, len(rejectionCases(kp, "subject")),
		"a rejection case was dropped between the table and the conformance map")
}
