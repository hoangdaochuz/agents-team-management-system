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
	"strconv"
	"sync"
	"time"

	"github.com/IBM/sarama"

	"github.com/aaks/server/internal/contracts/events"
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

// Publish serializes msg.Data to JSON and publishes to topic, keyed by msg.TaskID
// so all events for a task land on the same partition in publish order.
func Publish(ctx context.Context, p sarama.SyncProducer, topic string, msg events.EventEnvelope, log *slog.Logger) error {
	if msg.EventID == "" {
		msg.EventID = newID()
	}
	if msg.OccurredAt.IsZero() {
		msg.OccurredAt = time.Now().UTC()
	}
	if msg.EventType == "" {
		msg.EventType = topic
	}
	if msg.TaskID == "" && events.IsTaskPartitioned(topic) {
		// Task-partitioned topics preserve per-task ordering; an empty key would
		// silently collapse every such event onto a single partition, degrading
		// the invariant — fail fast instead. Non-task topics (signup, invite,
		// workspace, catalog projections, audit) key on their own correlation id
		// and are unaffected.
		return fmt.Errorf("kafka: publish to %s: TaskID is required for task-partitioned topics", topic)
	}
	buf, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("kafka: marshal event: %w", err)
	}
	pm := &sarama.ProducerMessage{
		Topic: topic,
		Key:   sarama.StringEncoder(msg.TaskID),
		Value: sarama.ByteEncoder(buf),
	}
	if _, _, err := p.SendMessage(pm); err != nil {
		return fmt.Errorf("kafka: send %s: %w", topic, err)
	}
	log.Debug("event published", "topic", topic, "event_id", msg.EventID, "task_id", msg.TaskID)
	return nil
}

// Handler processes one envelope; returning an error re-queues (at-least-once).
// The handler MUST be idempotent (dedup by msg.EventID or the entity id it carries).
type Handler func(ctx context.Context, msg events.EventEnvelope) error

// ConsumerGroup runs a consumer group for the given topics, dispatching each
// message to handler. It blocks until ctx is cancelled. Rebalance/errors are
// logged but do not stop the group.
type ConsumerGroup struct {
	groupID string
	cg      sarama.ConsumerGroup
	log     *slog.Logger
	// fromNewest starts the group at the topic's current offset rather than
	// the oldest, for one-shot tailers (e.g. the Gateway's SSE live tail) that
	// already replayed history via another path.
	fromNewest bool
	// dlq receives poison messages (handler failure after maxRedeliveries, or a
	// malformed envelope) so they are recoverable instead of silently dropped.
	// nil when the DLQ producer could not be created (best-effort log-and-mark).
	dlq sarama.SyncProducer
}

// NewConsumerGroup builds a consumer group bound to brokers and groupID.
func NewConsumerGroup(brokers Brokers, groupID string, log *slog.Logger) (*ConsumerGroup, error) {
	return NewConsumerGroupFrom(brokers, groupID, false, log)
}

// NewConsumerGroupFrom builds a consumer group that reads from the oldest
// offset, or the newest when fromNewest is set.
func NewConsumerGroupFrom(brokers Brokers, groupID string, fromNewest bool, log *slog.Logger) (*ConsumerGroup, error) {
	cfg := NewConfig()
	cfg.Consumer.Offsets.Initial = sarama.OffsetOldest
	if fromNewest {
		cfg.Consumer.Offsets.Initial = sarama.OffsetNewest
	}
	cg, err := sarama.NewConsumerGroup(brokers, groupID, cfg)
	if err != nil {
		return nil, fmt.Errorf("kafka: new consumer group %s: %w", groupID, err)
	}
	// Best-effort DLQ producer: poison messages are recoverable when it is up,
	// log-and-marked when it is not.
	dlq, dlqErr := sarama.NewSyncProducer(brokers, dlqConfig())
	if dlqErr != nil {
		log.Warn("kafka: DLQ producer unavailable; poison messages will be logged-and-marked only",
			"group", groupID, "err", dlqErr)
		dlq = nil
	}
	log.Info("kafka consumer group ready", "group", groupID, "brokers", brokers, "from_newest", fromNewest, "dlq", dlq != nil)
	return &ConsumerGroup{groupID: groupID, cg: cg, log: log, fromNewest: fromNewest, dlq: dlq}, nil
}

// dlqConfig returns a producer config for the DLQ. It reuses the idempotent
// producer settings but does not require Return.Errors to be observed.
func dlqConfig() *sarama.Config {
	c := NewConfig()
	return c
}

// DLQTopicSuffix appends to a source topic to form its dead-letter topic.
const DLQTopicSuffix = ".dlq"

// DLQTopic returns the dead-letter topic name for a source topic.
func DLQTopic(topic string) string { return topic + DLQTopicSuffix }

// Run consumes topics until ctx is cancelled, dispatching each message to handler.
// Transient consumer errors (e.g. group coordinator not available while Kafka
// elects) are retried with a capped backoff instead of killing the goroutine.
func (g *ConsumerGroup) Run(ctx context.Context, topics []string, handler Handler) error {
	disp := newDispatcher(handler, g.dlq, g.log)
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

// Close releases the consumer group and its DLQ producer.
func (g *ConsumerGroup) Close() error {
	if g.dlq != nil {
		_ = g.dlq.Close()
	}
	return g.cg.Close()
}

// maxRedeliveries is how many times a handler failure is retried
// (at-least-once) before the message is treated as poison and parked. Without
// this a persistently failing message stalls its task partition forever.
const maxRedeliveries = 5

// dispatcher implements sarama.ConsumerGroupHandler. ConsumeClaim runs one
// goroutine per claimed partition, so failures must be mutex-guarded.
type dispatcher struct {
	handler Handler
	dlq     sarama.SyncProducer // nil = log-and-mark only
	log     *slog.Logger
	// failures counts handler errors per event id across redeliveries so a
	// poison message is routed to the DLQ (then ACKed) instead of retried forever.
	mu       sync.Mutex
	failures map[string]int
}

func newDispatcher(handler Handler, dlq sarama.SyncProducer, log *slog.Logger) *dispatcher {
	return &dispatcher{handler: handler, dlq: dlq, log: log, failures: map[string]int{}}
}

func (d *dispatcher) Setup(sarama.ConsumerGroupSession) error   { return nil }
func (d *dispatcher) Cleanup(sarama.ConsumerGroupSession) error { return nil }

func (d *dispatcher) ConsumeClaim(sess sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for cm := range claim.Messages() {
		var msg events.EventEnvelope
		if err := json.Unmarshal(cm.Value, &msg); err != nil {
			// Malformed (unparseable) envelope is definitively poison: route to
			// the DLQ (best-effort) and ACK so the partition keeps progressing.
			d.log.Error("kafka: malformed message", "topic", cm.Topic, "err", err)
			d.routeToDLQ(sess.Context(), cm, msg, fmt.Sprintf("malformed envelope: %v", err))
			sess.MarkMessage(cm, "")
			continue
		}
		if err := d.handler(sess.Context(), msg); err != nil {
			key := eventKey(msg)
			d.mu.Lock()
			d.failures[key]++
			attempts := d.failures[key]
			poison := attempts >= maxRedeliveries
			if poison {
				delete(d.failures, key)
			}
			d.mu.Unlock()
			if poison {
				// Poison message after max retries: route to the DLQ, then ACK.
				// The partition keeps progressing and the message is recoverable
				// for offline inspection/replay rather than silently dropped.
				d.log.Error("kafka: poison message after max retries; routing to DLQ",
					"topic", cm.Topic, "dlq_topic", DLQTopic(cm.Topic),
					"event_id", msg.EventID, "task_id", msg.TaskID,
					"attempts", attempts, "err", err)
				d.routeToDLQ(sess.Context(), cm, msg, err.Error())
				sess.MarkMessage(cm, "")
				continue
			}
			d.log.Error("kafka: handler error (will NOT advance; at-least-once)", "topic", cm.Topic, "event_id", msg.EventID, "task_id", msg.TaskID, "attempts", attempts, "err", err)
			// Do not mark the message: it will be redelivered.
			return err
		}
		d.mu.Lock()
		delete(d.failures, eventKey(msg))
		d.mu.Unlock()
		sess.MarkMessage(cm, "")
	}
	return nil
}

// routeToDLQ publishes the raw poison message to the source topic's dead-letter
// topic with diagnostic headers, preserving the original bytes/key so it can be
// inspected or replayed. Best-effort: if no DLQ producer or the publish fails,
// the caller still ACKs the message (never stalls the partition) and the drop
// is logged loudly.
func (d *dispatcher) routeToDLQ(_ context.Context, cm *sarama.ConsumerMessage, msg events.EventEnvelope, reason string) {
	if d.dlq == nil {
		d.log.Error("kafka: no DLQ producer; dropping poison message",
			"topic", cm.Topic, "event_id", msg.EventID, "task_id", msg.TaskID, "reason", reason)
		return
	}
	out := &sarama.ProducerMessage{
		Topic: DLQTopic(cm.Topic),
		Key:   sarama.ByteEncoder(cm.Key),
		Value: sarama.ByteEncoder(cm.Value),
		Headers: []sarama.RecordHeader{
			{Key: []byte("original-topic"), Value: []byte(cm.Topic)},
			{Key: []byte("original-partition"), Value: []byte(strconv.Itoa(int(cm.Partition)))},
			{Key: []byte("original-offset"), Value: []byte(strconv.FormatInt(cm.Offset, 10))},
			{Key: []byte("original-event-id"), Value: []byte(msg.EventID)},
			{Key: []byte("reason"), Value: []byte(truncate(reason, 1024))},
		},
	}
	if _, _, err := d.dlq.SendMessage(out); err != nil {
		d.log.Error("kafka: DLQ publish failed; dropping poison message",
			"dlq_topic", out.Topic, "src_topic", cm.Topic, "event_id", msg.EventID, "err", err)
		return
	}
	d.log.Warn("kafka: poison message routed to DLQ",
		"src_topic", cm.Topic, "dlq_topic", out.Topic, "event_id", msg.EventID, "task_id", msg.TaskID)
}

// eventKey identifies a delivery for poison tracking. Fall back to the raw
// offset when the envelope has no EventID, so every redelivery of a malformed
// (but JSON-parsable) envelope is still accounted for.
func eventKey(msg events.EventEnvelope) string {
	if msg.EventID != "" {
		return msg.EventID
	}
	return msg.OccurredAt.String() + ":" + msg.EventType
}

// newID returns a short random hex id for event dedup. Not a UUID, but unique
// enough for idempotency keys.
func newID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// truncate caps s at n runes with an ellipsis, for bounded DLQ headers/logs.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
