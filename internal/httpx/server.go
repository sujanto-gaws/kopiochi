package httpx

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"

	"github.com/rs/zerolog"

	"github.com/sujanto-gaws/kopiochi/internal/config"
)

// Server wraps http.Server with a Serve that reports failures to its caller
// instead of ending the process.
//
// See docs/architectures/02-composition/lifecycle-and-shutdown.md, problems 2
// and 3: the previous implementation called log.Fatal inside the serving
// goroutine, so a port-in-use error at startup called os.Exit(1) with no
// deferred function run, no pool drained and no shutdown func fired — and
// main never learned why. Run also returned nothing, so RunE returned nil and
// the process exited 0 on a server that never started.
type Server struct {
	httpServer *http.Server
	log        zerolog.Logger

	// draining is set the moment shutdown begins, so /readyz can start
	// failing before the drain rather than after it. It is read from request
	// goroutines, hence atomic.
	draining atomic.Bool
}

// NewServer builds the HTTP server from config. It does not listen; Serve
// does.
func NewServer(cfg config.Server, handler http.Handler, log zerolog.Logger) *Server {
	return &Server{
		httpServer: &http.Server{
			Addr:              fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
			Handler:           handler,
			ReadTimeout:       cfg.ReadTimeout,
			ReadHeaderTimeout: cfg.ReadHeaderTimeout,
			WriteTimeout:      cfg.WriteTimeout,
			IdleTimeout:       cfg.IdleTimeout,
		},
		log: log,
	}
}

// Serve listens and blocks until ctx is cancelled or the listener fails.
//
// A listen failure is returned, not fatal-logged: it travels up to RunE,
// which gives the process a non-zero exit code — what supervisors, CI and
// container restart policies actually read.
//
// A cancelled ctx is a clean stop and returns nil. Serve does not shut the
// server down itself; that is the lifecycle stack's job, and doing it in both
// places is the double-ownership this design exists to remove.
func (s *Server) Serve(ctx context.Context) error {
	errCh := make(chan error, 1)

	go func() {
		s.log.Info().Str("addr", s.httpServer.Addr).Msg("starting http server")
		err := s.httpServer.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		errCh <- err
	}()

	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("http server: %w", err)
		}
		// ListenAndServe returned ErrServerClosed without ctx being
		// cancelled: something shut the server down out of band. Not an
		// error, but not a signal-driven stop either.
		return nil
	case <-ctx.Done():
		s.log.Info().Msg("shutdown signal received")
		return nil
	}
}

// Shutdown drains in-flight requests within ctx's deadline. It is the closer
// registered on the lifecycle stack.
//
// It flips readiness to false first: an orchestrator that keeps routing new
// requests at a server that is draining will see them fail, so /readyz must
// stop advertising the process before the drain starts, not after it ends.
func (s *Server) Shutdown(ctx context.Context) error {
	s.draining.Store(true)
	return s.httpServer.Shutdown(ctx)
}

// Draining reports whether shutdown has begun. It satisfies the readiness
// endpoint's need to fail closed during a drain without importing anything
// about the server itself.
func (s *Server) Draining() bool { return s.draining.Load() }
