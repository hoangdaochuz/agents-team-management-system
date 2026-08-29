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
	"strings"

	"github.com/IBM/sarama"

	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/platform/kafka"
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
// application packages. Intra-service topics dispatch synchronously via the
// in-process bus; topics listed in external are published to Kafka.
type Publisher struct {
	inproc   *InProc
	prod     sarama.SyncProducer
	external map[string]bool
	log      *slog.Logger
}

// NewPublisher builds the adapter. An empty broker list yields a no-op Kafka
// side (the in-process side always works).
func NewPublisher(brokers string, log *slog.Logger, inproc *InProc, external map[string]bool) *Publisher {
	p := &Publisher{inproc: inproc, external: external, log: log}
	if brokers == "" {
		return p
	}
	prod, err := kafka.NewProducer(kafka.Brokers(strings.Split(brokers, ",")), log)
	if err != nil {
		log.Warn("kafka producer unavailable; identity emits no external events", "error", err)
		return p
	}
	p.prod = prod
	return p
}

// Publish emits one event. In-process handlers run synchronously (errors are
// logged, matching the previous at-least-once/best-effort semantics); external
// topics go to Kafka.
func (p *Publisher) Publish(ctx context.Context, topic string, data any, key identity.ID) {
	if p.external[topic] {
		if p.prod == nil {
			return
		}
		msg := events.EventEnvelope{TaskID: key, Data: data}
		if err := kafka.Publish(ctx, p.prod, topic, msg, p.log); err != nil {
			p.log.Error("publish event failed", "topic", topic, "error", err)
		}
		return
	}
	msg := events.EventEnvelope{TaskID: key, Data: data, EventType: topic}
	for _, h := range p.inproc.handlers[topic] {
		if err := h(ctx, msg); err != nil {
			p.log.Error("in-process event handler failed", "topic", topic, "error", err)
		}
	}
}

// Close releases the Kafka producer.
func (p *Publisher) Close() {
	if p.prod != nil {
		_ = p.prod.Close()
	}
}
