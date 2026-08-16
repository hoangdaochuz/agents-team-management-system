// Package repository implements the Runner domain repository ports on Postgres.
// The parent package owns the pool and migrations and composes per-aggregate
// adapter subpackages (Ports & Adapters: the adapter side of the hexagon).
package repository

import (
	"context"
	"embed"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aaks/server/internal/platform/db"
	"github.com/aaks/server/services/runner/internal/domain"
	"github.com/aaks/server/services/runner/internal/infrastructure/repository/artifact"
	"github.com/aaks/server/services/runner/internal/infrastructure/repository/finding"
	"github.com/aaks/server/services/runner/internal/infrastructure/repository/run"
	"github.com/aaks/server/services/runner/internal/infrastructure/repository/step"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Repos owns the runner Postgres pool and exposes pool-backed adapters.
type Repos struct {
	pool *pgxpool.Pool
	log  *slog.Logger

	Runs      domain.RunRepository
	Steps     domain.StepRepository
	Findings  domain.FindingRepository
	Artifacts domain.ArtifactRepository
}

// New opens the runner database and runs migrations.
func New(ctx context.Context, dsn string, log *slog.Logger) (*Repos, error) {
	pool, err := db.Pool(ctx, dsn, log)
	if err != nil {
		return nil, err
	}
	if err := db.Migrate(ctx, pool, migrations, "migrations", log); err != nil {
		return nil, err
	}
	s := &Repos{pool: pool, log: log}
	s.Runs = run.New(pool)
	s.Steps = step.New(pool)
	s.Findings = finding.New(pool)
	s.Artifacts = artifact.New(pool)
	return s, nil
}

// Close releases the connection pool.
func (s *Repos) Close() { s.pool.Close() }
