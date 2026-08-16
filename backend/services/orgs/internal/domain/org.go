package domain

import (
	"context"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/workspaces"
)

// Organization is the org aggregate. Its wire shape is the shared kernel DTO
// (D7: no divergence, so no private type is introduced).
type Organization = workspaces.Organization

// OrganizationRepository is the org aggregate port (ISP: per-aggregate).
type OrganizationRepository interface {
	List(ctx context.Context) ([]Organization, error)
	Get(ctx context.Context, id identity.ID) (Organization, error)
	Create(ctx context.Context, ownerID identity.ID, name string, plan identity.Plan) (Organization, error)
	SetStatus(ctx context.Context, id identity.ID, status identity.OrgStatus) (Organization, error)
	ByUser(ctx context.Context, userID identity.ID) (Organization, error)
	Stats(ctx context.Context) (organizations, workspaces, openSeats int, err error)
}
