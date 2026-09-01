// Package bus adapts the application EventPublisher port for the consolidated
// Workspace service. Events that used to round-trip through Kafka between the
// catalog and resources services (mcp.created/mcp.deleted projections) now
// dispatch synchronously to in-process handlers; genuinely cross-service
// events still travel over Kafka (the task saga commands and facts to the
// Executor, and skill.* to the Agent service until that becomes a direct
// call).
package bus

import (
	"context"
	"log/slog"
	"strings"
	"sync"

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

// Publisher is the EventPublisher adapter shared by the task and catalog
// application packages. Intra-service topics dispatch synchronously via the
// in-process bus; topics listed in external are published to Kafka.
type Publisher struct {
	inproc   *InProc
	external map[string]bool
	log      *slog.Logger

	mu   sync.Mutex
	prod sarama.SyncProducer // attached once Kafka is reachable; nil until then
}

// NewPublisher builds the adapter. An empty broker list yields a no-op Kafka
// side (the in-process side always works). Otherwise the producer is attached
// asynchronously with retry: the service may boot before Kafka is ready, and a
// permanently-nil producer would silently swallow every saga command — facts
// would stop flowing with nothing but a single startup warning.
func NewPublisher(ctx context.Context, brokers string, log *slog.Logger, inproc *InProc, external map[string]bool) *Publisher {
	p := &Publisher{inproc: inproc, external: external, log: log}
	if brokers == "" {
		return p
	}
	go func() {
		prod, err := kafka.NewProducerUntilReady(ctx, kafka.Brokers(strings.Split(brokers, ",")), log)
		if err != nil {
			log.Warn("kafka producer stopped before Kafka was reachable; workspace emits no external events", "error", err)
			return
		}
		p.mu.Lock()
		p.prod = prod
		p.mu.Unlock()
	}()
	return p
}

// Enabled reports whether a real producer is behind the adapter (for startup
// logging).
func (p *Publisher) Enabled() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.prod != nil
}

// Publish emits one event. In-process handlers run synchronously (errors are
// logged, matching the previous best-effort semantics); external topics go to
// Kafka once its producer is attached.
func (p *Publisher) Publish(ctx context.Context, topic string, data any, key identity.ID) {
	if p.external[topic] {
		p.mu.Lock()
		prod := p.prod
		p.mu.Unlock()
		if prod == nil {
			p.log.Warn("kafka producer not ready; event dropped", "topic", topic, "task_id", key)
			return
		}
		msg := events.EventEnvelope{TaskID: key, Data: data}
		if err := kafka.Publish(ctx, prod, topic, msg, p.log); err != nil {
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
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.prod != nil {
		_ = p.prod.Close()
	}
}
