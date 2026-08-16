package application

import (
	"context"

	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/resources"
	"github.com/aaks/server/services/catalog/internal/domain"
)

// ListMcp returns the MCP server definitions visible in the caller's workspace
// set, newest first (empty set yields an empty list — fail closed).
func (a *App) ListMcp(ctx context.Context, ws []identity.ID) ([]resources.McpServer, error) {
	out, err := a.repo.Mcps.List(ctx, ws)
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = []resources.McpServer{}
	}
	return out, nil
}

// GetMcp returns one MCP server definition scoped to the caller's workspace set.
func (a *App) GetMcp(ctx context.Context, id identity.ID, ws []identity.ID) (resources.McpServer, error) {
	return a.repo.Mcps.Get(ctx, id, ws)
}

// CreateMcp inserts a definition and publishes mcp.created after the commit.
func (a *App) CreateMcp(ctx context.Context, workspaceID identity.ID, in domain.McpCreate) (resources.McpServer, error) {
	var out resources.McpServer
	err := a.uow.Do(ctx, func(tx *Tx) error {
		var err error
		out, err = tx.Mcps.Create(ctx, workspaceID, in)
		return err
	})
	if err != nil {
		return resources.McpServer{}, err
	}
	a.pub.Publish(ctx, events.TopicMcpCreated, events.McpCreatedData{
		McpServerID: out.ID, WorkspaceID: out.WorkspaceID,
		Name: out.Name, Command: out.Command, Args: out.Args, Env: out.Env,
	}, out.WorkspaceID)
	return out, nil
}

// UpdateMcp patches a definition scoped to the caller's workspace set.
func (a *App) UpdateMcp(ctx context.Context, id identity.ID, ws []identity.ID, in domain.McpUpdate) (resources.McpServer, error) {
	return a.repo.Mcps.Update(ctx, id, ws, in)
}

// DeleteMcp removes a definition and publishes mcp.deleted after the commit.
func (a *App) DeleteMcp(ctx context.Context, id identity.ID, ws []identity.ID) error {
	err := a.uow.Do(ctx, func(tx *Tx) error {
		return tx.Mcps.Delete(ctx, id, ws)
	})
	if err != nil {
		return err
	}
	a.pub.Publish(ctx, events.TopicMcpDeleted, events.McpDeletedData{McpServerID: id}, "")
	return nil
}

// ListMcpByIDs returns definitions for the given IDs (internal trusted
// callers — the Agent service hydrating an agent's attached servers).
func (a *App) ListMcpByIDs(ctx context.Context, ids []identity.ID) ([]resources.McpServer, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	return a.repo.Mcps.ListByIDs(ctx, ids)
}