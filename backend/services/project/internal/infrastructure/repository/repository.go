// Package repository implements the Project domain repository port on Postgres
// (Ports & Adapters: the adapter side of the hexagon). The pool-backed adapter
// satisfies the domain port; reads filter by the session's workspace set and
// mutations reject rows outside it (404), so a tenant can never observe or
// touch another tenant's projects even if the Gateway were misconfigured.
package repository

import (
	"context"
	"embed"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aaks/server/internal/platform/db"
	"github.com/aaks/server/services/project/internal/domain"
	"github.com/aaks/server/services/project/internal/infrastructure/repository/project"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Repos owns the project Postgres pool and exposes the pool-backed adapter for
// the domain port. It is the composition-root entrypoint of this package.
type Repos struct {
	pool *pgxpool.Pool
	log  *slog.Logger

	Projects domain.ProjectRepository
}

// New opens the project database and runs migrations.
func New(ctx context.Context, dsn string, log *slog.Logger) (*Repos, error) {
	pool, err := db.Pool(ctx, dsn, log)
	if err != nil {
		return nil, err
	}
	if err := db.Migrate(ctx, pool, migrations, "migrations", log); err != nil {
		return nil, err
	}
	r := &Repos{pool: pool, log: log}
	r.Projects = project.New(pool)
	return r, nil
}

// Close releases the connection pool.
func (r *Repos) Close() { r.pool.Close() }
