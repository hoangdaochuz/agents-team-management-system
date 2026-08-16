package messaging

import (
	"context"

	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/services/resources/internal/application"
)

// McpDeletedHandler removes the projected connection row (mcp.deleted).
// Idempotent: deleting a missing row is a no-op.
type McpDeletedHandler struct{ App *application.App }

func (h McpDeletedHandler) Topics() []string { return []string{events.TopicMcpDeleted} }

func (h McpDeletedHandler) Handle(ctx context.Context, msg events.EventEnvelope) error {
	var d events.McpDeletedData
	if err := msg.DecodeData(&d); err != nil {
		return err
	}
	return h.App.ProjectMcpDeleted(ctx, d)
}