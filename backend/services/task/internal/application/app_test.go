package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"

	"github.com/aaks/server/internal/contracts/agentexec"
	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/tasks"
	"github.com/aaks/server/services/task/internal/domain"
)

// ── Fakes ───────────────────────────────────────────────────────────────────

type fakeTasks struct {
	tasks  []tasks.Task
	next   int
	claims map[string]bool
	// fail injects an error into SetStatus/SetRoundNo to prove the
	// publish-after-commit ordering (no events on failed mutations).
	fail error
}

func (f *fakeTasks) find(id identity.ID) *tasks.Task {
	for i := range f.tasks {
		if f.tasks[i].ID == id {
			return &f.tasks[i]
		}
	}
	return nil
}

func (f *fakeTasks) List(_ context.Context, q domain.Query) ([]tasks.Task, error) {
	if len(q.Workspaces) == 0 {
		return []tasks.Task{}, nil // fail closed, mirroring the adapter
	}
	return f.tasks, nil
}

func (f *fakeTasks) Get(_ context.Context, id identity.ID, ws []identity.ID) (tasks.Task, error) {
	if len(ws) == 0 {
		return tasks.Task{}, domain.ErrNotFound
	}
	if t := f.find(id); t != nil {
		return *t, nil
	}
	return tasks.Task{}, domain.ErrNotFound
}

func (f *fakeTasks) GetUnscoped(_ context.Context, id identity.ID) (tasks.Task, error) {
	if t := f.find(id); t != nil {
		return *t, nil
	}
	return tasks.Task{}, domain.ErrNotFound
}

func (f *fakeTasks) Create(_ context.Context, workspaceID identity.ID, in domain.CreateInput) (tasks.Task, error) {
	f.next++
	t := tasks.Task{
		ID: identity.ID(fmt.Sprintf("t-%d", f.next)), WorkspaceID: workspaceID, ProjectID: in.ProjectID,
		Title: in.Title, Prompt: in.Prompt, Status: tasks.TaskBacklog, AgentID: in.AgentID,
		Labels: []string{}, RoundNo: 0,
	}
	f.tasks = append(f.tasks, t)
	return t, nil
}

func (f *fakeTasks) Update(_ context.Context, id identity.ID, _ []identity.ID, fields map[string]any) (tasks.Task, error) {
	t := f.find(id)
	if t == nil {
		return tasks.Task{}, domain.ErrNotFound
	}
	if v, ok := fields["title"].(string); ok {
		t.Title = v
	}
	return *t, nil
}

func (f *fakeTasks) SetStatus(_ context.Context, id identity.ID, status tasks.TaskStatus) (tasks.Task, error) {
	if f.fail != nil {
		return tasks.Task{}, f.fail
	}
	t := f.find(id)
	if t == nil {
		return tasks.Task{}, domain.ErrNotFound
	}
	t.Status = status
	return *t, nil
}

func (f *fakeTasks) SetRoundNo(_ context.Context, id identity.ID, roundNo int) error {
	if f.fail != nil {
		return f.fail
	}
	t := f.find(id)
	if t == nil {
		return domain.ErrNotFound
	}
	t.RoundNo = roundNo
	return nil
}

func (f *fakeTasks) Delete(_ context.Context, id identity.ID, _ []identity.ID) error {
	for i := range f.tasks {
		if f.tasks[i].ID == id {
			f.tasks = append(f.tasks[:i], f.tasks[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (f *fakeTasks) CountOpenByWorkspace(_ context.Context, _ identity.ID) (int, error) {
	n := 0
	for _, t := range f.tasks {
		if t.Status == tasks.TaskDoing || t.Status == tasks.TaskReview {
			n++
		}
	}
	return n, nil
}

func (f *fakeTasks) SagaNew(_ context.Context, taskID, runID identity.ID) (bool, error) {
	if f.claims == nil {
		f.claims = map[string]bool{}
	}
	key := string(taskID) + ":" + string(runID)
	if f.claims[key] {
		return false, nil
	}
	f.claims[key] = true
	return true, nil
}

type fakeFeedback struct {
	rows []tasks.Feedback
	next int
}

func (f *fakeFeedback) List(context.Context, identity.ID, []identity.ID) ([]tasks.Feedback, error) {
	return f.rows, nil
}

func (f *fakeFeedback) Add(_ context.Context, taskID identity.ID, _ []identity.ID, body string) (tasks.Feedback, error) {
	f.next++
	row := tasks.Feedback{ID: identity.ID(fmt.Sprintf("fb-%d", f.next)), TaskID: taskID, Author: "user", Body: body}
	f.rows = append(f.rows, row)
	return row, nil
}

type publishedEvent struct {
	topic string
	data  any
}

type fakePublisher struct {
	events []publishedEvent
}

func (p *fakePublisher) Publish(_ context.Context, topic string, data any, _ identity.ID) {
	p.events = append(p.events, publishedEvent{topic: topic, data: data})
}

func (p *fakePublisher) topics() []string {
	out := make([]string, 0, len(p.events))
	for _, e := range p.events {
		out = append(out, e.topic)
	}
	return out
}

func newTestApp() (*App, *fakeTasks, *fakeFeedback, *fakePublisher) {
	tf := &fakeTasks{}
	ff := &fakeFeedback{}
	p := &fakePublisher{}
	app := New(&Repository{Tasks: tf, Feedback: ff}, p, slog.New(slog.DiscardHandler))
	return app, tf, ff, p
}

// seedTask inserts a task into the fake store and returns it.
func seedTask(f *fakeTasks, id identity.ID, status tasks.TaskStatus, round int, agent *identity.ID) tasks.Task {
	t := tasks.Task{
		ID: id, WorkspaceID: "ws1", ProjectID: "p1", Title: "task", Prompt: "do it",
		Status: status, RoundNo: round, AgentID: agent, Labels: []string{},
	}
	f.tasks = append(f.tasks, t)
	return t
}

func agent(id string) *identity.ID {
	a := identity.ID(id)
	return &a
}

// ── Query / CRUD ────────────────────────────────────────────────────────────

func TestCreateTask(t *testing.T) {
	app, tf, _, p := newTestApp()
	agentID := identity.ID("a1")

	created, err := app.Create(context.Background(), "ws1", domain.CreateInput{
		ProjectID: "p1", Title: "task", Prompt: "do it", AgentID: &agentID,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Status != tasks.TaskBacklog || created.ID == "" || created.AgentID == nil || *created.AgentID != agentID {
		t.Fatalf("unexpected created task: %+v", created)
	}
	if len(p.events) != 0 {
		t.Fatalf("create must not publish events, got %v", p.topics())
	}
	if len(tf.tasks) != 1 || tf.tasks[0].Labels == nil {
		t.Fatalf("task must be stored with labels initialized, got %+v", tf.tasks)
	}
}

func TestGetUpdateDeletePassthrough(t *testing.T) {
	app, tf, _, _ := newTestApp()
	seedTask(tf, "t1", tasks.TaskBacklog, 0, agent("a1"))

	got, err := app.Get(context.Background(), "t1", []identity.ID{"ws1"})
	if err != nil || got.Title != "task" {
		t.Fatalf("get: %v %+v", err, got)
	}
	if _, err := app.Get(context.Background(), "missing", []identity.ID{"ws1"}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing task must be ErrNotFound, got %v", err)
	}

	updated, err := app.Update(context.Background(), "t1", []identity.ID{"ws1"}, map[string]any{"title": "renamed"})
	if err != nil || updated.Title != "renamed" {
		t.Fatalf("update: %v %+v", err, updated)
	}

	if err := app.Delete(context.Background(), "t1", []identity.ID{"ws1"}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(tf.tasks) != 0 {
		t.Fatalf("task must be gone after delete, got %d", len(tf.tasks))
	}
}

func TestListFeedbackAndAdd(t *testing.T) {
	app, _, ff, _ := newTestApp()

	empty, err := app.ListFeedback(context.Background(), "t1", []identity.ID{"ws1"})
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty feedback must be [], got %v (%v)", empty, err)
	}

	row, err := app.AddFeedback(context.Background(), "t1", []identity.ID{"ws1"}, "please fix")
	if err != nil {
		t.Fatalf("add feedback: %v", err)
	}
	if row.Author != "user" || row.Body != "please fix" || len(ff.rows) != 1 {
		t.Fatalf("unexpected feedback: %+v", row)
	}
}

// ── PATCH status → saga commands ────────────────────────────────────────────

func TestPatchStatusDoingPublishesRunRequested(t *testing.T) {
	app, tf, _, p := newTestApp()
	seedTask(tf, "t1", tasks.TaskBacklog, 0, agent("a1"))

	out, err := app.PatchStatus(context.Background(), "t1", []identity.ID{"ws1"}, tasks.TaskDoing)
	if err != nil {
		t.Fatalf("patch status: %v", err)
	}
	if out.Status != tasks.TaskDoing || tf.tasks[0].Status != tasks.TaskDoing {
		t.Fatalf("task must be persisted as doing, got %+v", tf.tasks[0])
	}
	if len(p.events) != 2 || p.events[0].topic != events.TopicTaskStatusChanged || p.events[1].topic != events.TopicTaskRunRequested {
		t.Fatalf("expected status-changed then run-requested, got %v", p.topics())
	}
	sc := p.events[0].data.(events.TaskStatusChangedData)
	if sc.From != tasks.TaskBacklog || sc.To != tasks.TaskDoing || sc.RoundNo != 0 {
		t.Fatalf("unexpected status-changed: %+v", sc)
	}
	rr := p.events[1].data.(events.RunRequestedData)
	if rr.TaskID != "t1" || rr.AgentID != "a1" || rr.ProjectID != "p1" || rr.RoundNo != 0 || rr.Prompt != "do it" {
		t.Fatalf("unexpected run-requested: %+v", rr)
	}
}

func TestPatchStatusDoingWithoutAgentBlocks(t *testing.T) {
	app, tf, _, p := newTestApp()
	seedTask(tf, "t1", tasks.TaskBacklog, 0, nil)

	out, err := app.PatchStatus(context.Background(), "t1", []identity.ID{"ws1"}, tasks.TaskDoing)
	if err != nil {
		t.Fatalf("patch status: %v", err)
	}
	// The response carries the requested status (pre-blocked), mirroring the
	// pre-refactor handler; the persisted task is blocked.
	if out.Status != tasks.TaskDoing {
		t.Fatalf("response must carry the requested doing status, got %s", out.Status)
	}
	if tf.tasks[0].Status != tasks.TaskBlocked {
		t.Fatalf("un-runnable task must be persisted as blocked, got %s", tf.tasks[0].Status)
	}
	want := []string{events.TopicTaskStatusChanged, events.TopicTaskStatusChanged}
	if len(p.events) != 2 || p.events[0].topic != want[0] || p.events[1].topic != want[1] {
		t.Fatalf("expected two status-changed facts, got %v", p.topics())
	}
	if p.events[0].data.(events.TaskStatusChangedData).To != tasks.TaskDoing {
		t.Fatalf("first fact must be backlog→doing, got %+v", p.events[0].data)
	}
	if p.events[1].data.(events.TaskStatusChangedData).To != tasks.TaskBlocked {
		t.Fatalf("second fact must be doing→blocked, got %+v", p.events[1].data)
	}
}

func TestPatchStatusNoopEmitsNothing(t *testing.T) {
	app, tf, _, p := newTestApp()
	seedTask(tf, "t1", tasks.TaskDoing, 1, agent("a1"))

	out, err := app.PatchStatus(context.Background(), "t1", []identity.ID{"ws1"}, tasks.TaskDoing)
	if err != nil {
		t.Fatalf("patch status: %v", err)
	}
	if out.Status != tasks.TaskDoing {
		t.Fatalf("status must be unchanged, got %s", out.Status)
	}
	if len(p.events) != 0 {
		t.Fatalf("noop patch must not re-emit events, got %v", p.topics())
	}
}

func TestPatchStatusStoppedPublishesStopRequested(t *testing.T) {
	app, tf, _, p := newTestApp()
	seedTask(tf, "t1", tasks.TaskDoing, 1, agent("a1"))

	out, err := app.PatchStatus(context.Background(), "t1", []identity.ID{"ws1"}, tasks.TaskStopped)
	if err != nil {
		t.Fatalf("patch status: %v", err)
	}
	if out.Status != tasks.TaskStopped || tf.tasks[0].Status != tasks.TaskStopped {
		t.Fatalf("task must be persisted as stopped, got %+v", tf.tasks[0])
	}
	want := []string{events.TopicTaskStatusChanged, events.TopicTaskStopRequested}
	if len(p.events) != 2 || p.events[0].topic != want[0] || p.events[1].topic != want[1] {
		t.Fatalf("expected status-changed then stop-requested, got %v", p.topics())
	}
	if p.events[0].data.(events.TaskStatusChangedData).To != tasks.TaskStopped {
		t.Fatalf("fact must be doing→stopped, got %+v", p.events[0].data)
	}
}

// ── Publish-after-commit ordering ───────────────────────────────────────────

func TestFailedTransitionPublishesNothing(t *testing.T) {
	app, tf, _, p := newTestApp()
	seedTask(tf, "t1", tasks.TaskBacklog, 0, agent("a1"))
	tf.fail = errors.New("db unavailable")

	if _, err := app.PatchStatus(context.Background(), "t1", []identity.ID{"ws1"}, tasks.TaskDoing); err == nil {
		t.Fatal("failed transition must surface an error")
	}
	if len(p.events) != 0 {
		t.Fatalf("no events may be published when the commit fails, got %v", p.topics())
	}
	if tf.tasks[0].Status != tasks.TaskBacklog {
		t.Fatalf("status must not change on failure, got %s", tf.tasks[0].Status)
	}
}

func TestFailedVerdictMutationPublishesNothing(t *testing.T) {
	app, tf, _, p := newTestApp()
	seedTask(tf, "t1", tasks.TaskReview, 1, agent("a1"))
	tf.fail = errors.New("db unavailable")

	err := app.Dispatch(context.Background(), events.EventEnvelope{
		EventType: events.TopicVerdict,
		Data:      events.VerdictData{TaskID: "t1", RunID: "r1", RoundNo: 1, Decision: agentexec.VerdictApprove},
	})
	if err == nil {
		t.Fatal("failed verdict mutation must surface an error")
	}
	if len(p.events) != 0 {
		t.Fatalf("no events may be published when the commit fails, got %v", p.topics())
	}
}

// ── run.completed → review ──────────────────────────────────────────────────

func TestRunCompletedAdvancesToReview(t *testing.T) {
	app, tf, _, p := newTestApp()
	seedTask(tf, "t1", tasks.TaskDoing, 2, agent("a1"))

	err := app.Dispatch(context.Background(), events.EventEnvelope{
		EventType: events.TopicRunCompleted,
		Data: events.RunCompletedData{
			TaskID: "t1", RunID: "r1", AgentID: "a1",
			Role: agentexec.RunRoleImplementer, Status: agentexec.RunDone, RoundNo: 2,
		},
	})
	if err != nil {
		t.Fatalf("dispatch run.completed: %v", err)
	}
	if tf.tasks[0].Status != tasks.TaskReview {
		t.Fatalf("task must advance to review, got %s", tf.tasks[0].Status)
	}
	want := []string{events.TopicTaskStatusChanged, events.TopicTaskReviewRequested}
	if len(p.events) != 2 || p.events[0].topic != want[0] || p.events[1].topic != want[1] {
		t.Fatalf("expected status-changed then review-requested, got %v", p.topics())
	}
	sc := p.events[0].data.(events.TaskStatusChangedData)
	if sc.From != tasks.TaskDoing || sc.To != tasks.TaskReview || sc.RoundNo != 2 {
		t.Fatalf("unexpected status-changed: %+v", sc)
	}
	rv := p.events[1].data.(events.ReviewRequestedData)
	if rv.TaskID != "t1" || rv.AgentID != "a1" || rv.RunID != "r1" || rv.WorkspaceID != "ws1" || rv.RoundNo != 2 || rv.Prompt != "do it" {
		t.Fatalf("unexpected review-requested: %+v", rv)
	}
}

func TestRunCompletedRedeliveryIsDeduped(t *testing.T) {
	app, tf, _, p := newTestApp()
	seedTask(tf, "t1", tasks.TaskDoing, 1, agent("a1"))
	msg := events.EventEnvelope{
		EventType: events.TopicRunCompleted,
		Data: events.RunCompletedData{
			TaskID: "t1", RunID: "r1", Role: agentexec.RunRoleImplementer,
			Status: agentexec.RunDone, RoundNo: 1,
		},
	}

	if err := app.Dispatch(context.Background(), msg); err != nil {
		t.Fatalf("first dispatch: %v", err)
	}
	first := len(p.events)
	if err := app.Dispatch(context.Background(), msg); err != nil {
		t.Fatalf("redelivered dispatch: %v", err)
	}
	if len(p.events) != first {
		t.Fatalf("redelivery must not re-emit events, got %v", p.topics())
	}
	if tf.tasks[0].Status != tasks.TaskReview {
		t.Fatalf("task must stay in review, got %s", tf.tasks[0].Status)
	}
}

func TestRunCompletedNonImplementerIgnored(t *testing.T) {
	app, tf, _, p := newTestApp()
	seedTask(tf, "t1", tasks.TaskDoing, 1, agent("a1"))

	for _, role := range []agentexec.RunRole{agentexec.RunRoleReviewer} {
		err := app.Dispatch(context.Background(), events.EventEnvelope{
			EventType: events.TopicRunCompleted,
			Data:      events.RunCompletedData{TaskID: "t1", RunID: "r1", Role: role, Status: agentexec.RunDone, RoundNo: 1},
		})
		if err != nil {
			t.Fatalf("dispatch %s: %v", role, err)
		}
	}
	err := app.Dispatch(context.Background(), events.EventEnvelope{
		EventType: events.TopicRunCompleted,
		Data:      events.RunCompletedData{TaskID: "t1", RunID: "r2", Role: agentexec.RunRoleImplementer, Status: agentexec.RunAborted, RoundNo: 1},
	})
	if err != nil {
		t.Fatalf("dispatch aborted: %v", err)
	}
	if tf.tasks[0].Status != tasks.TaskDoing || len(p.events) != 0 {
		t.Fatalf("reviewer/aborted runs must not advance the task, status=%s events=%v", tf.tasks[0].Status, p.topics())
	}
}

func TestRunCompletedNotDoingIgnored(t *testing.T) {
	app, tf, _, p := newTestApp()
	seedTask(tf, "t1", tasks.TaskBacklog, 1, agent("a1"))

	err := app.Dispatch(context.Background(), events.EventEnvelope{
		EventType: events.TopicRunCompleted,
		Data:      events.RunCompletedData{TaskID: "t1", RunID: "r1", Role: agentexec.RunRoleImplementer, Status: agentexec.RunDone, RoundNo: 1},
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if tf.tasks[0].Status != tasks.TaskBacklog || len(p.events) != 0 {
		t.Fatalf("run completion for a non-doing task must be ignored, status=%s events=%v", tf.tasks[0].Status, p.topics())
	}
}

// ── verdict → done / re-round / blocked ─────────────────────────────────────

func TestVerdictApprove(t *testing.T) {
	app, tf, _, p := newTestApp()
	seedTask(tf, "t1", tasks.TaskReview, 3, agent("a1"))

	err := app.Dispatch(context.Background(), events.EventEnvelope{
		EventType: events.TopicVerdict,
		Data:      events.VerdictData{TaskID: "t1", RunID: "r1", RoundNo: 3, Decision: agentexec.VerdictApprove},
	})
	if err != nil {
		t.Fatalf("dispatch verdict: %v", err)
	}
	if tf.tasks[0].Status != tasks.TaskDone {
		t.Fatalf("approved task must be done, got %s", tf.tasks[0].Status)
	}
	if len(p.events) != 1 || p.events[0].topic != events.TopicTaskStatusChanged {
		t.Fatalf("expected a single status-changed fact, got %v", p.topics())
	}
	sc := p.events[0].data.(events.TaskStatusChangedData)
	if sc.From != tasks.TaskReview || sc.To != tasks.TaskDone || sc.RoundNo != 3 {
		t.Fatalf("unexpected status-changed: %+v", sc)
	}
}

func TestVerdictRequestChangesNextRound(t *testing.T) {
	app, tf, _, p := newTestApp()
	seedTask(tf, "t1", tasks.TaskReview, 1, agent("a1"))

	err := app.Dispatch(context.Background(), events.EventEnvelope{
		EventType: events.TopicVerdict,
		Data:      events.VerdictData{TaskID: "t1", RunID: "r1", RoundNo: 1, Decision: agentexec.VerdictRequestChanges},
	})
	if err != nil {
		t.Fatalf("dispatch verdict: %v", err)
	}
	if tf.tasks[0].Status != tasks.TaskDoing || tf.tasks[0].RoundNo != 2 {
		t.Fatalf("task must return to doing on round %d, got status=%s round=%d", 2, tf.tasks[0].Status, tf.tasks[0].RoundNo)
	}
	want := []string{events.TopicTaskStatusChanged, events.TopicTaskRunRequested}
	if len(p.events) != 2 || p.events[0].topic != want[0] || p.events[1].topic != want[1] {
		t.Fatalf("expected status-changed then run-requested, got %v", p.topics())
	}
	sc := p.events[0].data.(events.TaskStatusChangedData)
	if sc.From != tasks.TaskReview || sc.To != tasks.TaskDoing || sc.RoundNo != 2 {
		t.Fatalf("unexpected status-changed: %+v", sc)
	}
	rr := p.events[1].data.(events.RunRequestedData)
	if rr.TaskID != "t1" || rr.AgentID != "a1" || rr.ProjectID != "p1" || rr.WorkspaceID != "ws1" || rr.RoundNo != 2 || rr.Prompt != "do it" {
		t.Fatalf("unexpected run-requested: %+v", rr)
	}
}

func TestVerdictRequestChangesRoundsExhausted(t *testing.T) {
	app, tf, _, p := newTestApp()
	seedTask(tf, "t1", tasks.TaskReview, maxReviewRounds, agent("a1"))

	err := app.Dispatch(context.Background(), events.EventEnvelope{
		EventType: events.TopicVerdict,
		Data:      events.VerdictData{TaskID: "t1", RunID: "r1", RoundNo: maxReviewRounds, Decision: agentexec.VerdictRequestChanges},
	})
	if err != nil {
		t.Fatalf("dispatch verdict: %v", err)
	}
	if tf.tasks[0].Status != tasks.TaskBlocked {
		t.Fatalf("round-exhausted task must be blocked, got %s", tf.tasks[0].Status)
	}
	if len(p.events) != 1 || p.events[0].topic != events.TopicTaskStatusChanged {
		t.Fatalf("expected a single status-changed fact, got %v", p.topics())
	}
	sc := p.events[0].data.(events.TaskStatusChangedData)
	if sc.From != tasks.TaskReview || sc.To != tasks.TaskBlocked || sc.RoundNo != maxReviewRounds {
		t.Fatalf("unexpected status-changed: %+v", sc)
	}
}

func TestVerdictIgnoredNotInReview(t *testing.T) {
	app, tf, _, p := newTestApp()
	seedTask(tf, "t1", tasks.TaskBacklog, 0, agent("a1"))

	err := app.Dispatch(context.Background(), events.EventEnvelope{
		EventType: events.TopicVerdict,
		Data:      events.VerdictData{TaskID: "t1", RunID: "r1", RoundNo: 0, Decision: agentexec.VerdictApprove},
	})
	if err != nil {
		t.Fatalf("dispatch verdict: %v", err)
	}
	if tf.tasks[0].Status != tasks.TaskBacklog || len(p.events) != 0 {
		t.Fatalf("verdict for a non-review task must be ignored, status=%s events=%v", tf.tasks[0].Status, p.topics())
	}
}

func TestPrOpenedIsLoggedOnly(t *testing.T) {
	app, tf, _, p := newTestApp()
	seedTask(tf, "t1", tasks.TaskDone, 1, agent("a1"))

	err := app.Dispatch(context.Background(), events.EventEnvelope{
		EventType: events.TopicPrOpened,
		Data:      events.PrOpenedData{TaskID: "t1", RunID: "r1", URL: "https://github.com/x/y/pull/1"},
	})
	if err != nil {
		t.Fatalf("dispatch pr.opened: %v", err)
	}
	if tf.tasks[0].Status != tasks.TaskDone || len(p.events) != 0 {
		t.Fatalf("pr.opened must not mutate the task, status=%s events=%v", tf.tasks[0].Status, p.topics())
	}
}

// ── Saga actions (re-run / stop / open-pr) ──────────────────────────────────

func TestReRun(t *testing.T) {
	app, tf, _, p := newTestApp()
	seedTask(tf, "t1", tasks.TaskDoing, 0, agent("a1"))

	out, err := app.ReRun(context.Background(), "t1", []identity.ID{"ws1"})
	if err != nil {
		t.Fatalf("re-run: %v", err)
	}
	if out.RoundNo != 1 || tf.tasks[0].RoundNo != 1 || tf.tasks[0].Status != tasks.TaskDoing {
		t.Fatalf("task must advance to round 1 in doing, got %+v", tf.tasks[0])
	}
	want := []string{events.TopicTaskStatusChanged, events.TopicTaskRunRequested}
	if len(p.events) != 2 || p.events[0].topic != want[0] || p.events[1].topic != want[1] {
		t.Fatalf("expected status-changed then run-requested, got %v", p.topics())
	}
	sc := p.events[0].data.(events.TaskStatusChangedData)
	if sc.From != tasks.TaskDoing || sc.To != tasks.TaskDoing || sc.RoundNo != 1 {
		t.Fatalf("unexpected status-changed: %+v", sc)
	}
	rr := p.events[1].data.(events.RunRequestedData)
	if rr.RoundNo != 1 || rr.AgentID != "a1" {
		t.Fatalf("unexpected run-requested: %+v", rr)
	}
}

func TestReRunNoAgent(t *testing.T) {
	app, tf, _, p := newTestApp()
	seedTask(tf, "t1", tasks.TaskDoing, 0, nil)

	if _, err := app.ReRun(context.Background(), "t1", []identity.ID{"ws1"}); !errors.Is(err, domain.ErrNoAgent) {
		t.Fatalf("unassigned re-run must be rejected with ErrNoAgent, got %v", err)
	}
	if len(p.events) != 0 {
		t.Fatalf("rejected re-run must not publish, got %v", p.topics())
	}
}

func TestStopPublishesStatusChangedAndStopRequested(t *testing.T) {
	app, tf, _, p := newTestApp()
	seedTask(tf, "t1", tasks.TaskDoing, 2, agent("a1"))

	out, err := app.Stop(context.Background(), "t1", []identity.ID{"ws1"})
	if err != nil {
		t.Fatalf("stop: %v", err)
	}
	if out.Status != tasks.TaskStopped || tf.tasks[0].Status != tasks.TaskStopped {
		t.Fatalf("task must be persisted as stopped, got %+v", tf.tasks[0])
	}
	want := []string{events.TopicTaskStatusChanged, events.TopicTaskStopRequested}
	if len(p.events) != 2 || p.events[0].topic != want[0] || p.events[1].topic != want[1] {
		t.Fatalf("expected status-changed then stop-requested, got %v", p.topics())
	}
}

func TestStopAlreadyStoppedPublishesOnlyStopRequested(t *testing.T) {
	app, tf, _, p := newTestApp()
	seedTask(tf, "t1", tasks.TaskStopped, 2, agent("a1"))

	if _, err := app.Stop(context.Background(), "t1", []identity.ID{"ws1"}); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if len(p.events) != 1 || p.events[0].topic != events.TopicTaskStopRequested {
		t.Fatalf("noop stop must publish only stop-requested, got %v", p.topics())
	}
}

func TestOpenPr(t *testing.T) {
	app, tf, _, p := newTestApp()
	seedTask(tf, "t1", tasks.TaskDone, 1, agent("a1"))
	seedTask(tf, "t2", tasks.TaskBacklog, 0, agent("a1"))

	if err := app.OpenPr(context.Background(), "t1", []identity.ID{"ws1"}); err != nil {
		t.Fatalf("open-pr on done task: %v", err)
	}
	if len(p.events) != 1 || p.events[0].topic != events.TopicPrOpenRequested {
		t.Fatalf("expected pr-open-requested, got %v", p.topics())
	}
	if err := app.OpenPr(context.Background(), "t2", []identity.ID{"ws1"}); !errors.Is(err, domain.ErrNotDone) {
		t.Fatalf("open-pr on non-done task must be ErrNotDone, got %v", err)
	}
	if len(p.events) != 1 {
		t.Fatalf("rejected open-pr must not publish, got %v", p.topics())
	}
}

// ── Internal surface ────────────────────────────────────────────────────────

func TestOpenTaskCountAndTaskWorkspace(t *testing.T) {
	app, tf, _, _ := newTestApp()
	seedTask(tf, "t1", tasks.TaskDoing, 1, agent("a1"))
	seedTask(tf, "t2", tasks.TaskReview, 1, agent("a1"))
	seedTask(tf, "t3", tasks.TaskDone, 1, agent("a1"))

	n, err := app.OpenTaskCount(context.Background(), "ws1")
	if err != nil || n != 2 {
		t.Fatalf("open task count must be 2 (doing+review), got %d (%v)", n, err)
	}
	ws, err := app.TaskWorkspace(context.Background(), "t1")
	if err != nil || ws != "ws1" {
		t.Fatalf("task workspace: %v %q", err, ws)
	}
	if _, err := app.TaskWorkspace(context.Background(), "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing task workspace must be ErrNotFound, got %v", err)
	}
}
