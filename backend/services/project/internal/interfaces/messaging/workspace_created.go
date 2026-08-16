package messaging

import (
	"context"

	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/services/project/internal/application"
)

// WorkspaceCreatedHandler establishes a default repo binding (project) for
// every new workspace (workspace.created). Idempotent and best-effort: the
// binding handler tolerates duplicates (redelivery) and logs failures.
type WorkspaceCreatedHandler struct{ App *application.App }

func (h WorkspaceCreatedHandler) Topics() []string { return []string{events.TopicWorkspaceCreated} }

func (h WorkspaceCreatedHandler) Handle(ctx context.Context, msg events.EventEnvelope) error {
	var d events.WorkspaceCreatedData
	if err := msg.DecodeData(&d); err != nil {
		return err
	}
	return h.App.BindWorkspace(ctx, d)
}