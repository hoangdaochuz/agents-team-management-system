package application

import (
	"context"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/resources"
)

// ListPlugins returns the workspace's plugins.
func (a *App) ListPlugins(ctx context.Context, workspaceID identity.ID) ([]resources.Plugin, error) {
	out, err := a.repo.Plugins.List(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = []resources.Plugin{}
	}
	return out, nil
}

// SetPluginEnabled toggles a plugin.
func (a *App) SetPluginEnabled(ctx context.Context, workspaceID, id identity.ID, enabled bool) (resources.Plugin, error) {
	return a.repo.Plugins.SetEnabled(ctx, workspaceID, id, enabled)
}
