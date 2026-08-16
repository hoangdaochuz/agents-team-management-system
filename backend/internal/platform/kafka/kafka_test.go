package kafka

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/IBM/sarama"
	"github.com/IBM/sarama/mocks"

	"github.com/aaks/server/internal/contracts/agentexec"
	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/internal/contracts/identity"
)

// brokerEnv names the env var holding a reachable Kafka bootstrap address. The
// test is skipped when unset so `go test ./...` is green without infrastructure.
const brokerEnv = "AAKS_KAFKA_TEST_BROKERS"

// skipIfNoBroker skips the test unless AAKS_KAFKA_TEST_BROKERS is set and a
// broker is dialable within 2s.
func skipIfNoBroker(t *testing.T) Brokers {
	t.Helper()
	addr := os.Getenv(brokerEnv)
	if addr == "" {
		t.Skipf("%s unset; skipping Kafka integration test", brokerEnv)
	}
	return Brokers{addr}
}

// testLogger writes structured logs to the test output buffer.
func testLogger(t *testing.T) *slog.Logger {
	t.Helper()
	var buf bytes.Buffer
	return slog.New(slog.NewJSONHandler(&buf, nil))
}

// TestPublishEmptyTaskIDGating verifies that task-partitioned topics reject an
// empty TaskID (the partition key), while non-task topics (signup, catalog
// projections, audit) are exempt — they key on their own correlation id.
func TestPublishEmptyTaskIDGating(t *testing.T) {
	log := testLogger(t)

	if events.IsTaskPartitioned(events.TopicTaskRunRequested) != true {
		t.Fatalf("%s should be task-partitioned", events.TopicTaskRunRequested)
	}
	if events.IsTaskPartitioned(events.TopicSignupRequested) {
		t.Fatalf("%s should NOT be task-partitioned", events.TopicSignupRequested)
	}
	if events.IsTaskPartitioned(events.TopicSkillDeleted) {
		t.Fatalf("%s should NOT be task-partitioned", events.TopicSkillDeleted)
	}

	// A mock producer lets us assert both branches without a broker: the
	// empty-key guard for task-partitioned topics fails before any send, and
	// the exempt topics must reach the producer.
	mockProd := mocks.NewSyncProducer(t, sarama.NewConfig())

	// Task-partitioned topic without a TaskID → hard error, no send attempted.
	err := Publish(context.Background(), mockProd, events.TopicTaskRunRequested,
		events.EventEnvelope{EventType: events.TopicTaskRunRequested}, log)
	if err == nil {
		t.Fatalf("task-partitioned publish with empty TaskID should fail, got nil")
	}

	// Non-task topic without a TaskID → must reach the producer (the send
	// succeeds on a mock producer), proving the TopicID guard exempts it.
	mockProd.ExpectSendMessageAndSucceed()
	err = Publish(context.Background(), mockProd, events.TopicSignupRequested,
		events.EventEnvelope{EventType: events.TopicSignupRequested}, log)
	if err != nil {
		t.Fatalf("signup.requested should not be blocked by the TaskID guard: %v", err)
	}
}

// TestPublishConsumeRoundTrip verifies the producer partitions by task_id and
// two independent consumer groups each receive the event (at-least-once).
func TestPublishConsumeRoundTrip(t *testing.T) {
	brokers := skipIfNoBroker(t)

	topic := "kafka-test-" + newID()
	taskID := identity.ID("task-123")

	// Create the topic explicitly so the consumer doesn't race auto-create.
	admin, err := sarama.NewClusterAdmin(brokers, NewConfig())
	if err != nil {
		t.Fatalf("cluster admin: %v", err)
	}
	defer func() { _ = admin.Close() }()
	if err := admin.CreateTopic(topic, &sarama.TopicDetail{NumPartitions: 1, ReplicationFactor: 1}, false); err != nil {
		t.Fatalf("create topic: %v", err)
	}
	defer func() { _ = admin.DeleteTopic(topic) }()

	log := testLogger(t)
	prod, err := NewProducer(brokers, log)
	if err != nil {
		t.Fatalf("producer: %v", err)
	}
	defer func() { _ = prod.Close() }()

	env := events.EventEnvelope{
		EventID:   "evt-" + newID(),
		EventType: topic,
		TaskID:    taskID,
		Data: events.StepData{Step: agentexec.Step{
			ID: "step-1", RunID: "run-1", Seq: 1, Kind: agentexec.StepMessage,
			Payload: []byte(`{"content":"hi"}`),
		}},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := Publish(ctx, prod, topic, env, log); err != nil {
		t.Fatalf("publish: %v", err)
	}

	// Each consumer group must independently receive the event.
	expect := func(groupID string) {
		got := make(chan events.EventEnvelope, 1)
		cg, err := NewConsumerGroup(brokers, groupID, log)
		if err != nil {
			t.Fatalf("consumer group %s: %v", groupID, err)
		}
		defer func() { _ = cg.Close() }()

		go func() {
			_ = cg.Run(ctx, []string{topic}, func(_ context.Context, e events.EventEnvelope) error {
				if e.TaskID == taskID {
					select {
					case got <- e:
					default:
					}
					cancel() // received: stop the loop
				}
				return nil
			})
		}()

		select {
		case recv := <-got:
			if recv.EventID != env.EventID {
				t.Errorf("%s: event_id mismatch: got %s want %s", groupID, recv.EventID, env.EventID)
			}
		case <-ctx.Done():
			t.Fatalf("%s: timed out waiting for event", groupID)
		}
	}

	expect("cg-a-" + newID())
	expect("cg-b-" + newID())
}

// --- DLQ behavior tests (review: route poison messages to DLQ, not drop) ---

// fakeSession satisfies sarama.ConsumerGroupSession for dispatcher tests.
type fakeSession struct {
	ctx    context.Context
	marked int
}

func (s *fakeSession) Claims() map[string][]int32                  { return nil }
func (s *fakeSession) MemberID() string                            { return "" }
func (s *fakeSession) GenerationID() int32                         { return 0 }
func (s *fakeSession) Errors() <-chan error                        { return nil }
func (s *fakeSession) Context() context.Context                    { return s.ctx }
func (s *fakeSession) MarkOffset(string, int32, int64, string)     {}
func (s *fakeSession) ResetOffset(string, int32, int64, string)    {}
func (s *fakeSession) MarkMessage(*sarama.ConsumerMessage, string) { s.marked++ }
func (s *fakeSession) Commit()                                     {}

// fakeClaim satisfies sarama.ConsumerGroupClaim for dispatcher tests.
type fakeClaim struct{ msgs chan *sarama.ConsumerMessage }

func (c *fakeClaim) Topic() string                            { return "" }
func (c *fakeClaim) Partition() int32                         { return 0 }
func (c *fakeClaim) InitialOffset() int64                     { return 0 }
func (c *fakeClaim) HighWaterMarkOffset() int64               { return 0 }
func (c *fakeClaim) Messages() <-chan *sarama.ConsumerMessage { return c.msgs }

func mustEnvBytes(t *testing.T, env events.EventEnvelope) []byte {
	t.Helper()
	b, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// oneMsgClaim builds a claim that yields exactly one message then closes.
func oneMsgClaim(msg *sarama.ConsumerMessage) *fakeClaim {
	ch := make(chan *sarama.ConsumerMessage, 1)
	ch <- msg
	close(ch)
	return &fakeClaim{msgs: ch}
}

// captureProducer is a minimal sarama.SyncProducer fake that records the last
// SendMessage so DLQ tests can assert the full ProducerMessage (topic/key/
// value/headers) — the sarama mock's ValueChecker only exposes the value bytes.
type captureProducer struct {
	got *sarama.ProducerMessage
	err error // when set, SendMessage fails (failure-path test)
}

func (p *captureProducer) SendMessage(m *sarama.ProducerMessage) (int32, int64, error) {
	p.got = m
	return 0, 0, p.err
}
func (p *captureProducer) SendMessages([]*sarama.ProducerMessage) error { return nil }
func (p *captureProducer) Close() error                                 { return nil }
func (p *captureProducer) TxnStatus() sarama.ProducerTxnStatusFlag      { return 0 }
func (p *captureProducer) IsTransactional() bool                        { return false }
func (p *captureProducer) BeginTxn() error                              { return errors.New("txn n/a") }
func (p *captureProducer) CommitTxn() error                             { return errors.New("txn n/a") }
func (p *captureProducer) AbortTxn() error                              { return errors.New("txn n/a") }
func (p *captureProducer) AddOffsetsToTxn(map[string][]*sarama.PartitionOffsetMetadata, string) error {
	return nil
}
func (p *captureProducer) AddOffsetsToTxnWithGroupMetadata(map[string][]*sarama.PartitionOffsetMetadata, *sarama.ConsumerGroupMetadata) error {
	return nil
}
func (p *captureProducer) AddMessageToTxn(*sarama.ConsumerMessage, string, *string) error { return nil }
func (p *captureProducer) AddMessageToTxnWithGroupMetadata(*sarama.ConsumerMessage, *sarama.ConsumerGroupMetadata, *string) error {
	return nil
}

// TestPoisonMessageRoutedToDLQ drives the dispatcher through maxRedeliveries
// failures and asserts the poison message is published to <topic>.dlq (with
// context headers) and then ACKed, not dropped.
func TestPoisonMessageRoutedToDLQ(t *testing.T) {
	log := testLogger(t)
	const topic = "task.run.requested"
	env := events.EventEnvelope{
		EventID: "evt-poison", EventType: topic, TaskID: "task-1",
		Data: map[string]string{"x": "y"},
	}
	raw := mustEnvBytes(t, env)

	prod := &captureProducer{}
	d := newDispatcher(
		func(_ context.Context, _ events.EventEnvelope) error { return errors.New("boom") },
		prod, log,
	)

	sess := &fakeSession{ctx: context.Background()}
	// First (maxRedeliveries - 1) deliveries: handler fails, message NOT marked.
	for i := 1; i < maxRedeliveries; i++ {
		msg := &sarama.ConsumerMessage{Topic: topic, Partition: 0, Offset: int64(i), Key: []byte("task-1"), Value: raw}
		if err := d.ConsumeClaim(sess, oneMsgClaim(msg)); err == nil {
			t.Fatalf("delivery %d: expected handler error, got nil", i)
		}
	}
	if prod.got != nil {
		t.Fatalf("no DLQ publish expected before the retry cap, got %+v", prod.got)
	}

	// Final delivery: routed to DLQ and marked (ACKed).
	msg := &sarama.ConsumerMessage{Topic: topic, Partition: 0, Offset: int64(maxRedeliveries), Key: []byte("task-1"), Value: raw}
	if err := d.ConsumeClaim(sess, oneMsgClaim(msg)); err != nil {
		t.Fatalf("final delivery: expected nil (DLQ+ACK), got %v", err)
	}

	if prod.got == nil {
		t.Fatal("DLQ message was not produced")
	}
	if want := DLQTopic(topic); prod.got.Topic != want {
		t.Errorf("DLQ topic = %q, want %q", prod.got.Topic, want)
	}
	if string(prod.got.Value.(sarama.ByteEncoder)) != string(raw) {
		t.Error("DLQ value does not preserve the original envelope bytes")
	}
	if string(prod.got.Key.(sarama.ByteEncoder)) != "task-1" {
		t.Errorf("DLQ key = %q, want task-1", prod.got.Key)
	}
	hdr := headerMap(prod.got.Headers)
	if hdr["original-topic"] != topic {
		t.Errorf("original-topic header = %q", hdr["original-topic"])
	}
	if hdr["original-event-id"] != "evt-poison" {
		t.Errorf("original-event-id header = %q", hdr["original-event-id"])
	}
	if hdr["reason"] == "" {
		t.Error("reason header should carry the handler error")
	}

	// The poison delivery must have been ACKed exactly once (the final one).
	if sess.marked != 1 {
		t.Errorf("marked = %d, want 1 (poison ACKed after DLQ)", sess.marked)
	}
}

// TestDLQPublishFailureStillAcks verifies that if the DLQ publish itself fails,
// the dispatcher still ACKs the poison message (never stalls the partition).
func TestDLQPublishFailureStillAcks(t *testing.T) {
	log := testLogger(t)
	const topic = "task.run.requested"
	env := events.EventEnvelope{EventID: "evt-dlqfail", EventType: topic, TaskID: "task-7"}
	raw := mustEnvBytes(t, env)

	prod := &captureProducer{err: errors.New("dlq broker down")}
	d := newDispatcher(func(_ context.Context, _ events.EventEnvelope) error { return errors.New("boom") }, prod, log)
	sess := &fakeSession{ctx: context.Background()}
	for i := 1; i < maxRedeliveries; i++ {
		_ = d.ConsumeClaim(sess, oneMsgClaim(&sarama.ConsumerMessage{Topic: topic, Value: raw}))
	}
	if err := d.ConsumeClaim(sess, oneMsgClaim(&sarama.ConsumerMessage{Topic: topic, Value: raw})); err != nil {
		t.Fatalf("final delivery with DLQ failure: %v", err)
	}
	if sess.marked != 1 {
		t.Errorf("marked = %d, want 1 (ACKed despite DLQ publish failure)", sess.marked)
	}
}

// TestPoisonMessageNoDLQProducerDropsSafely verifies that without a DLQ
// producer the dispatcher still ACKs the poison message (never stalls), with
// no panic.
func TestPoisonMessageNoDLQProducerDropsSafely(t *testing.T) {
	log := testLogger(t)
	const topic = "task.run.requested"
	env := events.EventEnvelope{EventID: "evt-2", EventType: topic, TaskID: "task-2"}
	raw := mustEnvBytes(t, env)

	d := newDispatcher(func(_ context.Context, _ events.EventEnvelope) error { return errors.New("boom") }, nil, log)
	sess := &fakeSession{ctx: context.Background()}
	for i := 1; i < maxRedeliveries; i++ {
		_ = d.ConsumeClaim(sess, oneMsgClaim(&sarama.ConsumerMessage{Topic: topic, Value: raw}))
	}
	if err := d.ConsumeClaim(sess, oneMsgClaim(&sarama.ConsumerMessage{Topic: topic, Value: raw})); err != nil {
		t.Fatalf("final delivery without DLQ: %v", err)
	}
	if sess.marked != 1 {
		t.Errorf("marked = %d, want 1 (ACKed, not stalled)", sess.marked)
	}
}

// TestMalformedMessageRoutedToDLQ verifies an unparseable envelope is routed to
// the DLQ and ACKed.
func TestMalformedMessageRoutedToDLQ(t *testing.T) {
	log := testLogger(t)
	const topic = "task.run.requested"

	prod := &captureProducer{}
	// A healthy handler that must never be called for malformed input.
	d := newDispatcher(func(_ context.Context, _ events.EventEnvelope) error {
		t.Fatal("handler must not be called for a malformed message")
		return nil
	}, prod, log)

	sess := &fakeSession{ctx: context.Background()}
	msg := &sarama.ConsumerMessage{Topic: topic, Value: []byte("not-json"), Key: []byte("task-9")}
	if err := d.ConsumeClaim(sess, oneMsgClaim(msg)); err != nil {
		t.Fatalf("malformed delivery: %v", err)
	}
	if prod.got == nil || prod.got.Topic != DLQTopic(topic) {
		t.Fatalf("malformed message not routed to DLQ: %+v", prod.got)
	}
	hdr := headerMap(prod.got.Headers)
	if !strings.Contains(hdr["reason"], "malformed") {
		t.Errorf("reason header = %q, want it to mention malformed", hdr["reason"])
	}
	if sess.marked != 1 {
		t.Errorf("marked = %d, want 1", sess.marked)
	}
}

// TestSuccessfulDeliveryResetsFailures ensures a later success clears the
// failure counter for an event id (no stale poison-routing on a transient blip).
func TestSuccessfulDeliveryResetsFailures(t *testing.T) {
	log := testLogger(t)
	const topic = "task.run.requested"
	env := events.EventEnvelope{EventID: "evt-flaky", EventType: topic, TaskID: "task-3"}
	raw := mustEnvBytes(t, env)

	d := newDispatcher(func(_ context.Context, _ events.EventEnvelope) error { return errors.New("boom") }, nil, log)
	sess := &fakeSession{ctx: context.Background()}
	// A couple of failures, then no DLQ expected because not yet at the cap.
	for i := 1; i < maxRedeliveries; i++ {
		_ = d.ConsumeClaim(sess, oneMsgClaim(&sarama.ConsumerMessage{Topic: topic, Value: raw}))
	}
	// Switch the handler to succeed: counter resets.
	d.handler = func(context.Context, events.EventEnvelope) error { return nil }
	if err := d.ConsumeClaim(sess, oneMsgClaim(&sarama.ConsumerMessage{Topic: topic, Value: raw})); err != nil {
		t.Fatalf("success delivery: %v", err)
	}
	if d.failures[eventKey(env)] != 0 {
		t.Errorf("failure counter not reset after success: %d", d.failures[eventKey(env)])
	}
}

// TestDLQTopicNaming pins the dead-letter topic derivation.
func TestDLQTopicNaming(t *testing.T) {
	if got := DLQTopic(events.TopicTaskRunRequested); got != events.TopicTaskRunRequested+".dlq" {
		t.Errorf("DLQTopic = %q", got)
	}
}

func headerMap(h []sarama.RecordHeader) map[string]string {
	out := make(map[string]string, len(h))
	for _, h := range h {
		out[string(h.Key)] = string(h.Value)
	}
	return out
}
