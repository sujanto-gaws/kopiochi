package main

import (
	"fmt"
	"os"

	"github.com/pressly/goose/v3"
	"github.com/spf13/cobra"

	"github.com/sujanto-gaws/kopiochi/internal/config"
	"github.com/sujanto-gaws/kopiochi/internal/db"
	"github.com/sujanto-gaws/kopiochi/internal/logger"
)

var (
	migrationsDir string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "kopiochi-migrate",
		Short: "Database migration tool for Kopiochi",
	}

	// Up command - run all pending migrations
	upCmd := &cobra.Command{
		Use:   "up",
		Short: "Run all pending migrations",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfgPath, _ := cmd.Flags().GetString("config")
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			logger.Init(cfg.Log.Level, cfg.Log.Format)

			dsn := db.BuildDSN(cfg.DB.Host, cfg.DB.Port, cfg.DB.User, cfg.DB.Password, cfg.DB.Name, cfg.DB.SSLMode)
			database, err := db.OpenDB(dsn)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer database.Close()

			if err := goose.Up(database, migrationsDir); err != nil {
				return fmt.Errorf("run migrations up: %w", err)
			}

			fmt.Println("✓ Migrations completed successfully")
			return nil
		},
	}

	// Down command - rollback the last migration
	downCmd := &cobra.Command{
		Use:   "down",
		Short: "Rollback the most recent migration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfgPath, _ := cmd.Flags().GetString("config")
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			logger.Init(cfg.Log.Level, cfg.Log.Format)

			dsn := db.BuildDSN(cfg.DB.Host, cfg.DB.Port, cfg.DB.User, cfg.DB.Password, cfg.DB.Name, cfg.DB.SSLMode)
			database, err := db.OpenDB(dsn)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer database.Close()

			if err := goose.Down(database, migrationsDir); err != nil {
				return fmt.Errorf("run migrations down: %w", err)
			}

			fmt.Println("✓ Rollback completed successfully")
			return nil
		},
	}

	// Status command - show migration status
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show migration status",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfgPath, _ := cmd.Flags().GetString("config")
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			logger.Init(cfg.Log.Level, cfg.Log.Format)

			dsn := db.BuildDSN(cfg.DB.Host, cfg.DB.Port, cfg.DB.User, cfg.DB.Password, cfg.DB.Name, cfg.DB.SSLMode)
			database, err := db.OpenDB(dsn)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer database.Close()

			if err := goose.Status(database, migrationsDir); err != nil {
				return fmt.Errorf("get migration status: %w", err)
			}

			return nil
		},
	}

	// Reset command - rollback all migrations
	resetCmd := &cobra.Command{
		Use:   "reset",
		Short: "Rollback all migrations",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfgPath, _ := cmd.Flags().GetString("config")
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			logger.Init(cfg.Log.Level, cfg.Log.Format)

			dsn := db.BuildDSN(cfg.DB.Host, cfg.DB.Port, cfg.DB.User, cfg.DB.Password, cfg.DB.Name, cfg.DB.SSLMode)
			database, err := db.OpenDB(dsn)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer database.Close()

			if err := goose.Reset(database, migrationsDir); err != nil {
				return fmt.Errorf("reset migrations: %w", err)
			}

			fmt.Println("✓ Reset completed successfully")
			return nil
		},
	}

	// Create command - create a new migration file
	createCmd := &cobra.Command{
		Use:   "create [name]",
		Short: "Create a new migration file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			migrationType, _ := cmd.Flags().GetString("type")

			if err := goose.Create(nil, migrationsDir, name, migrationType); err != nil {
				return fmt.Errorf("create migration: %w", err)
			}

			fmt.Printf("✓ Created migration file in %s/\n", migrationsDir)
			return nil
		},
	}

	// Add flags to all commands
	commands := []*cobra.Command{upCmd, downCmd, statusCmd, resetCmd, createCmd}
	for _, cmd := range commands {
		cmd.Flags().StringP("config", "c", "config/default.yaml", "Path to config file")
		cmd.Flags().StringVarP(&migrationsDir, "dir", "d", "migrations", "Path to migrations directory")
	}

	createCmd.Flags().StringP("type", "t", "sql", "Migration type (sql or go)")

	rootCmd.AddCommand(upCmd, downCmd, statusCmd, resetCmd, createCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
