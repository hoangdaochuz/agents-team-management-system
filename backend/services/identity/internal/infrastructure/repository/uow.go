package repository

import (
	"context"

	orgs "github.com/aaks/server/services/identity/internal/application/orgs"
	"github.com/aaks/server/services/identity/internal/infrastructure/repository/auth/user"
	orgsinvite "github.com/aaks/server/services/identity/internal/infrastructure/repository/orgs/invite"
	"github.com/aaks/server/services/identity/internal/infrastructure/repository/orgs/joinrequest"
	"github.com/aaks/server/services/identity/internal/infrastructure/repository/orgs/member"
	"github.com/aaks/server/services/identity/internal/infrastructure/repository/orgs/organization"
	"github.com/aaks/server/services/identity/internal/infrastructure/repository/orgs/orgrequest"
	"github.com/aaks/server/services/identity/internal/infrastructure/repository/orgs/workspace"
)

// UnitOfWork is the application-layer transaction boundary. The repositories
// it hands out are tx-scoped adapters, so multi-aggregate mutations commit
// atomically or roll back entirely.
type UnitOfWork struct {
	r *Repos
}

// NewUnitOfWork builds the UoW on top of the identity pool.
func NewUnitOfWork(r *Repos) *UnitOfWork { return &UnitOfWork{r: r} }

// Do runs fn inside a single Postgres transaction. Any error rolls the whole
// mutation back; success commits.
func (u *UnitOfWork) Do(ctx context.Context, fn func(tx *orgs.Tx) error) error {
	pgTx, err := u.r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = pgTx.Rollback(ctx) }()

	tx := &orgs.Tx{
		Organizations: organization.New(pgTx),
		Workspaces:    workspace.New(pgTx),
		Members:       member.New(pgTx),
		Invites:       orgsinvite.New(pgTx),
		JoinRequests:  joinrequest.New(pgTx),
		OrgRequests:   orgrequest.New(pgTx),
		Users:         user.New(pgTx),
	}
	if err := fn(tx); err != nil {
		return err
	}
	return pgTx.Commit(ctx)
}
