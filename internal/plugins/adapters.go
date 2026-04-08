package plugins

import (
	"context"
	"net/http"

	"github.com/sujanto-gaws/kopiochi/internal/plugins/auth"
	"github.com/sujanto-gaws/kopiochi/internal/plugins/middleware"
)

// middlewarePluginAdapter wraps middleware.Plugin to implement plugin.Plugin interface
type middlewarePluginAdapter struct {
	mw middleware.Plugin
}

func (a *middlewarePluginAdapter) Name() string {
	return a.mw.Name()
}

func (a *middlewarePluginAdapter) Initialize(cfg map[string]interface{}) error {
	return a.mw.Initialize(cfg)
}

func (a *middlewarePluginAdapter) Close() error {
	return a.mw.Close()
}

func (a *middlewarePluginAdapter) Middleware() func(http.Handler) http.Handler {
	return a.mw.Middleware()
}

// authPluginAdapter wraps auth.JWTPlugin to implement plugin interfaces
type authPluginAdapter struct {
	auth *auth.JWTPlugin
}

func (a *authPluginAdapter) Name() string {
	return a.auth.Name()
}

func (a *authPluginAdapter) Initialize(cfg map[string]interface{}) error {
	return a.auth.Initialize(cfg)
}

func (a *authPluginAdapter) Close() error {
	return a.auth.Close()
}

func (a *authPluginAdapter) Middleware() func(http.Handler) http.Handler {
	return a.auth.Middleware()
}

func (a *authPluginAdapter) AuthMiddleware() func(http.Handler) http.Handler {
	return a.auth.AuthMiddleware()
}

func (a *authPluginAdapter) ExtractUserID(ctx context.Context) string {
	return a.auth.ExtractUserID(ctx)
}

func (a *authPluginAdapter) Provider() interface{} {
	return a.auth.Provider()
}
