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

// TestSkillCRUD exercises the catalog skill lifecycle against a real
// catalog_db. Skipped unless AAKS_CATALOG_TEST_DSN is set, so `go test ./...`
// stays green without infrastructure.
func TestSkillCRUD(t *testing.T) {
	dsn := os.Getenv("AAKS_CATALOG_TEST_DSN")
	if dsn == "" {
		t.Skip("AAKS_CATALOG_TEST_DSN unset; skipping catalog integration test")
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

	created, err := st.CreateSkill(ctx, wsA, SkillCreateInput{
		Name: "go-testing", Description: "go test guide", BodyMd: "# Go testing\nrun go test ./...",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == "" || created.WorkspaceID != wsA {
		t.Fatalf("unexpected created skill: %+v", created)
	}

	got, err := st.GetSkill(ctx, created.ID, []contracts.ID{wsA})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "go-testing" {
		t.Fatalf("get mismatch: %+v", got)
	}

	if _, err := st.GetSkill(ctx, created.ID, []contracts.ID{wsB}); err != ErrSkillNotFound {
		t.Fatalf("expected ErrSkillNotFound for cross-workspace get, got %v", err)
	}

	listed, err := st.ListSkills(ctx, []contracts.ID{wsA})
	if err != nil || len(listed) == 0 {
		t.Fatalf("list: %v (n=%d)", err, len(listed))
	}
	if other, err := st.ListSkills(ctx, []contracts.ID{wsB}); err != nil || len(other) != 0 {
		t.Fatalf("cross-workspace list must be empty: %v (n=%d)", err, len(other))
	}

	desc := "updated description"
	updated, err := st.UpdateSkill(ctx, created.ID, []contracts.ID{wsA}, SkillUpdateInput{Description: &desc})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Description != desc {
		t.Fatalf("update did not apply: %+v", updated)
	}

	if err := st.DeleteSkill(ctx, created.ID, []contracts.ID{wsB}); err != ErrSkillNotFound {
		t.Fatalf("expected ErrSkillNotFound for cross-workspace delete, got %v", err)
	}
	if err := st.DeleteSkill(ctx, created.ID, []contracts.ID{wsA}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := st.GetSkill(ctx, created.ID, []contracts.ID{wsA}); err != ErrSkillNotFound {
		t.Fatalf("expected ErrSkillNotFound after delete, got %v", err)
	}
}
