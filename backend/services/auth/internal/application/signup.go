package application

import (
	"context"
	"errors"

	"golang.org/x/crypto/bcrypt"

	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/services/auth/internal/domain"
)

// Validation sentinels for the signup workflow; the HTTP layer maps these to
// status codes and user-safe messages.
var (
	ErrFieldsRequired       = errors.New("full_name, email and password are required")
	ErrPasswordTooShort     = errors.New("password must be at least 8 characters")
	ErrStartMode            = errors.New("start_mode must be 'join' or 'create'")
	ErrInviteCodeRequired   = errors.New("invite_code is required for join mode")
	ErrUnknownInviteCode    = errors.New("unknown invite code")
	ErrOrganizationRequired = errors.New("organization_name is required for create mode")
)

// SignupInput is the validated join/create-mode signup payload.
type SignupInput struct {
	FullName         string
	Email            string
	Password         string
	StartMode        string
	InviteCode       string
	OrganizationName string
}

// Signup records a pending access request (join or create mode) and emits
// signup.requested AFTER the request persisted. Join mode resolves the invite
// code to its workspace target; create mode requests the owner role.
func (a *App) Signup(ctx context.Context, in SignupInput) (identity.ID, error) {
	if in.FullName == "" || in.Email == "" || in.Password == "" {
		return "", ErrFieldsRequired
	}
	if len(in.Password) < 8 {
		return "", ErrPasswordTooShort
	}
	if in.StartMode != "join" && in.StartMode != "create" {
		return "", ErrStartMode
	}

	workspaceID := identity.ID("")
	workspaceName := ""
	var role identity.Role
	if in.StartMode == "join" {
		if in.InviteCode == "" {
			return "", ErrInviteCodeRequired
		}
		inv, err := a.repo.Invites.Lookup(ctx, in.InviteCode)
		if err != nil {
			return "", ErrUnknownInviteCode
		}
		workspaceID = inv.WorkspaceID
		workspaceName = inv.WorkspaceName
		role = inv.Role
	} else {
		if in.OrganizationName == "" {
			return "", ErrOrganizationRequired
		}
		role = identity.RoleOwner
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	req, err := a.repo.SignupRequests.Create(ctx, in.FullName, in.Email, string(hash),
		in.StartMode, in.InviteCode, workspaceName, in.OrganizationName, workspaceID, role)
	if err != nil {
		return "", err
	}
	a.pub.Publish(ctx, events.TopicSignupRequested, events.SignupRequestedData{
		RequestID: req.ID, UserID: req.UserID, Name: in.FullName, Email: in.Email,
		Mode: in.StartMode, InviteCode: in.InviteCode, WorkspaceID: req.WorkspaceID,
		OrganizationName: in.OrganizationName, RequestedRole: role,
	}, "")
	return req.ID, nil
}

// SignupStatus returns a pending request by id (signup-cookie status polling).
func (a *App) SignupStatus(ctx context.Context, requestID identity.ID) (domain.SignupRequest, error) {
	return a.repo.SignupRequests.Get(ctx, requestID)
}

// SignupStatusByEmail returns the most recent request for an email (cookie-less
// fallback).
func (a *App) SignupStatusByEmail(ctx context.Context, email string) (domain.SignupRequest, error) {
	return a.repo.SignupRequests.GetByEmail(ctx, email)
}

// HandleSignupApproved applies a signup.approved event: the request transitions
// to approved and the user is activated (by id when present, else by email).
func (a *App) HandleSignupApproved(ctx context.Context, d events.SignupApprovedData) error {
	if err := a.repo.SignupRequests.SetStatus(ctx, d.RequestID, identity.SignupApproved); err != nil {
		return err
	}
	if d.UserID != "" {
		return a.repo.Users.Activate(ctx, d.UserID)
	}
	return a.repo.Users.ActivateByEmail(ctx, d.Email)
}

// HandleSignupDeclined applies a signup.declined event.
func (a *App) HandleSignupDeclined(ctx context.Context, d events.SignupDeclinedData) error {
	return a.repo.SignupRequests.SetStatus(ctx, d.RequestID, identity.SignupDeclined)
}

// HandleInviteCreated projects an invite.created event into the local
// invite-code table so join-mode signups resolve locally.
func (a *App) HandleInviteCreated(ctx context.Context, d events.InviteCreatedData) error {
	return a.repo.Invites.Upsert(ctx, d.InviteCode, d.Email, d.Role, d.WorkspaceID, d.WorkspaceName)
}
