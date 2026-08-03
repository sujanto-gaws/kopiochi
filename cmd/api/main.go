package main

import (
	"fmt"
	"os"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/sujanto-gaws/kopiochi/internal/config"
	"github.com/sujanto-gaws/kopiochi/internal/db"
	"github.com/sujanto-gaws/kopiochi/internal/httpx"
	"github.com/sujanto-gaws/kopiochi/internal/infrastructure/http/server"
	"github.com/sujanto-gaws/kopiochi/internal/logger"
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
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			// Initialize logger
			log.Logger = logger.Init(cfg.Log.Level, cfg.Log.Format)
			log.Info().Msg("application starting")

			// Initialize database
			dsn := db.BuildDSN(cfg.DB.Host, cfg.DB.Port, cfg.DB.User, cfg.DB.Password.Reveal(), cfg.DB.Name, cfg.DB.SSLMode)
			bunDB, pool, err := db.NewDB(db.Config{
				DSN:      dsn,
				MaxConns: cfg.DB.MaxConns,
				MinConns: cfg.DB.MinConns,
			})
			if err != nil {
				return fmt.Errorf("init database: %w", err)
			}
			defer pool.Close()
			log.Info().Msg("database connected & bun ORM initialized")

			// Dependency Injection — all module wiring lives in container.go
			app, err := BuildApp(cfg, bunDB, log.Logger)
			if err != nil {
				return fmt.Errorf("build app: %w", err)
			}

			// Build the router: core middleware plus whichever cross-cutting
			// middlewares security config enables. There is no plugin
			// registry any more — CORS and rate limiting are constructed
			// directly from typed config (see internal/httpx/router.go).
			// closeRouter releases what the router owns (the rate limiter's
			// eviction goroutine) and runs as part of graceful shutdown.
			r, closeRouter, err := httpx.NewRouter(cfg.Server, cfg.Security)
			if err != nil {
				return fmt.Errorf("build router: %w", err)
			}

			// Mount operational endpoints (/healthz, /readyz, /swagger) and every
			// module's routes under /api/v1. The identity module owns its own
			// fail-closed auth middleware (see modules/identity/module.go) — main
			// no longer derives protected-route middleware from the jwt-auth
			// plugin, and there is no second router in scope for a module to
			// mount onto by mistake. /readyz pings pool directly (it satisfies
			// httpx.Pinger) so orchestrators stop routing traffic the moment the
			// database becomes unreachable.
			httpx.Mount(r, app.Modules, httpx.Deps{Pinger: pool})

			// Start server with graceful shutdown
			server.Run(
				cfg.Server,
				r,
				server.WithShutdownFunc(closeRouter),
				server.WithShutdownFunc(server.NewPoolShutdownFunc(pool)),
			)
			return nil
		},
	}

	serveCmd.Flags().StringP("config", "c", "config/default.yaml", "Path to config file")
	rootCmd.AddCommand(serveCmd)

	if err := rootCmd.Execute(); err != nil {
		log.Fatal().Err(err).Msg("cli execution failed")
		os.Exit(1)
	}
}
