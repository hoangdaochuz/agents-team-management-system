package domain

import (
	"context"
	"time"

	"github.com/aaks/server/internal/contracts/identity"
)

// SignupRequest is a pending access request (join or create mode) with its
// internal user id, workspace target and lifecycle state.
type SignupRequest struct {
	ID               identity.ID
	UserID           identity.ID
	Email            string
	Mode             string
	InviteCode       string
	WorkspaceID      identity.ID
	WorkspaceName    string
	OrganizationName string
	RequestedRole    identity.Role
	Status           identity.SignupState
	RequestedAt      time.Time
}

// SignupRequestRepository is the signup-request aggregate port.
type SignupRequestRepository interface {
	Create(ctx context.Context, name, email, passwordHash, mode, inviteCode, workspaceName, organizationName string, workspaceID identity.ID, role identity.Role) (SignupRequest, error)
	Get(ctx context.Context, id identity.ID) (SignupRequest, error)
	GetByEmail(ctx context.Context, email string) (SignupRequest, error)
	SetStatus(ctx context.Context, id identity.ID, status identity.SignupState) error
}