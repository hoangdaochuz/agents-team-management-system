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

// handler decodes and forwards one event envelope to the application.
type handler func(ctx context.Context, msg events.EventEnvelope) error

// Consumer subscribes to catalog MCP definition events (projected into
// connection rows) and to workspace.created (default rule seed). Idempotent:
// the connection upsert and the unique rule index make redelivery a no-op.
type Consumer struct {
	app *application.App
	log *slog.Logger
	// handlers registers one entry per subscribed event type; extending the
	// subscription means one map entry (plus its topic in Start), never
	// another branch in consume.
	handlers map[string]handler
}

// New builds the messaging adapter.
func New(app *application.App, log *slog.Logger) *Consumer {
	return &Consumer{app: app, log: log, handlers: map[string]handler{
		events.TopicMcpCreated: func(ctx context.Context, msg events.EventEnvelope) error {
			var d events.McpCreatedData
			if err := msg.DecodeData(&d); err != nil {
				return err
			}
			return app.ProjectMcpCreated(ctx, d)
		},
		events.TopicMcpDeleted: func(ctx context.Context, msg events.EventEnvelope) error {
			var d events.McpDeletedData
			if err := msg.DecodeData(&d); err != nil {
				return err
			}
			return app.ProjectMcpDeleted(ctx, d)
		},
		events.TopicWorkspaceCreated: func(ctx context.Context, msg events.EventEnvelope) error {
			var d events.WorkspaceCreatedData
			if err := msg.DecodeData(&d); err != nil {
				return err
			}
			return app.BootstrapWorkspace(ctx, d)
		},
	}}
}

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
func (c *Consumer) consume(ctx context.Context, msg events.EventEnvelope) error {
	h, ok := c.handlers[msg.EventType]
	if !ok {
		c.log.Warn("resources consumer dropping unhandled event", "type", msg.EventType, "task_id", msg.TaskID)
		return nil
	}
	return h(ctx, msg)
}