package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/aaks/server/internal/contracts/admin"
	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/services/admin/internal/domain"
)

// ── Fakes ───────────────────────────────────────────────────────────────────

type fakeAudit struct {
	rows []domain.AuditRow
	next int
}

func (f *fakeAudit) List(_ context.Context, workspaceID identity.ID, kind string) ([]domain.AuditRow, error) {
	out := []domain.AuditRow{}
	for _, a := range f.rows {
		if a.WorkspaceID != workspaceID {
			continue
		}
		if kind != "" && a.ActionKind != kind {
			continue
		}
		out = append(out, a)
	}
	return out, nil
}
func (f *fakeAudit) ListSystem(_ context.Context, limit int) ([]domain.AuditRow, error) {
	out := append([]domain.AuditRow{}, f.rows...)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
func (f *fakeAudit) Append(_ context.Context, workspaceID identity.ID, actorName string, actorID identity.ID, action, kind, target, ip string) error {
	f.next++
	f.rows = append(f.rows, domain.AuditRow{
		AuditEntry: admin.AuditEntry{
			ID: identity.ID(fmt.Sprintf("a-%d", f.next)), Actor: admin.AuditActor{Name: actorName},
			Action: action, ActionKind: kind, Target: target, IP: ip, CreatedAt: time.Now().UTC(),
		},
		ActorID: actorID, WorkspaceID: workspaceID,
	})
	return nil
}
func (f *fakeAudit) Count24h(_ context.Context, workspaceID identity.ID) (int, error) {
	n := 0
	for _, a := range f.rows {
		if a.WorkspaceID == workspaceID {
			n++
		}
	}
	return n, nil
}

type fakeFlags struct {
	flags []admin.FeatureFlag
}

func (f *fakeFlags) List(context.Context) ([]admin.FeatureFlag, error) { return f.flags, nil }
func (f *fakeFlags) SetEnabled(_ context.Context, key string, enabled bool) (admin.FeatureFlag, error) {
	for i := range f.flags {
		if f.flags[i].Key == key {
			f.flags[i].Enabled = enabled
			return f.flags[i], nil
		}
	}
	return admin.FeatureFlag{}, domain.ErrNotFound
}

type fakeRepo struct {
	audit    *fakeAudit
	flags    *fakeFlags
	baseRepo *Repository
}

func newFakeRepo() *fakeRepo {
	f := &fakeRepo{audit: &fakeAudit{}, flags: &fakeFlags{}}
	f.baseRepo = &Repository{Audit: f.audit, Flags: f.flags}
	return f
}

func newTestApp() (*App, *fakeRepo) {
	f := newFakeRepo()
	app := New(f.baseRepo, slog.New(slog.DiscardHandler))
	return app, f
}

// ── Audit queries ───────────────────────────────────────────────────────────

func TestListWorkspaceAudit(t *testing.T) {
	app, f := newTestApp()
	f.audit.rows = []domain.AuditRow{
		{AuditEntry: admin.AuditEntry{ID: "a1", Action: "member.role-changed", ActionKind: "member"}, WorkspaceID: "ws1"},
		{AuditEntry: admin.AuditEntry{ID: "a2", Action: "task.created", ActionKind: "task"}, WorkspaceID: "ws1"},
		{AuditEntry: admin.AuditEntry{ID: "a3", Action: "member.removed", ActionKind: "member"}, WorkspaceID: "ws2"},
	}

	all, err := app.ListWorkspaceAudit(context.Background(), "ws1", "")
	if err != nil || len(all) != 2 {
		t.Fatalf("workspace audit: %v (n=%d)", err, len(all))
	}
	members, err := app.ListWorkspaceAudit(context.Background(), "ws1", "member")
	if err != nil || len(members) != 1 || members[0].ID != "a1" {
		t.Fatalf("kind-filtered audit: %v (n=%d)", err, len(members))
	}
}

func TestListSystemAudit(t *testing.T) {
	app, f := newTestApp()
	for i := 0; i < 5; i++ {
		f.audit.rows = append(f.audit.rows, domain.AuditRow{
			AuditEntry: admin.AuditEntry{ID: identity.ID(fmt.Sprintf("a%d", i)), Action: "x"},
		})
	}

	out, err := app.ListSystemAudit(context.Background(), 3)
	if err != nil {
		t.Fatalf("system audit: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("limit must clamp to 3, got %d", len(out))
	}
}

func TestCountAudit24h(t *testing.T) {
	app, f := newTestApp()
	for i := 0; i < 4; i++ {
		f.audit.rows = append(f.audit.rows, domain.AuditRow{
			AuditEntry: admin.AuditEntry{ID: identity.ID(fmt.Sprintf("a%d", i))}, WorkspaceID: "ws1",
		})
	}

	n, err := app.CountAudit24h(context.Background(), "ws1")
	if err != nil || n != 4 {
		t.Fatalf("count 24h: %v (n=%d)", err, n)
	}
}

// ── Consume audit.recorded ──────────────────────────────────────────────────

func TestRecordAuditPersists(t *testing.T) {
	app, f := newTestApp()

	err := app.RecordAudit(context.Background(), events.AuditRecordedData{
		WorkspaceID: "ws1", ActorName: "Alice", ActorID: "u1",
		Action: "member.role-changed", ActionKind: "member", Target: "u2", IP: "10.0.0.1",
	})
	if err != nil {
		t.Fatalf("record audit: %v", err)
	}
	if len(f.audit.rows) != 1 {
		t.Fatalf("expected 1 persisted entry, got %d", len(f.audit.rows))
	}
	got := f.audit.rows[0]
	if got.WorkspaceID != "ws1" || got.Actor.Name != "Alice" || got.ActorID != "u1" ||
		got.Action != "member.role-changed" || got.ActionKind != "member" ||
		got.Target != "u2" || got.IP != "10.0.0.1" {
		t.Fatalf("entry not persisted faithfully: %+v", got)
	}
}

// ── Feature flags ───────────────────────────────────────────────────────────

func TestListAndToggleFlags(t *testing.T) {
	app, f := newTestApp()
	f.flags.flags = []admin.FeatureFlag{
		{Key: "parallel_runs", Label: "Parallel runs", Enabled: true},
	}

	out, err := app.ListFlags(context.Background())
	if err != nil || len(out) != 1 {
		t.Fatalf("list flags: %v (n=%d)", err, len(out))
	}
	flag, err := app.SetFlagEnabled(context.Background(), "parallel_runs", false)
	if err != nil {
		t.Fatalf("toggle flag: %v", err)
	}
	if flag.Enabled {
		t.Fatal("flag must be disabled")
	}
	if _, err := app.SetFlagEnabled(context.Background(), "nope", true); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing flag must map to ErrNotFound, got %v", err)
	}
}