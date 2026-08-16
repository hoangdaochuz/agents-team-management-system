// SSE stream orchestration: replay persisted steps from the Runner, then tail
// Kafka by task_id with per-connection consumer groups and step-id dedup.
package application

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/aaks/server/internal/contracts/agentexec"
	"github.com/aaks/server/internal/contracts/identity"
)

// StepsClient fetches a task's persisted steps from the Runner.
type StepsClient interface {
	Steps(ctx context.Context, taskID identity.ID) ([]agentexec.Step, error)
}

// StepTailer streams live steps for one task from Kafka. Each connection gets
// its own tailer (own consumer group) so concurrent viewers don't rebalance
// each other out of the partition.
type StepTailer interface {
	Run(ctx context.Context, onStep func(ctx context.Context, step agentexec.Step) error) error
	Close() error
}

// TailerFactory creates a per-connection tailer (the composition root wires
// the Kafka adapter; a fresh consumer group per call).
type TailerFactory func(taskID identity.ID) (StepTailer, error)

// SSEWriter receives the SSE events of one stream. The HTTP adapter flushes
// after every event.
type SSEWriter interface {
	Event(event string, data []byte)
}

// Stream replays a task's persisted steps, then tails live step events and
// merges them (dedup by step id), pinging every pingInterval.
type Stream struct {
	steps        StepsClient
	tailer       TailerFactory
	log          *slog.Logger
	pingInterval time.Duration
}

// NewStream builds the stream orchestrator with the injected clients.
func NewStream(steps StepsClient, tailer TailerFactory, log *slog.Logger) *Stream {
	return &Stream{steps: steps, tailer: tailer, log: log, pingInterval: 15 * time.Second}
}

// Serve streams the task's steps until ctx is cancelled or the tail ends.
// History is covered by the replay; the tail reads from the newest offset so
// no step is duplicated, and step ids seen in the replay are deduped again.
func (s *Stream) Serve(ctx context.Context, taskID identity.ID, w SSEWriter) error {
	// 1. Replay persisted steps from the Runner (all runs, seq order).
	var steps []agentexec.Step
	if fetched, err := s.steps.Steps(ctx, taskID); err == nil {
		steps = fetched
	}
	for _, st := range steps {
		w.Event("step", marshalStep(st))
	}

	// 2. Tail live step events for this task.
	tail, err := s.tailer(taskID)
	if err != nil {
		s.log.Warn("sse tail unavailable", "error", err)
		w.Event("error", []byte(`{"message":"live tail unavailable; replayed history only"}`))
		return nil
	}
	defer func() { _ = tail.Close() }()

	seen := map[string]bool{}
	for _, st := range steps {
		seen[string(st.ID)] = true
	}
	ticker := time.NewTicker(s.pingInterval)
	defer ticker.Stop()
	done := make(chan error, 1)
	go func() {
		err := tail.Run(ctx, func(_ context.Context, step agentexec.Step) error {
			if seen[string(step.ID)] {
				return nil
			}
			seen[string(step.ID)] = true
			w.Event("step", marshalStep(step))
			return nil
		})
		done <- err
	}()
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-done:
			if err != nil {
				s.log.Warn("sse tail ended with error", "error", err)
				w.Event("error", []byte(`{"message":"live tail unavailable"}`))
				return err
			}
			w.Event("error", []byte(`{"message":"stream ended"}`))
			return nil
		case <-ticker.C:
			w.Event("ping", []byte(`{}`))
		}
	}
}

func marshalStep(st agentexec.Step) []byte {
	buf, err := json.Marshal(st)
	if err != nil {
		return []byte(`{}`)
	}
	return buf
}
