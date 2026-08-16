package domain

import (
	"context"

	"github.com/aaks/server/internal/contracts/agentexec"
	"github.com/aaks/server/internal/contracts/identity"
)

// Finding is the finding aggregate (wire DTO as domain type, D7).
type Finding = agentexec.Finding

// FindingRepository is the finding aggregate port.
type FindingRepository interface {
	AddFinding(ctx context.Context, runID identity.ID, f Finding) (Finding, error)
	ListFindings(ctx context.Context, runID identity.ID) ([]Finding, error)
}