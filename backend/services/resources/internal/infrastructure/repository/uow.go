package repository

import (
	"context"

	"github.com/aaks/server/services/resources/internal/application"
)

// UnitOfWork is the application-layer transaction boundary. The repositories
// it hands out are tx-scoped adapters, so the workspace bootstrap seed (three
// default rules) commits atomically or rolls back entirely.
type UnitOfWork struct {
	st *Store
}

// NewUnitOfWork builds the UoW on top of the resources pool.
func NewUnitOfWork(st *Store) *UnitOfWork { return &UnitOfWork{st: st} }

// Do runs fn inside a single Postgres transaction. Any error rolls the whole
// mutation back; success commits.
func (u *UnitOfWork) Do(ctx context.Context, fn func(tx *application.Tx) error) error {
	pgTx, err := u.st.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = pgTx.Rollback(ctx) }()

	tx := &application.Tx{
		Knowledge: &knowledgeRepo{q: pgTx},
		Plugins:   &pluginRepo{q: pgTx},
		Rules:     &ruleRepo{q: pgTx},
		Mcp:       &mcpRepo{q: pgTx},
	}
	if err := fn(tx); err != nil {
		return err
	}
	return pgTx.Commit(ctx)
}
