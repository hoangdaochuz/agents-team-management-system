package application

import (
	"context"

	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/tasks"
	"github.com/aaks/server/services/project/internal/domain"
)

// List returns the projects in the caller's workspace set, newest first. An
// empty workspace set yields an empty list (fail closed).
func (a *App) List(ctx context.Context, ws []identity.ID) ([]tasks.Project, error) {
	ps, err := a.repo.Projects.List(ctx, ws)
	if err != nil {
		return nil, err
	}
	if ps == nil {
		ps = []tasks.Project{}
	}
	return ps, nil
}

// Get returns one project, scoped to the caller's workspace set.
func (a *App) Get(ctx context.Context, id identity.ID, ws []identity.ID) (tasks.Project, error) {
	return a.repo.Projects.Get(ctx, id, ws)
}

// Create registers a project in the given workspace.
func (a *App) Create(ctx context.Context, workspaceID identity.ID, in domain.CreateInput) (tasks.Project, error) {
	return a.repo.Projects.Create(ctx, workspaceID, in)
}

// Update partially updates a project, scoped to the caller's workspace set.
func (a *App) Update(ctx context.Context, id identity.ID, ws []identity.ID, in domain.UpdateInput) (tasks.Project, error) {
	return a.repo.Projects.Update(ctx, id, ws, in)
}

// Delete removes a project, scoped to the caller's workspace set.
func (a *App) Delete(ctx context.Context, id identity.ID, ws []identity.ID) error {
	return a.repo.Projects.Delete(ctx, id, ws)
}

// BindWorkspace bootstraps a default repo binding (project) for a newly created
// workspace (workspace.created). Best-effort and idempotent: a duplicate on
// redelivery or any other failure is logged, not fatal — the user can always
// create projects manually.
func (a *App) BindWorkspace(ctx context.Context, d events.WorkspaceCreatedData) error {
	if d.RepoSource == "" {
		return nil // no repo to bind; the user creates projects manually
	}
	name := d.Name
	if name == "" {
		name = "default"
	}
	if _, err := a.repo.Projects.Create(ctx, d.WorkspaceID, domain.CreateInput{
		Name: name, RepoSource: d.RepoSource, RepoType: identity.RepoType("git"), DefaultBranch: d.DefaultBranch,
	}); err != nil {
		// Duplicate project name per workspace is possible on redelivery; the
		// binding is best-effort so a failure is logged, not fatal.
		a.log.Warn("workspace repo binding failed", "workspace", d.WorkspaceID, "error", err)
	}
	return nil
}
