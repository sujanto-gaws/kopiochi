package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	app "github.com/sujanto-gaws/kopiochi/internal/application/auth"
	domain "github.com/sujanto-gaws/kopiochi/internal/domain/auth"
)

// AuthService is the set of application operations AuthHandler depends on.
type AuthService interface {
	Login(ctx context.Context, req app.LoginRequest) (*app.TokenResponse, error)
	Logout(ctx context.Context, userID string) error
	SetupMFA(ctx context.Context, userID string) (*app.MfaSetupResponse, error)
	VerifyMFASetup(ctx context.Context, userID string, code string) (*app.MfaVerifySetupResponse, error)
	VerifyMFA(ctx context.Context, mfaToken string, req app.MfaVerifyRequest) (*app.TokenResponse, error)
	Refresh(ctx context.Context, refreshToken string) (*app.TokenResponse, error)
}

// AuthHandler handles HTTP requests for authentication operations
type AuthHandler struct {
	svc        AuthService
	refreshTTL time.Duration
}

// NewAuthHandler creates a new auth handler
func NewAuthHandler(svc AuthService, refreshTTL time.Duration) *AuthHandler {
	return &AuthHandler{svc: svc, refreshTTL: refreshTTL}
}

// Login handles POST /auth/login
// @Summary Login with username and password
// @Description Authenticates a user and returns an access token and refresh token.
// @Description If MFA is enabled, returns HTTP 202 with an mfa_token instead.
// @Tags auth
// @Accept json
// @Produce json
// @Param request body app.LoginRequest true "Login credentials"
// @Success 200 {object} app.TokenResponse "Authentication successful"
// @Success 202 {object} app.MfaRequiredResponse "MFA required"
// @Failure 400 {object} auth.OAuth2Error "invalid_request or invalid_grant"
// @Failure 423 {object} auth.ProblemDetails "account_locked"
// @Failure 500 {object} auth.ProblemDetails "internal_error"
// @Router /auth/login [post]
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req app.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOAuth2Error(w, "invalid_request", "malformed request", http.StatusBadRequest)
		return
	}
	resp, err := h.svc.Login(r.Context(), req)
	if err != nil {
		var mfaErr *app.MFAError
		if errors.As(err, &mfaErr) {
			writeJSON(w, http.StatusAccepted, app.MfaRequiredResponse{
				MFARequired: true,
				MFAToken:    mfaErr.Token,
				User:        mfaErr.User,
			})
			return
		}
		if errors.Is(err, app.ErrInvalidCredentials) {
			writeOAuth2Error(w, "invalid_grant", "Invalid email or password", http.StatusBadRequest)
			return
		}
		if errors.Is(err, app.ErrAccountLocked) {
			writeProblemDetails(w, "account_locked", "Account locked", http.StatusLocked, "Too many failed attempts")
			return
		}
		writeProblemDetails(w, "internal_error", "Internal server error", http.StatusInternalServerError, "")
		return
	}
	setRefreshCookie(w, resp.RefreshToken, h.refreshTTL)
	resp.RefreshToken = ""
	writeJSON(w, http.StatusOK, resp)
}

// Logout handles POST /auth/logout
// @Summary Logout
// @Description Revokes the current user's refresh token. Requires a valid access token.
// @Tags auth
// @Produce json
// @Security BearerAuth
// @Success 204 "Logged out successfully"
// @Failure 401 {object} auth.ProblemDetails "unauthorized"
// @Failure 500 {object} auth.ProblemDetails "internal_error"
// @Router /auth/logout [post]
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	claims, ok := r.Context().Value(domain.ClaimsKey).(*domain.Claims)
	if !ok || claims.Subject == "" {
		writeProblemDetails(w, "unauthorized", "Unauthorized", http.StatusUnauthorized, "")
		return
	}
	if err := h.svc.Logout(r.Context(), claims.Subject); err != nil {
		writeProblemDetails(w, "internal_error", "Logout failed", http.StatusInternalServerError, "")
		return
	}
	clearRefreshCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// MFASetup handles POST /auth/mfa/setup
// @Summary Initiate MFA setup
// @Description Generates a TOTP secret and QR code URL for the authenticated user.
// @Description Confirm setup by calling POST /auth/mfa/setup/verify with a valid code.
// @Tags auth
// @Produce json
// @Security BearerAuth
// @Success 200 {object} app.MfaSetupResponse "Secret and QR code URL"
// @Failure 401 {object} auth.ProblemDetails "unauthorized"
// @Failure 500 {object} auth.ProblemDetails "internal_error"
// @Router /auth/mfa/setup [post]
func (h *AuthHandler) MFASetup(w http.ResponseWriter, r *http.Request) {
	claims, ok := r.Context().Value(domain.ClaimsKey).(*domain.Claims)
	if !ok || claims == nil {
		writeProblemDetails(w, "unauthorized", "Unauthorized", http.StatusUnauthorized, "")
		return
	}
	resp, err := h.svc.SetupMFA(r.Context(), claims.Subject)
	if err != nil {
		writeProblemDetails(w, "internal_error", "MFA setup failed", http.StatusInternalServerError, "")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// MFAVerifySetup handles POST /auth/mfa/setup/verify
// @Summary Confirm MFA setup
// @Description Validates a TOTP code to activate MFA for the authenticated user.
// @Description Returns one-time backup codes on success.
// @Tags auth
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body app.MfaVerifySetupRequest true "TOTP code from authenticator app"
// @Success 200 {object} app.MfaVerifySetupResponse "MFA enabled; backup codes returned"
// @Failure 400 {object} auth.ProblemDetails "invalid_code"
// @Failure 401 {object} auth.ProblemDetails "unauthorized"
// @Failure 500 {object} auth.ProblemDetails "internal_error"
// @Router /auth/mfa/setup/verify [post]
func (h *AuthHandler) MFAVerifySetup(w http.ResponseWriter, r *http.Request) {
	claims, ok := r.Context().Value(domain.ClaimsKey).(*domain.Claims)
	if !ok || claims == nil {
		writeProblemDetails(w, "unauthorized", "Unauthorized", http.StatusUnauthorized, "")
		return
	}
	var req app.MfaVerifySetupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblemDetails(w, "bad_request", "Invalid request", http.StatusBadRequest, "")
		return
	}
	resp, err := h.svc.VerifyMFASetup(r.Context(), claims.Subject, req.Code)
	if err != nil {
		if err == app.ErrInvalidMFACode {
			writeProblemDetails(w, "invalid_code", "Invalid TOTP code", http.StatusBadRequest, "")
			return
		}
		writeProblemDetails(w, "internal_error", "Verification failed", http.StatusInternalServerError, "")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// MFAVerify handles POST /auth/mfa/verify
// @Summary Verify MFA code during login
// @Description Completes the MFA login step. Supply the short-lived mfa_token from
// @Description the 202 login response as a Bearer token in the Authorization header.
// @Tags auth
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer <mfa_token>"
// @Param request body app.MfaVerifyRequest true "TOTP code or backup code"
// @Success 200 {object} app.TokenResponse "Authentication successful"
// @Failure 400 {object} auth.OAuth2Error "invalid_grant — wrong code"
// @Failure 401 {object} auth.OAuth2Error "invalid_token — bad or expired MFA token"
// @Failure 500 {object} auth.OAuth2Error "server_error"
// @Router /auth/mfa/verify [post]
func (h *AuthHandler) MFAVerify(w http.ResponseWriter, r *http.Request) {
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		writeOAuth2Error(w, "invalid_request", "Missing mfa token", http.StatusUnauthorized)
		return
	}
	mfaToken := strings.TrimPrefix(authHeader, "Bearer ")
	var req app.MfaVerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOAuth2Error(w, "invalid_request", "Invalid body", http.StatusBadRequest)
		return
	}
	resp, err := h.svc.VerifyMFA(r.Context(), mfaToken, req)
	if err != nil {
		if errors.Is(err, app.ErrInvalidMFAToken) {
			writeOAuth2Error(w, "invalid_token", "Invalid MFA token", http.StatusUnauthorized)
			return
		}
		if errors.Is(err, app.ErrInvalidMFACode) {
			writeOAuth2Error(w, "invalid_grant", "Invalid code", http.StatusBadRequest)
			return
		}
		writeOAuth2Error(w, "server_error", "Internal error", http.StatusInternalServerError)
		return
	}
	setRefreshCookie(w, resp.RefreshToken, h.refreshTTL)
	resp.RefreshToken = ""
	writeJSON(w, http.StatusOK, resp)
}

// Refresh handles POST /auth/refresh
// @Summary Refresh access token
// @Description Issues a new access token using a valid refresh token.
// @Description The refresh token may be supplied in the JSON body or as a "refresh_token" cookie.
// @Tags auth
// @Accept json
// @Produce json
// @Param request body app.RefreshRequest false "Refresh token (omit if using cookie)"
// @Success 200 {object} app.TokenResponse "New tokens issued"
// @Failure 400 {object} auth.OAuth2Error "invalid_request or invalid_grant"
// @Failure 500 {object} auth.ProblemDetails "internal_error"
// @Router /auth/refresh [post]
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req app.RefreshRequest
	// Accept from body or cookie
	if r.Body != nil {
		json.NewDecoder(r.Body).Decode(&req)
	}
	if req.RefreshToken == "" {
		cookie, _ := r.Cookie("refresh_token")
		if cookie != nil {
			req.RefreshToken = cookie.Value
		}
	}
	if req.RefreshToken == "" {
		writeOAuth2Error(w, "invalid_request", "refresh token missing", http.StatusBadRequest)
		return
	}
	resp, err := h.svc.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		if errors.Is(err, app.ErrRefreshTokenInvalid) {
			writeOAuth2Error(w, "invalid_grant", "Invalid or expired refresh token", http.StatusBadRequest)
			return
		}
		writeProblemDetails(w, "internal_error", "Internal error", http.StatusInternalServerError, "")
		return
	}
	setRefreshCookie(w, resp.RefreshToken, h.refreshTTL)
	resp.RefreshToken = ""
	writeJSON(w, http.StatusOK, resp)
}

// RegisterRoutes implements handlers.RouteRegistrar.
//
//	Public routes (no token required):
//	  POST /auth/login
//	  POST /auth/refresh
//	  POST /auth/mfa/verify      (uses short-lived MFA token, not access token)
//
//	Protected routes (access token required):
//	  POST /auth/logout
//	  POST /auth/mfa/setup
//	  POST /auth/mfa/setup/verify
func (h *AuthHandler) RegisterRoutes(g RouterGroup) {
	g.Public.Post("/auth/login", h.Login)
	g.Public.Post("/auth/refresh", h.Refresh)
	g.Public.Post("/auth/mfa/verify", h.MFAVerify)

	g.Protected.Post("/auth/logout", h.Logout)
	g.Protected.Post("/auth/mfa/setup", h.MFASetup)
	g.Protected.Post("/auth/mfa/setup/verify", h.MFAVerifySetup)
}
