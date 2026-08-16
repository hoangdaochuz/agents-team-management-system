package domain

import (
	"context"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/workspaces"
)

// Workspace is the workspace aggregate (wire DTO as domain type, D7).
type Workspace = workspaces.Workspace

// WorkspaceRepository is the workspace aggregate port.
type WorkspaceRepository interface {
	Create(ctx context.Context, orgID identity.ID, name, repoSource, defaultBranch, glyph, description string) (Workspace, error)
	ByID(ctx context.Context, id identity.ID) (Workspace, error)
	ListByUser(ctx context.Context, userID identity.ID) ([]Workspace, error)
	GetByUser(ctx context.Context, userID, workspaceID identity.ID) (Workspace, error)
}
