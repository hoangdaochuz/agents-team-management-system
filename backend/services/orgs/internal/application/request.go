package application

import (
	"context"
	"errors"

	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/services/orgs/internal/domain"
)

// ListPendingRequests returns pending join-mode signup requests (admin+).
func (a *App) ListPendingRequests(ctx context.Context, workspaceID identity.ID) ([]identity.SignupRequest, error) {
	rows, err := a.repo.JoinRequests.ListPending(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]identity.SignupRequest, 0, len(rows))
	for _, jr := range rows {
		out = append(out, jr.SignupRequest)
	}
	if out == nil {
		out = []identity.SignupRequest{}
	}
	return out, nil
}

// ApproveJoinRequest approves a pending join-mode request: the membership and
// the request status commit atomically in a UoW, then signup.approved is
// published and the action audited.
func (a *App) ApproveJoinRequest(ctx context.Context, actorID, workspaceID, requestID identity.ID) error {
	var jr domain.JoinRequest
	err := a.uow.Do(ctx, func(tx *Tx) error {
		var err error
		jr, err = tx.JoinRequests.Get(ctx, requestID)
		if err != nil {
			return err
		}
		if jr.Status != identity.SignupPending {
			return domain.ErrNotPending
		}
		if _, err := tx.Members.Add(ctx, jr.WorkspaceID, jr.UserID, jr.Name, jr.Email, jr.RequestedRole); err != nil {
			return err
		}
		return tx.JoinRequests.SetStatus(ctx, requestID, identity.SignupApproved)
	})
	if err != nil {
		return err
	}
	a.pub.Publish(ctx, events.TopicSignupApproved, events.SignupApprovedData{
		RequestID: requestID, UserID: jr.UserID, Email: jr.Email, Name: jr.Name,
		Mode: "join", WorkspaceID: jr.WorkspaceID, Role: jr.RequestedRole,
	}, jr.UserID)
	a.publishAudit(ctx, actorID, workspaceID, "signup.approved", "join-request", string(requestID))
	return nil
}

// DeclineJoinRequest marks a pending join-mode request declined and publishes
// signup.declined.
func (a *App) DeclineJoinRequest(ctx context.Context, workspaceID, requestID identity.ID) error {
	var jr domain.JoinRequest
	err := a.uow.Do(ctx, func(tx *Tx) error {
		var err error
		jr, err = tx.JoinRequests.Get(ctx, requestID)
		if err != nil {
			return err
		}
		return tx.JoinRequests.SetStatus(ctx, requestID, identity.SignupDeclined)
	})
	if err != nil {
		return err
	}
	a.pub.Publish(ctx, events.TopicSignupDeclined, events.SignupDeclinedData{
		RequestID: requestID, UserID: jr.UserID,
	}, jr.UserID)
	return nil
}

// SendInvites creates invites for the given emails and emits invite.created
// per invite.
func (a *App) SendInvites(ctx context.Context, workspaceID identity.ID, emails []string, role identity.Role) ([]domain.Invite, error) {
	if len(emails) == 0 {
		return nil, errEmailsRequired
	}
	if role != identity.RoleAdmin && role != identity.RoleMember {
		role = identity.RoleMember
	}
	ws, err := a.repo.Workspaces.ByID(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	invites, err := a.repo.Invites.Create(ctx, workspaceID, emails, role)
	if err != nil {
		return nil, err
	}
	for _, inv := range invites {
		a.pub.Publish(ctx, events.TopicInviteCreated, events.InviteCreatedData{
			InviteID: inv.ID, Email: inv.Email, Role: inv.Role,
			InviteCode: inv.InviteCode, WorkspaceID: workspaceID, WorkspaceName: ws.Name,
		}, workspaceID)
	}
	return invites, nil
}

// ProjectSignupRequest persists a signup.requested event: create-mode requests
// surface under the sysadmin surface, join-mode under the workspace. Idempotent
// (the stores upsert on the request id).
func (a *App) ProjectSignupRequest(ctx context.Context, d events.SignupRequestedData) error {
	if d.Mode == "create" {
		return a.repo.OrgRequests.Upsert(ctx, d)
	}
	return a.repo.JoinRequests.Upsert(ctx, d)
}

var errEmailsRequired = errors.New("emails is required")

// Audit records a workspace admin action for the Admin service (best-effort).
func (a *App) Audit(ctx context.Context, actorID, workspaceID identity.ID, action, kind, target string) {
	a.publishAudit(ctx, actorID, workspaceID, action, kind, target)
}

// publishAudit emits a workspace admin action to the Admin service.
func (a *App) publishAudit(ctx context.Context, actorID, workspaceID identity.ID, action, kind, target string) {
	a.pub.Publish(ctx, events.TopicAuditRecorded, events.AuditRecordedData{
		WorkspaceID: workspaceID, ActorID: actorID,
		Action: action, ActionKind: kind, Target: target,
	}, workspaceID)
}