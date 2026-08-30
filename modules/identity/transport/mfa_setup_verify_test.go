package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/sujanto-gaws/kopiochi/internal/testsupport"
	app "github.com/sujanto-gaws/kopiochi/modules/identity/application"
)

// This package had no handler tests, which is why an error branch could be
// added to MFAVerifySetup and land on the default 500 with nothing objecting.
// These cover the one route E10 touched. The rest of the handlers remain
// uncovered and that is a separate, larger gap.

const setupVerifySubject = "3f1b8a54-2c9e-4d77-9a1e-2b6c0d5e8f41"

// stubAuthService returns a fixed error from VerifyMFASetup and nothing else.
// Every other method exists to satisfy AuthService and must not be called.
type stubAuthService struct {
	verifySetupErr  error
	verifySetupResp *app.MfaVerifySetupResponse
}

func (s stubAuthService) VerifyMFASetup(context.Context, string, string) (*app.MfaVerifySetupResponse, error) {
	return s.verifySetupResp, s.verifySetupErr
}
func (s stubAuthService) Login(context.Context, app.LoginRequest) (*app.TokenResponse, error) {
	panic("Login is not part of this test")
}
func (s stubAuthService) Logout(context.Context, string) error {
	panic("Logout is not part of this test")
}
func (s stubAuthService) SetupMFA(context.Context, string) (*app.MfaSetupResponse, error) {
	panic("SetupMFA is not part of this test")
}
func (s stubAuthService) VerifyMFA(context.Context, string, app.MfaVerifyRequest) (*app.TokenResponse, error) {
	panic("VerifyMFA is not part of this test")
}
func (s stubAuthService) Refresh(context.Context, string) (*app.TokenResponse, error) {
	panic("Refresh is not part of this test")
}

func postSetupVerify(t *testing.T, svc AuthService) *httptest.ResponseRecorder {
	t.Helper()

	h := NewAuthHandler(svc, time.Hour, testsupport.FakeAuth(setupVerifySubject), nil)
	r := chi.NewRouter()
	r.Route("/api/v1", h.Routes)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/mfa/setup/verify",
		strings.NewReader(`{"code":"123456"}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// TestMFAVerifySetup_NotStartedIs400NotA500 — E10.
//
// Confirming a setup that was never begun is the caller's sequencing mistake,
// not a server fault. Without a mapping it fell through to the default 500,
// which tells an operator their service is broken and tells the caller nothing
// they can act on.
func TestMFAVerifySetup_NotStartedIs400NotA500(t *testing.T) {
	rec := postSetupVerify(t, stubAuthService{verifySetupErr: app.ErrMFANotStarted})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body: %s)", rec.Code, rec.Body)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v (%q)", err, rec.Body)
	}
	if got, _ := body["type"].(string); !strings.Contains(got, "mfa_not_started") {
		t.Errorf("type = %v, want it to name mfa_not_started so a client can tell "+
			"this from a mistyped code", body["type"])
	}
}

// TestMFAVerifySetup_BadCodeStaysDistinct is the control: the new branch must
// not have swallowed the existing one. Only one of these two is something a
// user can fix with their authenticator.
func TestMFAVerifySetup_BadCodeStaysDistinct(t *testing.T) {
	rec := postSetupVerify(t, stubAuthService{verifySetupErr: app.ErrInvalidMFACode})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body: %s)", rec.Code, rec.Body)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v (%q)", err, rec.Body)
	}
	if got, _ := body["type"].(string); !strings.Contains(got, "invalid_code") {
		t.Errorf("type = %v, want invalid_code", body["type"])
	}
}
