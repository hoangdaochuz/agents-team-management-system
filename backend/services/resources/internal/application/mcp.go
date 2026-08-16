package application

import (
	"context"

	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/resources"
)

// ListMcpConnections returns the workspace's MCP connections.
func (a *App) ListMcpConnections(ctx context.Context, workspaceID identity.ID) ([]resources.McpConnection, error) {
	out, err := a.repo.Mcp.List(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = []resources.McpConnection{}
	}
	return out, nil
}

// ReconnectMcpConnection marks a connection online (tool discovery is the
// runner's job at run time).
func (a *App) ReconnectMcpConnection(ctx context.Context, workspaceID, id identity.ID) (resources.McpConnection, error) {
	return a.repo.Mcp.Reconnect(ctx, workspaceID, id)
}

// ProjectMcpCreated projects a catalog mcp.created event into a connection row
// (idempotent: upsert on the server id).
func (a *App) ProjectMcpCreated(ctx context.Context, d events.McpCreatedData) error {
	return a.repo.Mcp.Upsert(ctx, d.McpServerID, d.WorkspaceID, d.Name)
}

// ProjectMcpDeleted projects a catalog mcp.deleted event (removes the
// connection rows for the server).
func (a *App) ProjectMcpDeleted(ctx context.Context, d events.McpDeletedData) error {
	return a.repo.Mcp.Delete(ctx, d.WorkspaceID, d.McpServerID)
}
