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

const projectCols = `id, name, repo_source, repo_type, cloned_path, default_branch, created_at`

func scanProject(row pgx.Row) (contracts.Project, error) {
	var p contracts.Project
	err := row.Scan(&p.ID, &p.Name, &p.RepoSource, &p.RepoType, &p.ClonedPath, &p.DefaultBranch, &p.CreatedAt)
	return p, err
}

// List returns all projects, newest first.
func (s *ProjectStore) List(ctx context.Context) ([]contracts.Project, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+projectCols+` FROM projects ORDER BY created_at DESC`)
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

// Get returns one project by id.
func (s *ProjectStore) Get(ctx context.Context, id contracts.ID) (contracts.Project, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+projectCols+` FROM projects WHERE id = $1`, id)
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

// Create inserts a project and returns it.
func (s *ProjectStore) Create(ctx context.Context, in CreateInput) (contracts.Project, error) {
	if in.DefaultBranch == "" {
		in.DefaultBranch = "main"
	}
	row := s.pool.QueryRow(ctx, `
		INSERT INTO projects (name, repo_source, repo_type, default_branch)
		VALUES ($1, $2, $3, $4)
		RETURNING `+projectCols,
		in.Name, in.RepoSource, in.RepoType, in.DefaultBranch)
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

// Update partially updates a project.
func (s *ProjectStore) Update(ctx context.Context, id contracts.ID, in UpdateInput) (contracts.Project, error) {
	sets := []string{}
	args := []any{id}
	idx := 2
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
		return s.Get(ctx, id)
	}
	q := `UPDATE projects SET ` + strings.Join(sets, ", ") + ` WHERE id = $1 RETURNING ` + projectCols
	row := s.pool.QueryRow(ctx, q, args...)
	p, err := scanProject(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return contracts.Project{}, ErrNotFound
	}
	return p, err
}

// Delete removes a project. Returns ErrNotFound if absent.
func (s *ProjectStore) Delete(ctx context.Context, id contracts.ID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM projects WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
