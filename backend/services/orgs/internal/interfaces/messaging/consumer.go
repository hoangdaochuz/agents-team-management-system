// Package messaging adapts the Kafka bus to the Orgs application handlers
// (Publish/Subscribe entrypoint).
package messaging

import (
	"context"
	"log/slog"
	"strings"

	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/internal/platform/kafka"
	"github.com/aaks/server/services/orgs/internal/application"
)

// Consumer subscribes to signup.requested so create-mode requests surface in
// the sysadmin surface and join-mode requests under /workspaces/{id}/requests.
// Idempotent: the stores upsert on the request id, so redelivery is a no-op.
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
	cg, err := kafka.NewConsumerGroup(kafka.Brokers(strings.Split(brokers, ",")), "orgs-signups", c.log)
	if err != nil {
		c.log.Warn("orgs consumers unavailable", "error", err)
		return
	}
	go func() {
		if err := cg.Run(ctx, []string{events.TopicSignupRequested}, c.consume); err != nil {
			c.log.Error("orgs consumer stopped", "error", err)
		}
		_ = cg.Close()
	}()
}

func (c *Consumer) consume(ctx context.Context, msg events.EventEnvelope) error {
	var d events.SignupRequestedData
	if err := msg.DecodeData(&d); err != nil {
		return err
	}
	return c.app.ProjectSignupRequest(ctx, d)
}
