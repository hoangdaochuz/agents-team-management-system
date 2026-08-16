package store

import (
	"context"
	"log/slog"
	"strings"

	"github.com/IBM/sarama"

	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/platform/kafka"
)

// Publisher is the EventPublisher adapter: it wraps the sarama sync producer
// behind the application port (DIP — application never imports sarama).
type Publisher struct {
	prod sarama.SyncProducer
	log  *slog.Logger
}

// NewPublisher builds the adapter from a KAFKA_BROKERS env list. A nil/empty
// broker list yields a no-op publisher (matches the pre-refactor behavior where
// catalog emitted no definition events when Kafka was unavailable).
func NewPublisher(brokers string, log *slog.Logger) *Publisher {
	if brokers == "" {
		return &Publisher{log: log}
	}
	p, err := kafka.NewProducer(kafka.Brokers(strings.Split(brokers, ",")), log)
	if err != nil {
		log.Warn("kafka producer unavailable; catalog emits no definition events", "error", err)
		return &Publisher{log: log}
	}
	return &Publisher{prod: p, log: log}
}

// Publish emits one event; non-fatal when the producer is unavailable.
func (p *Publisher) Publish(ctx context.Context, topic string, data any, key identity.ID) {
	if p.prod == nil {
		return
	}
	env := events.EventEnvelope{TaskID: key, Data: data}
	if err := kafka.Publish(ctx, p.prod, topic, env, p.log); err != nil {
		p.log.Error("publish definition event failed", "topic", topic, "error", err)
	}
}

// Close releases the producer.
func (p *Publisher) Close() {
	if p.prod != nil {
		_ = p.prod.Close()
	}
}