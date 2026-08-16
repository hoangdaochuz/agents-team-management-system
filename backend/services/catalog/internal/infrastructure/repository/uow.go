package repository

import (
	"context"

	"github.com/aaks/server/services/catalog/internal/application"
	"github.com/aaks/server/services/catalog/internal/infrastructure/repository/mcp"
	"github.com/aaks/server/services/catalog/internal/infrastructure/repository/skill"
)

// UnitOfWork is the application-layer transaction boundary. The repositories
// it hands out are tx-scoped adapters, so a definition mutation commits
// atomically before its created/deleted event is published.
type UnitOfWork struct {
	st *Repos
}

// NewUnitOfWork builds the UoW on top of the catalog pool.
func NewUnitOfWork(st *Repos) *UnitOfWork { return &UnitOfWork{st: st} }

// Do runs fn inside a single Postgres transaction. Any error rolls the whole
// mutation back; success commits.
func (u *UnitOfWork) Do(ctx context.Context, fn func(tx *application.Tx) error) error {
	pgTx, err := u.st.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = pgTx.Rollback(ctx) }()

	tx := &application.Tx{
		Skills: skill.New(pgTx),
		Mcps:   mcp.New(pgTx),
	}
	if err := fn(tx); err != nil {
		return err
	}
	return pgTx.Commit(ctx)
}
