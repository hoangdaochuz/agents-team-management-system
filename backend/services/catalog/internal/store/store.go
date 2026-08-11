// Package store is the Catalog service persistence layer (Skills + MCP servers).
package store

import (
	"context"
	"embed"
	"encoding/json"
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

var (
	ErrSkillNotFound = errors.New("skill not found")
	ErrMcpNotFound   = errors.New("mcp server not found")
)

// Store owns Skills + MCP server persistence.
type Store struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, dsn string, log *slog.Logger) (*Store, error) {
	pool, err := db.Pool(ctx, dsn, log)
	if err != nil {
		return nil, err
	}
	if err := db.MigrateFS(ctx, pool, migrations, "migrations", log); err != nil {
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

// ── Skills ──────────────────────────────────────────────────────────────────

const skillCols = `id, workspace_id, name, description, body_md, resources_path, enabled, trigger, step_count, created_at`

func scanSkill(row pgx.Row) (contracts.Skill, error) {
	var sk contracts.Skill
	var enabled *bool
	var stepCount *int
	err := row.Scan(&sk.ID, &sk.WorkspaceID, &sk.Name, &sk.Description, &sk.BodyMd, &sk.ResourcesPath,
		&enabled, &sk.Trigger, &stepCount, &sk.CreatedAt)
	if err != nil {
		return contracts.Skill{}, err
	}
	sk.Enabled = enabled
	sk.StepCount = stepCount
	return sk, nil
}

// whereScopedAt returns `AND workspace_id = ANY($start)` scoping plus args.
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

func (s *Store) ListSkills(ctx context.Context, ws []contracts.ID) ([]contracts.Skill, error) {
	where, args := whereScopedAt(1, ws)
	rows, err := s.pool.Query(ctx, `SELECT `+skillCols+` FROM skills WHERE 1=1`+where+` ORDER BY created_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []contracts.Skill{}
	for rows.Next() {
		sk, err := scanSkill(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sk)
	}
	return out, rows.Err()
}

func (s *Store) GetSkill(ctx context.Context, id contracts.ID, ws []contracts.ID) (contracts.Skill, error) {
	where, args := whereScopedAt(2, ws)
	row := s.pool.QueryRow(ctx, `SELECT `+skillCols+` FROM skills WHERE id = $1`+where, append([]any{id}, args...)...)
	sk, err := scanSkill(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return contracts.Skill{}, ErrSkillNotFound
	}
	return sk, err
}

// GetSkillUnscoped returns a skill without workspace filtering (event emission).
func (s *Store) GetSkillUnscoped(ctx context.Context, id contracts.ID) (contracts.Skill, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+skillCols+` FROM skills WHERE id = $1`, id)
	sk, err := scanSkill(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return contracts.Skill{}, ErrSkillNotFound
	}
	return sk, err
}

type SkillCreateInput struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	BodyMd      string `json:"body_md"`
	Trigger     string `json:"trigger,omitempty"`
}

func (s *Store) CreateSkill(ctx context.Context, workspaceID contracts.ID, in SkillCreateInput) (contracts.Skill, error) {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO skills (workspace_id, name, description, body_md, trigger)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+skillCols, workspaceID, in.Name, in.Description, in.BodyMd, in.Trigger)
	return scanSkill(row)
}

type SkillUpdateInput struct {
	Name          *string `json:"name,omitempty"`
	Description   *string `json:"description,omitempty"`
	BodyMd        *string `json:"body_md,omitempty"`
	ResourcesPath *string `json:"resources_path,omitempty"`
	Enabled       *bool   `json:"enabled,omitempty"`
	Trigger       *string `json:"trigger,omitempty"`
	StepCount     *int    `json:"step_count,omitempty"`
}

func (s *Store) UpdateSkill(ctx context.Context, id contracts.ID, ws []contracts.ID, in SkillUpdateInput) (contracts.Skill, error) {
	where, scopeArgs := whereScopedAt(2, ws)
	if len(scopeArgs) == 0 {
		return contracts.Skill{}, ErrSkillNotFound
	}
	sets, args := []string{}, append([]any{id}, scopeArgs...)
	idx := 2 + len(scopeArgs)
	add := func(col string, v any) { sets = append(sets, fmt.Sprintf("%s = $%d", col, idx)); args = append(args, v); idx++ }
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
		return s.GetSkill(ctx, id, ws)
	}
	q := `UPDATE skills SET ` + strings.Join(sets, ", ") + ` WHERE id = $1` + where + ` RETURNING ` + skillCols
	row := s.pool.QueryRow(ctx, q, args...)
	sk, err := scanSkill(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return contracts.Skill{}, ErrSkillNotFound
	}
	return sk, err
}

func (s *Store) DeleteSkill(ctx context.Context, id contracts.ID, ws []contracts.ID) error {
	where, args := whereScopedAt(2, ws)
	if len(args) == 0 {
		return ErrSkillNotFound
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM skills WHERE id = $1`+where, append([]any{id}, args...)...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrSkillNotFound
	}
	return nil
}

// ListSkillsByWorkspace lists skills of exactly one workspace (scoped path).
func (s *Store) ListSkillsByWorkspace(ctx context.Context, workspaceID contracts.ID) ([]contracts.Skill, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+skillCols+` FROM skills WHERE workspace_id = $1 ORDER BY created_at DESC`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []contracts.Skill{}
	for rows.Next() {
		sk, err := scanSkill(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sk)
	}
	return out, rows.Err()
}

// SetSkillEnabled toggles the workspace-level enable state (scoped path).
func (s *Store) SetSkillEnabled(ctx context.Context, workspaceID, id contracts.ID, enabled bool) (contracts.Skill, error) {
	row := s.pool.QueryRow(ctx, `
		UPDATE skills SET enabled = $3 WHERE id = $1 AND workspace_id = $2
		RETURNING `+skillCols, id, workspaceID, enabled)
	sk, err := scanSkill(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return contracts.Skill{}, ErrSkillNotFound
	}
	return sk, err
}

// ── MCP servers ─────────────────────────────────────────────────────────────

const mcpCols = `id, workspace_id, name, command, args, env, created_at`

func scanMcp(row pgx.Row) (contracts.McpServer, error) {
	var m contracts.McpServer
	var envRaw []byte
	if err := row.Scan(&m.ID, &m.WorkspaceID, &m.Name, &m.Command, &m.Args, &envRaw, &m.CreatedAt); err != nil {
		return contracts.McpServer{}, err
	}
	m.Env = map[string]string{}
	if len(envRaw) > 0 {
		_ = json.Unmarshal(envRaw, &m.Env)
	}
	return m, nil
}

func (s *Store) ListMcp(ctx context.Context, ws []contracts.ID) ([]contracts.McpServer, error) {
	where, args := whereScopedAt(1, ws)
	rows, err := s.pool.Query(ctx, `SELECT `+mcpCols+` FROM mcp_servers WHERE 1=1`+where+` ORDER BY created_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []contracts.McpServer{}
	for rows.Next() {
		m, err := scanMcp(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ListMcpByIDs returns MCP server definitions for the given IDs (internal,
// trusted callers — e.g. the Agent service hydrating an agent's attached
// servers). Empty ids returns nil.
func (s *Store) ListMcpByIDs(ctx context.Context, ids []contracts.ID) ([]contracts.McpServer, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	raw := make([]string, len(ids))
	for i, id := range ids {
		raw[i] = string(id)
	}
	rows, err := s.pool.Query(ctx, `SELECT `+mcpCols+` FROM mcp_servers WHERE id = ANY($1::uuid[]) ORDER BY created_at DESC`, raw)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []contracts.McpServer{}
	for rows.Next() {
		m, err := scanMcp(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) GetMcp(ctx context.Context, id contracts.ID, ws []contracts.ID) (contracts.McpServer, error) {
	where, args := whereScopedAt(2, ws)
	row := s.pool.QueryRow(ctx, `SELECT `+mcpCols+` FROM mcp_servers WHERE id = $1`+where, append([]any{id}, args...)...)
	m, err := scanMcp(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return contracts.McpServer{}, ErrMcpNotFound
	}
	return m, err
}

type McpCreateInput struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

func (s *Store) CreateMcp(ctx context.Context, workspaceID contracts.ID, in McpCreateInput) (contracts.McpServer, error) {
	if in.Args == nil {
		in.Args = []string{}
	}
	envJSON, _ := json.Marshal(orDefault(in.Env, map[string]string{}))
	row := s.pool.QueryRow(ctx, `
		INSERT INTO mcp_servers (workspace_id, name, command, args, env)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+mcpCols, workspaceID, in.Name, in.Command, in.Args, envJSON)
	return scanMcp(row)
}

type McpUpdateInput struct {
	Name    *string            `json:"name,omitempty"`
	Command *string            `json:"command,omitempty"`
	Args    *[]string          `json:"args,omitempty"`
	Env     *map[string]string `json:"env,omitempty"`
}

func (s *Store) UpdateMcp(ctx context.Context, id contracts.ID, ws []contracts.ID, in McpUpdateInput) (contracts.McpServer, error) {
	where, scopeArgs := whereScopedAt(2, ws)
	if len(scopeArgs) == 0 {
		return contracts.McpServer{}, ErrMcpNotFound
	}
	sets, args := []string{}, append([]any{id}, scopeArgs...)
	idx := 2 + len(scopeArgs)
	if in.Name != nil {
		sets = append(sets, fmt.Sprintf("name = $%d", idx)); args = append(args, *in.Name); idx++
	}
	if in.Command != nil {
		sets = append(sets, fmt.Sprintf("command = $%d", idx)); args = append(args, *in.Command); idx++
	}
	if in.Args != nil {
		sets = append(sets, fmt.Sprintf("args = $%d", idx)); args = append(args, *in.Args); idx++
	}
	if in.Env != nil {
		envJSON, _ := json.Marshal(*in.Env)
		sets = append(sets, fmt.Sprintf("env = $%d", idx)); args = append(args, envJSON); idx++
	}
	if len(sets) == 0 {
		return s.GetMcp(ctx, id, ws)
	}
	q := `UPDATE mcp_servers SET ` + strings.Join(sets, ", ") + ` WHERE id = $1` + where + ` RETURNING ` + mcpCols
	row := s.pool.QueryRow(ctx, q, args...)
	m, err := scanMcp(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return contracts.McpServer{}, ErrMcpNotFound
	}
	return m, err
}

func (s *Store) DeleteMcp(ctx context.Context, id contracts.ID, ws []contracts.ID) error {
	where, args := whereScopedAt(2, ws)
	if len(args) == 0 {
		return ErrMcpNotFound
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM mcp_servers WHERE id = $1`+where, append([]any{id}, args...)...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrMcpNotFound
	}
	return nil
}

func orDefault(m map[string]string, def map[string]string) map[string]string {
	if m == nil {
		return def
	}
	return m
}
