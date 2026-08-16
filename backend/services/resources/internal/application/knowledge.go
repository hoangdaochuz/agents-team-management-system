package application

import (
	"context"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/resources"
)

// ListKnowledge returns the workspace's knowledge sources, newest first.
func (a *App) ListKnowledge(ctx context.Context, workspaceID identity.ID) ([]resources.KnowledgeSource, error) {
	out, err := a.repo.Knowledge.List(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = []resources.KnowledgeSource{}
	}
	return out, nil
}

// CreateKnowledge inserts a knowledge source (async indexing status pending;
// the runner/indexer flips status later).
func (a *App) CreateKnowledge(ctx context.Context, workspaceID identity.ID, title, kind string) (resources.KnowledgeSource, error) {
	return a.repo.Knowledge.Create(ctx, workspaceID, title, kind)
}
