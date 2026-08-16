// Package repository implements the Orgs domain repository ports on Postgres
// (Ports & Adapters: the adapter side of the hexagon). Each aggregate has its
// own adapter subpackage; pool-backed instances serve single-aggregate use
// cases and tx-scoped instances are constructed by the UnitOfWork for
// multi-aggregate mutations.
package repository

import (
	"context"
	"embed"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aaks/server/internal/platform/db"
	"github.com/aaks/server/services/orgs/internal/domain"
	"github.com/aaks/server/services/orgs/internal/infrastructure/repository/invite"
	"github.com/aaks/server/services/orgs/internal/infrastructure/repository/joinrequest"
	"github.com/aaks/server/services/orgs/internal/infrastructure/repository/member"
	"github.com/aaks/server/services/orgs/internal/infrastructure/repository/organization"
	"github.com/aaks/server/services/orgs/internal/infrastructure/repository/orgrequest"
	"github.com/aaks/server/services/orgs/internal/infrastructure/repository/workspace"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Repos owns the orgs Postgres pool and exposes pool-backed adapters for the
// domain ports. It is the composition-root entrypoint of this package.
type Repos struct {
	pool *pgxpool.Pool
	log  *slog.Logger

	Organizations domain.OrganizationRepository
	Workspaces    domain.WorkspaceRepository
	Members       domain.MembershipRepository
	Invites       domain.InviteRepository
	JoinRequests  domain.JoinRequestRepository
	OrgRequests   domain.OrgRequestRepository
}

// New opens the orgs database and runs migrations.
func New(ctx context.Context, dsn string, log *slog.Logger) (*Repos, error) {
	pool, err := db.Pool(ctx, dsn, log)
	if err != nil {
		return nil, err
	}
	if err := db.Migrate(ctx, pool, migrations, "migrations", log); err != nil {
		return nil, err
	}
	r := &Repos{pool: pool, log: log}
	r.Organizations = organization.New(pool)
	r.Workspaces = workspace.New(pool)
	r.Members = member.New(pool)
	r.Invites = invite.New(pool)
	r.JoinRequests = joinrequest.New(pool)
	r.OrgRequests = orgrequest.New(pool)
	return r, nil
}

// Close releases the connection pool.
func (r *Repos) Close() { r.pool.Close() }