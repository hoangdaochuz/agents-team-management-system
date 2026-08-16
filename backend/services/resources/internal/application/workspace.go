package application

import (
	"context"

	"github.com/aaks/server/internal/contracts/events"
)

// defaultRules is the rule set seeded for every new workspace. The unique
// (workspace_id, name) index makes re-delivery of workspace.created a no-op.
var defaultRules = []struct{ name, desc string }{
	{"no-auto-merge", "never auto-merge pull requests"},
	{"review-before-merge", "require reviewer approval before merging"},
	{"test-gate", "run tests before merging"},
}

// BootstrapWorkspace seeds the default rule set for a newly created workspace.
// Multi-write: all three rules are inserted atomically in one UoW.
func (a *App) BootstrapWorkspace(ctx context.Context, d events.WorkspaceCreatedData) error {
	return a.uow.Do(ctx, func(tx *Tx) error {
		for _, r := range defaultRules {
			if err := tx.Rules.Create(ctx, d.WorkspaceID, r.name, r.desc, true); err != nil {
				return err
			}
		}
		return nil
	})
}
