// Package messaging adapts the Kafka bus to the Auth application handlers
// (Publish/Subscribe entrypoint).
package messaging

import (
	"context"
	"log/slog"
	"strings"

	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/internal/platform/kafka"
	"github.com/aaks/server/services/auth/internal/application"
)

// handler decodes and forwards one event envelope to the application.
type handler func(ctx context.Context, msg events.EventEnvelope) error

// Consumer subscribes to signup.approved/declined (approval transitions) and
// invite.created (join-mode invite-code projection).
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
		events.TopicSignupApproved: func(ctx context.Context, msg events.EventEnvelope) error {
			var d events.SignupApprovedData
			if err := msg.DecodeData(&d); err != nil {
				return err
			}
			return app.HandleSignupApproved(ctx, d)
		},
		events.TopicSignupDeclined: func(ctx context.Context, msg events.EventEnvelope) error {
			var d events.SignupDeclinedData
			if err := msg.DecodeData(&d); err != nil {
				return err
			}
			return app.HandleSignupDeclined(ctx, d)
		},
		events.TopicInviteCreated: func(ctx context.Context, msg events.EventEnvelope) error {
			var d events.InviteCreatedData
			if err := msg.DecodeData(&d); err != nil {
				return err
			}
			return app.HandleInviteCreated(ctx, d)
		},
	}}
}

// Start runs the consumer group on the lifecycle context until it is cancelled
// (graceful drain: in-flight messages finish before the group exits).
func (c *Consumer) Start(ctx context.Context, brokers string) {
	if brokers == "" {
		return
	}
	cg, err := kafka.NewConsumerGroup(kafka.Brokers(strings.Split(brokers, ",")), "auth-signup", c.log)
	if err != nil {
		c.log.Warn("auth consumers unavailable", "error", err)
		return
	}
	go func() {
		if err := cg.Run(ctx, []string{events.TopicSignupApproved, events.TopicSignupDeclined, events.TopicInviteCreated}, c.consume); err != nil {
			c.log.Error("auth consumer stopped", "error", err)
		}
		_ = cg.Close()
	}()
}

func (c *Consumer) consume(ctx context.Context, msg events.EventEnvelope) error {
	h, ok := c.handlers[msg.EventType]
	if !ok {
		c.log.Warn("auth consumer dropping unhandled event", "type", msg.EventType, "task_id", msg.TaskID)
		return nil
	}
	return h(ctx, msg)
}