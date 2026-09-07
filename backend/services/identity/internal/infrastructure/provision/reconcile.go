package provision

import (
	"context"
	"log/slog"
	"time"

	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/internal/contracts/workspaces"
)

// defaultSweepInterval is how often the reconciler retries unconfirmed
// workspace provisioning. The direct call made at approval time is best-effort;
// this sweep is the retry leg the former workspace.created Kafka consumer used
// to provide. Only unprovisioned workspaces are swept (confirmed ones are
// marked and never re-POSTed), so steady-state cost is zero.
const defaultSweepInterval = time.Minute

// sweepBudget bounds one sweep so a slow or unreachable Workspace service
// cannot run sweeps into each other (a sweep that exceeds the budget is
// abandoned; unfinished workspaces stay unprovisioned and retry next tick).
const sweepBudget = 50 * time.Second

// Sweeper reconciles workspace provisioning on an interval: it lists every
// unprovisioned workspace, re-issues the (idempotent) provisioning call, and
// marks the workspace provisioned on success. Failures are logged and retried
// on the next sweep — nothing is marked on failure, nothing is lost.
type Sweeper struct {
	client *Client
	list   func(ctx context.Context) ([]workspaces.Workspace, error)
	mark   func(ctx context.Context, id string) error
	log    *slog.Logger
	every  time.Duration
}

// NewSweeper builds the reconciler. The list function is the workspace
// repository's ListUnprovisioned and mark is MarkProvisioned (injected to keep
// the sweeper testable and the provision package free of repository
// dependencies).
func NewSweeper(
	client *Client,
	list func(ctx context.Context) ([]workspaces.Workspace, error),
	mark func(ctx context.Context, id string) error,
	log *slog.Logger,
) *Sweeper {
	return &Sweeper{client: client, list: list, mark: mark, log: log, every: defaultSweepInterval}
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

// sweep provisions every unprovisioned workspace, marking each on success and
// logging a summary so the sweep's cost and progress are observable.
func (s *Sweeper) sweep(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, sweepBudget)
	defer cancel()
	wss, err := s.list(ctx)
	if err != nil {
		s.log.Error("provisioning sweep: list unprovisioned workspaces failed", "error", err)
		return
	}
	ok, failed := 0, 0
	for _, w := range wss {
		if ctx.Err() != nil {
			break // budget exhausted; the rest retry next tick
		}
		d := events.WorkspaceCreatedData{
			WorkspaceID: w.ID, Name: w.Name, RepoSource: w.RepoSource, DefaultBranch: w.DefaultBranch,
		}
		if err := s.client.Provision(ctx, d); err != nil {
			failed++
			s.log.Error("provisioning sweep: workspace still unprovisioned (will retry)",
				"workspace_id", w.ID, "error", err)
			continue
		}
		if err := s.mark(ctx, string(w.ID)); err != nil {
			// Provisioned but unmarked: retried next sweep against an
			// idempotent endpoint — harmless, so warn rather than error.
			s.log.Warn("provisioning sweep: marking workspace provisioned failed (will re-sweep)",
				"workspace_id", w.ID, "error", err)
		}
		ok++
	}
	if ok+failed > 0 {
		s.log.Info("provisioning sweep complete", "provisioned", ok, "failed", failed)
	}
}
