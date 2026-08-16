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
	"github.com/aaks/server/services/resources/internal/domain"
)

// ── Fakes ───────────────────────────────────────────────────────────────────

type fakeKnowledge struct {
	items []resources.KnowledgeSource
	next  int
}

func (f *fakeKnowledge) List(context.Context, identity.ID) ([]resources.KnowledgeSource, error) {
	return f.items, nil
}
func (f *fakeKnowledge) Create(_ context.Context, wsID identity.ID, title, kind string) (resources.KnowledgeSource, error) {
	f.next++
	k := resources.KnowledgeSource{ID: identity.ID(fmt.Sprintf("k-%d", f.next)), Title: title, Kind: kind, Status: resources.IndexPending}
	f.items = append(f.items, k)
	return k, nil
}

type fakePlugins struct {
	items []resources.Plugin
	next  int
}

func (f *fakePlugins) List(context.Context, identity.ID) ([]resources.Plugin, error) { return f.items, nil }
func (f *fakePlugins) SetEnabled(_ context.Context, wsID, id identity.ID, enabled bool) (resources.Plugin, error) {
	for i := range f.items {
		if f.items[i].ID == id {
			f.items[i].Enabled = enabled
			return f.items[i], nil
		}
	}
	return resources.Plugin{}, domain.ErrNotFound
}

type fakeRules struct {
	items []resources.Rule
	next  int
}

func (f *fakeRules) List(context.Context, identity.ID) ([]resources.Rule, error) { return f.items, nil }
func (f *fakeRules) Create(_ context.Context, wsID identity.ID, name, description string, enabled bool) error {
	for _, r := range f.items {
		if r.Name == name {
			return nil // ON CONFLICT DO NOTHING
		}
	}
	f.next++
	f.items = append(f.items, resources.Rule{ID: identity.ID(fmt.Sprintf("r-%d", f.next)), Name: name, Description: description, Enabled: enabled})
	return nil
}
func (f *fakeRules) SetEnabled(_ context.Context, wsID, id identity.ID, enabled bool) (resources.Rule, error) {
	for i := range f.items {
		if f.items[i].ID == id {
			f.items[i].Enabled = enabled
			return f.items[i], nil
		}
	}
	return resources.Rule{}, domain.ErrNotFound
}
func (f *fakeRules) Enabled(context.Context, identity.ID) ([]resources.Rule, error) {
	out := []resources.Rule{}
	for _, r := range f.items {
		if r.Enabled {
			out = append(out, r)
		}
	}
	return out, nil
}

type fakeMcp struct {
	items []resources.McpConnection
	next  int
}

func (f *fakeMcp) List(context.Context, identity.ID) ([]resources.McpConnection, error) {
	return f.items, nil
}
func (f *fakeMcp) Upsert(_ context.Context, mcpID, wsID identity.ID, name string) error {
	for i := range f.items {
		if f.items[i].ID == mcpID {
			f.items[i].Name = name
			return nil
		}
	}
	f.next++
	f.items = append(f.items, resources.McpConnection{ID: mcpID, Name: name, Transport: "stdio", Status: "connected"})
	return nil
}
func (f *fakeMcp) Delete(_ context.Context, wsID, mcpID identity.ID) error {
	for i := range f.items {
		if f.items[i].ID == mcpID {
			f.items = append(f.items[:i], f.items[i+1:]...)
			return nil
		}
	}
	return nil
}
func (f *fakeMcp) Reconnect(_ context.Context, wsID, id identity.ID) (resources.McpConnection, error) {
	for i := range f.items {
		if f.items[i].ID == id {
			f.items[i].Status = "connected"
			return f.items[i], nil
		}
	}
	return resources.McpConnection{}, domain.ErrNotFound
}

type fakeRepo struct {
	knowledge *fakeKnowledge
	plugins   *fakePlugins
	rules     *fakeRules
	mcp       *fakeMcp
	baseRepo  *Repository
}

func newFakeRepo() *fakeRepo {
	f := &fakeRepo{
		knowledge: &fakeKnowledge{},
		plugins:   &fakePlugins{},
		rules:     &fakeRules{},
		mcp:       &fakeMcp{},
	}
	f.baseRepo = &Repository{
		Knowledge: f.knowledge,
		Plugins:   f.plugins,
		Rules:     f.rules,
		Mcp:       f.mcp,
	}
	return f
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
	snapKnowledge := append([]resources.KnowledgeSource(nil), u.repo.knowledge.items...)
	snapPlugins := append([]resources.Plugin(nil), u.repo.plugins.items...)
	snapRules := append([]resources.Rule(nil), u.repo.rules.items...)
	snapMcp := append([]resources.McpConnection(nil), u.repo.mcp.items...)
	snapNext := [4]int{u.repo.knowledge.next, u.repo.plugins.next, u.repo.rules.next, u.repo.mcp.next}

	tx := &Tx{
		Knowledge: u.repo.knowledge,
		Plugins:   u.repo.plugins,
		Rules:     u.repo.rules,
		Mcp:       u.repo.mcp,
	}
	if err := fn(tx); err != nil {
		return err
	}
	if u.failAfter > 0 {
		u.repo.knowledge.items = snapKnowledge
		u.repo.plugins.items = snapPlugins
		u.repo.rules.items = snapRules
		u.repo.mcp.items = snapMcp
		u.repo.knowledge.next, u.repo.plugins.next, u.repo.rules.next, u.repo.mcp.next = snapNext[0], snapNext[1], snapNext[2], snapNext[3]
		return errors.New("injected mid-transaction failure")
	}
	return nil
}

func newTestApp() (*App, *fakeRepo, *fakeUoW) {
	f := newFakeRepo()
	u := &fakeUoW{repo: f}
	app := New(f.baseRepo, u, slog.New(slog.DiscardHandler))
	return app, f, u
}

// ── Workspace bootstrap ─────────────────────────────────────────────────────

func TestBootstrapWorkspaceSeedsDefaultRules(t *testing.T) {
	app, f, _ := newTestApp()

	err := app.BootstrapWorkspace(context.Background(), events.WorkspaceCreatedData{WorkspaceID: "ws1"})
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if len(f.rules.items) != len(defaultRules) {
		t.Fatalf("expected %d default rules, got %d", len(defaultRules), len(f.rules.items))
	}
	for _, r := range f.rules.items {
		if !r.Enabled {
			t.Fatalf("default rule %s must be enabled", r.Name)
		}
	}
}

func TestBootstrapWorkspaceIdempotent(t *testing.T) {
	app, f, _ := newTestApp()
	ws := events.WorkspaceCreatedData{WorkspaceID: "ws1"}

	if err := app.BootstrapWorkspace(context.Background(), ws); err != nil {
		t.Fatalf("bootstrap 1: %v", err)
	}
	if err := app.BootstrapWorkspace(context.Background(), ws); err != nil {
		t.Fatalf("bootstrap 2: %v", err)
	}
	if len(f.rules.items) != len(defaultRules) {
		t.Fatalf("redelivery must not duplicate rules, got %d", len(f.rules.items))
	}
}

func TestBootstrapWorkspaceRollback(t *testing.T) {
	app, f, u := newTestApp()
	u.failAfter = 1

	err := app.BootstrapWorkspace(context.Background(), events.WorkspaceCreatedData{WorkspaceID: "ws1"})
	if err == nil {
		t.Fatal("injected failure must surface as an error")
	}
	if len(f.rules.items) != 0 {
		t.Fatalf("no rules may persist on rollback, got %d", len(f.rules.items))
	}
}

// ── MCP event projection ────────────────────────────────────────────────────

func TestProjectMcpCreated(t *testing.T) {
	app, f, _ := newTestApp()

	err := app.ProjectMcpCreated(context.Background(), events.McpCreatedData{
		McpServerID: "mcp1", WorkspaceID: "ws1", Name: "filesystem",
	})
	if err != nil {
		t.Fatalf("project mcp.created: %v", err)
	}
	if len(f.mcp.items) != 1 || f.mcp.items[0].ID != "mcp1" || f.mcp.items[0].Name != "filesystem" {
		t.Fatalf("connection must be upserted, got %+v", f.mcp.items)
	}

	// Upsert (redelivery) updates the name, not duplicates.
	err = app.ProjectMcpCreated(context.Background(), events.McpCreatedData{
		McpServerID: "mcp1", WorkspaceID: "ws1", Name: "fs-v2",
	})
	if err != nil {
		t.Fatalf("re-project mcp.created: %v", err)
	}
	if len(f.mcp.items) != 1 || f.mcp.items[0].Name != "fs-v2" {
		t.Fatalf("redelivery must upsert, got %+v", f.mcp.items)
	}
}

func TestProjectMcpDeleted(t *testing.T) {
	app, f, _ := newTestApp()
	if err := app.ProjectMcpCreated(context.Background(), events.McpCreatedData{McpServerID: "mcp1", WorkspaceID: "ws1", Name: "fs"}); err != nil {
		t.Fatalf("seed connection: %v", err)
	}

	err := app.ProjectMcpDeleted(context.Background(), events.McpDeletedData{McpServerID: "mcp1", WorkspaceID: "ws1"})
	if err != nil {
		t.Fatalf("project mcp.deleted: %v", err)
	}
	if len(f.mcp.items) != 0 {
		t.Fatalf("connection must be removed, got %+v", f.mcp.items)
	}
}

// ── CRUD handlers ───────────────────────────────────────────────────────────

func TestCreateAndListKnowledge(t *testing.T) {
	app, f, _ := newTestApp()

	k, err := app.CreateKnowledge(context.Background(), "ws1", "Onboarding", "file")
	if err != nil {
		t.Fatalf("create knowledge: %v", err)
	}
	if k.Title != "Onboarding" || k.Kind != "file" {
		t.Fatalf("unexpected knowledge source: %+v", k)
	}
	out, err := app.ListKnowledge(context.Background(), "ws1")
	if err != nil || len(out) != 1 {
		t.Fatalf("list knowledge: %v (n=%d)", err, len(out))
	}
	_ = f
}

func TestSetPluginEnabledNotFound(t *testing.T) {
	app, _, _ := newTestApp()

	_, err := app.SetPluginEnabled(context.Background(), "ws1", "nope", true)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing plugin must map to ErrNotFound, got %v", err)
	}
}

func TestSetRuleEnabledAndEnabledRules(t *testing.T) {
	app, f, _ := newTestApp()
	if err := app.BootstrapWorkspace(context.Background(), events.WorkspaceCreatedData{WorkspaceID: "ws1"}); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	rule, err := app.SetRuleEnabled(context.Background(), "ws1", f.rules.items[0].ID, false)
	if err != nil {
		t.Fatalf("set rule enabled: %v", err)
	}
	if rule.Enabled {
		t.Fatal("rule must be disabled")
	}
	if _, err := app.SetRuleEnabled(context.Background(), "ws1", "nope", true); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing rule must map to ErrNotFound, got %v", err)
	}

	enabled, err := app.EnabledRules(context.Background(), "ws1")
	if err != nil {
		t.Fatalf("enabled rules: %v", err)
	}
	if len(enabled) != len(defaultRules)-1 {
		t.Fatalf("expected %d enabled rules, got %d", len(defaultRules)-1, len(enabled))
	}
}

func TestReconnectMcpNotFound(t *testing.T) {
	app, _, _ := newTestApp()

	_, err := app.ReconnectMcpConnection(context.Background(), "ws1", "nope")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing connection must map to ErrNotFound, got %v", err)
	}
}