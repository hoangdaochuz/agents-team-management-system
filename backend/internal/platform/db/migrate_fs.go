package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// MigrateFS applies every *.sql file rooted in the embed.FS (sorted by name) to
// the pool. Embedding migrations keeps each service binary self-contained — no
// external migration files needed at runtime.
func MigrateFS(ctx context.Context, pool *pgxpool.Pool, efs embed.FS, root string, log *slog.Logger) error {
	entries, err := fs.ReadDir(efs, root)
	if err != nil {
		return fmt.Errorf("db: read embedded migrations %q: %w", root, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		buf, err := efs.ReadFile(root + "/" + name)
		if err != nil {
			return fmt.Errorf("db: read embedded %s: %w", name, err)
		}
		applyCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		if _, err := pool.Exec(applyCtx, string(buf)); err != nil {
			cancel()
			return fmt.Errorf("db: apply %s: %w", name, err)
		}
		cancel()
		log.Info("migration applied", "file", name)
	}
	return nil
}
