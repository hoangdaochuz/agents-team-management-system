package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"

	"github.com/aaks/server/internal/contracts/agentexec"
	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/resources"
	"github.com/aaks/server/services/agent/internal/domain"
)

// ── Fakes ───────────────────────────────────────────────────────────────────

type fakeAgents struct {
	agents []agentexec.Agent
	links  []string
	next   int
}

func (f *fakeAgents) List(_ context.Context, ws []identity.ID) ([]agentexec.Agent, error) {
	out := []agentexec.Agent{}
	for _, a := range f.agents {
		if contains(ws, a.WorkspaceID) {
			out = append(out, a)
		}
	}
	return out, nil
}
func (f *fakeAgents) Get(_ context.Context, id identity.ID, ws []identity.ID) (agentexec.Agent, error) {
	a, err := f.GetUnscoped(context.Background(), id)
	if err != nil {
		return agentexec.Agent{}, err
	}
	if !contains(ws, a.WorkspaceID) {
		return agentexec.Agent{}, domain.ErrAgentNotFound
	}
	return a, nil
}
func (f *fakeAgents) GetUnscoped(_ context.Context, id identity.ID) (agentexec.Agent, error) {
	for _, a := range f.agents {
		if a.ID == id {
			return a, nil
		}
	}
	return agentexec.Agent{}, domain.ErrAgentNotFound
}
func (f *fakeAgents) Create(_ context.Context, workspaceID identity.ID, in domain.AgentCreate) (agentexec.Agent, error) {
	f.next++
	a := agentexec.Agent{ID: identity.ID(fmt.Sprintf("a-%d", f.next)), WorkspaceID: workspaceID,
		Name: in.Name, Role: in.Role, SystemPrompt: in.SystemPrompt, DefaultModel: in.DefaultModel,
		AllowedTools: in.AllowedTools, RoleTitle: in.RoleTitle, Provider: in.Provider,
		Temperature: in.Temperature, MaxOutputTokens: in.MaxOutputTokens, AutonomyMode: in.AutonomyMode,
		UserPromptTemplate: in.UserPromptTemplate, KnowledgeSourceIDs: in.KnowledgeSourceIDs,
		Guardrails: in.Guardrails, SkillIDs: []identity.ID{}, McpIDs: []identity.ID{}}
	f.agents = append(f.agents, a)
	return a, nil
}
func (f *fakeAgents) Update(_ context.Context, id identity.ID, ws []identity.ID, in domain.AgentUpdate) (agentexec.Agent, error) {
	for i := range f.agents {
		if f.agents[i].ID == id {
			if in.Name != nil {
				f.agents[i].Name = *in.Name
			}
			if in.Role != nil {
				f.agents[i].Role = *in.Role
			}
			return f.agents[i], nil
		}
	}
	return agentexec.Agent{}, domain.ErrAgentNotFound
}
func (f *fakeAgents) Delete(_ context.Context, id identity.ID, ws []identity.ID) error {
	for i := range f.agents {
		if f.agents[i].ID == id {
			if !contains(ws, f.agents[i].WorkspaceID) {
				return domain.ErrAgentNotFound
			}
			f.agents = append(f.agents[:i], f.agents[i+1:]...)
			return nil
		}
	}
	return domain.ErrAgentNotFound
}
func (f *fakeAgents) CountByWorkspace(_ context.Context, workspaceID identity.ID) (int, error) {
	n := 0
	for _, a := range f.agents {
		if a.WorkspaceID == workspaceID {
			n++
		}
	}
	return n, nil
}
func (f *fakeAgents) LinkSkill(_ context.Context, agentID, skillID identity.ID) error {
	f.links = append(f.links, "skill:"+agentID+":"+skillID)
	return nil
}
func (f *fakeAgents) UnlinkSkill(context.Context, identity.ID, identity.ID) error { return nil }
func (f *fakeAgents) LinkMcp(_ context.Context, agentID, mcpID identity.ID) error {
	f.links = append(f.links, "mcp:"+agentID+":"+mcpID)
	return nil
}
func (f *fakeAgents) UnlinkMcp(context.Context, identity.ID, identity.ID) error { return nil }

type fakeProjections struct {
	skills map[identity.ID]identity.ID
	mcps   map[identity.ID]identity.ID
}

func (f *fakeProjections) SkillWorkspace(_ context.Context, skillID identity.ID) (identity.ID, error) {
	ws, ok := f.skills[skillID]
	if !ok {
		return "", domain.ErrUnknownDefinition
	}
	return ws, nil
}
func (f *fakeProjections) McpWorkspace(_ context.Context, mcpID identity.ID) (identity.ID, error) {
	ws, ok := f.mcps[mcpID]
	if !ok {
		return "", domain.ErrUnknownDefinition
	}
	return ws, nil
}

type fakeCatalog struct {
	servers []resources.McpServer
	err     error
}

func (f *fakeCatalog) FetchMcpServers(context.Context, []identity.ID) ([]resources.McpServer, error) {
	return f.servers, f.err
}

func contains(ids []identity.ID, id identity.ID) bool {
	for _, i := range ids {
		if i == id {
			return true
		}
	}
	return false
}

func newTestApp() (*App, *fakeAgents, *fakeProjections, *fakeCatalog) {
	ag := &fakeAgents{}
	pj := &fakeProjections{skills: map[identity.ID]identity.ID{}, mcps: map[identity.ID]identity.ID{}}
	cat := &fakeCatalog{}
	repo := &Repository{Agents: ag, Projections: pj}
	app := New(repo, cat, slog.New(slog.DiscardHandler))
	return app, ag, pj, cat
}

// ── CRUD ────────────────────────────────────────────────────────────────────

func TestAgentCRUD(t *testing.T) {
	app, ag, _, _ := newTestApp()
	ws := identity.ID("ws1")

	created, err := app.Create(context.Background(), ws, domain.AgentCreate{
		Name: "tester", Role: "implementer", DefaultModel: "simulated/sim", SystemPrompt: "Implement.",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.WorkspaceID != ws || len(ag.agents) != 1 {
		t.Fatalf("unexpected created agent: %+v (n=%d)", created, len(ag.agents))
	}

	got, err := app.Get(context.Background(), created.ID, []identity.ID{ws})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "tester" {
		t.Fatalf("get mismatch: %+v", got)
	}
	if _, err := app.Get(context.Background(), created.ID, []identity.ID{"ws2"}); !errors.Is(err, domain.ErrAgentNotFound) {
		t.Fatalf("cross-workspace get must 404, got %v", err)
	}

	name := "renamed"
	updated, err := app.Update(context.Background(), created.ID, []identity.ID{ws}, domain.AgentUpdate{Name: &name})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "renamed" {
		t.Fatalf("update did not apply: %+v", updated)
	}

	if err := app.Delete(context.Background(), created.ID, []identity.ID{"ws2"}); !errors.Is(err, domain.ErrAgentNotFound) {
		t.Fatalf("cross-workspace delete must 404, got %v", err)
	}
	if err := app.Delete(context.Background(), created.ID, []identity.ID{ws}); err != nil {
		t.Fatalf("delete: %v", err)
	}

	n, err := app.CountByWorkspace(context.Background(), ws)
	if err != nil || n != 0 {
		t.Fatalf("count: %v (n=%d)", err, n)
	}
}

// ── Attachments (cross-workspace + unknown-definition rejection) ────────────

func TestAttachSkill(t *testing.T) {
	app, ag, pj, _ := newTestApp()
	ws := identity.ID("ws1")
	agent, _ := app.Create(context.Background(), ws, domain.AgentCreate{Name: "t", Role: "implementer"})
	pj.skills["sk1"] = ws
	pj.skills["sk2"] = identity.ID("ws2")

	if err := app.AttachSkill(context.Background(), agent.ID, "sk1"); err != nil {
		t.Fatalf("same-workspace attach: %v", err)
	}
	if len(ag.links) != 1 || ag.links[0] != "skill:"+agent.ID+":sk1" {
		t.Fatalf("expected link recorded, got %v", ag.links)
	}

	if err := app.AttachSkill(context.Background(), agent.ID, "sk2"); !errors.Is(err, domain.ErrCrossWorkspace) {
		t.Fatalf("cross-workspace attach must be rejected, got %v", err)
	}
	if err := app.AttachSkill(context.Background(), agent.ID, "unknown"); !errors.Is(err, domain.ErrUnknownDefinition) {
		t.Fatalf("unknown definition must be rejected, got %v", err)
	}
	if err := app.AttachSkill(context.Background(), "missing-agent", "sk1"); !errors.Is(err, domain.ErrAgentNotFound) {
		t.Fatalf("unknown agent must 404, got %v", err)
	}
}

func TestAttachMcp(t *testing.T) {
	app, ag, pj, _ := newTestApp()
	ws := identity.ID("ws1")
	agent, _ := app.Create(context.Background(), ws, domain.AgentCreate{Name: "t", Role: "implementer"})
	pj.mcps["m1"] = ws
	pj.mcps["m2"] = identity.ID("ws2")

	if err := app.AttachMcp(context.Background(), agent.ID, "m1"); err != nil {
		t.Fatalf("same-workspace attach: %v", err)
	}
	if len(ag.links) != 1 || ag.links[0] != "mcp:"+agent.ID+":m1" {
		t.Fatalf("expected link recorded, got %v", ag.links)
	}
	if err := app.AttachMcp(context.Background(), agent.ID, "m2"); !errors.Is(err, domain.ErrCrossWorkspace) {
		t.Fatalf("cross-workspace attach must be rejected, got %v", err)
	}
	if err := app.AttachMcp(context.Background(), agent.ID, "unknown"); !errors.Is(err, domain.ErrUnknownDefinition) {
		t.Fatalf("unknown definition must be rejected, got %v", err)
	}
}

// ── MCP hydration (Runner bridge) ───────────────────────────────────────────

func TestAgentMcpServers(t *testing.T) {
	app, ag, _, cat := newTestApp()
	ws := identity.ID("ws1")
	agent, _ := app.Create(context.Background(), ws, domain.AgentCreate{Name: "t", Role: "implementer"})

	// No attached servers → empty list.
	out, err := app.AgentMcpServers(context.Background(), agent.ID)
	if err != nil {
		t.Fatalf("no-attachments fetch: %v", err)
	}
	if out == nil || len(out) != 0 {
		t.Fatalf("expected empty list, got %+v", out)
	}

	// Attached servers hydrate from the Catalog client.
	ag.agents[0].McpIDs = []identity.ID{"m1", "m2"}
	cat.servers = []resources.McpServer{
		{ID: "m1", Name: "fs", Command: "npx"},
		{ID: "m2", Name: "git", Command: "npx"},
	}
	out, err = app.AgentMcpServers(context.Background(), agent.ID)
	if err != nil {
		t.Fatalf("hydrated fetch: %v", err)
	}
	if len(out) != 2 || out[0].ID != "m1" {
		t.Fatalf("expected 2 hydrated servers, got %+v", out)
	}

	// Hydration failure degrades to an empty list (run setup must not fail).
	cat.err = errors.New("catalog unreachable")
	out, err = app.AgentMcpServers(context.Background(), agent.ID)
	if err != nil {
		t.Fatalf("degraded fetch must not fail: %v", err)
	}
	if out == nil || len(out) != 0 {
		t.Fatalf("expected empty list on degrade, got %+v", out)
	}
}

func TestAgentMcpServersUnknownAgent(t *testing.T) {
	app, _, _, _ := newTestApp()

	_, err := app.AgentMcpServers(context.Background(), "missing")
	if !errors.Is(err, domain.ErrAgentNotFound) {
		t.Fatalf("unknown agent must 404, got %v", err)
	}
}
