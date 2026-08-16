// Package repository implements the Catalog domain repository ports on Postgres
// (Ports & Adapters: the adapter side of the hexagon). Each aggregate has its
// own adapter subpackage; pool-backed instances serve single-aggregate use
// cases and tx-scoped instances are constructed by the UnitOfWork for the
// definition mutations that publish events after commit.
package repository

import (
	"context"
	"embed"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aaks/server/internal/platform/db"
	"github.com/aaks/server/services/catalog/internal/domain"
	"github.com/aaks/server/services/catalog/internal/infrastructure/repository/mcp"
	"github.com/aaks/server/services/catalog/internal/infrastructure/repository/skill"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Repos owns the catalog Postgres pool and exposes pool-backed adapters for
// the domain ports. It is the composition-root entrypoint of this package.
type Repos struct {
	pool   *pgxpool.Pool
	log    *slog.Logger
	Skills domain.SkillRepository
	Mcps   domain.McpRepository
}

// New opens the catalog database and runs migrations.
func New(ctx context.Context, dsn string, log *slog.Logger) (*Repos, error) {
	pool, err := db.Pool(ctx, dsn, log)
	if err != nil {
		return nil, err
	}
	if err := db.Migrate(ctx, pool, migrations, "migrations", log); err != nil {
		return nil, err
	}
	r := &Repos{pool: pool, log: log}
	r.Skills = skill.New(pool)
	r.Mcps = mcp.New(pool)
	return r, nil
}

// Close releases the connection pool.
func (r *Repos) Close() { r.pool.Close() }
