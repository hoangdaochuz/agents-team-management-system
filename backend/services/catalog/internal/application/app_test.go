package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"

	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/resources"
	"github.com/aaks/server/services/catalog/internal/domain"
)

// ── Fakes ───────────────────────────────────────────────────────────────────

type fakeSkills struct {
	skills []resources.Skill
	next   int
}

func (f *fakeSkills) List(_ context.Context, ws []identity.ID) ([]resources.Skill, error) {
	out := []resources.Skill{}
	for _, sk := range f.skills {
		if contains(ws, sk.WorkspaceID) {
			out = append(out, sk)
		}
	}
	return out, nil
}
func (f *fakeSkills) Get(_ context.Context, id identity.ID, ws []identity.ID) (resources.Skill, error) {
	for _, sk := range f.skills {
		if sk.ID == id {
			if !contains(ws, sk.WorkspaceID) {
				return resources.Skill{}, domain.ErrSkillNotFound
			}
			return sk, nil
		}
	}
	return resources.Skill{}, domain.ErrSkillNotFound
}
func (f *fakeSkills) Create(_ context.Context, workspaceID identity.ID, in domain.SkillCreate) (resources.Skill, error) {
	f.next++
	sk := resources.Skill{ID: identity.ID(fmt.Sprintf("sk-%d", f.next)), WorkspaceID: workspaceID,
		Name: in.Name, Description: in.Description, BodyMd: in.BodyMd, Trigger: in.Trigger}
	f.skills = append(f.skills, sk)
	return sk, nil
}
func (f *fakeSkills) Update(_ context.Context, id identity.ID, ws []identity.ID, in domain.SkillUpdate) (resources.Skill, error) {
	for i := range f.skills {
		if f.skills[i].ID == id {
			if in.Name != nil {
				f.skills[i].Name = *in.Name
			}
			if in.Enabled != nil {
				f.skills[i].Enabled = in.Enabled
			}
			return f.skills[i], nil
		}
	}
	return resources.Skill{}, domain.ErrSkillNotFound
}
func (f *fakeSkills) Delete(_ context.Context, id identity.ID, ws []identity.ID) error {
	for i := range f.skills {
		if f.skills[i].ID == id {
			if !contains(ws, f.skills[i].WorkspaceID) {
				return domain.ErrSkillNotFound
			}
			f.skills = append(f.skills[:i], f.skills[i+1:]...)
			return nil
		}
	}
	return domain.ErrSkillNotFound
}
func (f *fakeSkills) ListByWorkspace(_ context.Context, workspaceID identity.ID) ([]resources.Skill, error) {
	return f.List(context.Background(), []identity.ID{workspaceID})
}
func (f *fakeSkills) SetEnabled(_ context.Context, workspaceID, id identity.ID, enabled bool) (resources.Skill, error) {
	for i := range f.skills {
		if f.skills[i].ID == id {
			f.skills[i].Enabled = &enabled
			return f.skills[i], nil
		}
	}
	return resources.Skill{}, domain.ErrSkillNotFound
}

type fakeMcps struct {
	mcps []resources.McpServer
	next int
}

func (f *fakeMcps) List(_ context.Context, ws []identity.ID) ([]resources.McpServer, error) {
	out := []resources.McpServer{}
	for _, m := range f.mcps {
		if contains(ws, m.WorkspaceID) {
			out = append(out, m)
		}
	}
	return out, nil
}
func (f *fakeMcps) Get(_ context.Context, id identity.ID, ws []identity.ID) (resources.McpServer, error) {
	for _, m := range f.mcps {
		if m.ID == id {
			if !contains(ws, m.WorkspaceID) {
				return resources.McpServer{}, domain.ErrMcpNotFound
			}
			return m, nil
		}
	}
	return resources.McpServer{}, domain.ErrMcpNotFound
}
func (f *fakeMcps) Create(_ context.Context, workspaceID identity.ID, in domain.McpCreate) (resources.McpServer, error) {
	f.next++
	m := resources.McpServer{ID: identity.ID(fmt.Sprintf("mcp-%d", f.next)), WorkspaceID: workspaceID,
		Name: in.Name, Command: in.Command, Args: in.Args, Env: in.Env}
	f.mcps = append(f.mcps, m)
	return m, nil
}
func (f *fakeMcps) Update(_ context.Context, id identity.ID, ws []identity.ID, in domain.McpUpdate) (resources.McpServer, error) {
	for i := range f.mcps {
		if f.mcps[i].ID == id {
			if in.Name != nil {
				f.mcps[i].Name = *in.Name
			}
			return f.mcps[i], nil
		}
	}
	return resources.McpServer{}, domain.ErrMcpNotFound
}
func (f *fakeMcps) Delete(_ context.Context, id identity.ID, ws []identity.ID) error {
	for i := range f.mcps {
		if f.mcps[i].ID == id {
			f.mcps = append(f.mcps[:i], f.mcps[i+1:]...)
			return nil
		}
	}
	return domain.ErrMcpNotFound
}
func (f *fakeMcps) ListByIDs(_ context.Context, ids []identity.ID) ([]resources.McpServer, error) {
	out := []resources.McpServer{}
	for _, m := range f.mcps {
		if contains(ids, m.ID) {
			out = append(out, m)
		}
	}
	return out, nil
}

func contains(ids []identity.ID, id identity.ID) bool {
	for _, i := range ids {
		if i == id {
			return true
		}
	}
	return false
}

// fakeUoW runs fn against the same fakes as the plain repo — commit-on-success
// semantics with an optional injected failure for rollback tests. To emulate a
// real transaction it snapshots the fakes before fn and restores them on
// failure, so no partial state survives (mirrors pgx Tx rollback).
type fakeUoW struct {
	repo *fakeRepo
	// failAfter lets a test inject a mid-transaction error after fn succeeded.
	failAfter int
}

func (u *fakeUoW) Do(ctx context.Context, fn func(tx *Tx) error) error {
	snapSkills := append([]resources.Skill(nil), u.repo.skills.skills...)
	snapMcps := append([]resources.McpServer(nil), u.repo.mcps.mcps...)
	snapNext := [2]int{u.repo.skills.next, u.repo.mcps.next}

	tx := &Tx{Skills: u.repo.skills, Mcps: u.repo.mcps}
	if err := fn(tx); err != nil {
		return err
	}
	if u.failAfter > 0 {
		u.repo.skills.skills = snapSkills
		u.repo.mcps.mcps = snapMcps
		u.repo.skills.next, u.repo.mcps.next = snapNext[0], snapNext[1]
		return errors.New("injected mid-transaction failure")
	}
	return nil
}

// fakePublisher records published event topics.
type fakePublisher struct {
	events []string
}

func (p *fakePublisher) Publish(_ context.Context, topic string, _ any, _ identity.ID) {
	p.events = append(p.events, topic)
}

type fakeRepo struct {
	skills   *fakeSkills
	mcps     *fakeMcps
	baseRepo *Repository
}

func newTestApp() (*App, *fakeRepo, *fakeUoW, *fakePublisher) {
	f := &fakeRepo{skills: &fakeSkills{}, mcps: &fakeMcps{}}
	f.baseRepo = &Repository{Skills: f.skills, Mcps: f.mcps}
	u := &fakeUoW{repo: f}
	p := &fakePublisher{}
	app := New(f.baseRepo, u, p, slog.New(slog.DiscardHandler))
	return app, f, u, p
}

// ── Skill flows ─────────────────────────────────────────────────────────────

func TestCreateSkillPublishesAfterCommit(t *testing.T) {
	app, f, _, p := newTestApp()
	ws := identity.ID("ws1")

	sk, err := app.CreateSkill(context.Background(), ws, domain.SkillCreate{Name: "go", BodyMd: "# Go"})
	if err != nil {
		t.Fatalf("create skill: %v", err)
	}
	if len(f.skills.skills) != 1 || f.skills.skills[0].ID != sk.ID || f.skills.skills[0].WorkspaceID != ws {
		t.Fatalf("skill must be persisted in the workspace, got %+v", f.skills.skills)
	}
	if len(p.events) != 1 || p.events[0] != events.TopicSkillCreated {
		t.Fatalf("expected skill.created only, got %v", p.events)
	}
}

func TestCreateSkillRollbackDoesNotPublish(t *testing.T) {
	app, f, u, p := newTestApp()
	u.failAfter = 1

	_, err := app.CreateSkill(context.Background(), "ws1", domain.SkillCreate{Name: "go", BodyMd: "# Go"})
	if err == nil {
		t.Fatal("injected failure must surface as an error")
	}
	if len(f.skills.skills) != 0 {
		t.Fatal("no skill may persist on rollback")
	}
	if len(p.events) != 0 {
		t.Fatalf("no events may be published on rollback, got %v", p.events)
	}
}

func TestDeleteSkillPublishesAfterCommit(t *testing.T) {
	app, f, _, p := newTestApp()
	ws := identity.ID("ws1")
	sk, _ := app.CreateSkill(context.Background(), ws, domain.SkillCreate{Name: "go", BodyMd: "# Go"})

	err := app.DeleteSkill(context.Background(), sk.ID, []identity.ID{ws})
	if err != nil {
		t.Fatalf("delete skill: %v", err)
	}
	if len(f.skills.skills) != 0 {
		t.Fatal("skill must be removed")
	}
	if len(p.events) != 2 || p.events[1] != events.TopicSkillDeleted {
		t.Fatalf("expected skill.created + skill.deleted, got %v", p.events)
	}
}

func TestDeleteSkillScoped(t *testing.T) {
	app, f, _, p := newTestApp()
	sk, _ := app.CreateSkill(context.Background(), "ws1", domain.SkillCreate{Name: "go", BodyMd: "# Go"})

	err := app.DeleteSkill(context.Background(), sk.ID, []identity.ID{"ws2"})
	if !errors.Is(err, domain.ErrSkillNotFound) {
		t.Fatalf("cross-workspace delete must be rejected, got %v", err)
	}
	if len(f.skills.skills) != 1 {
		t.Fatal("skill must survive a cross-workspace delete")
	}
	if len(p.events) != 1 {
		t.Fatalf("no delete event may be published, got %v", p.events)
	}
}

func TestUpdateAndSetEnabledSkill(t *testing.T) {
	app, _, _, _ := newTestApp()
	ws := identity.ID("ws1")
	sk, _ := app.CreateSkill(context.Background(), ws, domain.SkillCreate{Name: "go", BodyMd: "# Go"})

	name := "golang"
	updated, err := app.UpdateSkill(context.Background(), sk.ID, []identity.ID{ws}, domain.SkillUpdate{Name: &name})
	if err != nil {
		t.Fatalf("update skill: %v", err)
	}
	if updated.Name != "golang" {
		t.Fatalf("update did not apply: %+v", updated)
	}

	toggled, err := app.SetSkillEnabled(context.Background(), ws, sk.ID, true)
	if err != nil {
		t.Fatalf("set enabled: %v", err)
	}
	if toggled.Enabled == nil || !*toggled.Enabled {
		t.Fatalf("enabled flag not set: %+v", toggled)
	}
	if _, err := app.GetSkill(context.Background(), sk.ID, []identity.ID{"ws2"}); !errors.Is(err, domain.ErrSkillNotFound) {
		t.Fatalf("cross-workspace get must 404, got %v", err)
	}
	if got, err := app.ListWorkspaceSkills(context.Background(), ws); err != nil || len(got) != 1 {
		t.Fatalf("workspace list: %v (n=%d)", err, len(got))
	}
}

// ── MCP flows ───────────────────────────────────────────────────────────────

func TestCreateMcpPublishesAfterCommit(t *testing.T) {
	app, f, _, p := newTestApp()
	ws := identity.ID("ws1")

	m, err := app.CreateMcp(context.Background(), ws, domain.McpCreate{
		Name: "filesystem", Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-filesystem"},
		Env: map[string]string{"ROOT": "/tmp"},
	})
	if err != nil {
		t.Fatalf("create mcp: %v", err)
	}
	if len(f.mcps.mcps) != 1 || f.mcps.mcps[0].WorkspaceID != ws {
		t.Fatalf("mcp must be persisted in the workspace, got %+v", f.mcps.mcps)
	}
	if len(p.events) != 1 || p.events[0] != events.TopicMcpCreated {
		t.Fatalf("expected mcp.created only, got %v", p.events)
	}
	_ = m
}

func TestDeleteMcpPublishesAfterCommit(t *testing.T) {
	app, f, _, p := newTestApp()
	ws := identity.ID("ws1")
	m, _ := app.CreateMcp(context.Background(), ws, domain.McpCreate{Name: "fs", Command: "npx"})

	err := app.DeleteMcp(context.Background(), m.ID, []identity.ID{ws})
	if err != nil {
		t.Fatalf("delete mcp: %v", err)
	}
	if len(f.mcps.mcps) != 0 {
		t.Fatal("mcp must be removed")
	}
	if len(p.events) != 2 || p.events[1] != events.TopicMcpDeleted {
		t.Fatalf("expected mcp.created + mcp.deleted, got %v", p.events)
	}
}

func TestListMcpByIDs(t *testing.T) {
	app, _, _, _ := newTestApp()
	ws := identity.ID("ws1")
	m1, _ := app.CreateMcp(context.Background(), ws, domain.McpCreate{Name: "fs", Command: "npx"})
	m2, _ := app.CreateMcp(context.Background(), ws, domain.McpCreate{Name: "git", Command: "npx"})

	out, err := app.ListMcpByIDs(context.Background(), []identity.ID{m1.ID, m2.ID, "unknown"})
	if err != nil {
		t.Fatalf("list by ids: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 definitions, got %d", len(out))
	}
	if got, err := app.ListMcpByIDs(context.Background(), nil); err != nil || got != nil {
		t.Fatalf("empty ids must return nil, got %v (%v)", got, err)
	}
}
