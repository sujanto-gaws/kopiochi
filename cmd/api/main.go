package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/sujanto-gaws/kopiochi/internal/config"
	"github.com/sujanto-gaws/kopiochi/internal/db"
	"github.com/sujanto-gaws/kopiochi/internal/httpx"
	"github.com/sujanto-gaws/kopiochi/internal/lifecycle"
	"github.com/sujanto-gaws/kopiochi/internal/logger"
	"github.com/sujanto-gaws/kopiochi/internal/metrics"
)

// @title Kopiochi API
// @version 1.0
// @description A Go Web API boilerplate with chi, bun, pgx, cobra, viper & zerolog
// @description This API provides user management and authentication endpoints

// @host localhost:8080
// @BasePath /api/v1
// @schemes http https

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Type "Bearer" followed by a space and JWT token

// forcedExitCode is what a second interrupt exits with: 128 + SIGINT(2), the
// shell convention for "terminated by signal 2".
const forcedExitCode = 130

func main() {
	rootCmd := &cobra.Command{
		Use:   "kopiochi",
		Short: "Go Web API with chi, bun, pgx, cobra, viper & zerolog",
	}

	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the API server",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfgPath, _ := cmd.Flags().GetString("config")
			return serve(cfgPath)
		},
	}

	serveCmd.Flags().StringP("config", "c", "config/default.yaml", "Path to config file")
	rootCmd.AddCommand(serveCmd)

	if err := rootCmd.Execute(); err != nil {
		log.Error().Err(err).Msg("cli execution failed")
		os.Exit(1)
	}
}

// serve is the whole application lifecycle, start to finish, as one linear
// function that returns errors instead of exiting.
//
// Every resource is registered on the lifecycle stack exactly once, by the
// code that created it, and released in reverse order at the end. There are
// deliberately no `defer x.Close()` calls for anything on that stack: the
// arrangement this replaces closed each resource twice — once from a defer
// here and once from a shutdown func on the server — and only got away with
// it because pgxpool.Close happens to be idempotent.
//
// See docs/architectures/02-composition/lifecycle-and-shutdown.md.
func serve(cfgPath string) error {
	// Phase 1 — config. Validate before anything is allocated, so a bad
	// config costs nothing to reject.
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Phase 2 — logger. appLog is the one every constructor below is handed;
	// nothing in this tree reads zerolog's package global. The global is still
	// assigned so that a dependency which does reach for it writes in our
	// format and at our level rather than to a default stderr writer.
	appLog := logger.Init(cfg.Log.Level, cfg.Log.Format)
	log.Logger = appLog
	appLog.Info().Msg("application starting")

	// The base context is cancelled by the first SIGINT/SIGTERM, so
	// "we are shutting down" is observable process-wide rather than known
	// only to the goroutine that received the signal.
	ctx, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()
	watchForSecondSignal(ctx, appLog)

	stack := lifecycle.New(appLog)

	// Phase 3 — database. It comes before module construction because
	// modules may need it while being built, and its startup budget is
	// bounded so an unreachable database fails boot fast.
	dbCtx, cancelDB := context.WithTimeout(ctx, cfg.DB.StartupTimeout)
	defer cancelDB()

	database, err := db.Open(dbCtx, cfg.DB)
	if err != nil {
		return fmt.Errorf("init database: %w", err)
	}
	stack.PushCloser("database", database.Close)
	appLog.Info().Msg("database connected & bun ORM initialized")

	// Phase 4 — modules. BuildApp refuses to return an application with zero
	// modules, so a server that would start and serve nothing fails here.
	app, err := BuildApp(cfg, database.Bun, appLog)
	if err != nil {
		return shutdownAfter(stack, cfg, appLog, fmt.Errorf("build app: %w", err))
	}
	for _, m := range app.Modules {
		if m.Close != nil {
			stack.PushCloser("module "+m.Name, m.Close)
		}
	}

	// Phase 5 — metrics. Built before the router so the HTTP middleware can be
	// registered, and after the database so the pool collector has a pool to
	// read. nil when disabled, which the router understands as "no
	// instrumentation" rather than requiring a second code path.
	var m *metrics.Metrics
	if cfg.Metrics.Enabled {
		m = metrics.New()
		// Read at scrape time rather than on a ticker: no goroutine to own,
		// and no staleness in a saturation signal.
		if err := m.RegisterPool(func() metrics.PoolStats {
			if database.Pool == nil {
				return nil
			}
			return database.Pool.Stat()
		}); err != nil {
			return shutdownAfter(stack, cfg, appLog, fmt.Errorf("register pool metrics: %w", err))
		}
	}

	// Phase 6 — router and routes. CORS and rate limiting are constructed
	// directly from typed config; closeRouter releases what they own (the
	// rate limiter's eviction goroutine).
	r, closeRouter, err := httpx.NewRouter(cfg.Server, cfg.Security, appLog, m)
	if err != nil {
		return shutdownAfter(stack, cfg, appLog, fmt.Errorf("build router: %w", err))
	}
	stack.PushCloser("router", closeRouter)

	// Phase 7 — servers. Registered on the stack before they start listening,
	// so a failure between here and Serve still drains cleanly.
	srv := httpx.NewServer(cfg.Server, r, appLog)
	stack.Push("http server", srv.Shutdown)

	// The metrics listener is a second server on its own address. It goes on
	// the same stack so it drains like anything else, and it is pushed after
	// the API so LIFO stops it first — a scrape landing mid-drain would
	// otherwise report a process that is already going away.
	if m != nil {
		adminMux := http.NewServeMux()
		adminMux.Handle(cfg.Metrics.Path, m.Handler())

		admin := httpx.NewAdminServer(cfg.Metrics, adminMux, appLog.With().Str("component", "metrics").Logger())
		stack.Push("metrics server", admin.Shutdown)

		go func() {
			// A failed metrics listener must not take the API down with it:
			// losing observability is bad, losing the service is worse. It is
			// logged at error level so the gap is visible.
			if err := admin.Serve(ctx); err != nil {
				appLog.Error().Err(err).Str("addr", cfg.Metrics.Addr).Msg("metrics server stopped with error")
			}
		}()
	}

	// /readyz pings the pool directly (it satisfies httpx.Pinger) so
	// orchestrators stop routing traffic the moment the database becomes
	// unreachable, and reports not-ready as soon as the server starts
	// draining — before in-flight requests finish, not after.
	httpx.Mount(r, app.Modules, httpx.Deps{
		Pinger:   database.Pool,
		Draining: srv.Draining,
	})

	// Phase 8 — serve until the first signal, or until listening fails.
	serveErr := srv.Serve(ctx)
	if serveErr != nil {
		appLog.Error().Err(serveErr).Msg("server stopped with error")
	}

	return shutdownAfter(stack, cfg, appLog, serveErr)
}

// shutdownAfter tears the stack down and folds the teardown outcome into
// whatever error already occurred.
//
// The shutdown context is deliberately built from context.Background() and
// not from the (already-cancelled) signal context: the drain needs its full
// budget precisely in the case where the process is being asked to stop.
func shutdownAfter(stack *lifecycle.Stack, cfg *config.Config, log zerolog.Logger, cause error) error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	if err := stack.Shutdown(shutdownCtx); err != nil {
		if cause != nil {
			// Report the original failure; the teardown errors are already
			// logged per-resource by the stack.
			return cause
		}
		return fmt.Errorf("shutdown: %w", err)
	}

	log.Info().Msg("server exited properly")
	return cause
}

// watchForSecondSignal exits the process on a second interrupt.
//
// Without it, an operator who wants to abort a stuck drain has to reach for
// SIGKILL. The first signal cancels ctx and begins a graceful shutdown; the
// second says the operator is no longer willing to wait.
func watchForSecondSignal(ctx context.Context, log zerolog.Logger) {
	go func() {
		<-ctx.Done() // first signal

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		defer signal.Stop(sigCh)

		<-sigCh // second signal
		log.Warn().Msg("second signal received, forcing exit")
		os.Exit(forcedExitCode)
	}()
}
