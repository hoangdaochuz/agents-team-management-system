// Package messaging adapts the Kafka bus to the Task application saga
// coordinator (Publish/Subscribe entrypoint).
package messaging

import (
	"context"
	"log/slog"
	"strings"

	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/internal/platform/kafka"
	"github.com/aaks/server/services/task/internal/application"
)

// Consumer subscribes the saga coordinator to execution facts. Idempotency:
// each (task_id, run_id) is claimed exactly once via the saga_runs table, so
// at-least-once redelivery cannot double-transition a task.
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
	cg, err := kafka.NewConsumerGroup(kafka.Brokers(strings.Split(brokers, ",")), "task-saga", c.log)
	if err != nil {
		c.log.Warn("task saga consumers unavailable", "error", err)
		return
	}
	go func() {
		if err := cg.Run(ctx, []string{
			events.TopicRunCompleted, events.TopicVerdict, events.TopicPrOpened,
		}, c.consume); err != nil {
			c.log.Error("task saga consumer stopped", "error", err)
		}
		_ = cg.Close()
	}()
}

func (c *Consumer) consume(ctx context.Context, env events.EventEnvelope) error {
	return c.app.Dispatch(ctx, env)
}