package messaging

import (
	"context"

	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/services/auth/internal/application"
)

// SignupDeclinedHandler applies a signup-request decline (signup.declined).
type SignupDeclinedHandler struct{ App *application.App }

func (h SignupDeclinedHandler) Topics() []string { return []string{events.TopicSignupDeclined} }

func (h SignupDeclinedHandler) Handle(ctx context.Context, msg events.EventEnvelope) error {
	return events.Forward(ctx, msg, h.App.HandleSignupDeclined)
}