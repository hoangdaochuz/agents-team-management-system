package messaging

import (
	"context"

	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/services/resources/internal/application"
)

// WorkspaceCreatedHandler seeds default rules on workspace creation
// (workspace.created). Idempotent: the unique rule index makes redelivery
// a no-op.
type WorkspaceCreatedHandler struct{ App *application.App }

func (h WorkspaceCreatedHandler) Topics() []string { return []string{events.TopicWorkspaceCreated} }

func (h WorkspaceCreatedHandler) Handle(ctx context.Context, msg events.EventEnvelope) error {
	return events.Forward(ctx, msg, h.App.BootstrapWorkspace)
}