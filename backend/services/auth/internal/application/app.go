// Package application holds the Auth use-case handlers: login/session
// lifecycle, the signup workflow, and the signup.approved/declined and
// invite.created projections. It depends only on domain ports plus the
// abstractions declared here (DIP: no sarama, no pgx, no net/http).
package application

import (
	"context"
	"log/slog"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/services/auth/internal/domain"
)

// EventPublisher publishes events to the bus (DIP: application never imports
// sarama; the adapter lives in infrastructure).
type EventPublisher interface {
	Publish(ctx context.Context, topic string, data any, key identity.ID)
}

// Repository is the set of aggregate ports the Auth use cases operate on.
type Repository struct {
	Users          domain.UserRepository
	Sessions       domain.SessionRepository
	SignupRequests domain.SignupRequestRepository
	Invites        domain.InviteRepository
}

// App is the Auth application service: the composition root injects concrete
// repositories and the event publisher.
type App struct {
	repo *Repository
	pub  EventPublisher
	log  *slog.Logger

	// loginRate limits login attempts per IP+email; key -> first-failure time.
	loginRate loginRate
}

// New builds the application service with its injected dependencies.
func New(repo *Repository, pub EventPublisher, log *slog.Logger) *App {
	return &App{repo: repo, pub: pub, log: log}
}
