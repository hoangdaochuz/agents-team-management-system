// Package application holds the Task use-case handlers and the task-lifecycle
// saga coordinator. It depends only on domain ports plus the abstractions
// declared here (DIP: no sarama, no pgx, no net/http).
package application

import (
	"context"
	"log/slog"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/services/task/internal/domain"
)

// EventPublisher publishes events to the bus (DIP: application never imports
// sarama; the adapter lives in infrastructure).
type EventPublisher interface {
	Publish(ctx context.Context, topic string, data any, key identity.ID)
}

// Repository is the non-transactional store of aggregate ports (plain pool).
type Repository struct {
	Tasks    domain.TaskRepository
	Feedback domain.FeedbackRepository
}

// App is the Task application service: the composition root injects the
// concrete repositories and the event publisher.
type App struct {
	repo *Repository
	pub  EventPublisher
	log  *slog.Logger
}

// New builds the application service with its injected dependencies.
func New(repo *Repository, pub EventPublisher, log *slog.Logger) *App {
	return &App{repo: repo, pub: pub, log: log}
}
