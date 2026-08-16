package application

import (
	"context"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/resources"
)

// ListRules returns the workspace's rules.
func (a *App) ListRules(ctx context.Context, workspaceID identity.ID) ([]resources.Rule, error) {
	out, err := a.repo.Rules.List(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = []resources.Rule{}
	}
	return out, nil
}

// SetRuleEnabled toggles a rule.
func (a *App) SetRuleEnabled(ctx context.Context, workspaceID, id identity.ID, enabled bool) (resources.Rule, error) {
	return a.repo.Rules.SetEnabled(ctx, workspaceID, id, enabled)
}

// EnabledRules returns the enforced (enabled) rules for the Runner's
// guardrails (internal surface).
func (a *App) EnabledRules(ctx context.Context, workspaceID identity.ID) ([]resources.Rule, error) {
	out, err := a.repo.Rules.Enabled(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = []resources.Rule{}
	}
	return out, nil
}
