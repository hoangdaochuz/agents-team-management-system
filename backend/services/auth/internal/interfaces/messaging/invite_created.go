package messaging

import (
	"context"

	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/services/auth/internal/application"
)

// InviteCreatedHandler projects invite.created into the join-mode
// invite-code registry.
type InviteCreatedHandler struct{ App *application.App }

func (h InviteCreatedHandler) Topics() []string { return []string{events.TopicInviteCreated} }

func (h InviteCreatedHandler) Handle(ctx context.Context, msg events.EventEnvelope) error {
	var d events.InviteCreatedData
	if err := msg.DecodeData(&d); err != nil {
		return err
	}
	return h.App.HandleInviteCreated(ctx, d)
}