package repository

import (
	"context"

	"github.com/aaks/server/services/resources/internal/application"
	"github.com/aaks/server/services/resources/internal/infrastructure/repository/knowledge"
	"github.com/aaks/server/services/resources/internal/infrastructure/repository/mcp"
	"github.com/aaks/server/services/resources/internal/infrastructure/repository/plugin"
	"github.com/aaks/server/services/resources/internal/infrastructure/repository/rule"
)

// UnitOfWork is the application-layer transaction boundary. The repositories
// it hands out are tx-scoped adapters, so the workspace bootstrap seed (three
// default rules) commits atomically or rolls back entirely.
type UnitOfWork struct {
	st *Repos
}

// NewUnitOfWork builds the UoW on top of the resources pool.
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
		Knowledge: knowledge.New(pgTx),
		Plugins:   plugin.New(pgTx),
		Rules:     rule.New(pgTx),
		Mcp:       mcp.New(pgTx),
	}
	if err := fn(tx); err != nil {
		return err
	}
	return pgTx.Commit(ctx)
}