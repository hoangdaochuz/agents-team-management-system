// Package repository implements the Agent domain repository ports on Postgres
// (Ports & Adapters: the adapter side of the hexagon). It owns a single pool
// over agent_db — the consolidated agent/settings schema — covering the agent
// aggregate (including the agent-builder fields), the skill/mcp link tables,
// the local catalog projections used to validate attachments within a
// workspace, and the encrypted provider-key store.
package repository

import (
	"context"
	"embed"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aaks/server/internal/platform/db"
	agentdomain "github.com/aaks/server/services/agent/internal/domain/agent"
	settingsdomain "github.com/aaks/server/services/agent/internal/domain/settings"
	"github.com/aaks/server/services/agent/internal/infrastructure/repository/agent"
	"github.com/aaks/server/services/agent/internal/infrastructure/repository/agent/projection"
	"github.com/aaks/server/services/agent/internal/infrastructure/repository/settings/key"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Repos owns the agent Postgres pool and exposes pool-backed adapters for the
// agent and settings aggregate ports. It is the composition-root entrypoint of
// this package.
type Repos struct {
	pool *pgxpool.Pool
	log  *slog.Logger

	// Agent plane.
	Agents      agentdomain.AgentRepository
	Projections agentdomain.CatalogProjectionRepository

	// Settings plane (encrypted provider keys).
	Keys settingsdomain.ProviderKeyRepository
}

// New opens the agent database (agent + settings schemas, one pool) and runs
// the merged migrations.
func New(ctx context.Context, dsn string, log *slog.Logger) (*Repos, error) {
	pool, err := db.Pool(ctx, dsn, log)
	if err != nil {
		return nil, err
	}
	if err := db.Migrate(ctx, pool, migrations, "migrations", log); err != nil {
		return nil, err
	}
	st := &Repos{pool: pool, log: log}
	st.Agents = agent.New(pool)
	st.Projections = projection.New(pool)
	st.Keys = key.New(pool)
	return st, nil
}

// Close releases the connection pool.
func (st *Repos) Close() { st.pool.Close() }
