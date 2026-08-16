package repository

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/services/agent/internal/domain"
)

func testLogger(t *testing.T) *slog.Logger {
	t.Helper()
	var buf bytes.Buffer
	return slog.New(slog.NewJSONHandler(&buf, nil))
}

// TestAgentCRUD exercises the agent lifecycle against a real agent_db.
// Skipped unless AAKS_AGENT_TEST_DSN is set, so `go test ./...` stays green
// without infrastructure.
func TestAgentCRUD(t *testing.T) {
	dsn := os.Getenv("AAKS_AGENT_TEST_DSN")
	if dsn == "" {
		t.Skip("AAKS_AGENT_TEST_DSN unset; skipping agent integration test")
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

	created, err := st.Agents.Create(ctx, wsA, domain.AgentCreate{
		Name: "tester", Role: "implementer", DefaultModel: "simulated/sim",
		SystemPrompt: "You implement tasks.",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == "" || created.WorkspaceID != wsA {
		t.Fatalf("unexpected created agent: %+v", created)
	}

	got, err := st.Agents.Get(ctx, created.ID, []identity.ID{wsA})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "tester" {
		t.Fatalf("get mismatch: %+v", got)
	}

	if _, err := st.Agents.Get(ctx, created.ID, []identity.ID{wsB}); err != domain.ErrAgentNotFound {
		t.Fatalf("expected ErrAgentNotFound for cross-workspace get, got %v", err)
	}

	listed, err := st.Agents.List(ctx, []identity.ID{wsA})
	if err != nil || len(listed) == 0 {
		t.Fatalf("list: %v (n=%d)", err, len(listed))
	}
	if other, err := st.Agents.List(ctx, []identity.ID{wsB}); err != nil || len(other) != 0 {
		t.Fatalf("cross-workspace list must be empty: %v (n=%d)", err, len(other))
	}

	role := "reviewer"
	updated, err := st.Agents.Update(ctx, created.ID, []identity.ID{wsA}, domain.AgentUpdate{Role: &role})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Role != role {
		t.Fatalf("update did not apply: %+v", updated)
	}

	if err := st.Agents.Delete(ctx, created.ID, []identity.ID{wsB}); err != domain.ErrAgentNotFound {
		t.Fatalf("expected ErrAgentNotFound for cross-workspace delete, got %v", err)
	}
	if err := st.Agents.Delete(ctx, created.ID, []identity.ID{wsA}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := st.Agents.Get(ctx, created.ID, []identity.ID{wsA}); err != domain.ErrAgentNotFound {
		t.Fatalf("expected ErrAgentNotFound after delete, got %v", err)
	}
}
