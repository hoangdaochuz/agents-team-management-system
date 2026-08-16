package repository

import (
	"context"

	"github.com/aaks/server/services/orgs/internal/application"
	"github.com/aaks/server/services/orgs/internal/infrastructure/repository/invite"
	"github.com/aaks/server/services/orgs/internal/infrastructure/repository/joinrequest"
	"github.com/aaks/server/services/orgs/internal/infrastructure/repository/member"
	"github.com/aaks/server/services/orgs/internal/infrastructure/repository/organization"
	"github.com/aaks/server/services/orgs/internal/infrastructure/repository/orgrequest"
	"github.com/aaks/server/services/orgs/internal/infrastructure/repository/workspace"
)

// UnitOfWork is the application-layer transaction boundary. The repositories
// it hands out are tx-scoped adapters, so multi-aggregate mutations commit
// atomically or roll back entirely.
type UnitOfWork struct {
	r *Repos
}

// NewUnitOfWork builds the UoW on top of the orgs pool.
func NewUnitOfWork(r *Repos) *UnitOfWork { return &UnitOfWork{r: r} }

// Do runs fn inside a single Postgres transaction. Any error rolls the whole
// mutation back; success commits.
func (u *UnitOfWork) Do(ctx context.Context, fn func(tx *application.Tx) error) error {
	pgTx, err := u.r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = pgTx.Rollback(ctx) }()

	tx := &application.Tx{
		Organizations: organization.New(pgTx),
		Workspaces:    workspace.New(pgTx),
		Members:       member.New(pgTx),
		Invites:       invite.New(pgTx),
		JoinRequests:  joinrequest.New(pgTx),
		OrgRequests:   orgrequest.New(pgTx),
	}
	if err := fn(tx); err != nil {
		return err
	}
	return pgTx.Commit(ctx)
}