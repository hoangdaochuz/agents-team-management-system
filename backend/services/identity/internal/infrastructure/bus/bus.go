// Package bus adapts the application EventPublisher port for the consolidated
// Identity service. Events that used to round-trip through Kafka between the
// auth/orgs/admin services now dispatch synchronously to in-process handlers
// (the in-process event bus). The Identity service emits nothing to Kafka:
// the one former cross-service event, workspace.created, is a direct HTTP
// provisioning call to the Workspace service (see infrastructure/provision),
// with a periodic reconcile sweep as its retry leg.
package bus

import (
	"context"
	"log/slog"

	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/internal/contracts/identity"
)

// HandlerFunc reacts to one in-process event. Handlers receive the envelope
// exactly as the Kafka consumers did, so events.Forward-based decoding works
// unchanged.
type HandlerFunc func(ctx context.Context, msg events.EventEnvelope) error

// InProc is the in-process event bus: a topic → handlers dispatch table.
type InProc struct {
	log      *slog.Logger
	handlers map[string][]HandlerFunc
}

// NewInProc builds the bus from the handler routes.
func NewInProc(log *slog.Logger, routes map[string][]HandlerFunc) *InProc {
	return &InProc{log: log, handlers: routes}
}

// Publisher is the EventPublisher adapter shared by the auth, orgs, and admin
// application packages: purely in-process synchronous dispatch. (An earlier
// revision kept a dormant Kafka side for topics marked external — it was never
// wired and its envelope shape had drifted from what the consumers decode, so
// it was removed rather than left to rot. If Identity ever needs to emit to
// Kafka again, add it deliberately with the shared envelope.)
type Publisher struct {
	inproc *InProc
	log    *slog.Logger
}

// NewPublisher builds the in-process-only adapter.
func NewPublisher(log *slog.Logger, inproc *InProc) *Publisher {
	return &Publisher{inproc: inproc, log: log}
}

// Publish emits one event. Handlers run synchronously; errors are logged,
// matching the previous at-least-once/best-effort semantics.
func (p *Publisher) Publish(ctx context.Context, topic string, data any, key identity.ID) {
	msg := events.EventEnvelope{TaskID: key, Data: data, EventType: topic}
	for _, h := range p.inproc.handlers[topic] {
		if err := h(ctx, msg); err != nil {
			p.log.Error("in-process event handler failed", "topic", topic, "error", err)
		}
	}
}
