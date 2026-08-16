// Package application holds the Project use-case handlers. It depends only on
// domain ports (DIP: no pgx, no net/http).
package application

import (
	"log/slog"

	"github.com/aaks/server/services/project/internal/domain"
)

// Repository is the non-transactional store of aggregate ports (plain pool).
type Repository struct {
	Projects domain.ProjectRepository
}

// App is the Project application service: the composition root injects the
// concrete repository.
type App struct {
	repo *Repository
	log  *slog.Logger
}

// New builds the application service with its injected dependencies.
func New(repo *Repository, log *slog.Logger) *App {
	return &App{repo: repo, log: log}
}