package domain

import (
	"context"

	"github.com/aaks/server/internal/contracts/identity"
)

// InviteCode resolves a join-mode invite code to its workspace target. It is
// the local projection of orgs invite.created events.
type InviteCode struct {
	Code          string
	Email         string
	Role          identity.Role
	WorkspaceID   identity.ID
	WorkspaceName string
}

// InviteRepository is the invite-code projection port.
type InviteRepository interface {
	Lookup(ctx context.Context, code string) (InviteCode, error)
	Upsert(ctx context.Context, code, email string, role identity.Role, workspaceID identity.ID, workspaceName string) error
}
