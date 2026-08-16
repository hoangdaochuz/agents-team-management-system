// Package store implements the Project domain repository port on Postgres
// (Ports & Adapters: the adapter side of the hexagon). The pool-backed adapter
// satisfies the domain port; reads filter by the session's workspace set and
// mutations reject rows outside it (404), so a tenant can never observe or
// touch another tenant's projects even if the Gateway were misconfigured.
package store

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/tasks"
	"github.com/aaks/server/internal/platform/db"
	"github.com/aaks/server/services/project/internal/domain"
)

//go:embed migrations/*.sql
var migrations embed.FS

// querier is satisfied by both *pgxpool.Pool and pgx.Tx, letting one adapter
// implementation serve plain and transactional access.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Store owns the project Postgres pool and exposes the pool-backed adapter for
// the domain port. It is the composition-root entrypoint of this package.
type Store struct {
	pool *pgxpool.Pool
	log  *slog.Logger

	Projects domain.ProjectRepository
}

// New opens the project database and runs migrations.
func New(ctx context.Context, dsn string, log *slog.Logger) (*Store, error) {
	pool, err := db.Pool(ctx, dsn, log)
	if err != nil {
		return nil, err
	}
	if err := db.Migrate(ctx, pool, migrations, "migrations", log); err != nil {
		return nil, err
	}
	s := &Store{pool: pool, log: log}
	s.Projects = &projectRepo{q: pool}
	return s, nil
}

// Close releases the connection pool.
func (s *Store) Close() { s.pool.Close() }

const projectCols = `id, workspace_id, name, repo_source, repo_type, cloned_path, default_branch, created_at`

func scanProject(row pgx.Row) (tasks.Project, error) {
	var p tasks.Project
	err := row.Scan(&p.ID, &p.WorkspaceID, &p.Name, &p.RepoSource, &p.RepoType, &p.ClonedPath, &p.DefaultBranch, &p.CreatedAt)
	return p, err
}

// whereScopedAt builds `AND workspace_id = ANY($n)` style scoping for a
// workspace set. start is the 1-based index of the next statement parameter.
func whereScopedAt(start int, ws []identity.ID) (string, []any) {
	if len(ws) == 0 {
		return " AND false", nil
	}
	ids := make([]string, len(ws))
	for i, id := range ws {
		ids[i] = string(id)
	}
	return fmt.Sprintf(" AND workspace_id = ANY($%d::uuid[])", start), []any{ids}
}

// whereScoped is whereScopedAt at parameter index 1.
func whereScoped(ws []identity.ID) (string, []any) {
	return whereScopedAt(1, ws)
}

type projectRepo struct{ q querier }

// List returns all projects in the workspace set, newest first. An empty
// workspace set returns no rows (fail closed).
func (r *projectRepo) List(ctx context.Context, ws []identity.ID) ([]tasks.Project, error) {
	where, args := whereScoped(ws)
	rows, err := r.q.Query(ctx, `SELECT `+projectCols+` FROM projects WHERE 1=1`+where+` ORDER BY created_at DESC`, args...)
	if err != nil {
		return nil, fmt.Errorf("project list: %w", err)
	}
	defer rows.Close()
	out := []tasks.Project{}
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Get returns one project by id, scoped to the workspace set.
func (r *projectRepo) Get(ctx context.Context, id identity.ID, ws []identity.ID) (tasks.Project, error) {
	where, args := whereScopedAt(2, ws) // $1=id, $2=ws ids
	row := r.q.QueryRow(ctx, `SELECT `+projectCols+` FROM projects WHERE id = $1`+where, append([]any{id}, args...)...)
	p, err := scanProject(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return tasks.Project{}, domain.ErrNotFound
	}
	return p, err
}

// Create inserts a project in the given workspace and returns it.
func (r *projectRepo) Create(ctx context.Context, workspaceID identity.ID, in domain.CreateInput) (tasks.Project, error) {
	if in.DefaultBranch == "" {
		in.DefaultBranch = "main"
	}
	row := r.q.QueryRow(ctx, `
		INSERT INTO projects (workspace_id, name, repo_source, repo_type, default_branch)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+projectCols,
		workspaceID, in.Name, in.RepoSource, in.RepoType, in.DefaultBranch)
	return scanProject(row)
}

// Update partially updates a project, scoped to the workspace set.
func (r *projectRepo) Update(ctx context.Context, id identity.ID, ws []identity.ID, in domain.UpdateInput) (tasks.Project, error) {
	where, scopeArgs := whereScoped(ws)
	if len(scopeArgs) == 0 {
		return tasks.Project{}, domain.ErrNotFound
	}
	args := append(scopeArgs, id) // [$1..$n = workspace ids, then id]
	idIdx := len(args)
	sets := []string{}
	idx := idIdx + 1
	if in.Name != nil {
		sets = append(sets, fmt.Sprintf("name = $%d", idx)); args = append(args, *in.Name); idx++
	}
	if in.RepoSource != nil {
		sets = append(sets, fmt.Sprintf("repo_source = $%d", idx)); args = append(args, *in.RepoSource); idx++
	}
	if in.RepoType != nil {
		sets = append(sets, fmt.Sprintf("repo_type = $%d", idx)); args = append(args, *in.RepoType); idx++
	}
	if in.DefaultBranch != nil {
		sets = append(sets, fmt.Sprintf("default_branch = $%d", idx)); args = append(args, *in.DefaultBranch); idx++
	}
	if in.ClonedPath != nil {
		sets = append(sets, fmt.Sprintf("cloned_path = $%d", idx)); args = append(args, *in.ClonedPath)
	}
	if len(sets) == 0 {
		// Nothing to update; return current.
		return r.Get(ctx, id, ws)
	}
	// id binds to $idIdx (the last placeholder) since whereScoped($1..) precedes it.
	q := `UPDATE projects SET ` + strings.Join(sets, ", ") + ` WHERE id = $` + fmt.Sprint(idIdx) + where + ` RETURNING ` + projectCols
	row := r.q.QueryRow(ctx, q, args...)
	p, err := scanProject(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return tasks.Project{}, domain.ErrNotFound
	}
	return p, err
}

// Delete removes a project, scoped to the workspace set. Returns ErrNotFound
// if absent or outside the workspace context.
func (r *projectRepo) Delete(ctx context.Context, id identity.ID, ws []identity.ID) error {
	where, args := whereScopedAt(2, ws) // $1=id, $2=ws ids
	if len(args) == 0 {
		return domain.ErrNotFound
	}
	tag, err := r.q.Exec(ctx, `DELETE FROM projects WHERE id = $1`+where, append([]any{id}, args...)...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}