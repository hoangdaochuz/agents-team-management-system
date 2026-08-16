package messaging

import (
	"context"

	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/services/auth/internal/application"
)

// SignupApprovedHandler applies a signup-request approval (signup.approved).
type SignupApprovedHandler struct{ App *application.App }

func (h SignupApprovedHandler) Topics() []string { return []string{events.TopicSignupApproved} }

func (h SignupApprovedHandler) Handle(ctx context.Context, msg events.EventEnvelope) error {
	var d events.SignupApprovedData
	if err := msg.DecodeData(&d); err != nil {
		return err
	}
	return h.App.HandleSignupApproved(ctx, d)
}