// Package repository implements the Resources domain repository ports on Postgres
// (Ports & Adapters: the adapter side of the hexagon). Each aggregate has its
// own adapter subpackage; pool-backed instances serve single-aggregate use
// cases and tx-scoped instances are constructed by the UnitOfWork for the
// workspace bootstrap seed.
package repository

import (
	"context"
	"embed"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aaks/server/internal/platform/db"
	"github.com/aaks/server/services/resources/internal/domain"
	"github.com/aaks/server/services/resources/internal/infrastructure/repository/knowledge"
	"github.com/aaks/server/services/resources/internal/infrastructure/repository/mcp"
	"github.com/aaks/server/services/resources/internal/infrastructure/repository/plugin"
	"github.com/aaks/server/services/resources/internal/infrastructure/repository/rule"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Repos owns the resources Postgres pool and exposes pool-backed adapters for
// the domain ports. It is the composition-root entrypoint of this package.
type Repos struct {
	pool *pgxpool.Pool
	log  *slog.Logger

	Knowledge domain.KnowledgeRepository
	Plugins   domain.PluginRepository
	Rules     domain.RuleRepository
	Mcp       domain.McpConnectionRepository
}

// New opens the resources database and runs migrations.
func New(ctx context.Context, dsn string, log *slog.Logger) (*Repos, error) {
	pool, err := db.Pool(ctx, dsn, log)
	if err != nil {
		return nil, err
	}
	if err := db.Migrate(ctx, pool, migrations, "migrations", log); err != nil {
		return nil, err
	}
	s := &Repos{pool: pool, log: log}
	s.Knowledge = knowledge.New(pool)
	s.Plugins = plugin.New(pool)
	s.Rules = rule.New(pool)
	s.Mcp = mcp.New(pool)
	return s, nil
}

// Close releases the connection pool.
func (s *Repos) Close() { s.pool.Close() }