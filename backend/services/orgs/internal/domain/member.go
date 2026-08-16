package domain

import (
	"context"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/workspaces"
)

// Member is a membership with its user identity snapshot.
type Member struct {
	workspaces.Member
	WorkspaceID identity.ID
	UserID      identity.ID
}

// MembershipRepository is the membership aggregate port.
type MembershipRepository interface {
	List(ctx context.Context, workspaceID identity.ID) ([]Member, error)
	Add(ctx context.Context, workspaceID, userID identity.ID, userName, userEmail string, role identity.Role) (Member, error)
	SetRole(ctx context.Context, workspaceID, memberID identity.ID, role identity.Role) (Member, error)
	Remove(ctx context.Context, workspaceID, memberID identity.ID) error
	OwnerCount(ctx context.Context, workspaceID identity.ID) (int, error)
	UserRoleIn(ctx context.Context, userID, workspaceID identity.ID) (identity.Role, error)
}
