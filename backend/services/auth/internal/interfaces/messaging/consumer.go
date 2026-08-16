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

// Consumer subscribes to signup.approved/declined (approval transitions) and
// invite.created (join-mode invite-code projection).
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

func (c *Consumer) consume(ctx context.Context, env events.EventEnvelope) error {
	switch env.EventType {
	case events.TopicSignupApproved:
		var d events.SignupApprovedData
		if err := env.DecodeData(&d); err != nil {
			return err
		}
		return c.app.HandleSignupApproved(ctx, d)
	case events.TopicSignupDeclined:
		var d events.SignupDeclinedData
		if err := env.DecodeData(&d); err != nil {
			return err
		}
		return c.app.HandleSignupDeclined(ctx, d)
	case events.TopicInviteCreated:
		var d events.InviteCreatedData
		if err := env.DecodeData(&d); err != nil {
			return err
		}
		return c.app.HandleInviteCreated(ctx, d)
	}
	return nil
}