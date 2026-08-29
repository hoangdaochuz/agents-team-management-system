// Package repository implements the Workspace domain repository ports on
// Postgres (Ports & Adapters: the adapter side of the hexagon). It owns a
// single pool over workspace_db — the consolidated project/task/catalog/
// resources schema — and exposes pool-backed adapters for every aggregate;
// tx-scoped instances are constructed by the UnitOfWorks for multi-aggregate
// mutations.
package repository

import (
	"context"
	"embed"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aaks/server/internal/platform/db"
	catalogdomain "github.com/aaks/server/services/workspace/internal/domain/catalog"
	projectdomain "github.com/aaks/server/services/workspace/internal/domain/project"
	resourcesdomain "github.com/aaks/server/services/workspace/internal/domain/resources"
	taskdomain "github.com/aaks/server/services/workspace/internal/domain/task"
	"github.com/aaks/server/services/workspace/internal/infrastructure/repository/catalog/mcp"
	"github.com/aaks/server/services/workspace/internal/infrastructure/repository/catalog/skill"
	"github.com/aaks/server/services/workspace/internal/infrastructure/repository/project/project"
	"github.com/aaks/server/services/workspace/internal/infrastructure/repository/resources/knowledge"
	resmcp "github.com/aaks/server/services/workspace/internal/infrastructure/repository/resources/mcp"
	"github.com/aaks/server/services/workspace/internal/infrastructure/repository/resources/plugin"
	"github.com/aaks/server/services/workspace/internal/infrastructure/repository/resources/rule"
	"github.com/aaks/server/services/workspace/internal/infrastructure/repository/task/feedback"
	tasktask "github.com/aaks/server/services/workspace/internal/infrastructure/repository/task/task"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Repos owns the workspace Postgres pool and exposes pool-backed adapters for
// the project, task, catalog, and resources aggregate ports. It is the
// composition-root entrypoint of this package.
type Repos struct {
	pool *pgxpool.Pool
	log  *slog.Logger

	// Project plane.
	Projects projectdomain.ProjectRepository

	// Task plane.
	Tasks    taskdomain.TaskRepository
	Feedback taskdomain.FeedbackRepository

	// Catalog plane.
	Skills catalogdomain.SkillRepository
	Mcps   catalogdomain.McpRepository

	// Resources plane.
	Knowledge resourcesdomain.KnowledgeRepository
	Plugins   resourcesdomain.PluginRepository
	Rules     resourcesdomain.RuleRepository
	Mcp       resourcesdomain.McpConnectionRepository
}

// New opens the workspace database (project + task + catalog + resources
// schemas, one pool) and runs the merged migrations.
func New(ctx context.Context, dsn string, log *slog.Logger) (*Repos, error) {
	pool, err := db.Pool(ctx, dsn, log)
	if err != nil {
		return nil, err
	}
	if err := db.Migrate(ctx, pool, migrations, "migrations", log); err != nil {
		return nil, err
	}
	st := &Repos{pool: pool, log: log}
	st.Projects = project.New(pool)
	st.Tasks = tasktask.New(pool)
	st.Feedback = feedback.New(pool)
	st.Skills = skill.New(pool)
	st.Mcps = mcp.New(pool)
	st.Knowledge = knowledge.New(pool)
	st.Plugins = plugin.New(pool)
	st.Rules = rule.New(pool)
	st.Mcp = resmcp.New(pool)
	return st, nil
}

// Close releases the connection pool.
func (st *Repos) Close() { st.pool.Close() }
