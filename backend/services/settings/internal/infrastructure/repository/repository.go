// Package repository implements the Settings domain repository port on Postgres
// (Ports & Adapters: the adapter side of the hexagon). Provider keys are
// stored as opaque ciphertext; the crypto port lives in infrastructure/crypto.
package repository

import (
	"context"
	"embed"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aaks/server/internal/platform/db"
	"github.com/aaks/server/services/settings/internal/domain"
	"github.com/aaks/server/services/settings/internal/infrastructure/repository/key"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Repos owns the settings Postgres pool and exposes the pool-backed adapter
// for the domain port. It is the composition-root entrypoint of this package.
type Repos struct {
	pool *pgxpool.Pool
	log  *slog.Logger
	Keys domain.ProviderKeyRepository
}

// New opens the settings database and runs migrations.
func New(ctx context.Context, dsn string, log *slog.Logger) (*Repos, error) {
	pool, err := db.Pool(ctx, dsn, log)
	if err != nil {
		return nil, err
	}
	if err := db.Migrate(ctx, pool, migrations, "migrations", log); err != nil {
		return nil, err
	}
	s := &Repos{pool: pool, log: log}
	s.Keys = key.New(pool)
	return s, nil
}

// Close releases the connection pool.
func (s *Repos) Close() { s.pool.Close() }
