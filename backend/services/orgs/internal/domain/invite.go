package domain

import (
	"context"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/workspaces"
)

// Invite is an invite row with its internal invite code.
type Invite struct {
	workspaces.Invite
	WorkspaceID identity.ID
	InviteCode  string
}

// InviteRepository is the invite aggregate port.
type InviteRepository interface {
	Create(ctx context.Context, workspaceID identity.ID, emails []string, role identity.Role) ([]Invite, error)
}
