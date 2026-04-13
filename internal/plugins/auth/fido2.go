package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
)

// FIDO2Plugin implements middleware.Plugin for FIDO2/WebAuthn passwordless authentication.
type FIDO2Plugin struct {
	initialized  bool
	webauthn     *webauthn.WebAuthn
	rpID         string
	rpOrigin     string
	rpName       string
	userStore    UserStore
	sessionStore SessionStore
}

// UserStore is an interface for managing FIDO2 users and credentials.
type UserStore interface {
	GetUserByID(ctx context.Context, userID []byte) (WebAuthnUser, error)
	GetUserByName(ctx context.Context, name string) (WebAuthnUser, error)
	SaveUser(ctx context.Context, user WebAuthnUser) error
	GetUserByCredentialID(ctx context.Context, credentialID []byte) (WebAuthnUser, error)
}

// SessionStore is an interface for managing FIDO2 sessions.
type SessionStore interface {
	CreateSession(ctx context.Context, sessionID string, data map[string]interface{}) error
	GetSession(ctx context.Context, sessionID string) (map[string]interface{}, error)
	DeleteSession(ctx context.Context, sessionID string) error
}

// WebAuthnUser represents a FIDO2 user with credentials.
type WebAuthnUser struct {
	ID          []byte
	Name        string
	DisplayName string
	Credentials []webauthn.Credential
}

var _ webauthn.User = (*WebAuthnUser)(nil)

func (u *WebAuthnUser) WebAuthnID() []byte                         { return u.ID }
func (u *WebAuthnUser) WebAuthnName() string                       { return u.Name }
func (u *WebAuthnUser) WebAuthnDisplayName() string                { return u.DisplayName }
func (u *WebAuthnUser) WebAuthnIcon() string                       { return "" }
func (u *WebAuthnUser) WebAuthnCredentials() []webauthn.Credential { return u.Credentials }

// Context key for FIDO2 user ID
type contextKeyType struct{}

var FIDO2UserIDContextKey = contextKeyType{}

// Name returns the plugin name.
func (p *FIDO2Plugin) Name() string {
	return "fido2-auth"
}

// Initialize sets up the FIDO2 plugin with configuration.
func (p *FIDO2Plugin) Initialize(cfg map[string]interface{}) error {
	rpID, ok := cfg["rp_id"].(string)
	if !ok || rpID == "" {
		return fmt.Errorf("fido2-auth: rp_id (Relying Party ID) is required")
	}
	p.rpID = rpID

	rpOrigin, ok := cfg["rp_origin"].(string)
	if !ok || rpOrigin == "" {
		return fmt.Errorf("fido2-auth: rp_origin (Relying Party Origin) is required")
	}
	p.rpOrigin = rpOrigin

	if rpName, ok := cfg["rp_name"].(string); ok && rpName != "" {
		p.rpName = rpName
	} else {
		p.rpName = "Kopiochi"
	}

	displayName := "Kopiochi"
	if dn, ok := cfg["display_name"].(string); ok && dn != "" {
		displayName = dn
	}

	// UserStore is required
	if cfg["user_store"] != nil {
		if store, ok := cfg["user_store"].(UserStore); ok {
			p.userStore = store
		} else {
			return fmt.Errorf("fido2-auth: user_store must implement UserStore interface")
		}
	} else {
		return fmt.Errorf("fido2-auth: user_store is required")
	}

	// SessionStore is optional, defaults to memory
	if cfg["session_store"] != nil {
		if store, ok := cfg["session_store"].(SessionStore); ok {
			p.sessionStore = store
		} else {
			return fmt.Errorf("fido2-auth: session_store must implement SessionStore interface")
		}
	} else {
		p.sessionStore = NewMemorySessionStore()
	}

	config := &webauthn.Config{
		RPDisplayName: displayName,
		RPID:          rpID,
		RPOrigins:     []string{rpOrigin},
	}

	w, err := webauthn.New(config)
	if err != nil {
		return fmt.Errorf("fido2-auth: failed to create webauthn instance: %w", err)
	}

	p.webauthn = w
	p.initialized = true
	return nil
}

// Close performs cleanup.
func (p *FIDO2Plugin) Close() error {
	p.initialized = false
	p.webauthn = nil
	return nil
}

// Middleware returns the FIDO2 authentication middleware.
func (p *FIDO2Plugin) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !p.initialized {
				http.Error(w, `{"error":"fido2 plugin not initialized"}`, http.StatusServiceUnavailable)
				return
			}

			cookie, err := r.Cookie("fido2_session")
			if err != nil || cookie.Value == "" {
				next.ServeHTTP(w, r)
				return
			}

			ctx := r.Context()
			sessionData, err := p.sessionStore.GetSession(ctx, cookie.Value)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			if userID, ok := sessionData["user_id"].(string); ok {
				ctx = context.WithValue(ctx, FIDO2UserIDContextKey, userID)
				r = r.WithContext(ctx)
			}

			next.ServeHTTP(w, r)
		})
	}
}

// AuthMiddleware returns the authentication middleware (alias for Middleware).
func (p *FIDO2Plugin) AuthMiddleware() func(http.Handler) http.Handler {
	return p.Middleware()
}

// ExtractUserID extracts user ID from request context.
func (p *FIDO2Plugin) ExtractUserID(ctx context.Context) string {
	if userID, ok := ctx.Value(FIDO2UserIDContextKey).(string); ok {
		return userID
	}
	return ""
}

// Provider returns the plugin instance.
func (p *FIDO2Plugin) Provider() interface{} {
	return p
}

// BeginRegistration starts the FIDO2 registration process.
func (p *FIDO2Plugin) BeginRegistration(ctx context.Context, userID, userName, displayName string) (interface{}, string, error) {
	if !p.initialized {
		return nil, "", fmt.Errorf("fido2-auth: plugin not initialized")
	}

	user := &WebAuthnUser{
		ID:          []byte(userID),
		Name:        userName,
		DisplayName: displayName,
	}

	options, session, err := p.webauthn.BeginRegistration(
		user,
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired),
	)
	if err != nil {
		return nil, "", fmt.Errorf("fido2-auth: failed to begin registration: %w", err)
	}

	sessionID := uuid.New().String()
	if err := p.sessionStore.CreateSession(ctx, sessionID, map[string]interface{}{
		"type":    "registration",
		"session": session,
		"user_id": userID,
	}); err != nil {
		return nil, "", fmt.Errorf("fido2-auth: failed to store session: %w", err)
	}

	return options, sessionID, nil
}

// FinishRegistration completes the FIDO2 registration.
func (p *FIDO2Plugin) FinishRegistration(ctx context.Context, sessionID string, r *http.Request) (string, error) {
	if !p.initialized {
		return "", fmt.Errorf("fido2-auth: plugin not initialized")
	}

	sessionData, err := p.sessionStore.GetSession(ctx, sessionID)
	if err != nil {
		return "", fmt.Errorf("fido2-auth: session not found: %w", err)
	}

	session, ok := sessionData["session"].(webauthn.SessionData)
	if !ok {
		return "", fmt.Errorf("fido2-auth: invalid session data")
	}

	userID := sessionData["user_id"].(string)
	user := &WebAuthnUser{
		ID: []byte(userID),
	}

	credential, err := p.webauthn.FinishRegistration(user, session, r)
	if err != nil {
		return "", fmt.Errorf("fido2-auth: registration failed: %w", err)
	}

	user.Credentials = append(user.Credentials, *credential)
	if err := p.userStore.SaveUser(ctx, *user); err != nil {
		return "", fmt.Errorf("fido2-auth: failed to save user: %w", err)
	}

	p.sessionStore.DeleteSession(ctx, sessionID)

	return userID, nil
}

// BeginLogin starts the FIDO2 authentication process.
func (p *FIDO2Plugin) BeginLogin(ctx context.Context) (interface{}, string, error) {
	if !p.initialized {
		return nil, "", fmt.Errorf("fido2-auth: plugin not initialized")
	}

	options, session, err := p.webauthn.BeginDiscoverableLogin()
	if err != nil {
		return nil, "", fmt.Errorf("fido2-auth: failed to begin login: %w", err)
	}

	sessionID := uuid.New().String()
	if err := p.sessionStore.CreateSession(ctx, sessionID, map[string]interface{}{
		"type":    "login",
		"session": session,
	}); err != nil {
		return nil, "", fmt.Errorf("fido2-auth: failed to store session: %w", err)
	}

	return options, sessionID, nil
}

// FinishLogin completes the FIDO2 authentication.
func (p *FIDO2Plugin) FinishLogin(ctx context.Context, sessionID string, r *http.Request) (string, error) {
	if !p.initialized {
		return "", fmt.Errorf("fido2-auth: plugin not initialized")
	}

	sessionData, err := p.sessionStore.GetSession(ctx, sessionID)
	if err != nil {
		return "", fmt.Errorf("fido2-auth: session not found: %w", err)
	}

	session, ok := sessionData["session"].(webauthn.SessionData)
	if !ok {
		return "", fmt.Errorf("fido2-auth: invalid session data")
	}

	// Parse the credential assertion response from request
	parsedResponse, err := protocol.ParseCredentialRequestResponseBody(r.Body)
	if err != nil {
		return "", fmt.Errorf("fido2-auth: failed to parse credential assertion: %w", err)
	}

	_, err = p.webauthn.ValidateDiscoverableLogin(func(_, userHandle []byte) (webauthn.User, error) {
		if len(userHandle) == 0 {
			return nil, fmt.Errorf("user handle is required")
		}
		user, err := p.userStore.GetUserByID(ctx, userHandle)
		return &user, err
	}, session, parsedResponse)

	if err != nil {
		return "", fmt.Errorf("fido2-auth: login validation failed: %w", err)
	}

	// Get user ID from session
	userID := sessionData["user_id"].(string)
	sessionData["user_id"] = userID
	p.sessionStore.CreateSession(ctx, sessionID, sessionData)

	return userID, nil
}

// GetWebAuthn returns the underlying webauthn instance.
func (p *FIDO2Plugin) GetWebAuthn() *webauthn.WebAuthn {
	return p.webauthn
}

// NewFIDO2Plugin creates a new FIDO2 authentication plugin instance.
func NewFIDO2Plugin() *FIDO2Plugin {
	return &FIDO2Plugin{}
}

// MemorySessionStore is a simple in-memory session store for development.
type MemorySessionStore struct {
	sessions map[string]map[string]interface{}
}

func NewMemorySessionStore() *MemorySessionStore {
	return &MemorySessionStore{
		sessions: make(map[string]map[string]interface{}),
	}
}

func (s *MemorySessionStore) CreateSession(ctx context.Context, sessionID string, data map[string]interface{}) error {
	s.sessions[sessionID] = data
	return nil
}

func (s *MemorySessionStore) GetSession(ctx context.Context, sessionID string) (map[string]interface{}, error) {
	if data, ok := s.sessions[sessionID]; ok {
		return data, nil
	}
	return nil, fmt.Errorf("session not found")
}

func (s *MemorySessionStore) DeleteSession(ctx context.Context, sessionID string) error {
	delete(s.sessions, sessionID)
	return nil
}

// FIDO2Response is a helper struct for JSON responses
type FIDO2Response struct {
	Success   bool        `json:"success"`
	Data      interface{} `json:"data,omitempty"`
	SessionID string      `json:"session_id,omitempty"`
	Error     string      `json:"error,omitempty"`
}

func FIDO2Success(data interface{}, sessionID string) FIDO2Response {
	return FIDO2Response{
		Success:   true,
		Data:      data,
		SessionID: sessionID,
	}
}

func FIDO2Error(message string) FIDO2Response {
	return FIDO2Response{
		Success: false,
		Error:   message,
	}
}

func WriteJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
