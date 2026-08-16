package domain

import (
	"context"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/resources"
)

// Plugin is the workspace plugin aggregate (wire DTO as domain type, D7).
type Plugin = resources.Plugin

// PluginRepository is the plugin aggregate port.
type PluginRepository interface {
	List(ctx context.Context, workspaceID identity.ID) ([]Plugin, error)
	SetEnabled(ctx context.Context, workspaceID, id identity.ID, enabled bool) (Plugin, error)
}
