// Package messaging adapts the Kafka bus to the Resources application handlers
// (Publish/Subscribe entrypoint).
package messaging

import (
	"context"
	"log/slog"
	"strings"

	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/internal/platform/kafka"
	"github.com/aaks/server/services/resources/internal/application"
)

// Consumer subscribes to catalog MCP definition events (projected into
// connection rows) and to workspace.created (default rule seed). Idempotent:
// the connection upsert and the unique rule index make redelivery a no-op.
type Consumer struct {
	app *application.App
	log *slog.Logger
}

// New builds the messaging adapter.
func New(app *application.App, log *slog.Logger) *Consumer { return &Consumer{app: app, log: log} }

// Start runs the consumer group on the lifecycle context until it is cancelled
// (graceful drain: in-flight messages finish before the group exits).
func (c *Consumer) Start(ctx context.Context, brokers string) {
	if brokers == "" {
		return
	}
	cg, err := kafka.NewConsumerGroup(kafka.Brokers(strings.Split(brokers, ",")), "resources-mcp", c.log)
	if err != nil {
		c.log.Warn("resources consumers unavailable", "error", err)
		return
	}
	go func() {
		if err := cg.Run(ctx, []string{events.TopicMcpCreated, events.TopicMcpDeleted, events.TopicWorkspaceCreated}, c.consume); err != nil {
			c.log.Error("resources consumer stopped", "error", err)
		}
		_ = cg.Close()
	}()
}

// consume projects MCP connections and seeds default rules on workspace
// creation.
func (c *Consumer) consume(ctx context.Context, env events.EventEnvelope) error {
	switch env.EventType {
	case events.TopicMcpCreated:
		var d events.McpCreatedData
		if err := env.DecodeData(&d); err != nil {
			return err
		}
		return c.app.ProjectMcpCreated(ctx, d)
	case events.TopicMcpDeleted:
		var d events.McpDeletedData
		if err := env.DecodeData(&d); err != nil {
			return err
		}
		return c.app.ProjectMcpDeleted(ctx, d)
	case events.TopicWorkspaceCreated:
		var d events.WorkspaceCreatedData
		if err := env.DecodeData(&d); err != nil {
			return err
		}
		return c.app.BootstrapWorkspace(ctx, d)
	}
	return nil
}