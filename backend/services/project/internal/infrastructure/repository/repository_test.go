package repository

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/services/project/internal/domain"
)

func testLogger(t *testing.T) *slog.Logger {
	t.Helper()
	var buf bytes.Buffer
	return slog.New(slog.NewJSONHandler(&buf, nil))
}

// TestProjectCRUD exercises the full project lifecycle against a real
// project_db. Skipped unless AAKS_PROJECT_TEST_DSN is set, so `go test ./...`
// stays green without infrastructure.
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

	wsA := identity.ID("11111111-1111-1111-1111-111111111111")
	wsB := identity.ID("22222222-2222-2222-2222-222222222222")

	created, err := st.Projects.Create(ctx, wsA, domain.CreateInput{
		Name: "proj-test", RepoSource: "https://example.com/repo.git", RepoType: identity.RepoTypeURL,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == "" || created.DefaultBranch != "main" || created.WorkspaceID != wsA {
		t.Fatalf("unexpected created project: %+v", created)
	}

	got, err := st.Projects.Get(ctx, created.ID, []identity.ID{wsA})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "proj-test" {
		t.Fatalf("get mismatch: %+v", got)
	}

	// Cross-workspace reads must be rejected (fail closed).
	if _, err := st.Projects.Get(ctx, created.ID, []identity.ID{wsB}); err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound for cross-workspace get, got %v", err)
	}

	listed, err := st.Projects.List(ctx, []identity.ID{wsA})
	if err != nil || len(listed) == 0 {
		t.Fatalf("list: %v (n=%d)", err, len(listed))
	}
	if other, err := st.Projects.List(ctx, []identity.ID{wsB}); err != nil || len(other) != 0 {
		t.Fatalf("cross-workspace list must be empty: %v (n=%d)", err, len(other))
	}

	newName := "proj-renamed"
	updated, err := st.Projects.Update(ctx, created.ID, []identity.ID{wsA}, domain.UpdateInput{Name: &newName})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != newName {
		t.Fatalf("update did not apply: %+v", updated)
	}

	if err := st.Projects.Delete(ctx, created.ID, []identity.ID{wsB}); err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound for cross-workspace delete, got %v", err)
	}
	if err := st.Projects.Delete(ctx, created.ID, []identity.ID{wsA}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := st.Projects.Get(ctx, created.ID, []identity.ID{wsA}); err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}
