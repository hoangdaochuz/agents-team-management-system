package domain

import (
	"context"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/resources"
)

// McpConnection is the workspace MCP connection projection (wire DTO as domain
// type, D7). Connection rows are projected from catalog mcp.created/deleted
// events; the definition itself lives in the Catalog service.
type McpConnection = resources.McpConnection

// McpConnectionRepository is the MCP connection projection port.
type McpConnectionRepository interface {
	List(ctx context.Context, workspaceID identity.ID) ([]McpConnection, error)
	// Upsert projects a catalog mcp.created event.
	Upsert(ctx context.Context, mcpID, workspaceID identity.ID, name string) error
	// Delete projects a catalog mcp.deleted event.
	Delete(ctx context.Context, workspaceID, mcpID identity.ID) error
	// Reconnect marks a connection online (tool discovery is the runner's job).
	Reconnect(ctx context.Context, workspaceID, id identity.ID) (McpConnection, error)
}
