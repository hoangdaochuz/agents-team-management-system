package messaging

import (
	"context"

	"github.com/aaks/server/internal/contracts/events"
	taskapp "github.com/aaks/server/services/workspace/internal/application/task"
)

// DispatchHandler forwards every subscribed saga fact to the coordinator's
// application-level router (App.Dispatch), where the event switch is the
// state-machine definition rather than bus plumbing.
type DispatchHandler struct{ App *taskapp.App }

func (h DispatchHandler) Topics() []string {
	return []string{events.TopicRunCompleted, events.TopicVerdict, events.TopicPrOpened}
}

func (h DispatchHandler) Handle(ctx context.Context, msg events.EventEnvelope) error {
	return h.App.Dispatch(ctx, msg)
}
