package domain

import (
	"context"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/resources"
)

// KnowledgeSource is the workspace knowledge-source aggregate (wire DTO as
// domain type, D7).
type KnowledgeSource = resources.KnowledgeSource

// KnowledgeRepository is the knowledge-source aggregate port.
type KnowledgeRepository interface {
	List(ctx context.Context, workspaceID identity.ID) ([]KnowledgeSource, error)
	Create(ctx context.Context, workspaceID identity.ID, title, kind string) (KnowledgeSource, error)
}