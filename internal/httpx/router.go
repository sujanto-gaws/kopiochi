package httpx

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/sujanto-gaws/kopiochi/internal/config"
	corenet "github.com/sujanto-gaws/kopiochi/internal/middleware"
)

// NewRouter builds the chi router with the core middleware stack applied,
// plus whichever cross-cutting middlewares the security config enables.
//
// It replaces the plugin registry that used to assemble this chain
// (internal/plugin.NewMiddlewareChainFromRegistry). CORS and rate limiting
// are not business capabilities; they are HTTP concerns with typed config, so
// roughly 1,100 LOC of registration framework collapses into an `if` per
// middleware. See
// docs/architectures/01-modularity/extension-framework.md, "What about
// genuinely optional middleware?".
//
// The returned close function releases everything the router constructed that
// owns a resource -- today, the rate limiter's eviction goroutine. Callers
// must call it on shutdown. It is safe to call even when nothing was
// constructed.
func NewRouter(srv config.Server, sec config.Security, mw ...func(http.Handler) http.Handler) (*chi.Mux, func() error, error) {
	r := chi.NewRouter()

	// Core middleware stack (order matters).
	r.Use(chimw.Recoverer)
	r.Use(chimw.RequestID)
	// Cheap and applies to every response, including ones later middleware
	// or the router itself produces (panics recovered by Recoverer above,
	// 404s, 405s): headers set here are already on the ResponseWriter by the
	// time anything downstream writes a status.
	r.Use(SecurityHeaders(SecurityHeadersConfig{EnableHSTS: srv.EnableHSTS}))
	// corenet.RealIP replaces chi's middleware.RealIP, which trusts
	// forwarded headers from any peer unconditionally. Ours resolves the
	// client IP from trusted proxies only (empty by default = trust nothing)
	// and stores it in the request context for the request logger and the
	// rate limiter to consume, instead of every consumer re-parsing headers
	// itself. It must run before anything that keys or logs on client IP --
	// including the rate limiter registered below.
	r.Use(corenet.RealIP(corenet.ParseTrustedProxies(srv.TrustedProxies)))
	r.Use(chimw.Timeout(srv.RequestTimeout))
	r.Use(corenet.ZerologRequestLogger)

	closers := make([]func() error, 0, 1)
	closeAll := func() error {
		var err error
		// LIFO: release in reverse construction order.
		for i := len(closers) - 1; i >= 0; i-- {
			if cerr := closers[i](); cerr != nil && err == nil {
				err = cerr
			}
		}
		return err
	}

	// CORS is registered before the rate limiter so that a throttled
	// cross-origin request still carries Access-Control-Allow-Origin: a 429
	// without it reaches the browser as an opaque network error, and the
	// caller cannot see the status it is supposed to back off on.
	//
	// The consequence is that a preflight short-circuits before the limiter
	// and so does not consume budget. Preflight handling is a few header
	// writes and no I/O, so that is the cheaper side of the trade.
	if sec.CORS.Enabled {
		r.Use(CORS(sec.CORS))
	}

	if sec.RateLimit.Enabled {
		limiter, err := NewRateLimiter(sec.RateLimit)
		if err != nil {
			// Nothing constructed so far owns a resource, but run the
			// closers anyway so this stays correct if that changes.
			_ = closeAll()
			return nil, nil, fmt.Errorf("build rate limiter: %w", err)
		}
		closers = append(closers, limiter.Close)
		r.Use(limiter.Middleware())
	}

	// Caller-supplied middleware applied after the core stack.
	if len(mw) > 0 {
		r.Use(mw...)
	}

	return r, closeAll, nil
}
