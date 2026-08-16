// Package repository implements the Task domain repository ports on Postgres
// (Ports & Adapters: the adapter side of the hexagon). The parent package owns
// the pool and migrations and composes per-aggregate adapter subpackages;
// pool-backed instances serve the use cases and the saga coordinator.
package repository

import (
	"context"
	"embed"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aaks/server/internal/platform/db"
	"github.com/aaks/server/services/task/internal/domain"
	"github.com/aaks/server/services/task/internal/infrastructure/repository/feedback"
	"github.com/aaks/server/services/task/internal/infrastructure/repository/task"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Repos owns the task Postgres pool and exposes pool-backed adapters for the
// domain ports. It is the composition-root entrypoint of this package.
type Repos struct {
	pool *pgxpool.Pool
	log  *slog.Logger

	Tasks    domain.TaskRepository
	Feedback domain.FeedbackRepository
}

// New opens the task database and runs migrations.
func New(ctx context.Context, dsn string, log *slog.Logger) (*Repos, error) {
	pool, err := db.Pool(ctx, dsn, log)
	if err != nil {
		return nil, err
	}
	if err := db.Migrate(ctx, pool, migrations, "migrations", log); err != nil {
		return nil, err
	}
	s := &Repos{pool: pool, log: log}
	s.Tasks = task.New(pool)
	s.Feedback = feedback.New(pool)
	return s, nil
}

// Close releases the connection pool.
func (s *Repos) Close() { s.pool.Close() }