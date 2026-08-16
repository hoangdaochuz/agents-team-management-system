// Package messaging adapts the Kafka bus to the Admin application handlers
// (Publish/Subscribe entrypoint).
package messaging

import (
	"context"
	"log/slog"
	"strings"

	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/internal/platform/kafka"
	"github.com/aaks/server/services/admin/internal/application"
)

// Consumer subscribes to audit.recorded so workspace admin actions emitted by
// other services land in the audit log. Idempotent: every event appends a fresh
// row, and at-least-once redelivery is tolerated by the append-only design.
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
	cg, err := kafka.NewConsumerGroup(kafka.Brokers(strings.Split(brokers, ",")), "admin-audit", c.log)
	if err != nil {
		c.log.Warn("admin consumers unavailable", "error", err)
		return
	}
	go func() {
		if err := cg.Run(ctx, []string{events.TopicAuditRecorded}, c.consume); err != nil {
			c.log.Error("admin consumer stopped", "error", err)
		}
		_ = cg.Close()
	}()
}

func (c *Consumer) consume(ctx context.Context, msg events.EventEnvelope) error {
	if msg.EventType != events.TopicAuditRecorded {
		return nil
	}
	var d events.AuditRecordedData
	if err := msg.DecodeData(&d); err != nil {
		return err
	}
	return c.app.RecordAudit(ctx, d)
}
