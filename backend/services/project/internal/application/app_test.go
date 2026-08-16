package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"

	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/tasks"
	"github.com/aaks/server/services/project/internal/domain"
)

// ── Fakes ───────────────────────────────────────────────────────────────────

type fakeProjects struct {
	ps   []tasks.Project
	next int
	err  error
}

func (f *fakeProjects) List(_ context.Context, ws []identity.ID) ([]tasks.Project, error) {
	if f.err != nil {
		return nil, f.err
	}
	if len(ws) == 0 {
		return []tasks.Project{}, nil // fail closed, mirroring the adapter
	}
	return f.ps, nil
}

func (f *fakeProjects) Get(_ context.Context, id identity.ID, _ []identity.ID) (tasks.Project, error) {
	for _, p := range f.ps {
		if p.ID == id {
			return p, nil
		}
	}
	return tasks.Project{}, domain.ErrNotFound
}

func (f *fakeProjects) Create(_ context.Context, workspaceID identity.ID, in domain.CreateInput) (tasks.Project, error) {
	if f.err != nil {
		return tasks.Project{}, f.err
	}
	f.next++
	p := tasks.Project{
		ID: identity.ID(fmt.Sprintf("p-%d", f.next)), WorkspaceID: workspaceID,
		Name: in.Name, RepoSource: in.RepoSource, RepoType: in.RepoType, DefaultBranch: in.DefaultBranch,
	}
	if p.DefaultBranch == "" {
		p.DefaultBranch = "main"
	}
	f.ps = append(f.ps, p)
	return p, nil
}

func (f *fakeProjects) Update(_ context.Context, id identity.ID, _ []identity.ID, in domain.UpdateInput) (tasks.Project, error) {
	for i := range f.ps {
		if f.ps[i].ID == id {
			if in.Name != nil {
				f.ps[i].Name = *in.Name
			}
			return f.ps[i], nil
		}
	}
	return tasks.Project{}, domain.ErrNotFound
}

func (f *fakeProjects) Delete(_ context.Context, id identity.ID, _ []identity.ID) error {
	for i := range f.ps {
		if f.ps[i].ID == id {
			f.ps = append(f.ps[:i], f.ps[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

func newTestApp() (*App, *fakeProjects) {
	f := &fakeProjects{}
	app := New(&Repository{Projects: f}, slog.New(slog.DiscardHandler))
	return app, f
}

// ── Workspace bootstrap (workspace.created) ─────────────────────────────────

// TestBindWorkspaceCreatesDefaultProject asserts a new workspace with a repo
// source gets a default project binding named after the workspace.
func TestBindWorkspaceCreatesDefaultProject(t *testing.T) {
	app, f := newTestApp()

	err := app.BindWorkspace(context.Background(), events.WorkspaceCreatedData{
		WorkspaceID: "ws1", Name: "Team", RepoSource: "git@x/y.git", DefaultBranch: "develop",
	})
	if err != nil {
		t.Fatalf("bind workspace: %v", err)
	}
	if len(f.ps) != 1 {
		t.Fatalf("expected one bound project, got %d", len(f.ps))
	}
	p := f.ps[0]
	if p.WorkspaceID != "ws1" || p.Name != "Team" || p.RepoSource != "git@x/y.git" {
		t.Fatalf("unexpected bound project: %+v", p)
	}
	if p.RepoType != identity.RepoType("git") {
		t.Fatalf("repo type must be git, got %q", p.RepoType)
	}
	if p.DefaultBranch != "develop" {
		t.Fatalf("default branch must carry over, got %q", p.DefaultBranch)
	}
}

// TestBindWorkspaceSkipsEmptyRepoSource asserts no project is created when the
// workspace has no repo to bind.
func TestBindWorkspaceSkipsEmptyRepoSource(t *testing.T) {
	app, f := newTestApp()

	err := app.BindWorkspace(context.Background(), events.WorkspaceCreatedData{WorkspaceID: "ws1", Name: "Team"})
	if err != nil {
		t.Fatalf("bind workspace: %v", err)
	}
	if len(f.ps) != 0 {
		t.Fatalf("no project may be bound without a repo source, got %d", len(f.ps))
	}
}

// TestBindWorkspaceDefaultsName asserts an unnamed workspace binds as "default".
func TestBindWorkspaceDefaultsName(t *testing.T) {
	app, f := newTestApp()

	err := app.BindWorkspace(context.Background(), events.WorkspaceCreatedData{
		WorkspaceID: "ws1", RepoSource: "git@x/y.git",
	})
	if err != nil {
		t.Fatalf("bind workspace: %v", err)
	}
	if len(f.ps) != 1 || f.ps[0].Name != "default" {
		t.Fatalf("expected default-named project, got %+v", f.ps)
	}
}

// TestBindWorkspaceBestEffort asserts a failed binding is logged, not fatal
// (redelivery duplicate, DB hiccup) — the consumer must not poison the topic.
func TestBindWorkspaceBestEffort(t *testing.T) {
	app, f := newTestApp()
	f.err = errors.New("duplicate project name")

	err := app.BindWorkspace(context.Background(), events.WorkspaceCreatedData{
		WorkspaceID: "ws1", Name: "Team", RepoSource: "git@x/y.git",
	})
	if err != nil {
		t.Fatalf("binding failure must be absorbed, got %v", err)
	}
	if len(f.ps) != 0 {
		t.Fatalf("failed binding must not persist, got %d", len(f.ps))
	}
}

// ── CRUD passthrough ────────────────────────────────────────────────────────

func TestProjectCRUDHandlers(t *testing.T) {
	app, f := newTestApp()

	created, err := app.Create(context.Background(), "ws1", domain.CreateInput{
		Name: "proj", RepoSource: "https://x/repo.git", RepoType: identity.RepoTypeURL,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.DefaultBranch != "main" {
		t.Fatalf("default branch must default to main, got %q", created.DefaultBranch)
	}

	got, err := app.Get(context.Background(), created.ID, []identity.ID{"ws1"})
	if err != nil || got.Name != "proj" {
		t.Fatalf("get: %v %+v", err, got)
	}
	if _, err := app.Get(context.Background(), "nope", []identity.ID{"ws1"}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing project must be ErrNotFound, got %v", err)
	}

	list, err := app.List(context.Background(), []identity.ID{"ws1"})
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v (n=%d)", err, len(list))
	}
	// An empty workspace set must yield an empty list, never nil (fail closed).
	empty, err := app.List(context.Background(), nil)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty workspace set must yield [], got %v (%v)", empty, err)
	}

	renamed := "renamed"
	updated, err := app.Update(context.Background(), created.ID, []identity.ID{"ws1"}, domain.UpdateInput{Name: &renamed})
	if err != nil || updated.Name != renamed {
		t.Fatalf("update: %v %+v", err, updated)
	}

	if err := app.Delete(context.Background(), created.ID, []identity.ID{"ws1"}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(f.ps) != 0 {
		t.Fatalf("project must be gone after delete, got %d", len(f.ps))
	}
}
