// Package repository implements the Auth domain repository ports on Postgres
// (Ports & Adapters: the adapter side of the hexagon). Each aggregate has its
// own adapter subpackage; pool-backed instances serve the single-aggregate use
// cases.
package repository

import (
	"context"
	"embed"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aaks/server/internal/platform/db"
	"github.com/aaks/server/services/auth/internal/domain"
	"github.com/aaks/server/services/auth/internal/infrastructure/repository/invite"
	"github.com/aaks/server/services/auth/internal/infrastructure/repository/session"
	"github.com/aaks/server/services/auth/internal/infrastructure/repository/signup"
	"github.com/aaks/server/services/auth/internal/infrastructure/repository/user"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Repos owns the auth Postgres pool and exposes pool-backed adapters for the
// domain ports. It is the composition-root entrypoint of this package.
type Repos struct {
	pool *pgxpool.Pool
	log  *slog.Logger

	Users          domain.UserRepository
	Sessions       domain.SessionRepository
	SignupRequests domain.SignupRequestRepository
	Invites        domain.InviteRepository
}

// New opens the auth database and runs migrations.
func New(ctx context.Context, dsn string, log *slog.Logger) (*Repos, error) {
	pool, err := db.Pool(ctx, dsn, log)
	if err != nil {
		return nil, err
	}
	if err := db.Migrate(ctx, pool, migrations, "migrations", log); err != nil {
		return nil, err
	}
	s := &Repos{pool: pool, log: log}
	s.Users = user.New(pool)
	s.Sessions = session.New(pool)
	s.SignupRequests = signup.New(pool)
	s.Invites = invite.New(pool)
	return s, nil
}

// Close releases the connection pool.
func (s *Repos) Close() { s.pool.Close() }
