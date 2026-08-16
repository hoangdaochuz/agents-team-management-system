package application

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/aaks/server/internal/contracts/agentexec"
	"github.com/aaks/server/internal/contracts/identity"
)

// ── Fakes ───────────────────────────────────────────────────────────────────

// fakeSteps scripts the Runner's persisted-step replay.
type fakeSteps struct {
	steps []agentexec.Step
	err   error
}

func (f *fakeSteps) Steps(context.Context, identity.ID) ([]agentexec.Step, error) {
	return f.steps, f.err
}

// fakeTailer feeds live steps through the handler, then blocks until the
// context is cancelled (or returns immediately when done=true to simulate an
// ended stream).
type fakeTailer struct {
	steps []agentexec.Step
	done  bool
	err   error
}

func (f *fakeTailer) Run(ctx context.Context, onStep func(context.Context, agentexec.Step) error) error {
	if f.err != nil {
		return f.err
	}
	for _, st := range f.steps {
		if err := onStep(ctx, st); err != nil {
			return err
		}
	}
	if f.done {
		return nil
	}
	<-ctx.Done()
	return nil
}

func (f *fakeTailer) Close() error { return nil }

// recordingSSEWriter collects the raw wire events of a stream.
type recordingSSEWriter struct {
	events []string
}

func (w *recordingSSEWriter) Event(event string, data []byte) {
	w.events = append(w.events, "event: "+event+"\ndata: "+string(data)+"\n")
}

func newTestStream(steps *fakeSteps, tail *fakeTailer, tailErr error, ping time.Duration) (*Stream, *fakeSteps, *fakeTailer) {
	factory := func(identity.ID) (StepTailer, error) {
		if tailErr != nil {
			return nil, tailErr
		}
		return tail, nil
	}
	s := NewStream(steps, factory, slog.New(slog.DiscardHandler))
	if ping > 0 {
		s.pingInterval = ping
	}
	return s, steps, tail
}

// ── Replay + tail merge ─────────────────────────────────────────────────────

func TestStreamReplaysThenTails(t *testing.T) {
	replayed := []agentexec.Step{
		{ID: "s1", Seq: 1, Kind: agentexec.StepMessage},
		{ID: "s2", Seq: 2, Kind: agentexec.StepReasoning},
	}
	tail := &fakeTailer{steps: []agentexec.Step{
		{ID: "s2", Seq: 2, Kind: agentexec.StepReasoning}, // dup of replay → deduped
		{ID: "s3", Seq: 3, Kind: agentexec.StepToolCall},  // new → emitted
	}}
	s, _, _ := newTestStream(&fakeSteps{steps: replayed}, tail, nil, 0)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &recordingSSEWriter{}
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx, "t1", w) }()

	// Wait for the tailed step to arrive, then cancel.
	deadline := time.After(2 * time.Second)
	for len(w.events) < 3 {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for events: %d (%v)", len(w.events), w.events)
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("serve: %v", err)
	}

	got := append([]string(nil), w.events...)
	// s1 + s2 from the replay, s3 from the tail — and s2 NOT duplicated.
	if len(got) != 3 {
		t.Fatalf("events: got %d want 3 (%v)", len(got), got)
	}
	if !strings.Contains(got[0], `"id":"s1"`) || !strings.Contains(got[1], `"id":"s2"`) {
		t.Fatalf("replay order: %v", got)
	}
	if !strings.Contains(got[2], `"id":"s3"`) {
		t.Fatalf("tail merge: %v", got)
	}
	if strings.Count(strings.Join(got, ""), `"id":"s2"`) != 1 {
		t.Fatalf("dedup failed: %v", got)
	}
}

// TestStreamReplayFailureDegrades: a failed replay must not fail the stream —
// the tail still covers new steps.
func TestStreamReplayFailureDegrades(t *testing.T) {
	tail := &fakeTailer{steps: []agentexec.Step{{ID: "s3", Seq: 3}}}
	s, _, _ := newTestStream(&fakeSteps{err: errors.New("runner down")}, tail, nil, 0)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &recordingSSEWriter{}
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx, "t1", w) }()

	deadline := time.After(2 * time.Second)
	for len(w.events) < 1 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for the tailed step")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	<-done

	if !strings.Contains(w.events[0], `"id":"s3"`) {
		t.Fatalf("tail must still stream after replay failure: %v", w.events)
	}
}

// TestStreamTailUnavailable: a failed tailer creation writes the degraded
// error event after the replay and returns.
func TestStreamTailUnavailable(t *testing.T) {
	replayed := []agentexec.Step{{ID: "s1", Seq: 1}}
	s, _, _ := newTestStream(&fakeSteps{steps: replayed}, nil, errors.New("kafka down"), 0)

	w := &recordingSSEWriter{}
	err := s.Serve(context.Background(), "t1", w)

	if err != nil {
		t.Fatalf("serve: %v", err)
	}
	if len(w.events) != 2 {
		t.Fatalf("events: got %d want 2 (%v)", len(w.events), w.events)
	}
	if !strings.Contains(w.events[0], `"id":"s1"`) {
		t.Fatalf("replay missing: %v", w.events[0])
	}
	if !strings.Contains(w.events[1], "live tail unavailable") {
		t.Fatalf("degraded event missing: %v", w.events[1])
	}
}

// TestStreamEnded: when the tail ends (consumer group exits), the stream
// writes the terminal error event.
func TestStreamEnded(t *testing.T) {
	s, _, _ := newTestStream(&fakeSteps{}, &fakeTailer{done: true}, nil, 0)

	w := &recordingSSEWriter{}
	if err := s.Serve(context.Background(), "t1", w); err != nil {
		t.Fatalf("serve: %v", err)
	}
	if len(w.events) != 1 || !strings.Contains(w.events[0], "stream ended") {
		t.Fatalf("terminal event missing: %v", w.events)
	}
}

// TestStreamPing: keepalive pings are emitted on the interval while the tail
// runs.
func TestStreamPing(t *testing.T) {
	s, _, _ := newTestStream(&fakeSteps{}, &fakeTailer{}, nil, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &recordingSSEWriter{}
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx, "t1", w) }()

	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for a ping")
		case <-time.After(5 * time.Millisecond):
		}
		if len(w.events) >= 1 && strings.Contains(w.events[0], "event: ping") {
			cancel()
			<-done
			return
		}
	}
}

// TestStreamClientDisconnect: cancelling the context returns without a
// terminal event (client gone — nothing to write to).
func TestStreamClientDisconnect(t *testing.T) {
	s, _, _ := newTestStream(&fakeSteps{}, &fakeTailer{}, nil, time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	w := &recordingSSEWriter{}
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx, "t1", w) }()
	time.Sleep(20 * time.Millisecond)
	cancel()

	if err := <-done; err != nil {
		t.Fatalf("serve: %v", err)
	}
	if len(w.events) != 0 {
		t.Fatalf("no events expected on disconnect, got %v", w.events)
	}
}