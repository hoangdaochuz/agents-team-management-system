package messaging

import (
	"context"

	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/services/runner/internal/application"
)

// DispatchHandler forwards every subscribed command envelope to the
// application-level router (Runner.Dispatch), where the event switch is the
// command router rather than bus plumbing.
type DispatchHandler struct{ App *application.Runner }

func (h DispatchHandler) Topics() []string {
	return []string{
		events.TopicTaskRunRequested,
		events.TopicTaskReviewRequested,
		events.TopicTaskStopRequested,
		events.TopicPrOpenRequested,
	}
}

func (h DispatchHandler) Handle(ctx context.Context, msg events.EventEnvelope) error {
	return h.App.Dispatch(ctx, msg)
}