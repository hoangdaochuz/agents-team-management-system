package store

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/tasks"
	"github.com/aaks/server/services/task/internal/domain"
)

func testLogger(t *testing.T) *slog.Logger {
	t.Helper()
	var buf bytes.Buffer
	return slog.New(slog.NewJSONHandler(&buf, nil))
}

// TestTaskCRUD exercises the task lifecycle against a real task_db, including
// the workspace-scoping guards and the saga status transitions.
// Skipped unless AAKS_TASK_TEST_DSN is set, so `go test ./...` stays green
// without infrastructure.
func TestTaskCRUD(t *testing.T) {
	dsn := os.Getenv("AAKS_TASK_TEST_DSN")
	if dsn == "" {
		t.Skip("AAKS_TASK_TEST_DSN unset; skipping task integration test")
	}
	log := testLogger(t)
	ctx := context.Background()

	st, err := New(ctx, dsn, log)
	if err != nil {
		t.Fatalf("store new: %v", err)
	}
	defer st.Close()

	wsA := identity.ID("11111111-1111-1111-1111-111111111111")
	wsB := identity.ID("22222222-2222-2222-2222-222222222222")
	proj := identity.ID("33333333-3333-3333-3333-333333333333")

	created, err := st.Tasks.Create(ctx, wsA, domain.CreateInput{
		ProjectID: proj, Title: "task-test", Prompt: "Do the thing.",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == "" || created.WorkspaceID != wsA || created.Status != tasks.TaskBacklog {
		t.Fatalf("unexpected created task: %+v", created)
	}

	got, err := st.Tasks.Get(ctx, created.ID, []identity.ID{wsA})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != "task-test" {
		t.Fatalf("get mismatch: %+v", got)
	}

	if _, err := st.Tasks.Get(ctx, created.ID, []identity.ID{wsB}); err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound for cross-workspace get, got %v", err)
	}

	listed, err := st.Tasks.List(ctx, domain.Query{Workspaces: []identity.ID{wsA}})
	if err != nil || len(listed) == 0 {
		t.Fatalf("list: %v (n=%d)", err, len(listed))
	}
	if other, err := st.Tasks.List(ctx, domain.Query{Workspaces: []identity.ID{wsB}}); err != nil || len(other) != 0 {
		t.Fatalf("cross-workspace list must be empty: %v (n=%d)", err, len(other))
	}
	// Fail closed: no workspace context returns nothing, never everything.
	if all, err := st.Tasks.List(ctx, domain.Query{}); err != nil || len(all) != 0 {
		t.Fatalf("unscoped list must be empty: %v (n=%d)", err, len(all))
	}

	status, err := st.Tasks.SetStatus(ctx, created.ID, tasks.TaskDoing)
	if err != nil {
		t.Fatalf("set status: %v", err)
	}
	if status.Status != tasks.TaskDoing {
		t.Fatalf("status transition did not apply: %+v", status)
	}

	if err := st.Tasks.Delete(ctx, created.ID, []identity.ID{wsB}); err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound for cross-workspace delete, got %v", err)
	}
	if err := st.Tasks.Delete(ctx, created.ID, []identity.ID{wsA}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := st.Tasks.Get(ctx, created.ID, []identity.ID{wsA}); err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

// TestTaskSagaDedup exercises the idempotency hook (task 6.6): the same
// (task, run) pair is processed exactly once.
func TestTaskSagaDedup(t *testing.T) {
	dsn := os.Getenv("AAKS_TASK_TEST_DSN")
	if dsn == "" {
		t.Skip("AAKS_TASK_TEST_DSN unset; skipping task saga test")
	}
	log := testLogger(t)
	ctx := context.Background()

	st, err := New(ctx, dsn, log)
	if err != nil {
		t.Fatalf("store new: %v", err)
	}
	defer st.Close()

	taskID := identity.ID("55555555-5555-5555-5555-555555555555")
	runID := identity.ID("66666666-6666-6666-6666-666666666666")

	first, err := st.Tasks.SagaNew(ctx, taskID, runID)
	if err != nil {
		t.Fatalf("saga new: %v", err)
	}
	if !first {
		t.Fatalf("expected first saga claim to succeed")
	}
	second, err := st.Tasks.SagaNew(ctx, taskID, runID)
	if err != nil {
		t.Fatalf("saga new redelivery: %v", err)
	}
	if second {
		t.Fatalf("redelivered saga event must be deduplicated")
	}
}