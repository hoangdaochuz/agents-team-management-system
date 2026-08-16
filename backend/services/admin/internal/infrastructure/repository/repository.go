// Package repository implements the Admin domain repository ports on Postgres
// (Ports & Adapters: the adapter side of the hexagon). Each aggregate has its
// own adapter; admin use cases are single-aggregate writes, so pool-backed
// adapters are sufficient.
package repository

import (
	"context"
	"embed"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aaks/server/internal/platform/db"
	"github.com/aaks/server/services/admin/internal/domain"
	"github.com/aaks/server/services/admin/internal/infrastructure/repository/audit"
	"github.com/aaks/server/services/admin/internal/infrastructure/repository/flag"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Repos owns the admin Postgres pool and exposes pool-backed adapters for the
// domain ports. It is the composition-root entrypoint of this package.
type Repos struct {
	pool *pgxpool.Pool
	log  *slog.Logger

	Audit domain.AuditRepository
	Flags domain.FlagRepository
}

// New opens the admin database and runs migrations.
func New(ctx context.Context, dsn string, log *slog.Logger) (*Repos, error) {
	pool, err := db.Pool(ctx, dsn, log)
	if err != nil {
		return nil, err
	}
	if err := db.Migrate(ctx, pool, migrations, "migrations", log); err != nil {
		return nil, err
	}
	s := &Repos{pool: pool, log: log}
	s.Audit = audit.New(pool)
	s.Flags = flag.New(pool)
	return s, nil
}

// Close releases the connection pool.
func (s *Repos) Close() { s.pool.Close() }
