package repository

import (
	"context"
	"log/slog"
	"strings"

	"github.com/IBM/sarama"

	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/platform/kafka"
)

// Publisher is the EventPublisher adapter wrapping the sarama sync producer
// (DIP — application never imports sarama).
type Publisher struct {
	prod sarama.SyncProducer
	log  *slog.Logger
}

// NewPublisher builds the adapter; an empty broker list yields a no-op
// publisher (runner then emits no facts, matching pre-refactor behavior).
func NewPublisher(brokers string, log *slog.Logger) *Publisher {
	if brokers == "" {
		return &Publisher{log: log}
	}
	p, err := kafka.NewProducer(kafka.Brokers(strings.Split(brokers, ",")), log)
	if err != nil {
		log.Warn("kafka producer unavailable; runner emits no facts", "error", err)
		return &Publisher{log: log}
	}
	return &Publisher{prod: p, log: log}
}

// Publish emits one fact; non-fatal when the producer is unavailable.
func (p *Publisher) Publish(ctx context.Context, topic string, data any, key identity.ID) {
	if p.prod == nil {
		return
	}
	msg := events.EventEnvelope{TaskID: key, Data: data}
	if err := kafka.Publish(ctx, p.prod, topic, msg, p.log); err != nil {
		p.log.Error("publish event failed", "topic", topic, "task_id", key, "error", err)
	}
}

// Close releases the producer.
func (p *Publisher) Close() {
	if p.prod != nil {
		_ = p.prod.Close()
	}
}
