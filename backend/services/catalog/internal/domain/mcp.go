package domain

import (
	"context"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/resources"
)

// McpServer is the MCP server definition aggregate (wire DTO as domain type).
type McpServer = resources.McpServer

// McpCreate carries the fields of a new MCP server definition.
type McpCreate struct {
	Name    string
	Command string
	Args    []string
	Env     map[string]string
}

// McpUpdate carries the optional fields of an MCP definition patch; nil means
// "leave unchanged".
type McpUpdate struct {
	Name    *string
	Command *string
	Args    *[]string
	Env     *map[string]string
}

// McpRepository is the MCP server definition aggregate port.
type McpRepository interface {
	List(ctx context.Context, ws []identity.ID) ([]McpServer, error)
	Get(ctx context.Context, id identity.ID, ws []identity.ID) (McpServer, error)
	Create(ctx context.Context, workspaceID identity.ID, in McpCreate) (McpServer, error)
	Update(ctx context.Context, id identity.ID, ws []identity.ID, in McpUpdate) (McpServer, error)
	Delete(ctx context.Context, id identity.ID, ws []identity.ID) error
	ListByIDs(ctx context.Context, ids []identity.ID) ([]McpServer, error)
}