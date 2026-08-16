package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/aaks/server/internal/contracts/agentexec"
	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/resources"
	"github.com/aaks/server/services/runner/internal/driver"
	"github.com/aaks/server/services/runner/internal/domain"
)

// ── Fakes ───────────────────────────────────────────────────────────────────

type fakeRuns struct {
	runs []agentexec.Run
	next int
	mu   sync.Mutex
}

func (f *fakeRuns) CreateRun(_ context.Context, taskID identity.ID, role agentexec.RunRole, agentID identity.ID, model string, roundNo int) (identity.ID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.next++
	id := identity.ID("run-" + fmt.Sprint(f.next))
	f.runs = append(f.runs, agentexec.Run{ID: id, TaskID: taskID, Role: role, AgentID: agentID, Model: model, RoundNo: roundNo})
	return id, nil
}
func (f *fakeRuns) ListRunsByTask(context.Context, identity.ID) ([]agentexec.Run, error) { return f.runs, nil }
func (f *fakeRuns) GetRun(_ context.Context, id identity.ID) (agentexec.Run, error) {
	for _, r := range f.runs {
		if r.ID == id {
			return r, nil
		}
	}
	return agentexec.Run{}, domain.ErrNotFound
}
func (f *fakeRuns) LatestRun(_ context.Context, taskID identity.ID) (agentexec.Run, error) {
	for i := len(f.runs) - 1; i >= 0; i-- {
		if f.runs[i].TaskID == taskID {
			return f.runs[i], nil
		}
	}
	return agentexec.Run{}, domain.ErrNotFound
}
func (f *fakeRuns) FinishRun(_ context.Context, runID identity.ID, status agentexec.RunStatus, tokenUsage int, errMsg string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.runs {
		if f.runs[i].ID == runID {
			f.runs[i].Status = status
			f.runs[i].TokenUsage = tokenUsage
			f.runs[i].Error = errMsg
			return nil
		}
	}
	return domain.ErrNotFound
}

type fakeSteps struct{}

func (f *fakeSteps) AppendStep(context.Context, identity.ID, int, agentexec.StepKind, []byte) (agentexec.Step, error) {
	return agentexec.Step{}, nil
}
func (f *fakeSteps) ListSteps(context.Context, identity.ID) ([]agentexec.Step, error) { return nil, nil }
func (f *fakeSteps) ListStepsByTask(context.Context, identity.ID) ([]agentexec.Step, error) {
	return nil, nil
}
func (f *fakeSteps) MaxStepSeq(context.Context, identity.ID) (int, error) { return 0, nil }

type fakeFindings struct{}

func (f *fakeFindings) AddFinding(context.Context, identity.ID, agentexec.Finding) (agentexec.Finding, error) {
	return agentexec.Finding{}, nil
}
func (f *fakeFindings) ListFindings(context.Context, identity.ID) ([]agentexec.Finding, error) {
	return nil, nil
}

type fakeArtifacts struct{}

func (f *fakeArtifacts) AddArtifact(context.Context, identity.ID, *identity.ID, string, string, string, int, int) (agentexec.Artifact, error) {
	return agentexec.Artifact{}, nil
}
func (f *fakeArtifacts) ListArtifactsByTask(context.Context, identity.ID) ([]agentexec.Artifact, error) {
	return nil, nil
}

// fakeDriver returns a canned result, optionally honouring context cancellation.
type fakeDriver struct {
	res       driver.Result
	err       error
	executed  chan struct{}
	honorCtx  bool
	mu        sync.Mutex
	execCount int
}

func (d *fakeDriver) Execute(ctx context.Context, rc driver.RunContext, sink driver.StepSink) (driver.Result, error) {
	d.mu.Lock()
	d.execCount++
	d.mu.Unlock()
	if d.executed != nil {
		close(d.executed)
	}
	if d.honorCtx {
		<-ctx.Done()
		return driver.Result{Status: agentexec.RunStopped}, ctx.Err()
	}
	return d.res, d.err
}

type fakePub struct {
	mu     sync.Mutex
	events []string
}

func (p *fakePub) Publish(_ context.Context, topic string, _ any, _ identity.ID) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, topic)
}

func (p *fakePub) topics() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.events...)
}

type fakeSettings struct{ key string; err error }

func (f *fakeSettings) FetchKey(context.Context, string) (string, error) { return f.key, f.err }

type fakeResources struct{ rules []string; err error }

func (f *fakeResources) FetchEnabledRules(context.Context, identity.ID) ([]string, error) {
	return f.rules, f.err
}

type fakeAgents struct{ servers []resources.McpServer; err error }

func (f *fakeAgents) FetchMcpServers(context.Context, identity.ID) ([]resources.McpServer, error) {
	return f.servers, f.err
}

type fakeTools struct{}

func (f *fakeTools) SetupTools(context.Context, identity.ID, identity.ID, identity.ID, string) (driver.ToolSet, func()) {
	return driver.ToolSet{}, nil
}

func newTestRunner(settings SettingsKeyClient, resources ResourcesRulesClient) (*Runner, *fakePub, *fakeDriver) {
	pub := &fakePub{}
	d := &fakeDriver{res: driver.Result{Status: agentexec.RunDone, TokenUsage: 12}}
	r := New(
		&fakeRuns{}, &fakeSteps{}, &fakeFindings{}, &fakeArtifacts{},
		d, driver.Caps{MaxSteps: 5},
		settings, resources, &fakeAgents{}, &fakeTools{}, pub,
		slog.New(slog.DiscardHandler), "",
	)
	return r, pub, d
}

// ── Run orchestration ───────────────────────────────────────────────────────

func TestStartImplementerEmitsCompletionFacts(t *testing.T) {
	r, pub, _ := newTestRunner(&fakeSettings{key: "k"}, &fakeResources{rules: []string{"no-secrets"}})

	r.StartImplementer(context.Background(), events.RunRequestedData{
		TaskID: "t1", AgentID: "a1", ModelOverride: "gpt-x", RoundNo: 1, WorkspaceID: "ws1",
	})
	time.Sleep(20 * time.Millisecond)

	got := pub.topics()
	want := []string{events.TopicRunStarted, events.TopicRunCompleted}
	for _, w := range want {
		found := false
		for _, g := range got {
			if g == w {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected topic %s in published %v", w, got)
		}
	}
}

func TestStartReviewerEmitsVerdict(t *testing.T) {
	settings := &fakeSettings{key: "k"}
	r, pub, d := newTestRunner(settings, &fakeResources{})
	d.res = driver.Result{Status: agentexec.RunDone, Verdict: "APPROVE", VerdictSummary: "looks good"}

	// Pre-seed the implementer run the reviewer reads.
	r.StartImplementer(context.Background(), events.RunRequestedData{TaskID: "t1", AgentID: "a1", RoundNo: 1})
	time.Sleep(10 * time.Millisecond)
	run := r.runs.(*fakeRuns).runs[0]

	r.StartReviewer(context.Background(), events.ReviewRequestedData{
		TaskID: "t1", RunID: run.ID, AgentID: "a2", RoundNo: 1,
	})
	time.Sleep(20 * time.Millisecond)

	got := pub.topics()
	found := false
	for _, g := range got {
		if g == events.TopicVerdict {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected verdict topic in %v", got)
	}
}

func TestDriverErrorMarksRunAborted(t *testing.T) {
	r, _, d := newTestRunner(&fakeSettings{}, &fakeResources{})
	d.err = errors.New("llm boom")

	r.StartImplementer(context.Background(), events.RunRequestedData{TaskID: "t1", AgentID: "a1"})
	time.Sleep(20 * time.Millisecond)

	runs := r.runs.(*fakeRuns).runs
	if len(runs) == 0 || runs[0].Status != agentexec.RunAborted {
		t.Fatalf("run must be finished aborted, got %+v", runs)
	}
}

// ── PR-open flow ────────────────────────────────────────────────────────────

func TestOpenPrEmitsPrOpened(t *testing.T) {
	r, pub, _ := newTestRunner(&fakeSettings{}, &fakeResources{})
	r.prBaseURL = "https://github.com/acme/repo"

	r.StartImplementer(context.Background(), events.RunRequestedData{TaskID: "t1", AgentID: "a1"})
	time.Sleep(10 * time.Millisecond)

	err := r.OpenPr(context.Background(), events.PrOpenRequestedData{TaskID: "t1"})
	if err != nil {
		t.Fatalf("open pr: %v", err)
	}
	found := false
	for _, g := range pub.topics() {
		if g == events.TopicPrOpened {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected pr.opened in %v", pub.topics())
	}
}

func TestOpenPrWithoutRunIsNoOp(t *testing.T) {
	r, pub, _ := newTestRunner(&fakeSettings{}, &fakeResources{})
	if err := r.OpenPr(context.Background(), events.PrOpenRequestedData{TaskID: "nope"}); err != nil {
		t.Fatalf("open pr without run must not error: %v", err)
	}
	if len(pub.topics()) != 0 {
		t.Fatalf("no events expected, got %v", pub.topics())
	}
}

// ── In-flight guard + cancellation ──────────────────────────────────────────

func TestDispatchSkipsConcurrentRun(t *testing.T) {
	r, _, d := newTestRunner(&fakeSettings{}, &fakeResources{})
	d.honorCtx = true
	d.executed = make(chan struct{})

	r.startRun(context.Background(), "t1", func(ctx context.Context) { _, _ = d.Execute(ctx, driver.RunContext{}, nil) })
	r.startRun(context.Background(), "t1", func(ctx context.Context) { _, _ = d.Execute(ctx, driver.RunContext{}, nil) })
	<-d.executed
	time.Sleep(20 * time.Millisecond)

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.execCount != 1 {
		t.Fatalf("second dispatch must be skipped, executed %d times", d.execCount)
	}
}

func TestCancelTaskCancelsInFlightRun(t *testing.T) {
	r, _, d := newTestRunner(&fakeSettings{}, &fakeResources{})
	d.honorCtx = true
	d.executed = make(chan struct{})

	r.startRun(context.Background(), "t1", func(ctx context.Context) { _, _ = d.Execute(ctx, driver.RunContext{}, nil) })
	<-d.executed
	r.CancelTask("t1")
	time.Sleep(20 * time.Millisecond)

	// The goroutine finishes (ctx cancelled) and unregisters the task.
	if _, ok := r.cancels.Load("t1"); ok {
		t.Fatal("cancelled task must be unregistered")
	}
}