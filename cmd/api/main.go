package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/sujanto-gaws/kopiochi/cmd/api/container"
	"github.com/sujanto-gaws/kopiochi/internal/config"
	"github.com/sujanto-gaws/kopiochi/internal/db"
	"github.com/sujanto-gaws/kopiochi/internal/infrastructure/http/handlers"
	"github.com/sujanto-gaws/kopiochi/internal/infrastructure/http/routes"
	"github.com/sujanto-gaws/kopiochi/internal/infrastructure/http/server"
	"github.com/sujanto-gaws/kopiochi/internal/logger"
	"github.com/sujanto-gaws/kopiochi/internal/plugin"
	"github.com/sujanto-gaws/kopiochi/internal/plugins"
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

			// Initialize plugins
			// Step 1: Create registry and register built-in plugins
			pluginRegistry := plugin.NewRegistry()
			plugins.RegisterBuiltinPlugins(pluginRegistry)

			// Step 2: Initialize plugins from configuration
			if _, err := plugin.InitializeFromConfig(pluginRegistry, &cfg.Plugins); err != nil {
				return fmt.Errorf("initialize plugins: %w", err)
			}
			defer pluginRegistry.Close()
			log.Info().Strs("plugins", pluginRegistry.ListInitialized()).Msg("plugins initialized")

			// Initialize database
			dsn := db.BuildDSN(cfg.DB.Host, cfg.DB.Port, cfg.DB.User, cfg.DB.Password, cfg.DB.Name, cfg.DB.SSLMode)
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

			// Dependency Injection — all handler wiring lives in container.go
			c, err := container.New(cfg, bunDB)
			if err != nil {
				return fmt.Errorf("build container: %w", err)
			}

			// Setup router with plugin middleware chain
			r := server.NewRouter(cfg.Server)

			// Apply plugin middleware chain to router
			middlewareChain := plugin.NewMiddlewareChainFromRegistry(pluginRegistry, plugin.GetMiddlewareNames(&cfg.Plugins))
			if middlewareChain.Len() > 0 {
				r.Use(func(next http.Handler) http.Handler {
					return middlewareChain.Build(next)
				})
			}

			// Resolve auth middleware from the jwt-auth plugin if initialized.
			var authMiddleware func(http.Handler) http.Handler
			if authPlugin := pluginRegistry.GetAuth("jwt-auth"); authPlugin != nil {
				authMiddleware = authPlugin.AuthMiddleware()
			}

			// Build the router group: Protected applies auth middleware when available.
			v1 := r.With() // scoped sub-router for /api/v1 context
			var protected chi.Router
			if authMiddleware != nil {
				protected = v1.With(authMiddleware)
			} else {
				protected = v1
			}
			g := handlers.RouterGroup{Public: v1, Protected: protected}

			routes.Setup(r, g, c.Registrars()...)

			// Start server with graceful shutdown
			server.Run(
				cfg.Server,
				r,
				server.WithShutdownFunc(server.NewPoolShutdownFunc(pool)),
				server.WithPluginRegistry(pluginRegistry),
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
