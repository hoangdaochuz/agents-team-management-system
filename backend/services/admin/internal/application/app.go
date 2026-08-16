// Package application holds the Admin use-case handlers. It depends only on
// domain ports plus the abstractions declared here (DIP: no sarama, no pgx,
// no net/http).
package application

import (
	"log/slog"

	"github.com/aaks/server/services/admin/internal/domain"
)

// Repository is the non-transactional store of aggregate ports (plain pool).
// Admin use cases are single-aggregate writes, so no UnitOfWork is needed.
type Repository struct {
	Audit domain.AuditRepository
	Flags domain.FlagRepository
}

// App is the Admin application service: the composition root injects concrete
// repositories.
type App struct {
	repo *Repository
	log  *slog.Logger
}

// New builds the application service with its injected dependencies.
func New(repo *Repository, log *slog.Logger) *App {
	return &App{repo: repo, log: log}
}