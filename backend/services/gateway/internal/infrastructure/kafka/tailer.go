// Package kafka adapts the platform Kafka consumer group into the Gateway's
// per-connection step tailer for the SSE stream. Each connection gets its own
// consumer group (gateway-sse-<taskID>-<connID>) reading from the newest
// offset, since history is covered by the Runner replay.
package kafka

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"

	"github.com/aaks/server/internal/contracts/agentexec"
	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/platform/kafka"
)

// StepTailer tails the step topic for one task and delivers matching steps to
// the application's handler.
type StepTailer struct {
	cg     *kafka.ConsumerGroup
	taskID identity.ID
}

// NewStepTailer creates a tailer for taskID with a fresh consumer group.
func NewStepTailer(brokers []string, taskID identity.ID, log *slog.Logger) (*StepTailer, error) {
	cg, err := kafka.NewConsumerGroupFrom(kafka.Brokers(brokers), "gateway-sse-"+string(taskID)+"-"+connID(), true, log)
	if err != nil {
		return nil, err
	}
	return &StepTailer{cg: cg, taskID: taskID}, nil
}

// Run delivers the task's step events to onStep until ctx is cancelled.
// Envelopes for other tasks and undecodable payloads are skipped silently
// (the stream is best-effort).
func (t *StepTailer) Run(ctx context.Context, onStep func(ctx context.Context, step agentexec.Step) error) error {
	return t.cg.Run(ctx, []string{events.TopicStep}, func(ctx context.Context, env events.EventEnvelope) error {
		if env.TaskID != t.taskID {
			return nil
		}
		var d events.StepData
		if err := env.DecodeData(&d); err != nil {
			return nil
		}
		return onStep(ctx, d.Step)
	})
}

// Close releases the consumer group.
func (t *StepTailer) Close() error { return t.cg.Close() }

// connID returns a random short identifier for a single SSE connection, used
// to give each connection its own consumer group.
func connID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}