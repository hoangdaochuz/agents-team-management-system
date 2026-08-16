package application

import (
	"context"

	"github.com/aaks/server/internal/contracts/admin"
)

// ListFlags returns all feature flags.
func (a *App) ListFlags(ctx context.Context) ([]admin.FeatureFlag, error) {
	out, err := a.repo.Flags.List(ctx)
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = []admin.FeatureFlag{}
	}
	return out, nil
}

// SetFlagEnabled toggles a feature flag.
func (a *App) SetFlagEnabled(ctx context.Context, key string, enabled bool) (admin.FeatureFlag, error) {
	return a.repo.Flags.SetEnabled(ctx, key, enabled)
}
