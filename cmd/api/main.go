package main

import (
	"fmt"
	"os"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	appUser "github.com/sujanto-gaws/kopiochi/internal/application/user"
	"github.com/sujanto-gaws/kopiochi/internal/config"
	"github.com/sujanto-gaws/kopiochi/internal/db"
	"github.com/sujanto-gaws/kopiochi/internal/infrastructure/http/handlers"
	"github.com/sujanto-gaws/kopiochi/internal/infrastructure/http/routes"
	"github.com/sujanto-gaws/kopiochi/internal/infrastructure/persistence/repository"
	"github.com/sujanto-gaws/kopiochi/internal/logger"
	"github.com/sujanto-gaws/kopiochi/internal/server"
)

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

			// Dependency Injection (DDD)
			// Infrastructure: Repository
			userRepo := repository.NewUserRepository(bunDB)
			// Application: Service
			userSvc := appUser.NewService(userRepo)
			// Infrastructure: HTTP Handler
			userHandler := handlers.NewUserHandler(userSvc)

			// Setup router
			r := server.NewRouter()
			routes.Setup(r, userHandler)

			// Start server with graceful shutdown
			server.Run(
				cfg.Server.Host,
				cfg.Server.Port,
				r,
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
