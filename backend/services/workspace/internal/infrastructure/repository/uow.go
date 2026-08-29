package repository

import (
	"context"

	catalog "github.com/aaks/server/services/workspace/internal/application/catalog"
	resources "github.com/aaks/server/services/workspace/internal/application/resources"
	"github.com/aaks/server/services/workspace/internal/infrastructure/repository/catalog/mcp"
	"github.com/aaks/server/services/workspace/internal/infrastructure/repository/catalog/skill"
	"github.com/aaks/server/services/workspace/internal/infrastructure/repository/resources/knowledge"
	resmcp "github.com/aaks/server/services/workspace/internal/infrastructure/repository/resources/mcp"
	"github.com/aaks/server/services/workspace/internal/infrastructure/repository/resources/plugin"
	"github.com/aaks/server/services/workspace/internal/infrastructure/repository/resources/rule"
)

// CatalogUnitOfWork is the catalog application-layer transaction boundary. The
// repositories it hands out are tx-scoped adapters, so a definition mutation
// commits atomically before its created/deleted event is published.
type CatalogUnitOfWork struct {
	st *Repos
}

// NewCatalogUnitOfWork builds the UoW on top of the workspace pool.
func NewCatalogUnitOfWork(st *Repos) *CatalogUnitOfWork { return &CatalogUnitOfWork{st: st} }

// Do runs fn inside a single Postgres transaction. Any error rolls the whole
// mutation back; success commits.
func (u *CatalogUnitOfWork) Do(ctx context.Context, fn func(tx *catalog.Tx) error) error {
	pgTx, err := u.st.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = pgTx.Rollback(ctx) }()

	tx := &catalog.Tx{
		Skills: skill.New(pgTx),
		Mcps:   mcp.New(pgTx),
	}
	if err := fn(tx); err != nil {
		return err
	}
	return pgTx.Commit(ctx)
}

// ResourcesUnitOfWork is the resources application-layer transaction boundary:
// the workspace bootstrap seed (three default rules) commits atomically or
// rolls back entirely.
type ResourcesUnitOfWork struct {
	st *Repos
}

// NewResourcesUnitOfWork builds the UoW on top of the workspace pool.
func NewResourcesUnitOfWork(st *Repos) *ResourcesUnitOfWork { return &ResourcesUnitOfWork{st: st} }

// Do runs fn inside a single Postgres transaction. Any error rolls the whole
// mutation back; success commits.
func (u *ResourcesUnitOfWork) Do(ctx context.Context, fn func(tx *resources.Tx) error) error {
	pgTx, err := u.st.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = pgTx.Rollback(ctx) }()

	tx := &resources.Tx{
		Knowledge: knowledge.New(pgTx),
		Plugins:   plugin.New(pgTx),
		Rules:     rule.New(pgTx),
		Mcp:       resmcp.New(pgTx),
	}
	if err := fn(tx); err != nil {
		return err
	}
	return pgTx.Commit(ctx)
}
