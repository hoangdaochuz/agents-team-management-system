// Package kafka wraps sarama with project-specific helpers: a producer that
// always partitions by task_id, and a consumer-group runner that delivers
// at-least-once with an idempotency hook so redelivery cannot double-apply.
package kafka

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/IBM/sarama"

	"github.com/aaks/server/internal/contracts"
)

// Brokers holds the Kafka bootstrap addresses.
type Brokers []string

// NewConfig returns a sarama Config tuned for KRaft single-broker dev and
// idempotent producer semantics.
func NewConfig() *sarama.Config {
	c := sarama.NewConfig()
	c.Version = sarama.V3_7_0_0 // KRaft-era

	// Idempotent, safe producer.
	c.Producer.RequiredAcks = sarama.WaitForAll
	c.Producer.Idempotent = true
	c.Producer.Return.Successes = true
	c.Producer.Return.Errors = true
	c.Net.MaxOpenRequests = 1 // required for idempotent producer

	// Consumer reads the latest committed offset for its group.
	c.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{sarama.NewBalanceStrategyRoundRobin()}
	c.Consumer.Offsets.Initial = sarama.OffsetOldest
	c.Consumer.Return.Errors = true

	return c
}

// NewProducer returns a sync producer safe for concurrent use.
func NewProducer(brokers Brokers, log *slog.Logger) (sarama.SyncProducer, error) {
	p, err := sarama.NewSyncProducer(brokers, NewConfig())
	if err != nil {
		return nil, fmt.Errorf("kafka: new producer: %w", err)
	}
	log.Info("kafka producer ready", "brokers", brokers)
	return p, nil
}

// Publish serializes env.Data to JSON and publishes to topic, keyed by env.TaskID
// so all events for a task land on the same partition in publish order.
func Publish(ctx context.Context, p sarama.SyncProducer, topic string, env contracts.EventEnvelope, log *slog.Logger) error {
	if env.EventID == "" {
		env.EventID = newID()
	}
	if env.OccurredAt.IsZero() {
		env.OccurredAt = time.Now().UTC()
	}
	if env.EventType == "" {
		env.EventType = topic
	}
	buf, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("kafka: marshal event: %w", err)
	}
	msg := &sarama.ProducerMessage{
		Topic: topic,
		Key:   sarama.StringEncoder(env.TaskID),
		Value: sarama.ByteEncoder(buf),
	}
	if _, _, err := p.SendMessage(msg); err != nil {
		return fmt.Errorf("kafka: send %s: %w", topic, err)
	}
	log.Debug("event published", "topic", topic, "event_id", env.EventID, "task_id", env.TaskID)
	return nil
}

// Handler processes one envelope; returning an error re-queues (at-least-once).
// The handler MUST be idempotent (dedup by env.EventID or the entity id it carries).
type Handler func(ctx context.Context, env contracts.EventEnvelope) error

// ConsumerGroup runs a consumer group for the given topics, dispatching each
// message to handler. It blocks until ctx is cancelled. Rebalance/errors are
// logged but do not stop the group.
type ConsumerGroup struct {
	groupID string
	cg      sarama.ConsumerGroup
	log     *slog.Logger
}

// NewConsumerGroup builds a consumer group bound to brokers and groupID.
func NewConsumerGroup(brokers Brokers, groupID string, log *slog.Logger) (*ConsumerGroup, error) {
	cg, err := sarama.NewConsumerGroup(brokers, groupID, NewConfig())
	if err != nil {
		return nil, fmt.Errorf("kafka: new consumer group %s: %w", groupID, err)
	}
	log.Info("kafka consumer group ready", "group", groupID, "brokers", brokers)
	return &ConsumerGroup{groupID: groupID, cg: cg, log: log}, nil
}

// Run consumes topics until ctx is cancelled, dispatching each message to handler.
// Transient consumer errors (e.g. group coordinator not available while Kafka
// elects) are retried with a capped backoff instead of killing the goroutine.
func (g *ConsumerGroup) Run(ctx context.Context, topics []string, handler Handler) error {
	disp := &dispatcher{handler: handler, log: g.log}
	backoff := time.Second
	for {
		if err := g.cg.Consume(ctx, topics, disp); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			g.log.Warn("kafka: consume error, retrying", "group", g.groupID, "err", err, "backoff", backoff)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(backoff):
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		if ctx.Err() != nil {
			return nil
		}
	}
}

// Close releases the consumer group.
func (g *ConsumerGroup) Close() error { return g.cg.Close() }

// dispatcher implements sarama.ConsumerGroupHandler.
type dispatcher struct {
	handler Handler
	log     *slog.Logger
}

func (d *dispatcher) Setup(sarama.ConsumerGroupSession) error   { return nil }
func (d *dispatcher) Cleanup(sarama.ConsumerGroupSession) error { return nil }

func (d *dispatcher) ConsumeClaim(sess sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for msg := range claim.Messages() {
		var env contracts.EventEnvelope
		if err := json.Unmarshal(msg.Value, &env); err != nil {
			d.log.Error("kafka: malformed message, skipping", "topic", msg.Topic, "err", err)
			sess.MarkMessage(msg, "")
			continue
		}
		if err := d.handler(sess.Context(), env); err != nil {
			d.log.Error("kafka: handler error (will NOT advance; at-least-once)", "topic", msg.Topic, "event_id", env.EventID, "err", err)
			// Do not mark the message: it will be redelivered.
			return err
		}
		sess.MarkMessage(msg, "")
	}
	return nil
}

// newID returns a short random hex id for event dedup. Not a UUID, but unique
// enough for idempotency keys.
func newID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
