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

const skillCols = `id, name, description, body_md, resources_path, created_at`

func scanSkill(row pgx.Row) (contracts.Skill, error) {
	var sk contracts.Skill
	err := row.Scan(&sk.ID, &sk.Name, &sk.Description, &sk.BodyMd, &sk.ResourcesPath, &sk.CreatedAt)
	return sk, err
}

func (s *Store) ListSkills(ctx context.Context) ([]contracts.Skill, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+skillCols+` FROM skills ORDER BY created_at DESC`)
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

func (s *Store) GetSkill(ctx context.Context, id contracts.ID) (contracts.Skill, error) {
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
}

func (s *Store) CreateSkill(ctx context.Context, in SkillCreateInput) (contracts.Skill, error) {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO skills (name, description, body_md)
		VALUES ($1, $2, $3)
		RETURNING `+skillCols, in.Name, in.Description, in.BodyMd)
	return scanSkill(row)
}

type SkillUpdateInput struct {
	Name          *string `json:"name,omitempty"`
	Description   *string `json:"description,omitempty"`
	BodyMd        *string `json:"body_md,omitempty"`
	ResourcesPath *string `json:"resources_path,omitempty"`
}

func (s *Store) UpdateSkill(ctx context.Context, id contracts.ID, in SkillUpdateInput) (contracts.Skill, error) {
	sets, args := []string{}, []any{id}
	idx := 2
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
	if len(sets) == 0 {
		return s.GetSkill(ctx, id)
	}
	q := `UPDATE skills SET ` + strings.Join(sets, ", ") + ` WHERE id = $1 RETURNING ` + skillCols
	row := s.pool.QueryRow(ctx, q, args...)
	sk, err := scanSkill(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return contracts.Skill{}, ErrSkillNotFound
	}
	return sk, err
}

func (s *Store) DeleteSkill(ctx context.Context, id contracts.ID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM skills WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrSkillNotFound
	}
	return nil
}

// ── MCP servers ─────────────────────────────────────────────────────────────

const mcpCols = `id, name, command, args, env, created_at`

func scanMcp(row pgx.Row) (contracts.McpServer, error) {
	var m contracts.McpServer
	var envRaw []byte
	if err := row.Scan(&m.ID, &m.Name, &m.Command, &m.Args, &envRaw, &m.CreatedAt); err != nil {
		return contracts.McpServer{}, err
	}
	m.Env = map[string]string{}
	if len(envRaw) > 0 {
		_ = json.Unmarshal(envRaw, &m.Env)
	}
	return m, nil
}

func (s *Store) ListMcp(ctx context.Context) ([]contracts.McpServer, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+mcpCols+` FROM mcp_servers ORDER BY created_at DESC`)
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

func (s *Store) GetMcp(ctx context.Context, id contracts.ID) (contracts.McpServer, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+mcpCols+` FROM mcp_servers WHERE id = $1`, id)
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

func (s *Store) CreateMcp(ctx context.Context, in McpCreateInput) (contracts.McpServer, error) {
	if in.Args == nil {
		in.Args = []string{}
	}
	envJSON, _ := json.Marshal(orDefault(in.Env, map[string]string{}))
	row := s.pool.QueryRow(ctx, `
		INSERT INTO mcp_servers (name, command, args, env)
		VALUES ($1, $2, $3, $4)
		RETURNING `+mcpCols, in.Name, in.Command, in.Args, envJSON)
	return scanMcp(row)
}

type McpUpdateInput struct {
	Name    *string            `json:"name,omitempty"`
	Command *string            `json:"command,omitempty"`
	Args    *[]string          `json:"args,omitempty"`
	Env     *map[string]string `json:"env,omitempty"`
}

func (s *Store) UpdateMcp(ctx context.Context, id contracts.ID, in McpUpdateInput) (contracts.McpServer, error) {
	sets, args := []string{}, []any{id}
	idx := 2
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
		return s.GetMcp(ctx, id)
	}
	q := `UPDATE mcp_servers SET ` + strings.Join(sets, ", ") + ` WHERE id = $1 RETURNING ` + mcpCols
	row := s.pool.QueryRow(ctx, q, args...)
	m, err := scanMcp(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return contracts.McpServer{}, ErrMcpNotFound
	}
	return m, err
}

func (s *Store) DeleteMcp(ctx context.Context, id contracts.ID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM mcp_servers WHERE id = $1`, id)
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
