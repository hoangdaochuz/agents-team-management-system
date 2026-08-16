package domain

import (
	"context"

	"github.com/aaks/server/internal/contracts/agentexec"
	"github.com/aaks/server/internal/contracts/identity"
)

// Step is the step aggregate (wire DTO as domain type, D7).
type Step = agentexec.Step

// StepRepository is the step aggregate port.
type StepRepository interface {
	AppendStep(ctx context.Context, runID identity.ID, seq int, kind agentexec.StepKind, payload []byte) (Step, error)
	ListSteps(ctx context.Context, runID identity.ID) ([]Step, error)
	ListStepsByTask(ctx context.Context, taskID identity.ID) ([]Step, error)
	MaxStepSeq(ctx context.Context, runID identity.ID) (int, error)
}