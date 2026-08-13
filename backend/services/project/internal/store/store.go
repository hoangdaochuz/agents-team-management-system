// Package store is the Project service's persistence layer over pgx.
package store

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aaks/server/internal/contracts"
	"github.com/aaks/server/internal/db"
)

//go:embed migrations/*.sql
var migrations embed.FS

// ErrNotFound is returned when a project id does not exist.
var ErrNotFound = errors.New("project not found")

// ProjectStore owns project persistence.
type ProjectStore struct {
	pool *pgxpool.Pool
}

// New opens the pool, runs embedded migrations, and returns the store.
func New(ctx context.Context, dsn string, log *slog.Logger) (*ProjectStore, error) {
	pool, err := db.Pool(ctx, dsn, log)
	if err != nil {
		return nil, err
	}
	if err := db.MigrateFS(ctx, pool, migrations, "migrations", log); err != nil {
		return nil, err
	}
	return &ProjectStore{pool: pool}, nil
}

// Close releases the pool.
func (s *ProjectStore) Close() { s.pool.Close() }

const projectCols = `id, workspace_id, name, repo_source, repo_type, cloned_path, default_branch, created_at`

func scanProject(row pgx.Row) (contracts.Project, error) {
	var p contracts.Project
	err := row.Scan(&p.ID, &p.WorkspaceID, &p.Name, &p.RepoSource, &p.RepoType, &p.ClonedPath, &p.DefaultBranch, &p.CreatedAt)
	return p, err
}

// whereScoped builds `AND workspace_id = ANY($n)` style scoping for a workspace set.
func whereScopedAt(start int, ws []contracts.ID) (string, []any) {
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
func whereScoped(ws []contracts.ID) (string, []any) {
	return whereScopedAt(1, ws)
}

// List returns all projects in the workspace set, newest first. An empty
// workspace set returns no rows (fail closed).
func (s *ProjectStore) List(ctx context.Context, ws []contracts.ID) ([]contracts.Project, error) {
	where, args := whereScoped(ws)
	rows, err := s.pool.Query(ctx, `SELECT `+projectCols+` FROM projects WHERE 1=1`+where+` ORDER BY created_at DESC`, args...)
	if err != nil {
		return nil, fmt.Errorf("project list: %w", err)
	}
	defer rows.Close()
	out := []contracts.Project{}
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
func (s *ProjectStore) Get(ctx context.Context, id contracts.ID, ws []contracts.ID) (contracts.Project, error) {
	where, args := whereScopedAt(2, ws) // $1=id, $2=ws ids
	row := s.pool.QueryRow(ctx, `SELECT `+projectCols+` FROM projects WHERE id = $1`+where, append([]any{id}, args...)...)
	p, err := scanProject(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return contracts.Project{}, ErrNotFound
	}
	return p, err
}

// CreateInput is the body of POST /projects.
type CreateInput struct {
	Name          string                `json:"name"`
	RepoSource    string                `json:"repo_source"`
	RepoType      contracts.RepoType    `json:"repo_type"`
	DefaultBranch string                `json:"default_branch,omitempty"`
}

// Create inserts a project in the given workspace and returns it.
func (s *ProjectStore) Create(ctx context.Context, workspaceID contracts.ID, in CreateInput) (contracts.Project, error) {
	if in.DefaultBranch == "" {
		in.DefaultBranch = "main"
	}
	row := s.pool.QueryRow(ctx, `
		INSERT INTO projects (workspace_id, name, repo_source, repo_type, default_branch)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+projectCols,
		workspaceID, in.Name, in.RepoSource, in.RepoType, in.DefaultBranch)
	return scanProject(row)
}

// Update applies non-nil fields of UpdateInput to the project.
type UpdateInput struct {
	Name           *string             `json:"name,omitempty"`
	RepoSource     *string             `json:"repo_source,omitempty"`
	RepoType       *contracts.RepoType `json:"repo_type,omitempty"`
	DefaultBranch  *string             `json:"default_branch,omitempty"`
	ClonedPath     *string             `json:"cloned_path,omitempty"`
}

// Update partially updates a project, scoped to the workspace set.
func (s *ProjectStore) Update(ctx context.Context, id contracts.ID, ws []contracts.ID, in UpdateInput) (contracts.Project, error) {
	where, scopeArgs := whereScoped(ws)
	if len(scopeArgs) == 0 {
		return contracts.Project{}, ErrNotFound
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
		sets = append(sets, fmt.Sprintf("cloned_path = $%d", idx)); args = append(args, *in.ClonedPath); idx++
	}
	if len(sets) == 0 {
		// Nothing to update; return current.
		return s.Get(ctx, id, ws)
	}
	// id binds to $idIdx (the last placeholder) since whereScoped($1..) precedes it.
	q := `UPDATE projects SET ` + strings.Join(sets, ", ") + ` WHERE id = $` + fmt.Sprint(idIdx) + where + ` RETURNING ` + projectCols
	row := s.pool.QueryRow(ctx, q, args...)
	p, err := scanProject(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return contracts.Project{}, ErrNotFound
	}
	return p, err
}

// Delete removes a project, scoped to the workspace set. Returns ErrNotFound
// if absent or outside the workspace context.
func (s *ProjectStore) Delete(ctx context.Context, id contracts.ID, ws []contracts.ID) error {
	where, args := whereScopedAt(2, ws) // $1=id, $2=ws ids
	if len(args) == 0 {
		return ErrNotFound
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM projects WHERE id = $1`+where, append([]any{id}, args...)...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
