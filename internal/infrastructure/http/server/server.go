package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

	"github.com/sujanto-gaws/kopiochi/internal/config"
	zlog "github.com/sujanto-gaws/kopiochi/internal/middleware"
	"github.com/sujanto-gaws/kopiochi/internal/plugin"
)

// ShutdownFunc is a function that performs cleanup during shutdown
type ShutdownFunc func() error

// Server represents the HTTP server with graceful shutdown support
type Server struct {
	httpServer      *http.Server
	shutdownFuncs   []ShutdownFunc
	shutdownTimeout time.Duration
	pluginRegistry  *plugin.Registry
}

// ServerOption is a function that configures the Server
type ServerOption func(*Server)

// WithShutdownFunc adds a cleanup function to be called during shutdown
func WithShutdownFunc(fn ShutdownFunc) ServerOption {
	return func(s *Server) {
		s.shutdownFuncs = append(s.shutdownFuncs, fn)
	}
}

// WithPluginRegistry sets the plugin registry for the server
func WithPluginRegistry(registry *plugin.Registry) ServerOption {
	return func(s *Server) {
		s.pluginRegistry = registry
	}
}

// NewRouter creates a new chi router with core middleware applied.
// Timeouts are sourced from cfg; additional middlewares are applied after the core stack.
func NewRouter(cfg config.Server, mw ...func(http.Handler) http.Handler) *chi.Mux {
	r := chi.NewRouter()

	// Core middleware stack (order matters)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Timeout(cfg.RequestTimeout))
	r.Use(zlog.ZerologRequestLogger)

	// Caller-supplied middleware applied after the core stack
	if len(mw) > 0 {
		r.Use(mw...)
	}

	return r
}

// NewServer creates a new server instance from the provided config and options
func NewServer(cfg config.Server, router *chi.Mux, opts ...ServerOption) *Server {
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	srv := &Server{
		httpServer: &http.Server{
			Addr:              addr,
			Handler:           router,
			ReadTimeout:       cfg.ReadTimeout,
			ReadHeaderTimeout: cfg.ReadHeaderTimeout,
			WriteTimeout:      cfg.WriteTimeout,
			IdleTimeout:       cfg.IdleTimeout,
		},
		shutdownTimeout: cfg.ShutdownTimeout,
		shutdownFuncs:   make([]ShutdownFunc, 0),
	}

	for _, opt := range opts {
		opt(srv)
	}

	return srv
}

// Run starts the server and handles graceful shutdown
func Run(cfg config.Server, router *chi.Mux, opts ...ServerOption) {
	srv := NewServer(cfg, router, opts...)
	srv.Run()
}

// Run starts the server and handles graceful shutdown
func (s *Server) Run() {
	addr := s.httpServer.Addr
	log.Info().Str("addr", addr).Msg("starting http server")

	// Channel to listen for interrupt signals
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Start server in a goroutine
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server failed to start")
		}
	}()

	// Wait for interrupt signal
	sig := <-quit
	signal.Stop(quit)
	log.Info().Str("signal", sig.String()).Msg("shutting down server gracefully...")

	// Create shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
	defer cancel()

	if err := s.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("shutdown completed with errors")
	}

	log.Info().Msg("server exited properly")
}

// Shutdown manually triggers server shutdown, collecting all errors.
func (s *Server) Shutdown(ctx context.Context) error {
	var errs []error

	if err := s.httpServer.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("http server shutdown: %w", err))
	}

	for i, fn := range s.shutdownFuncs {
		if err := fn(); err != nil {
			errs = append(errs, fmt.Errorf("shutdown func[%d]: %w", i, err))
		}
	}

	if s.pluginRegistry != nil {
		if err := s.pluginRegistry.Close(); err != nil {
			errs = append(errs, fmt.Errorf("plugin registry shutdown: %w", err))
		}
	}

	return errors.Join(errs...)
}

// NewShutdownFunc creates a shutdown function for pgxpool
func NewPoolShutdownFunc(pool *pgxpool.Pool) ShutdownFunc {
	return func() error {
		if pool != nil {
			log.Info().Msg("closing database connection pool")
			pool.Close()
			log.Info().Msg("database connection pool closed")
		}
		return nil
	}
}
