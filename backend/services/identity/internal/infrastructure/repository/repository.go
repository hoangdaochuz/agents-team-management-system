// Package repository implements the Identity domain repository ports on
// Postgres (Ports & Adapters: the adapter side of the hexagon). It owns a
// single pool over identity_db — the consolidated auth/orgs/admin schema —
// and exposes pool-backed adapters for every aggregate; tx-scoped instances
// are constructed by the UnitOfWork for multi-aggregate mutations.
package repository

import (
	"context"
	"embed"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aaks/server/internal/platform/db"
	authdomain "github.com/aaks/server/services/identity/internal/domain/auth"
	orgsdomain "github.com/aaks/server/services/identity/internal/domain/orgs"
	admindomain "github.com/aaks/server/services/identity/internal/domain/admin"
	"github.com/aaks/server/services/identity/internal/infrastructure/repository/admin/audit"
	"github.com/aaks/server/services/identity/internal/infrastructure/repository/admin/flag"
	authinvite "github.com/aaks/server/services/identity/internal/infrastructure/repository/auth/invite"
	"github.com/aaks/server/services/identity/internal/infrastructure/repository/auth/session"
	"github.com/aaks/server/services/identity/internal/infrastructure/repository/auth/signup"
	"github.com/aaks/server/services/identity/internal/infrastructure/repository/auth/user"
	orgsinvite "github.com/aaks/server/services/identity/internal/infrastructure/repository/orgs/invite"
	"github.com/aaks/server/services/identity/internal/infrastructure/repository/orgs/joinrequest"
	"github.com/aaks/server/services/identity/internal/infrastructure/repository/orgs/member"
	"github.com/aaks/server/services/identity/internal/infrastructure/repository/orgs/organization"
	"github.com/aaks/server/services/identity/internal/infrastructure/repository/orgs/orgrequest"
	"github.com/aaks/server/services/identity/internal/infrastructure/repository/orgs/workspace"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Repos owns the identity Postgres pool and exposes pool-backed adapters for
// the auth, orgs, and admin aggregate ports. It is the composition-root
// entrypoint of this package.
type Repos struct {
	pool *pgxpool.Pool
	log  *slog.Logger

	// Auth plane: users, sessions, signup requests, invite-code projection.
	Users          authdomain.UserRepository
	Sessions       authdomain.SessionRepository
	SignupRequests authdomain.SignupRequestRepository
	AuthInvites    authdomain.InviteRepository // invite-code projection (invite_codes)

	// Orgs plane: organizations, workspaces, memberships, invites, projections.
	Organizations orgsdomain.OrganizationRepository
	Workspaces    orgsdomain.WorkspaceRepository
	Members       orgsdomain.MembershipRepository
	Invites       orgsdomain.InviteRepository
	JoinRequests  orgsdomain.JoinRequestRepository
	OrgRequests   orgsdomain.OrgRequestRepository

	// Admin plane: audit entries and feature flags.
	Audit admindomain.AuditRepository
	Flags admindomain.FlagRepository
}

// New opens the identity database (auth + orgs + admin schemas, one pool) and
// runs the merged migrations.
func New(ctx context.Context, dsn string, log *slog.Logger) (*Repos, error) {
	pool, err := db.Pool(ctx, dsn, log)
	if err != nil {
		return nil, err
	}
	if err := db.Migrate(ctx, pool, migrations, "migrations", log); err != nil {
		return nil, err
	}
	r := &Repos{pool: pool, log: log}
	r.Users = user.New(pool)
	r.Sessions = session.New(pool)
	r.SignupRequests = signup.New(pool)
	r.AuthInvites = authinvite.New(pool)
	r.Organizations = organization.New(pool)
	r.Workspaces = workspace.New(pool)
	r.Members = member.New(pool)
	r.Invites = orgsinvite.New(pool)
	r.JoinRequests = joinrequest.New(pool)
	r.OrgRequests = orgrequest.New(pool)
	r.Audit = audit.New(pool)
	r.Flags = flag.New(pool)
	return r, nil
}

// Close releases the connection pool.
func (r *Repos) Close() { r.pool.Close() }
