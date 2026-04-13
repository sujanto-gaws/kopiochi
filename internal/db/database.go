package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

type Config struct {
	DSN      string
	MaxConns int32
	MinConns int32
}

// NewDB initializes pgxpool and bun ORM
func NewDB(cfg Config) (*bun.DB, *pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, nil, fmt.Errorf("parse dsn: %w", err)
	}

	poolCfg.MaxConns = cfg.MaxConns
	poolCfg.MinConns = cfg.MinConns
	poolCfg.MaxConnLifetime = time.Hour
	poolCfg.MaxConnIdleTime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("create pgxpool: %w", err)
	}

	if err := pool.Ping(context.Background()); err != nil {
		return nil, nil, fmt.Errorf("ping database: %w", err)
	}

	sqldb := stdlib.OpenDBFromPool(pool)

	// bun supports pgxpool directly
	db := bun.NewDB(sqldb, pgdialect.New())
	return db, pool, nil
}

// OpenDB opens a standard *sql.DB connection for use with migration tools
func OpenDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return db, nil
}

// BuildDSN creates a PostgreSQL connection string
func BuildDSN(host string, port int, user, pass, name, ssl string) string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s", user, pass, host, port, name, ssl)
}
