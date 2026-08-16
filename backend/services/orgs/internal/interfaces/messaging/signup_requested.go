package messaging

import (
	"context"

	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/services/orgs/internal/application"
)

// SignupRequestedHandler surfaces signup.requested so create-mode requests
// land in the sysadmin surface and join-mode requests under
// /workspaces/{id}/requests. Idempotent: the stores upsert on the request
// id, so redelivery is a no-op.
type SignupRequestedHandler struct{ App *application.App }

func (h SignupRequestedHandler) Topics() []string { return []string{events.TopicSignupRequested} }

func (h SignupRequestedHandler) Handle(ctx context.Context, msg events.EventEnvelope) error {
	var d events.SignupRequestedData
	if err := msg.DecodeData(&d); err != nil {
		return err
	}
	return h.App.ProjectSignupRequest(ctx, d)
}