// Package repository implements the Catalog domain repository ports on Postgres
// (Ports & Adapters: the adapter side of the hexagon). Each aggregate has its
// own adapter; pool-backed instances serve single-aggregate use cases and
// tx-scoped instances are constructed by the UnitOfWork for the definition
// mutations that publish events after commit.
package repository

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/resources"
	"github.com/aaks/server/internal/platform/db"
	"github.com/aaks/server/services/catalog/internal/domain"
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

// Store owns the catalog Postgres pool and exposes pool-backed adapters for
// the domain ports. It is the composition-root entrypoint of this package.
type Store struct {
	pool   *pgxpool.Pool
	log    *slog.Logger
	Skills domain.SkillRepository
	Mcps   domain.McpRepository
}

// New opens the catalog database and runs migrations.
func New(ctx context.Context, dsn string, log *slog.Logger) (*Store, error) {
	pool, err := db.Pool(ctx, dsn, log)
	if err != nil {
		return nil, err
	}
	if err := db.Migrate(ctx, pool, migrations, "migrations", log); err != nil {
		return nil, err
	}
	s := &Store{pool: pool, log: log}
	s.Skills = &skillRepo{q: pool}
	s.Mcps = &mcpRepo{q: pool}
	return s, nil
}

// Close releases the connection pool.
func (s *Store) Close() { s.pool.Close() }

// whereScopedAt returns `AND workspace_id = ANY($start)` scoping plus args.
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

// ── Skills ─────────────────────────────────────────────────────────────────

const skillCols = `id, workspace_id, name, description, body_md, resources_path, enabled, trigger, step_count, created_at`

func scanSkill(row pgx.Row) (resources.Skill, error) {
	var sk resources.Skill
	var enabled *bool
	var stepCount *int
	err := row.Scan(&sk.ID, &sk.WorkspaceID, &sk.Name, &sk.Description, &sk.BodyMd, &sk.ResourcesPath,
		&enabled, &sk.Trigger, &stepCount, &sk.CreatedAt)
	if err != nil {
		return resources.Skill{}, err
	}
	sk.Enabled = enabled
	sk.StepCount = stepCount
	return sk, nil
}

type skillRepo struct{ q querier }

func (r *skillRepo) List(ctx context.Context, ws []identity.ID) ([]resources.Skill, error) {
	where, args := whereScopedAt(1, ws)
	rows, err := r.q.Query(ctx, `SELECT `+skillCols+` FROM skills WHERE 1=1`+where+` ORDER BY created_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []resources.Skill{}
	for rows.Next() {
		sk, err := scanSkill(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sk)
	}
	return out, rows.Err()
}

func (r *skillRepo) Get(ctx context.Context, id identity.ID, ws []identity.ID) (resources.Skill, error) {
	where, args := whereScopedAt(2, ws)
	row := r.q.QueryRow(ctx, `SELECT `+skillCols+` FROM skills WHERE id = $1`+where, append([]any{id}, args...)...)
	sk, err := scanSkill(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return resources.Skill{}, domain.ErrSkillNotFound
	}
	return sk, err
}

func (r *skillRepo) Create(ctx context.Context, workspaceID identity.ID, in domain.SkillCreate) (resources.Skill, error) {
	row := r.q.QueryRow(ctx, `
		INSERT INTO skills (workspace_id, name, description, body_md, trigger)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+skillCols, workspaceID, in.Name, in.Description, in.BodyMd, in.Trigger)
	return scanSkill(row)
}

func (r *skillRepo) Update(ctx context.Context, id identity.ID, ws []identity.ID, in domain.SkillUpdate) (resources.Skill, error) {
	where, scopeArgs := whereScopedAt(2, ws)
	if len(scopeArgs) == 0 {
		return resources.Skill{}, domain.ErrSkillNotFound
	}
	sets, args := []string{}, append([]any{id}, scopeArgs...)
	idx := 2 + len(scopeArgs)
	add := func(col string, v any) {
		sets = append(sets, fmt.Sprintf("%s = $%d", col, idx))
		args = append(args, v)
		idx++
	}
	if in.Name != nil {
		add("name", *in.Name)
	}
	if in.Description != nil {
		add("description", *in.Description)
	}
	if in.BodyMd != nil {
		add("body_md", *in.BodyMd)
	}
	if in.ResourcesPath != nil {
		add("resources_path", *in.ResourcesPath)
	}
	if in.Enabled != nil {
		add("enabled", *in.Enabled)
	}
	if in.Trigger != nil {
		add("trigger", *in.Trigger)
	}
	if in.StepCount != nil {
		add("step_count", *in.StepCount)
	}
	if len(sets) == 0 {
		return r.Get(ctx, id, ws)
	}
	q := `UPDATE skills SET ` + strings.Join(sets, ", ") + ` WHERE id = $1` + where + ` RETURNING ` + skillCols
	row := r.q.QueryRow(ctx, q, args...)
	sk, err := scanSkill(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return resources.Skill{}, domain.ErrSkillNotFound
	}
	return sk, err
}

func (r *skillRepo) Delete(ctx context.Context, id identity.ID, ws []identity.ID) error {
	where, args := whereScopedAt(2, ws)
	if len(args) == 0 {
		return domain.ErrSkillNotFound
	}
	tag, err := r.q.Exec(ctx, `DELETE FROM skills WHERE id = $1`+where, append([]any{id}, args...)...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrSkillNotFound
	}
	return nil
}

func (r *skillRepo) ListByWorkspace(ctx context.Context, workspaceID identity.ID) ([]resources.Skill, error) {
	rows, err := r.q.Query(ctx, `SELECT `+skillCols+` FROM skills WHERE workspace_id = $1 ORDER BY created_at DESC`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []resources.Skill{}
	for rows.Next() {
		sk, err := scanSkill(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sk)
	}
	return out, rows.Err()
}

func (r *skillRepo) SetEnabled(ctx context.Context, workspaceID, id identity.ID, enabled bool) (resources.Skill, error) {
	row := r.q.QueryRow(ctx, `
		UPDATE skills SET enabled = $3 WHERE id = $1 AND workspace_id = $2
		RETURNING `+skillCols, id, workspaceID, enabled)
	sk, err := scanSkill(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return resources.Skill{}, domain.ErrSkillNotFound
	}
	return sk, err
}

// ── MCP servers ─────────────────────────────────────────────────────────────

const mcpCols = `id, workspace_id, name, command, args, env, created_at`

func scanMcp(row pgx.Row) (resources.McpServer, error) {
	var m resources.McpServer
	var envRaw []byte
	if err := row.Scan(&m.ID, &m.WorkspaceID, &m.Name, &m.Command, &m.Args, &envRaw, &m.CreatedAt); err != nil {
		return resources.McpServer{}, err
	}
	m.Env = map[string]string{}
	if len(envRaw) > 0 {
		_ = json.Unmarshal(envRaw, &m.Env)
	}
	return m, nil
}

type mcpRepo struct{ q querier }

func (r *mcpRepo) List(ctx context.Context, ws []identity.ID) ([]resources.McpServer, error) {
	where, args := whereScopedAt(1, ws)
	rows, err := r.q.Query(ctx, `SELECT `+mcpCols+` FROM mcp_servers WHERE 1=1`+where+` ORDER BY created_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []resources.McpServer{}
	for rows.Next() {
		m, err := scanMcp(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *mcpRepo) Get(ctx context.Context, id identity.ID, ws []identity.ID) (resources.McpServer, error) {
	where, args := whereScopedAt(2, ws)
	row := r.q.QueryRow(ctx, `SELECT `+mcpCols+` FROM mcp_servers WHERE id = $1`+where, append([]any{id}, args...)...)
	m, err := scanMcp(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return resources.McpServer{}, domain.ErrMcpNotFound
	}
	return m, err
}

func (r *mcpRepo) Create(ctx context.Context, workspaceID identity.ID, in domain.McpCreate) (resources.McpServer, error) {
	if in.Args == nil {
		in.Args = []string{}
	}
	envJSON, _ := json.Marshal(orDefault(in.Env, map[string]string{}))
	row := r.q.QueryRow(ctx, `
		INSERT INTO mcp_servers (workspace_id, name, command, args, env)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+mcpCols, workspaceID, in.Name, in.Command, in.Args, envJSON)
	return scanMcp(row)
}

func (r *mcpRepo) Update(ctx context.Context, id identity.ID, ws []identity.ID, in domain.McpUpdate) (resources.McpServer, error) {
	where, scopeArgs := whereScopedAt(2, ws)
	if len(scopeArgs) == 0 {
		return resources.McpServer{}, domain.ErrMcpNotFound
	}
	sets, args := []string{}, append([]any{id}, scopeArgs...)
	idx := 2 + len(scopeArgs)
	if in.Name != nil {
		sets = append(sets, fmt.Sprintf("name = $%d", idx))
		args = append(args, *in.Name)
		idx++
	}
	if in.Command != nil {
		sets = append(sets, fmt.Sprintf("command = $%d", idx))
		args = append(args, *in.Command)
		idx++
	}
	if in.Args != nil {
		sets = append(sets, fmt.Sprintf("args = $%d", idx))
		args = append(args, *in.Args)
		idx++
	}
	if in.Env != nil {
		envJSON, _ := json.Marshal(*in.Env)
		sets = append(sets, fmt.Sprintf("env = $%d", idx))
		args = append(args, envJSON)
	}
	if len(sets) == 0 {
		return r.Get(ctx, id, ws)
	}
	q := `UPDATE mcp_servers SET ` + strings.Join(sets, ", ") + ` WHERE id = $1` + where + ` RETURNING ` + mcpCols
	row := r.q.QueryRow(ctx, q, args...)
	m, err := scanMcp(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return resources.McpServer{}, domain.ErrMcpNotFound
	}
	return m, err
}

func (r *mcpRepo) Delete(ctx context.Context, id identity.ID, ws []identity.ID) error {
	where, args := whereScopedAt(2, ws)
	if len(args) == 0 {
		return domain.ErrMcpNotFound
	}
	tag, err := r.q.Exec(ctx, `DELETE FROM mcp_servers WHERE id = $1`+where, append([]any{id}, args...)...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrMcpNotFound
	}
	return nil
}

// ListByIDs returns definitions for the given IDs (internal trusted callers —
// e.g. the Agent service hydrating an agent's attached servers). Empty ids
// returns nil.
func (r *mcpRepo) ListByIDs(ctx context.Context, ids []identity.ID) ([]resources.McpServer, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	raw := make([]string, len(ids))
	for i, id := range ids {
		raw[i] = string(id)
	}
	rows, err := r.q.Query(ctx, `SELECT `+mcpCols+` FROM mcp_servers WHERE id = ANY($1::uuid[]) ORDER BY created_at DESC`, raw)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []resources.McpServer{}
	for rows.Next() {
		m, err := scanMcp(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func orDefault(m map[string]string, def map[string]string) map[string]string {
	if m == nil {
		return def
	}
	return m
}
