// Package repository implements the Agent domain repository ports on Postgres
// (Ports & Adapters: the adapter side of the hexagon). Each aggregate has its
// own adapter subpackage: agents (including the agent-builder fields), the
// skill/mcp link tables, and local projections of the catalog's skill/MCP
// definitions used to validate attachments within a workspace (no
// service-to-service sync calls).
package repository

import (
	"context"
	"embed"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aaks/server/internal/platform/db"
	"github.com/aaks/server/services/agent/internal/domain"
	"github.com/aaks/server/services/agent/internal/infrastructure/repository/agent"
	"github.com/aaks/server/services/agent/internal/infrastructure/repository/projection"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Repos owns the agent Postgres pool and exposes pool-backed adapters for the
// domain ports. It is the composition-root entrypoint of this package.
type Repos struct {
	pool        *pgxpool.Pool
	log         *slog.Logger
	Agents      domain.AgentRepository
	Projections domain.CatalogProjectionRepository
}

// New opens the agent database and runs migrations.
func New(ctx context.Context, dsn string, log *slog.Logger) (*Repos, error) {
	pool, err := db.Pool(ctx, dsn, log)
	if err != nil {
		return nil, err
	}
	if err := db.Migrate(ctx, pool, migrations, "migrations", log); err != nil {
		return nil, err
	}
	s := &Repos{pool: pool, log: log}
	s.Agents = agent.New(pool)
	s.Projections = projection.New(pool)
	return s, nil
}

// Close releases the connection pool.
func (s *Repos) Close() { s.pool.Close() }