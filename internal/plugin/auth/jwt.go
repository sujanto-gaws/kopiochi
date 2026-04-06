package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Plugin is the interface that JWTPlugin implements
type Plugin interface {
	Name() string
	Initialize(cfg map[string]interface{}) error
	Close() error
	Middleware() func(http.Handler) http.Handler
	AuthMiddleware() func(http.Handler) http.Handler
	ExtractUserID(ctx context.Context) string
	Provider() interface{}
}

// Context keys
type contextKey string

const (
	UserIDContextKey    contextKey = "user_id"
	UserNameContextKey  contextKey = "user_name"
	UserEmailContextKey contextKey = "user_email"
)

// JWTPlugin implements plugin.AuthPlugin for JWT-based authentication.
type JWTPlugin struct {
	initialized bool
	secret      []byte
	expiry      time.Duration
	issuer      string
}

// JWTConfig holds configuration for the JWT plugin.
type JWTConfig struct {
	Secret string `mapstructure:"secret"`
	Expiry string `mapstructure:"expiry"` // e.g., "24h", "7d"
	Issuer string `mapstructure:"issuer"`
}

// Name returns the plugin name.
func (p *JWTPlugin) Name() string {
	return "jwt-auth"
}

// Initialize sets up the JWT plugin with configuration.
func (p *JWTPlugin) Initialize(cfg map[string]interface{}) error {
	secret, ok := cfg["secret"].(string)
	if !ok || secret == "" {
		return fmt.Errorf("jwt-auth: secret is required")
	}

	p.secret = []byte(secret)

	// Parse expiry if provided
	if expiryStr, ok := cfg["expiry"].(string); ok && expiryStr != "" {
		d, err := time.ParseDuration(expiryStr)
		if err != nil {
			return fmt.Errorf("jwt-auth: invalid expiry duration: %w", err)
		}
		p.expiry = d
	} else {
		p.expiry = 24 * time.Hour // Default 24 hours
	}

	// Parse issuer if provided
	if issuer, ok := cfg["issuer"].(string); ok {
		p.issuer = issuer
	}

	p.initialized = true
	return nil
}

// Close performs cleanup (no-op for JWT plugin).
func (p *JWTPlugin) Close() error {
	p.initialized = false
	return nil
}

// Middleware returns the JWT authentication middleware.
func (p *JWTPlugin) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
				return
			}

			// Extract token from "Bearer <token>"
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
				http.Error(w, `{"error":"invalid authorization format"}`, http.StatusUnauthorized)
				return
			}

			tokenStr := parts[1]

			// Parse and validate token
			claims := &jwt.RegisteredClaims{}
			token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
				return p.secret, nil
			})

			if err != nil || !token.Valid {
				http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
				return
			}

			// Add user info to context
			ctx := r.Context()
			if claims.Subject != "" {
				ctx = context.WithValue(ctx, UserIDContextKey, claims.Subject)
			}
			if claims.Issuer != "" {
				ctx = context.WithValue(ctx, UserNameContextKey, claims.Issuer)
			}

			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
		})
	}
}

// AuthMiddleware returns the authentication middleware (satisfies AuthPlugin).
func (p *JWTPlugin) AuthMiddleware() func(http.Handler) http.Handler {
	return p.Middleware()
}

// ExtractUserID extracts user ID from request context.
func (p *JWTPlugin) ExtractUserID(ctx context.Context) string {
	if userID, ok := ctx.Value(UserIDContextKey).(string); ok {
		return userID
	}
	return ""
}

// Provider returns the plugin instance itself.
func (p *JWTPlugin) Provider() interface{} {
	return p
}

// GenerateToken creates a new JWT token for a user.
func (p *JWTPlugin) GenerateToken(userID, name, email string) (string, error) {
	if !p.initialized {
		return "", fmt.Errorf("jwt-auth: plugin not initialized")
	}

	now := time.Now()
	claims := &jwt.RegisteredClaims{
		Subject:   userID,
		Issuer:    name,
		ExpiresAt: jwt.NewNumericDate(now.Add(p.expiry)),
		IssuedAt:  jwt.NewNumericDate(now),
		NotBefore: jwt.NewNumericDate(now),
	}

	if p.issuer != "" {
		claims.Issuer = p.issuer
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(p.secret)
}

// NewJWTPlugin creates a new JWT authentication plugin instance.
func NewJWTPlugin() *JWTPlugin {
	return &JWTPlugin{}
}
