// Package messaging adapts the Kafka bus to the Project application handlers
// (Publish/Subscribe entrypoint).
package messaging

import (
	"context"
	"log/slog"
	"strings"

	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/internal/platform/kafka"
	"github.com/aaks/server/services/project/internal/application"
)

// Consumer subscribes to workspace.created so a default repo binding (project)
// is established for every new workspace. Idempotent and best-effort: the
// binding handler tolerates duplicates (redelivery) and logs failures.
type Consumer struct {
	app *application.App
	log *slog.Logger
	// handlers registers one entry per subscribed event type; extending the
	// subscription means one map entry (plus its topic in Start), never
	// another branch in consume.
	handlers map[string]handler
}

// handler decodes and forwards one event envelope to the application.
type handler func(ctx context.Context, msg events.EventEnvelope) error

// New builds the messaging adapter.
func New(app *application.App, log *slog.Logger) *Consumer {
	return &Consumer{app: app, log: log, handlers: map[string]handler{
		events.TopicWorkspaceCreated: func(ctx context.Context, msg events.EventEnvelope) error {
			var d events.WorkspaceCreatedData
			if err := msg.DecodeData(&d); err != nil {
				return err
			}
			return app.BindWorkspace(ctx, d)
		},
	}}
}

// Start runs the consumer group on the lifecycle context until it is cancelled
// (graceful drain: in-flight messages finish before the group exits).
func (c *Consumer) Start(ctx context.Context, brokers string) {
	if brokers == "" {
		return
	}
	cg, err := kafka.NewConsumerGroup(kafka.Brokers(strings.Split(brokers, ",")), "project-workspaces", c.log)
	if err != nil {
		c.log.Warn("project consumers unavailable", "error", err)
		return
	}
	go func() {
		if err := cg.Run(ctx, []string{events.TopicWorkspaceCreated}, c.consume); err != nil {
			c.log.Error("project consumer stopped", "error", err)
		}
		_ = cg.Close()
	}()
}

func (c *Consumer) consume(ctx context.Context, msg events.EventEnvelope) error {
	h, ok := c.handlers[msg.EventType]
	if !ok {
		c.log.Warn("project consumer dropping unhandled event", "type", msg.EventType, "task_id", msg.TaskID)
		return nil
	}
	return h(ctx, msg)
}
