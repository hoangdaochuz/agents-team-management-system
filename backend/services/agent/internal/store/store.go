// Package store is the Agent service persistence layer.
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

var ErrAgentNotFound = errors.New("agent not found")

type Store struct{ pool *pgxpool.Pool }

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

// agentSelect returns one agent row including aggregated skill/mcp id arrays.
const agentSelect = `
SELECT a.id, a.name, a.role, a.system_prompt, a.default_model, a.allowed_tools,
       a.status, a.load, a.current_task_id::text, a.created_at,
       COALESCE((SELECT array_agg(skill_id) FROM agent_skills WHERE agent_id = a.id), '{}'),
       COALESCE((SELECT array_agg(mcp_id)   FROM agent_mcps   WHERE agent_id = a.id), '{}')
FROM agents a`

func scanAgent(row pgx.Row) (contracts.Agent, error) {
	var a contracts.Agent
	var load *int
	var currentTask *string
	err := row.Scan(&a.ID, &a.Name, &a.Role, &a.SystemPrompt, &a.DefaultModel, &a.AllowedTools,
		&a.Status, &load, &currentTask, &a.CreatedAt, &a.SkillIDs, &a.McpIDs)
	if err != nil {
		return contracts.Agent{}, err
	}
	a.Load = load
	if currentTask != nil {
		id := contracts.ID(*currentTask)
		a.CurrentTaskID = &id
	}
	if a.AllowedTools == nil {
		a.AllowedTools = []string{}
	}
	if a.SkillIDs == nil {
		a.SkillIDs = []contracts.ID{}
	}
	if a.McpIDs == nil {
		a.McpIDs = []contracts.ID{}
	}
	return a, nil
}

func (s *Store) List(ctx context.Context) ([]contracts.Agent, error) {
	rows, err := s.pool.Query(ctx, agentSelect+` ORDER BY a.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []contracts.Agent{}
	for rows.Next() {
		a, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) Get(ctx context.Context, id contracts.ID) (contracts.Agent, error) {
	row := s.pool.QueryRow(ctx, agentSelect+` WHERE a.id = $1`, id)
	a, err := scanAgent(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return contracts.Agent{}, ErrAgentNotFound
	}
	return a, err
}

type CreateInput struct {
	Name         string   `json:"name"`
	Role         string   `json:"role"`
	SystemPrompt string   `json:"system_prompt,omitempty"`
	DefaultModel string   `json:"default_model,omitempty"`
	AllowedTools []string `json:"allowed_tools,omitempty"`
}

func (s *Store) Create(ctx context.Context, in CreateInput) (contracts.Agent, error) {
	if in.AllowedTools == nil {
		in.AllowedTools = []string{}
	}
	row := s.pool.QueryRow(ctx, `
		INSERT INTO agents (name, role, system_prompt, default_model, allowed_tools)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`, in.Name, in.Role, in.SystemPrompt, in.DefaultModel, in.AllowedTools)
	var id contracts.ID
	if err := row.Scan(&id); err != nil {
		return contracts.Agent{}, err
	}
	return s.Get(ctx, id)
}

type UpdateInput struct {
	Name           *string  `json:"name,omitempty"`
	Role           *string  `json:"role,omitempty"`
	SystemPrompt   *string  `json:"system_prompt,omitempty"`
	DefaultModel   *string  `json:"default_model,omitempty"`
	AllowedTools   *[]string `json:"allowed_tools,omitempty"`
	Status         *string  `json:"status,omitempty"`
}

func (s *Store) Update(ctx context.Context, id contracts.ID, in UpdateInput) (contracts.Agent, error) {
	sets, args := []string{}, []any{id}
	idx := 2
	add := func(col string, v any) { sets = append(sets, fmt.Sprintf("%s = $%d", col, idx)); args = append(args, v); idx++ }
	if in.Name != nil {
		add("name", *in.Name)
	}
	if in.Role != nil {
		add("role", *in.Role)
	}
	if in.SystemPrompt != nil {
		add("system_prompt", *in.SystemPrompt)
	}
	if in.DefaultModel != nil {
		add("default_model", *in.DefaultModel)
	}
	if in.AllowedTools != nil {
		add("allowed_tools", *in.AllowedTools)
	}
	if in.Status != nil {
		add("status", *in.Status)
	}
	if len(sets) > 0 {
		q := `UPDATE agents SET ` + strings.Join(sets, ", ") + ` WHERE id = $1`
		tag, err := s.pool.Exec(ctx, q, args...)
		if err != nil {
			return contracts.Agent{}, err
		}
		if tag.RowsAffected() == 0 {
			return contracts.Agent{}, ErrAgentNotFound
		}
	}
	return s.Get(ctx, id)
}

func (s *Store) Delete(ctx context.Context, id contracts.ID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM agents WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrAgentNotFound
	}
	return nil
}

// AttachSkill / DetachSkill manage the agent_skills link.
func (s *Store) AttachSkill(ctx context.Context, agentID, skillID contracts.ID) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO agent_skills (agent_id, skill_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, agentID, skillID)
	return err
}

func (s *Store) DetachSkill(ctx context.Context, agentID, skillID contracts.ID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM agent_skills WHERE agent_id = $1 AND skill_id = $2`, agentID, skillID)
	return err
}

// AttachMcp / DetachMcp manage the agent_mcps link.
func (s *Store) AttachMcp(ctx context.Context, agentID, mcpID contracts.ID) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO agent_mcps (agent_id, mcp_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, agentID, mcpID)
	return err
}

func (s *Store) DetachMcp(ctx context.Context, agentID, mcpID contracts.ID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM agent_mcps WHERE agent_id = $1 AND mcp_id = $2`, agentID, mcpID)
	return err
}
