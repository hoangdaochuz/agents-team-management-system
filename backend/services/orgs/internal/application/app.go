// Package application holds the Orgs use-case handlers. It depends only on
// domain ports plus the abstractions declared here (DIP: no sarama, no pgx,
// no net/http).
package application

import (
	"context"
	"log/slog"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/services/orgs/internal/domain"
)

// EventPublisher publishes events to the bus (DIP: application never imports
// sarama; the adapter lives in infrastructure).
type EventPublisher interface {
	Publish(ctx context.Context, topic string, data any, key identity.ID)
}

// Tx carries transactional repository handles scoped to one UnitOfWork
// boundary. Repositories are the domain ports, so the UoW stays infra-shaped
// without leaking SQL into domain.
type Tx struct {
	Organizations domain.OrganizationRepository
	Workspaces    domain.WorkspaceRepository
	Members       domain.MembershipRepository
	Invites       domain.InviteRepository
	JoinRequests  domain.JoinRequestRepository
	OrgRequests   domain.OrgRequestRepository
}

// UnitOfWork commits fn's repository operations atomically. Multi-aggregate
// mutations (createWorkspace, approveOrgRequest) run through it; single
// aggregate mutations may use Repository directly.
type UnitOfWork interface {
	Do(ctx context.Context, fn func(tx *Tx) error) error
}

// Repository is the non-transactional store of aggregate ports (plain pool).
type Repository struct {
	Organizations domain.OrganizationRepository
	Workspaces    domain.WorkspaceRepository
	Members       domain.MembershipRepository
	Invites       domain.InviteRepository
	JoinRequests  domain.JoinRequestRepository
	OrgRequests   domain.OrgRequestRepository
}

// App is the Orgs application service: the composition root injects concrete
// repositories, the UoW and the event publisher.
type App struct {
	repo *Repository
	uow  UnitOfWork
	pub  EventPublisher
	log  *slog.Logger
}

// New builds the application service with its injected dependencies.
func New(repo *Repository, uow UnitOfWork, pub EventPublisher, log *slog.Logger) *App {
	return &App{repo: repo, uow: uow, pub: pub, log: log}
}