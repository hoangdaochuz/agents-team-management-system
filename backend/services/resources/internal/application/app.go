// Package application holds the Resources use-case handlers. It depends only on
// domain ports plus the abstractions declared here (DIP: no sarama, no pgx,
// no net/http).
package application

import (
	"context"
	"log/slog"

	"github.com/aaks/server/services/resources/internal/domain"
)

// Tx carries transactional repository handles scoped to one UnitOfWork
// boundary. Repositories are the domain ports, so the UoW stays infra-shaped
// without leaking SQL into domain.
type Tx struct {
	Knowledge domain.KnowledgeRepository
	Plugins   domain.PluginRepository
	Rules     domain.RuleRepository
	Mcp       domain.McpConnectionRepository
}

// UnitOfWork commits fn's repository operations atomically. Multi-write
// mutations (workspace bootstrap seeds several rules) run through it so the
// seed is either fully applied or rolled back.
type UnitOfWork interface {
	Do(ctx context.Context, fn func(tx *Tx) error) error
}

// Repository is the non-transactional store of aggregate ports (plain pool).
type Repository struct {
	Knowledge domain.KnowledgeRepository
	Plugins   domain.PluginRepository
	Rules     domain.RuleRepository
	Mcp       domain.McpConnectionRepository
}

// App is the Resources application service: the composition root injects
// concrete repositories and the UoW.
type App struct {
	repo *Repository
	uow  UnitOfWork
	log  *slog.Logger
}

// New builds the application service with its injected dependencies.
func New(repo *Repository, uow UnitOfWork, log *slog.Logger) *App {
	return &App{repo: repo, uow: uow, log: log}
}
