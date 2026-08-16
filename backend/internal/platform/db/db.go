// Package db provides shared Postgres helpers: connection-pool creation and a
// trivial SQL-file migrator. Each service owns its own logical database and
// embeds its migrations under internal/store/migrations.
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
// pool. Services pass their embedded migrations FS; os.DirFS covers on-disk
// directories. Each file is applied whole (single Exec) — sufficient for this
// project's simple per-service schemas.
func Migrate(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS, root string, log *slog.Logger) error {
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
		buf, err := fs.ReadFile(fsys, root+"/"+name)
		if err != nil {
			return fmt.Errorf("db: read %s: %w", name, err)
		}
		applyCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		_, err = pool.Exec(applyCtx, string(buf))
		cancel()
		if err != nil {
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
