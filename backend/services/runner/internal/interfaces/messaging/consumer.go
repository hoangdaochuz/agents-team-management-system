// Package messaging adapts the Kafka command topics to the Runner application
// dispatcher (Publish/Subscribe entrypoint).
package messaging

import (
	"context"
	"log/slog"
	"strings"

	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/internal/platform/kafka"
	"github.com/aaks/server/services/runner/internal/application"
)

// Consumer subscribes to the runner command topics.
type Consumer struct {
	app *application.Runner
	log *slog.Logger
}

// New builds the messaging adapter.
func New(app *application.Runner, log *slog.Logger) *Consumer { return &Consumer{app: app, log: log} }

// Start runs the consumer group on the lifecycle context until it is cancelled
// (graceful drain: in-flight messages finish before the group exits).
func (c *Consumer) Start(ctx context.Context, brokers string) {
	if brokers == "" {
		return
	}
	cg, err := kafka.NewConsumerGroup(kafka.Brokers(strings.Split(brokers, ",")), "runner-commands", c.log)
	if err != nil {
		c.log.Warn("runner consumers unavailable", "error", err)
		return
	}
	go func() {
		if err := cg.Run(ctx, []string{
			events.TopicTaskRunRequested, events.TopicTaskReviewRequested,
			events.TopicTaskStopRequested, events.TopicPrOpenRequested,
		}, c.consume); err != nil {
			c.log.Error("runner consumer stopped", "error", err)
		}
		_ = cg.Close()
	}()
}

func (c *Consumer) consume(ctx context.Context, env events.EventEnvelope) error {
	return c.app.Dispatch(ctx, env)
}