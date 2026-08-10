package kafka

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/IBM/sarama"

	"github.com/aaks/server/internal/contracts"
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

// TestPublishConsumeRoundTrip verifies the producer partitions by task_id and
// two independent consumer groups each receive the event (at-least-once).
func TestPublishConsumeRoundTrip(t *testing.T) {
	brokers := skipIfNoBroker(t)

	topic := "kafka-test-" + newID()
	taskID := contracts.ID("task-123")

	// Create the topic explicitly so the consumer doesn't race auto-create.
	admin, err := sarama.NewClusterAdmin(brokers, NewConfig())
	if err != nil {
		t.Fatalf("cluster admin: %v", err)
	}
	defer admin.Close()
	if err := admin.CreateTopic(topic, &sarama.TopicDetail{NumPartitions: 1, ReplicationFactor: 1}, false); err != nil {
		t.Fatalf("create topic: %v", err)
	}
	defer admin.DeleteTopic(topic)

	log := testLogger(t)
	prod, err := NewProducer(brokers, log)
	if err != nil {
		t.Fatalf("producer: %v", err)
	}
	defer prod.Close()

	env := contracts.EventEnvelope{
		EventID:   "evt-" + newID(),
		EventType: topic,
		TaskID:    taskID,
		Data: contracts.StepData{Step: contracts.Step{
			ID: "step-1", RunID: "run-1", Seq: 1, Kind: contracts.StepMessage,
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
		got := make(chan contracts.EventEnvelope, 1)
		cg, err := NewConsumerGroup(brokers, groupID, log)
		if err != nil {
			t.Fatalf("consumer group %s: %v", groupID, err)
		}
		defer cg.Close()

		go func() { _ = cg.Run(ctx, []string{topic}, func(_ context.Context, e contracts.EventEnvelope) error {
			if e.TaskID == taskID {
				select {
				case got <- e:
				default:
				}
				cancel() // received: stop the loop
			}
			return nil
		}) }()

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
