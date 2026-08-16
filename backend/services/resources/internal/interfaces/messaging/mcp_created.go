package messaging

import (
	"context"

	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/services/resources/internal/application"
)

// McpCreatedHandler projects a catalog MCP definition into a connection row
// (mcp.created). Idempotent: the connection upsert makes redelivery a no-op.
type McpCreatedHandler struct{ App *application.App }

func (h McpCreatedHandler) Topics() []string { return []string{events.TopicMcpCreated} }

func (h McpCreatedHandler) Handle(ctx context.Context, msg events.EventEnvelope) error {
	return events.Forward(ctx, msg, h.App.ProjectMcpCreated)
}