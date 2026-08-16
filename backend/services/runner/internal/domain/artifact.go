package domain

import (
	"context"

	"github.com/aaks/server/internal/contracts/agentexec"
	"github.com/aaks/server/internal/contracts/identity"
)

// Artifact is the artifact aggregate (wire DTO as domain type, D7).
type Artifact = agentexec.Artifact

// ArtifactRepository is the artifact aggregate port.
type ArtifactRepository interface {
	AddArtifact(ctx context.Context, taskID identity.ID, runID *identity.ID, filename, kind, summary string, additions, deletions int) (Artifact, error)
	ListArtifactsByTask(ctx context.Context, taskID identity.ID) ([]Artifact, error)
}