package domain

import (
	"context"

	"github.com/aaks/server/internal/contracts/agentexec"
	"github.com/aaks/server/internal/contracts/identity"
)

// Run is the run aggregate (wire DTO as domain type, D7).
type Run = agentexec.Run

// RunRepository is the run aggregate port.
type RunRepository interface {
	CreateRun(ctx context.Context, taskID identity.ID, role agentexec.RunRole, agentID identity.ID, model string, roundNo int) (identity.ID, error)
	ListRunsByTask(ctx context.Context, taskID identity.ID) ([]Run, error)
	GetRun(ctx context.Context, runID identity.ID) (Run, error)
	LatestRun(ctx context.Context, taskID identity.ID) (Run, error)
	FinishRun(ctx context.Context, runID identity.ID, status agentexec.RunStatus, tokenUsage int, errMsg string) error
}