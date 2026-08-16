// Package messaging adapts the Kafka bus to the Orgs application handlers
// (Publish/Subscribe entrypoint).
package messaging

import (
	"context"
	"log/slog"
	"strings"

	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/internal/platform/kafka"
)

// Handler reacts to the event types it declares via Topics. Each topic's
// reactions live in one dedicated implementation (its own file), so reacting
// to a new topic is a new Handler — never a change to the consumer plumbing.
type Handler interface {
	// Topics lists the event types this handler reacts to.
	Topics() []string
	// Handle decodes msg and forwards it to the application.
	Handle(ctx context.Context, msg events.EventEnvelope) error
}

// Consumer runs one consumer group whose subscription is the union of the
// registered handlers' topics and dispatches each envelope to the handler
// registered for its event type (unknown types are logged and dropped).
type Consumer struct {
	log      *slog.Logger
	topics   []string
	handlers map[string]Handler
}

// New builds the messaging adapter from the given handlers. Subscribing to
// a new topic is passing one more handler here; the plumbing never changes.
func New(log *slog.Logger, handlers ...Handler) *Consumer {
	reg := make(map[string]Handler, len(handlers))
	seen := make(map[string]bool, len(handlers))
	topics := make([]string, 0, len(handlers))
	for _, h := range handlers {
		for _, t := range h.Topics() {
			if seen[t] {
				log.Warn("dropping duplicate handler for topic", "topic", t)
				continue
			}
			seen[t] = true
			reg[t] = h
			topics = append(topics, t)
		}
	}
	return &Consumer{log: log, topics: topics, handlers: reg}
}

// Start runs the consumer group on the lifecycle context until it is cancelled
// (graceful drain: in-flight messages finish before the group exits).
func (c *Consumer) Start(ctx context.Context, brokers string) {
	if brokers == "" {
		return
	}
	cg, err := kafka.NewConsumerGroup(kafka.Brokers(strings.Split(brokers, ",")), "orgs-signups", c.log)
	if err != nil {
		c.log.Warn("orgs consumers unavailable", "error", err)
		return
	}
	go func() {
		if err := cg.Run(ctx, c.topics, c.consume); err != nil {
			c.log.Error("orgs consumer stopped", "error", err)
		}
		_ = cg.Close()
	}()
}

func (c *Consumer) consume(ctx context.Context, msg events.EventEnvelope) error {
	h, ok := c.handlers[msg.EventType]
	if !ok {
		c.log.Warn("orgs consumer dropping unhandled event", "type", msg.EventType, "task_id", msg.TaskID)
		return nil
	}
	return h.Handle(ctx, msg)
}