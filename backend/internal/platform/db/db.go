// Package db provides shared Postgres helpers: connection-pool creation and a
// SQL-file migrator with schema_migrations tracking. Each service owns its own
// logical database and embeds its migrations under internal/store/migrations.
package db

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

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

// Migrate applies every *.sql file rooted in root (sorted by name) to the
// pool, tracking applied files in a schema_migrations table so each file runs
// exactly once — a service restart (or a second container replica) never
// re-executes a migration. Services pass their embedded migrations FS;
// os.DirFS covers on-disk directories. Each file is applied whole (single
// Exec) — sufficient for this project's simple per-service schemas — and
// recorded in the same transaction so a failed file is never marked applied.
func Migrate(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS, root string, log *slog.Logger) error {
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		name       text PRIMARY KEY,
		applied_at timestamptz NOT NULL DEFAULT now()
	)`); err != nil {
		return fmt.Errorf("db: create schema_migrations: %w", err)
	}
	entries, err := fs.ReadDir(fsys, root)
	if err != nil {
		return fmt.Errorf("db: read migrations %q: %w", root, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		var applied bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE name = $1)`, name).Scan(&applied); err != nil {
			return fmt.Errorf("db: check %s: %w", name, err)
		}
		if applied {
			continue
		}
		buf, err := fs.ReadFile(fsys, root+"/"+name)
		if err != nil {
			return fmt.Errorf("db: read %s: %w", name, err)
		}
		applyCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		tx, err := pool.Begin(applyCtx)
		if err != nil {
			cancel()
			return fmt.Errorf("db: begin %s: %w", name, err)
		}
		if _, err := tx.Exec(applyCtx, string(buf)); err != nil {
			_ = tx.Rollback(applyCtx)
			cancel()
			return fmt.Errorf("db: apply %s: %w", name, err)
		}
		if _, err := tx.Exec(applyCtx, `INSERT INTO schema_migrations(name) VALUES($1)`, name); err != nil {
			_ = tx.Rollback(applyCtx)
			cancel()
			return fmt.Errorf("db: record %s: %w", name, err)
		}
		if err := tx.Commit(applyCtx); err != nil {
			cancel()
			return fmt.Errorf("db: commit %s: %w", name, err)
		}
		cancel()
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
