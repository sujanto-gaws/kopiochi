package httpx

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"

	"github.com/sujanto-gaws/kopiochi/internal/config"
	"github.com/sujanto-gaws/kopiochi/internal/metrics"
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
// m may be nil, which disables instrumentation entirely — no counters, no
// middleware, no cost.
func NewRouter(srv config.Server, sec config.Security, log zerolog.Logger, m *metrics.Metrics, mw ...func(http.Handler) http.Handler) (*chi.Mux, func() error, error) {
	r := chi.NewRouter()

	// Errors the router itself produces get the same problem+json shape as
	// everything a handler emits, rather than chi's bare text/plain default.
	r.NotFound(NotFound())
	r.MethodNotAllowed(MethodNotAllowed(r))

	// Core middleware stack (order matters).
	//
	// RequestID comes first, ahead of recovery. It used to run second, which
	// meant a recovered panic — the one log line where correlation matters
	// most — could not carry a request ID even in principle, because none had
	// been generated yet. See observability.md, Problem 3.
	r.Use(chimw.RequestID)
	// chi's RequestID only puts the id in the request context — it sets no
	// response header. So the id appears in our logs and in problem+json
	// bodies, but a client that got a *successful* response has nothing to
	// quote when they later report that it was wrong. Echoing it makes every
	// response correlatable, not just the failures.
	r.Use(EchoRequestID)
	// Recovery replaces chi's middleware.Recoverer: problem+json instead of an
	// empty body, and a structured log line instead of a plain-text stack
	// dump on stderr that no log pipeline can read.
	r.Use(Recovery(log))
	// Cheap and applies to every response, including ones later middleware
	// or the router itself produces (recovered panics, 404s, 405s): headers
	// set here are already on the ResponseWriter by the time anything
	// downstream writes a status.
	r.Use(SecurityHeaders(SecurityHeadersConfig{EnableHSTS: srv.EnableHSTS}))
	// corenet.RealIP replaces chi's middleware.RealIP, which trusts
	// forwarded headers from any peer unconditionally. Ours resolves the
	// client IP from trusted proxies only (empty by default = trust nothing)
	// and stores it in the request context for the request logger and the
	// rate limiter to consume, instead of every consumer re-parsing headers
	// itself. It must run before anything that keys or logs on client IP --
	// including the rate limiter registered below.
	r.Use(corenet.RealIP(corenet.ParseTrustedProxies(srv.TrustedProxies, log)))
	r.Use(chimw.Timeout(srv.RequestTimeout))
	// RequestLogger runs after RealIP so the resolved client IP is available
	// to bind onto the request-scoped logger, and after Recovery so that the
	// 500 a panic produces is counted and logged like any other response.
	r.Use(corenet.RequestLogger(log))

	// Ahead of CORS and the rate limiter, so a preflight and a 429 are both
	// counted. A limiter that short-circuits below an instrumentation
	// middleware makes rejected traffic invisible — exactly the traffic worth
	// seeing. The cost is that a short-circuited request never reaches the
	// mux, so it has no route pattern and is labelled "unmatched"; the status
	// label still distinguishes it.
	if m != nil {
		r.Use(m.Middleware())
	}

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
