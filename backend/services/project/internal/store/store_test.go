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

// TestProjectCRUD exercises the full project lifecycle against a real
// project_db. Skipped unless AAKS_PROJECT_TEST_DSN is set, so `go test ./...`
// stays green without infrastructure. The other CRUD services follow the same
// pattern (catalog/agent/task).
func TestProjectCRUD(t *testing.T) {
	dsn := os.Getenv("AAKS_PROJECT_TEST_DSN")
	if dsn == "" {
		t.Skip("AAKS_PROJECT_TEST_DSN unset; skipping project integration test")
	}
	log := testLogger(t)
	ctx := context.Background()

	st, err := New(ctx, dsn, log)
	if err != nil {
		t.Fatalf("store new: %v", err)
	}
	defer st.Close()

	created, err := st.Create(ctx, CreateInput{
		Name: "proj-test", RepoSource: "https://example.com/repo.git", RepoType: contracts.RepoTypeURL,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == "" || created.DefaultBranch != "main" {
		t.Fatalf("unexpected created project: %+v", created)
	}

	got, err := st.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "proj-test" {
		t.Fatalf("get mismatch: %+v", got)
	}

	listed, err := st.List(ctx)
	if err != nil || len(listed) == 0 {
		t.Fatalf("list: %v (n=%d)", err, len(listed))
	}

	newName := "proj-renamed"
	updated, err := st.Update(ctx, created.ID, UpdateInput{Name: &newName})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != newName {
		t.Fatalf("update did not apply: %+v", updated)
	}

	if err := st.Delete(ctx, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := st.Get(ctx, created.ID); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}
