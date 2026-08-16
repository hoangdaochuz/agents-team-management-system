package domain

import (
	"context"

	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/internal/contracts/identity"
)

// JoinRequest mirrors identity.SignupRequest for a workspace (join mode).
type JoinRequest struct {
	identity.SignupRequest
	UserID identity.ID
	Status identity.SignupState
}

// JoinRequestRepository is the join-mode request projection port.
type JoinRequestRepository interface {
	ListPending(ctx context.Context, workspaceID identity.ID) ([]JoinRequest, error)
	Get(ctx context.Context, requestID identity.ID) (JoinRequest, error)
	Upsert(ctx context.Context, req events.SignupRequestedData) error
	SetStatus(ctx context.Context, requestID identity.ID, status identity.SignupState) error
}

// OrgRequest mirrors identity.SignupRequest for create-mode requests.
type OrgRequest struct {
	identity.SignupRequest
	UserID           identity.ID
	OrganizationName string
	Status           identity.SignupState
}

// OrgRequestRepository is the create-mode request projection port.
type OrgRequestRepository interface {
	ListPending(ctx context.Context) ([]OrgRequest, error)
	Get(ctx context.Context, requestID identity.ID) (OrgRequest, error)
	Upsert(ctx context.Context, req events.SignupRequestedData) error
	SetStatus(ctx context.Context, requestID identity.ID, status identity.SignupState) error
}