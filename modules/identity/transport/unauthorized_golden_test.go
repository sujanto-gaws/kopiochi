package transport

import (
	"context"
	"encoding/json"
	"flag"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/sujanto-gaws/kopiochi/internal/httpx"
	"github.com/sujanto-gaws/kopiochi/internal/testsupport"
	app "github.com/sujanto-gaws/kopiochi/modules/identity/application"
	"github.com/sujanto-gaws/kopiochi/modules/identity/infrastructure/token"
)

// This file is a camera, not a specification.
//
// It records — byte for byte — what a protected route answers today for every
// way a caller can fail authentication, so that the canonical problem+json 401
// arriving later is a *measured* change rather than a guessed one. The goldens
// it writes are committed on their own so the pre-change shapes survive in git
// history as the audit trail.
//
// Two consequences follow, and both are deliberate:
//
//   - Nothing here asserts the cases agree with each other. They do not, and
//     recording the disagreement is the entire point. Do not "tidy" a golden.
//   - The real token verifier is wired up, not a stub. A stub would decide for
//     itself whether an expired or alg=none token is rejected, and the answer
//     to that question is exactly what is being captured.
//
// Regenerate with: go test ./modules/identity/transport/... -run TestCurrent401Shapes -update

// updateGolden rewrites the golden files instead of comparing against them.
// The repository has no prior golden-test convention, so this is the plain
// flag.Bool spelling; anything added later should match it.
var updateGolden = flag.Bool("update", false,
	"rewrite modules/identity/transport/testdata/golden/401_*.json to match current behavior")

// goldenDir is relative to this package, which is where `go test` runs.
const goldenDir = "testdata/golden"

// protectedRoute is the route every case below is aimed at. It is one of the
// three routes AuthHandler.Routes puts behind h.authMW, and it is mounted
// under the real /api/v1 prefix so the recorded shape (and, after the
// canonical 401 lands, its "instance" member) matches what a client sees.
const (
	protectedMethod = http.MethodPost
	protectedPath   = "/api/v1/auth/logout"
)

// unauthorizedGolden is the recorded response. The four members are the ones
// §8.1 of the impact analysis names: the status, the two headers a client
// keys off, and the raw body.
//
// Body is a string rather than an embedded object on purpose: a body that is
// not JSON at all — or that is JSON with a trailing newline — must round-trip
// unchanged. Re-encoding it through a struct would normalize away precisely
// the details this capture exists to preserve.
type unauthorizedGolden struct {
	Status          int    `json:"status"`
	ContentType     string `json:"content_type"`
	WWWAuthenticate string `json:"www_authenticate"`
	Body            string `json:"body"`
}

// TestCurrent401Shapes drives five rejection paths through the assembled route
// tree and records each response.
//
// The cases are ordered, not a map, so a failure names the same file every
// run.
func TestCurrent401Shapes(t *testing.T) {
	kp := testsupport.NewKeypair(t)
	subject := uuid.New().String()

	svc := &recordingAuthService{}
	router := protectedRouter(t, kp, svc)

	for _, tc := range rejectionCases(kp, subject) {
		t.Run(tc.name, func(t *testing.T) {
			svc.reset()

			req := testsupport.JSONRequest(t, protectedMethod, protectedPath, nil)
			if h := tc.authorization(t); h != "" {
				req.Header.Set("Authorization", h)
			}
			rec := testsupport.Do(t, router, req)

			got := unauthorizedGolden{
				Status:          rec.Code,
				ContentType:     rec.Header().Get("Content-Type"),
				WWWAuthenticate: rec.Header().Get("WWW-Authenticate"),
				Body:            rec.Body.String(),
			}
			compareGolden(t, filepath.Join(goldenDir, "401_"+tc.name+".json"), got)

			// Not part of the recorded shape, but the one thing worth
			// asserting here: no rejection path may reach the handler. If it
			// does, the golden above is recording an application response
			// rather than an authentication failure, and the capture is
			// measuring the wrong thing.
			if svc.calls > 0 {
				t.Errorf("the %s request reached AuthHandler.Logout (%d service call(s)); "+
					"it was not rejected by AuthRequired and this golden does not record a rejection",
					tc.name, svc.calls)
			}
		})
	}
}

// rejectionCase is one way a caller can fail authentication.
type rejectionCase struct {
	// name doubles as the golden file's suffix: 401_<name>.json.
	name string
	// authorization is the Authorization header value. The empty string
	// means the header is not set at all, which is not the same request as
	// one carrying an empty header.
	authorization func(t *testing.T) string
}

// rejectionCases is the shared table. It is a function rather than two literals
// so that TestCurrent401Shapes (which records) and TestCanonical401Contract
// (which asserts) can never drift apart: a sixth rejection path added here is
// immediately both recorded and checked.
//
// The slice is ordered, not a map, so a failure names the same file every run.
func rejectionCases(kp testsupport.Keypair, subject string) []rejectionCase {
	return []rejectionCase{
		{
			name:          "missing_header",
			authorization: func(*testing.T) string { return "" },
		},
		{
			name:          "malformed_bearer",
			authorization: func(*testing.T) string { return "Bearer not-a-jwt" },
		},
		{
			name: "expired_token",
			authorization: func(t *testing.T) string {
				return "Bearer " + testsupport.ExpiredToken(t, kp, subject)
			},
		},
		{
			name: "wrong_algorithm",
			authorization: func(t *testing.T) string {
				return "Bearer " + algNoneToken(t, subject)
			},
		},
		{
			// The domain has no "refresh" class — refresh tokens are opaque,
			// stored server-side, and never presented to this middleware. A
			// token that claims cls=refresh is therefore the purest form of
			// the wrong-class case: structurally valid, correctly signed,
			// correct issuer and audience, and still not an access token.
			name: "refresh_class",
			authorization: func(t *testing.T) string {
				return "Bearer " + testsupport.MintToken(t, kp, testsupport.Claims{
					Subject: subject,
					Class:   "refresh",
				})
			},
		},
	}
}

// TestCanonical401Contract asserts the canonical 401 by value.
//
// The goldens prove the five cases agree with each other. Agreement alone is
// weak evidence: it would still hold if all five regressed together, and it
// says nothing about *which* shape they agreed on. This test names the exact
// bytes the contract requires — the media type, the challenge, and every RFC
// 9457 member — so that a regression has to be a deliberate edit here rather
// than a silently re-recorded golden.
//
// It is the specification the file above is deliberately not.
func TestCanonical401Contract(t *testing.T) {
	kp := testsupport.NewKeypair(t)
	subject := uuid.New().String()

	svc := &recordingAuthService{}
	router := protectedRouter(t, kp, svc)

	// Spelled as literals rather than as internal/httpx's constants on
	// purpose: asserting a value against the constant that produced it proves
	// only that the constant equals itself. These are the bytes a client sees,
	// and changing either side alone must fail.
	const (
		wantContentType = "application/problem+json"
		wantChallenge   = `Bearer realm="api"`
		wantType        = "about:blank"
		wantTitle       = "Unauthorized"
		wantDetail      = "authentication required"
	)

	// reasonVocabulary is the wording the pre-A4 bodies leaked, plus the terms
	// the verifier distinguishes internally. None may appear in a response
	// body: which of them applies is exactly the oracle the canonical detail
	// exists to deny an attacker.
	reasonVocabulary := []string{
		"missing", "invalid", "malformed", "expired",
		"signature", "algorithm", "alg", "class", "token",
	}

	var bodies []string
	for _, tc := range rejectionCases(kp, subject) {
		t.Run(tc.name, func(t *testing.T) {
			svc.reset()

			req := testsupport.JSONRequest(t, protectedMethod, protectedPath, nil)
			if h := tc.authorization(t); h != "" {
				req.Header.Set("Authorization", h)
			}
			rec := testsupport.Do(t, router, req)

			require.Equal(t, http.StatusUnauthorized, rec.Code)
			require.Equal(t, wantContentType, rec.Header().Get("Content-Type"))
			require.Equal(t, wantChallenge, rec.Header().Get("WWW-Authenticate"),
				"RFC 9110 requires a 401 to carry a challenge")
			// Not captured by the golden struct, so asserted here rather than
			// left to a claim in a review: WriteProblem sets it, and a
			// problem+json body served without it is sniffable.
			require.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))

			var got httpx.Problem
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
			require.Equal(t, wantType, got.Type)
			require.Equal(t, wantTitle, got.Title)
			require.Equal(t, http.StatusUnauthorized, got.Status)
			require.Equal(t, wantDetail, got.Detail)
			require.Equal(t, protectedPath, got.Instance)
			// This rig mounts no chi RequestID middleware, so the extension
			// member is omitted. Asserting it is empty pins that the goldens
			// are stable rather than carrying a value that changes per run.
			require.Empty(t, got.RequestID)

			lower := strings.ToLower(rec.Body.String())
			for _, word := range reasonVocabulary {
				require.NotContains(t, lower, word,
					"the %s response body names why the credential failed", tc.name)
			}

			bodies = append(bodies, rec.Body.String())
		})
	}

	// Mutual equality, asserted last and worth little on its own — the value
	// assertions above are what give it meaning. Together they say: every
	// rejection emits this one shape, and this one shape is the right one.
	require.Len(t, bodies, 5, "every case must have produced a body")
	for i, body := range bodies {
		require.Equal(t, bodies[0], body,
			"rejection bodies differ; case %d leaks which check failed", i)
	}
}

// compareGolden writes want to path under -update, and otherwise fails with a
// readable diff of the two encodings.
func compareGolden(t *testing.T, path string, got unauthorizedGolden) {
	t.Helper()

	encoded, err := json.MarshalIndent(got, "", "  ")
	require.NoError(t, err)
	encoded = append(encoded, '\n')

	if *updateGolden {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, encoded, 0o644))
		t.Logf("wrote %s", path)
		return
	}

	want, err := os.ReadFile(path) //nolint:gosec // fixed test-data path
	if err != nil {
		t.Fatalf("read golden %s: %v\nrun the test with -update to create it", path, err)
	}

	// Compared as text so that a change in field order or indentation shows up
	// too: these files are read by humans auditing what the 401 used to be.
	if string(want) != string(encoded) {
		t.Errorf("response no longer matches %s\n--- want (golden)\n%s\n--- got\n%s\n"+
			"if the change is intended, rerun with -update and commit the diff",
			path, want, encoded)
	}
}

// protectedRouter mounts AuthHandler's real route tree, behind the real
// AuthRequired middleware, over the real RS256 verifier built from kp.
//
// No database is involved: every case in this file is refused before a handler
// runs, which is why svc can be a recorder rather than a working service.
func protectedRouter(t *testing.T, kp testsupport.Keypair, svc AuthService) http.Handler {
	t.Helper()

	// Leeway is pinned to something small and explicit rather than left to the
	// service default, so the "expired" case cannot start depending on the
	// 30-second default skew allowance.
	jwtSvc, err := token.NewJWTService(kp.PrivatePath, kp.PublicPath,
		testsupport.TestIssuer, testsupport.TestAudience, time.Second)
	require.NoError(t, err)

	// keys is nil: this handler's JWKS route is not under test and omitting it
	// keeps the captured route tree to the protected surface.
	h := NewAuthHandler(svc, time.Hour, AuthRequired(jwtSvc), nil)

	r := chi.NewRouter()
	r.Route("/api/v1", h.Routes)
	return r
}

// algNoneToken mints an unsigned ("alg":"none") token carrying otherwise
// impeccable access-token claims.
//
// testsupport.MintToken cannot produce this — it always signs RS256, which is
// the correct default for a helper whose job is minting tokens the application
// should mostly accept. The alg-confusion case has to be built by hand, and
// building it here keeps the unsafe signing constant out of shared test code.
func algNoneToken(t *testing.T, subject string) string {
	t.Helper()

	now := time.Now()
	claims := jwt.MapClaims{
		"sub":         subject,
		"roles":       []string{"user"},
		"permissions": []string{},
		"scope":       testsupport.ClassAccess,
		"cls":         testsupport.ClassAccess,
		"iss":         testsupport.TestIssuer,
		"aud":         testsupport.TestAudience,
		"iat":         now.Unix(),
		"exp":         now.Add(15 * time.Minute).Unix(),
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodNone, claims).
		SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)
	return signed
}

// recordingAuthService satisfies AuthService without doing anything. It counts
// calls so the test can prove the middleware, not the handler, produced every
// response recorded above.
type recordingAuthService struct {
	calls int
}

func (s *recordingAuthService) reset() { s.calls = 0 }

func (s *recordingAuthService) Login(context.Context, app.LoginRequest) (*app.TokenResponse, error) {
	s.calls++
	return nil, nil
}

func (s *recordingAuthService) Logout(context.Context, string) error {
	s.calls++
	return nil
}

func (s *recordingAuthService) SetupMFA(context.Context, string) (*app.MfaSetupResponse, error) {
	s.calls++
	return nil, nil
}

func (s *recordingAuthService) VerifyMFASetup(context.Context, string, string) (*app.MfaVerifySetupResponse, error) {
	s.calls++
	return nil, nil
}

func (s *recordingAuthService) VerifyMFA(context.Context, string, app.MfaVerifyRequest) (*app.TokenResponse, error) {
	s.calls++
	return nil, nil
}

func (s *recordingAuthService) Refresh(context.Context, string) (*app.TokenResponse, error) {
	s.calls++
	return nil, nil
}
