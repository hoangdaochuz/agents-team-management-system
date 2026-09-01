// Package application holds the Orgs use-case handlers. It depends only on
// domain ports plus the abstractions declared here (DIP: no sarama, no pgx,
// no net/http).
package application

import (
	"context"
	"log/slog"

	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/services/identity/internal/domain/orgs"
)

// EventPublisher publishes events to the bus (DIP: application never imports
// sarama; the adapter lives in infrastructure).

// WorkspaceProvisioner asks the Workspace service to provision a newly created
// workspace (default repo binding + default rules) — the former
// workspace.created Kafka event, now a direct call. A nil provisioner is a
// no-op (Workspace URL unconfigured).
type WorkspaceProvisioner interface {
	Provision(ctx context.Context, d events.WorkspaceCreatedData) error
}
type EventPublisher interface {
	Publish(ctx context.Context, topic string, data any, key identity.ID)
}

// UserActivator activates a user account — the auth plane's half of signup
// approval. It is folded into the approving UnitOfWork (rather than left to
// the in-process signup.approved event, whose handlers are best-effort) so the
// approval and the activation commit atomically: a transient failure rolls
// both back and the operator can retry, instead of a 200 with a user who can
// never log in.
type UserActivator interface {
	Activate(ctx context.Context, id identity.ID) error
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
	Users         UserActivator
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
// repositories, the UoW, the event publisher and the workspace provisioner.
type App struct {
	repo         *Repository
	uow          UnitOfWork
	pub          EventPublisher
	provisioners []WorkspaceProvisioner
	log          *slog.Logger
}

// New builds the application service with its injected dependencies.
func New(repo *Repository, uow UnitOfWork, pub EventPublisher, log *slog.Logger, provisioners ...WorkspaceProvisioner) *App {
	return &App{repo: repo, uow: uow, pub: pub, provisioners: provisioners, log: log}
}

// provisionWorkspace notifies every provisioner of a new workspace. Best-effort
// with logged failures, matching the former publish semantics. A confirmed
// provisioning marks the workspace provisioned so the reconciler's sweep stops
// re-issuing the POST (a failed mark just leaves it in the sweep set — the
// endpoint is idempotent, so an extra sweep is harmless).
func (a *App) provisionWorkspace(ctx context.Context, d events.WorkspaceCreatedData) {
	for _, p := range a.provisioners {
		if p == nil {
			continue
		}
		if err := p.Provision(ctx, d); err != nil {
			a.log.Error("workspace provisioning failed", "workspace_id", d.WorkspaceID, "error", err)
			return
		}
	}
	if err := a.repo.Workspaces.MarkProvisioned(ctx, d.WorkspaceID); err != nil {
		a.log.Error("workspace provisioning mark failed", "workspace_id", d.WorkspaceID, "error", err)
	}
}
