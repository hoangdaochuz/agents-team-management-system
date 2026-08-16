package application

import (
	"context"
	"errors"

	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/workspaces"
	"github.com/aaks/server/services/orgs/internal/domain"
)

// ListWorkspaces returns the caller's workspaces with their role.
func (a *App) ListWorkspaces(ctx context.Context, userID identity.ID) ([]workspaces.Workspace, error) {
	wss, err := a.repo.Workspaces.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if wss == nil {
		wss = []workspaces.Workspace{}
	}
	return wss, nil
}

// GetWorkspace returns one workspace the caller belongs to.
func (a *App) GetWorkspace(ctx context.Context, userID, workspaceID identity.ID) (workspaces.Workspace, error) {
	return a.repo.Workspaces.GetByUser(ctx, userID, workspaceID)
}

// CreateWorkspace creates a workspace under the caller's organization (the
// organization is auto-created on first workspace), makes the caller owner,
// and emits workspace.created. Multi-aggregate: runs atomically in a UoW.
func (a *App) CreateWorkspace(ctx context.Context, userID identity.ID, name, repoSource, defaultBranch string) (workspaces.Workspace, error) {
	var ws workspaces.Workspace
	err := a.uow.Do(ctx, func(tx *Tx) error {
		org, err := tx.Organizations.ByUser(ctx, userID)
		if errors.Is(err, domain.ErrNoOrg) {
			org, err = tx.Organizations.Create(ctx, userID, "", identity.PlanFree)
		}
		if err != nil {
			return err
		}
		ws, err = tx.Workspaces.Create(ctx, org.ID, name, repoSource, defaultBranch, "", "")
		if err != nil {
			return err
		}
		if _, err := tx.Members.Add(ctx, ws.ID, userID, "", "", identity.RoleOwner); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return workspaces.Workspace{}, err
	}
	a.pub.Publish(ctx, events.TopicWorkspaceCreated, events.WorkspaceCreatedData{
		WorkspaceID: ws.ID, Name: ws.Name, RepoSource: ws.RepoSource, DefaultBranch: ws.DefaultBranch,
	}, ws.ID)
	ws.Role = identity.RoleOwner
	return ws, nil
}
