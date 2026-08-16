package domain

import (
	"context"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/resources"
)

// Rule is the workspace guardrail-rule aggregate (wire DTO as domain type, D7).
type Rule = resources.Rule

// RuleRepository is the rule aggregate port.
type RuleRepository interface {
	List(ctx context.Context, workspaceID identity.ID) ([]Rule, error)
	// Create inserts a rule idempotently (unique per workspace+name).
	Create(ctx context.Context, workspaceID identity.ID, name, description string, enabled bool) error
	SetEnabled(ctx context.Context, workspaceID, id identity.ID, enabled bool) (Rule, error)
	// Enabled returns the enforced (enabled) rules for the Runner's guardrails.
	Enabled(ctx context.Context, workspaceID identity.ID) ([]Rule, error)
}