// Package db provides shared Postgres helpers: connection-pool creation and a
// trivial SQL-file migrator. Each service owns its own logical database and
// embeds its migrations under internal/store/migrations.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
)

// Config holds the connection parameters for one logical database.
type Config struct {
	DSN string // full Postgres DSN for this service's database
}

// Pool opens a pgx connection pool configured for a backend service.
func Pool(ctx context.Context, dsn string, log *slog.Logger) (*pgxpool.Pool, error) {
	if dsn == "" {
		return nil, fmt.Errorf("db: DSN is empty")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("db: parse DSN: %w", err)
	}
	cfg.MaxConns = 10
	cfg.MinConns = 1
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 30 * time.Minute
	cfg.HealthCheckPeriod = time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("db: connect: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: ping: %w", err)
	}
	log.Info("postgres connected", "dsn", redactDSN(dsn))
	return pool, nil
}

// OpenStdSQL returns a *database/sql handle backed by pgx, for tooling that
// prefers the database/sql interface.
func OpenStdSQL(dsn string) (*sql.DB, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	return stdlib.OpenDB(*cfg.ConnConfig), nil
}

// MigrateDir applies every *.sql file in dir (sorted by name) in order against
// the provided pool. Each file is split on ";" statements naively — sufficient
// for this project's simple per-service schemas. Statements run outside an
// explicit transaction; add transactional migration if a schema needs it.
func MigrateDir(ctx context.Context, pool *pgxpool.Pool, dir string, log *slog.Logger) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("db: read migrations dir %q: %w", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		path := filepath.Join(dir, name)
		buf, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("db: read %s: %w", name, err)
		}
		if _, err := pool.Exec(ctx, string(buf)); err != nil {
			return fmt.Errorf("db: apply %s: %w", name, err)
		}
		log.Info("migration applied", "file", name)
	}
	return nil
}

// redactDSN hides the password if present in a DSN URL.
func redactDSN(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil || u.User == nil {
		return dsn
	}
	if _, has := u.User.Password(); has {
		u.User = url.UserPassword(u.User.Username(), "***")
	}
	return u.String()
}
