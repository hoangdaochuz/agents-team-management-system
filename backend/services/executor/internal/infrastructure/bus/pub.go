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

// Publisher is the EventPublisher adapter wrapping the sarama sync producer
// (DIP — application never imports sarama).
type Publisher struct {
	log *slog.Logger

	mu   sync.Mutex
	prod sarama.SyncProducer // attached once Kafka is reachable; nil until then
}

// NewPublisher builds the adapter; an empty broker list yields a no-op
// publisher. Otherwise the producer is attached asynchronously with retry:
// the executor may boot before Kafka is ready, and a permanently-nil producer
// would silently swallow every run.completed/step fact — stalling the saga
// with nothing but a single startup warning.
func NewPublisher(ctx context.Context, brokers string, log *slog.Logger) *Publisher {
	p := &Publisher{log: log}
	if brokers == "" {
		return p
	}
	go func() {
		prod, err := kafka.NewProducerUntilReady(ctx, kafka.Brokers(strings.Split(brokers, ",")), log)
		if err != nil {
			log.Warn("kafka producer stopped before Kafka was reachable; executor emits no facts", "error", err)
			return
		}
		p.mu.Lock()
		p.prod = prod
		p.mu.Unlock()
	}()
	return p
}

// Publish emits one fact; non-fatal when the producer is unavailable.
func (p *Publisher) Publish(ctx context.Context, topic string, data any, key identity.ID) {
	p.mu.Lock()
	prod := p.prod
	p.mu.Unlock()
	if prod == nil {
		p.log.Warn("kafka producer not ready; fact dropped", "topic", topic, "task_id", key)
		return
	}
	msg := events.EventEnvelope{TaskID: key, Data: data}
	if err := kafka.Publish(ctx, prod, topic, msg, p.log); err != nil {
		p.log.Error("publish event failed", "topic", topic, "task_id", key, "error", err)
	}
}

// Close releases the producer.
func (p *Publisher) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.prod != nil {
		_ = p.prod.Close()
	}
}
