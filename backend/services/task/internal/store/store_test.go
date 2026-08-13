package store

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/aaks/server/internal/contracts"
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

	wsA := "11111111-1111-1111-1111-111111111111"
	wsB := "22222222-2222-2222-2222-222222222222"
	proj := "33333333-3333-3333-3333-333333333333"

	created, err := st.Create(ctx, wsA, CreateInput{
		ProjectID: proj, Title: "task-test", Prompt: "Do the thing.",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == "" || created.WorkspaceID != wsA || created.Status != contracts.TaskBacklog {
		t.Fatalf("unexpected created task: %+v", created)
	}

	got, err := st.Get(ctx, created.ID, []contracts.ID{wsA})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != "task-test" {
		t.Fatalf("get mismatch: %+v", got)
	}

	if _, err := st.Get(ctx, created.ID, []contracts.ID{wsB}); err != ErrTaskNotFound {
		t.Fatalf("expected ErrTaskNotFound for cross-workspace get, got %v", err)
	}

	listed, err := st.List(ctx, Query{Workspaces: []contracts.ID{wsA}})
	if err != nil || len(listed) == 0 {
		t.Fatalf("list: %v (n=%d)", err, len(listed))
	}
	if other, err := st.List(ctx, Query{Workspaces: []contracts.ID{wsB}}); err != nil || len(other) != 0 {
		t.Fatalf("cross-workspace list must be empty: %v (n=%d)", err, len(other))
	}
	// Fail closed: no workspace context returns nothing, never everything.
	if all, err := st.List(ctx, Query{}); err != nil || len(all) != 0 {
		t.Fatalf("unscoped list must be empty: %v (n=%d)", err, len(all))
	}

	status, err := st.SetStatus(ctx, created.ID, contracts.TaskDoing)
	if err != nil {
		t.Fatalf("set status: %v", err)
	}
	if status.Status != contracts.TaskDoing {
		t.Fatalf("status transition did not apply: %+v", status)
	}

	if err := st.Delete(ctx, created.ID, []contracts.ID{wsB}); err != ErrTaskNotFound {
		t.Fatalf("expected ErrTaskNotFound for cross-workspace delete, got %v", err)
	}
	if err := st.Delete(ctx, created.ID, []contracts.ID{wsA}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := st.Get(ctx, created.ID, []contracts.ID{wsA}); err != ErrTaskNotFound {
		t.Fatalf("expected ErrTaskNotFound after delete, got %v", err)
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

	taskID := "55555555-5555-5555-5555-555555555555"
	runID := "66666666-6666-6666-6666-666666666666"

	first, err := st.SagaNew(ctx, taskID, runID)
	if err != nil {
		t.Fatalf("saga new: %v", err)
	}
	if !first {
		t.Fatalf("expected first saga claim to succeed")
	}
	second, err := st.SagaNew(ctx, taskID, runID)
	if err != nil {
		t.Fatalf("saga new redelivery: %v", err)
	}
	if second {
		t.Fatalf("redelivered saga event must be deduplicated")
	}
}
