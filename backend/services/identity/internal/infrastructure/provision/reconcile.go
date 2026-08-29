package provision

import (
	"context"
	"log/slog"
	"time"

	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/internal/contracts/workspaces"
)

// defaultSweepInterval is how often the reconciler re-POSTs every workspace to
// the Workspace service's (idempotent) provisioning endpoint. The direct call
// made at approval time is best-effort; this sweep is the retry leg the former
// workspace.created Kafka consumer used to provide. A short interval is fine
// at single-operator scale — the workspace count is small and each POST is a
// no-op once provisioned.
const defaultSweepInterval = time.Minute

// Sweeper reconciles workspace provisioning on an interval: it lists every
// workspace and re-issues the provisioning call, relying on the endpoint's
// idempotency (repo binding + rule seeding upsert) so already-provisioned
// workspaces are unaffected. Failures are logged and retried on the next
// sweep — nothing is marked, nothing is lost.
type Sweeper struct {
	client *Client
	list   func(ctx context.Context) ([]workspaces.Workspace, error)
	log    *slog.Logger
	every  time.Duration
}

// NewSweeper builds the reconciler. The list function is the workspace
// repository's List (injected to keep the sweeper testable and the provision
// package free of repository dependencies).
func NewSweeper(client *Client, list func(ctx context.Context) ([]workspaces.Workspace, error), log *slog.Logger) *Sweeper {
	return &Sweeper{client: client, list: list, log: log, every: defaultSweepInterval}
}

// Run sweeps immediately (catching anything missed while this process was
// down) and then on every tick until ctx is cancelled.
func (s *Sweeper) Run(ctx context.Context) {
	s.sweep(ctx)
	t := time.NewTicker(s.every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.sweep(ctx)
		}
	}
}

// sweep provisions every known workspace, logging per-workspace failures.
func (s *Sweeper) sweep(ctx context.Context) {
	wss, err := s.list(ctx)
	if err != nil {
		s.log.Error("provisioning sweep: list workspaces failed", "error", err)
		return
	}
	for _, w := range wss {
		d := events.WorkspaceCreatedData{
			WorkspaceID: w.ID, Name: w.Name, RepoSource: w.RepoSource, DefaultBranch: w.DefaultBranch,
		}
		if err := s.client.Provision(ctx, d); err != nil {
			s.log.Error("provisioning sweep: workspace still unprovisioned (will retry)",
				"workspace_id", w.ID, "error", err)
		}
	}
}
