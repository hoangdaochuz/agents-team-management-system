package application

import (
	"context"

	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/workspaces"
	"github.com/aaks/server/services/orgs/internal/domain"
)

// ListOrgs returns all organizations (sysadmin), newest first.
func (a *App) ListOrgs(ctx context.Context) ([]workspaces.Organization, error) {
	orgs, err := a.repo.Organizations.List(ctx)
	if err != nil {
		return nil, err
	}
	if orgs == nil {
		orgs = []workspaces.Organization{}
	}
	return orgs, nil
}

// CreateOrg inserts an organization (sysadmin).
func (a *App) CreateOrg(ctx context.Context, name string, plan identity.Plan) (workspaces.Organization, error) {
	return a.repo.Organizations.Create(ctx, "", name, plan)
}

// SetOrgStatus suspends/restores an organization (sysadmin).
func (a *App) SetOrgStatus(ctx context.Context, id identity.ID, status identity.OrgStatus) (workspaces.Organization, error) {
	return a.repo.Organizations.SetStatus(ctx, id, status)
}

// ListOrgRequests returns pending create-mode requests (sysadmin).
func (a *App) ListOrgRequests(ctx context.Context) ([]identity.SignupRequest, error) {
	rows, err := a.repo.OrgRequests.ListPending(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]identity.SignupRequest, 0, len(rows))
	for _, o := range rows {
		sr := o.SignupRequest
		sr.WorkspaceName = o.OrganizationName
		out = append(out, sr)
	}
	if out == nil {
		out = []identity.SignupRequest{}
	}
	return out, nil
}

// ApproveOrgRequest approves a create-mode request: org → workspace →
// membership → status commit atomically in a UoW, then signup.approved and
// workspace.created are published.
func (a *App) ApproveOrgRequest(ctx context.Context, requestID identity.ID) error {
	var org workspaces.Organization
	var ws workspaces.Workspace
	var o domain.OrgRequest
	err := a.uow.Do(ctx, func(tx *Tx) error {
		var err error
		o, err = tx.OrgRequests.Get(ctx, requestID)
		if err != nil {
			return err
		}
		if o.Status != identity.SignupPending {
			return domain.ErrNotPending
		}
		name := o.OrganizationName
		if name == "" {
			name = o.Name + "'s Org"
		}
		org, err = tx.Organizations.Create(ctx, o.UserID, name, identity.PlanFree)
		if err != nil {
			return err
		}
		ws, err = tx.Workspaces.Create(ctx, org.ID, name+" Workspace", "", "main", "", "")
		if err != nil {
			return err
		}
		if _, err := tx.Members.Add(ctx, ws.ID, o.UserID, o.Name, o.Email, identity.RoleOwner); err != nil {
			return err
		}
		return tx.OrgRequests.SetStatus(ctx, requestID, identity.SignupApproved)
	})
	if err != nil {
		return err
	}
	a.pub.Publish(ctx, events.TopicSignupApproved, events.SignupApprovedData{
		RequestID: requestID, UserID: o.UserID, Email: o.Email, Name: o.Name,
		Mode: "create", WorkspaceID: ws.ID, OrganizationName: o.OrganizationName, Role: identity.RoleOwner,
	}, o.UserID)
	a.pub.Publish(ctx, events.TopicWorkspaceCreated, events.WorkspaceCreatedData{
		WorkspaceID: ws.ID, Name: ws.Name, RepoSource: ws.RepoSource, DefaultBranch: ws.DefaultBranch,
	}, ws.ID)
	return nil
}

// InternalWorkspaces returns the workspaces of a user (Gateway composition).
func (a *App) InternalWorkspaces(ctx context.Context, userID identity.ID) ([]workspaces.Workspace, error) {
	wss, err := a.repo.Workspaces.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if wss == nil {
		wss = []workspaces.Workspace{}
	}
	return wss, nil
}

// Stats returns cross-org KPIs for the Gateway composition.
func (a *App) Stats(ctx context.Context) (organizations, workspaces, openSeats int, err error) {
	return a.repo.Organizations.Stats(ctx)
}