package server

import (
	"context"
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

	zlog "github.com/sujanto-gaws/kopiochi/internal/middleware"
)

// ShutdownFunc is a function that performs cleanup during shutdown
type ShutdownFunc func() error

// Server represents the HTTP server with graceful shutdown support
type Server struct {
	httpServer      *http.Server
	shutdownFuncs   []ShutdownFunc
	shutdownTimeout time.Duration
}

// ServerOption is a function that configures the Server
type ServerOption func(*Server)

// WithShutdownTimeout sets the shutdown timeout duration
func WithShutdownTimeout(timeout time.Duration) ServerOption {
	return func(s *Server) {
		s.shutdownTimeout = timeout
	}
}

// WithShutdownFunc adds a cleanup function to be called during shutdown
func WithShutdownFunc(fn ShutdownFunc) ServerOption {
	return func(s *Server) {
		s.shutdownFuncs = append(s.shutdownFuncs, fn)
	}
}

// NewRouter creates a new chi router with middleware
func NewRouter() *chi.Mux {
	r := chi.NewRouter()

	// Chi core middleware
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Timeout(60 * time.Second))
	r.Use(zlog.ZerologRequestLogger)

	return r
}

// NewServer creates a new server instance with options
func NewServer(host string, port int, router *chi.Mux, opts ...ServerOption) *Server {
	addr := fmt.Sprintf("%s:%d", host, port)
	srv := &Server{
		httpServer: &http.Server{
			Addr:         addr,
			Handler:      router,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  120 * time.Second,
		},
		shutdownTimeout: 30 * time.Second,
		shutdownFuncs:   make([]ShutdownFunc, 0),
	}

	for _, opt := range opts {
		opt(srv)
	}

	return srv
}

// Run starts the server and handles graceful shutdown
func Run(host string, port int, router *chi.Mux, opts ...ServerOption) {
	srv := NewServer(host, port, router, opts...)
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
	<-quit
	log.Info().Msg("shutting down server gracefully...")

	// Create shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
	defer cancel()

	// Shutdown HTTP server
	if err := s.httpServer.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("http server shutdown error")
	} else {
		log.Info().Msg("http server stopped")
	}

	// Execute cleanup functions (database pool, etc.)
	for i, fn := range s.shutdownFuncs {
		if err := fn(); err != nil {
			log.Error().Err(err).Int("index", i).Msg("shutdown function error")
		}
	}

	log.Info().Msg("server exited properly")
}

// Shutdown manually triggers server shutdown
func (s *Server) Shutdown(ctx context.Context) error {
	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("http server shutdown: %w", err)
	}

	for _, fn := range s.shutdownFuncs {
		if err := fn(); err != nil {
			return fmt.Errorf("cleanup function: %w", err)
		}
	}

	return nil
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
