// Package application holds the Agent use-case handlers. It depends only on
// domain ports plus the abstractions declared here (DIP: no pgx, no net/http).
package application

import (
	"context"
	"log/slog"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/resources"
	"github.com/aaks/server/services/agent/internal/domain"
)

// McpCatalogClient hydrates MCP server definitions from the Catalog service
// (Anti-Corruption Layer port; the HTTP adapter lives in infrastructure).
type McpCatalogClient interface {
	FetchMcpServers(ctx context.Context, ids []identity.ID) ([]resources.McpServer, error)
}

// Repository is the non-transactional store of aggregate ports (plain pool).
type Repository struct {
	Agents      domain.AgentRepository
	Projections domain.CatalogProjectionRepository
}

// App is the Agent application service: the composition root injects the
// concrete repositories and the Catalog client.
type App struct {
	repo    *Repository
	catalog McpCatalogClient
	log     *slog.Logger
}

// New builds the application service with its injected dependencies. catalog
// may be nil when CATALOG_URL is unset (the MCP hydration endpoint then
// returns an empty list).
func New(repo *Repository, catalog McpCatalogClient, log *slog.Logger) *App {
	return &App{repo: repo, catalog: catalog, log: log}
}
