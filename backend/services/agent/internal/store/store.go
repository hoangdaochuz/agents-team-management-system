// Package store is the Agent service persistence layer: agents (including the
// agent-builder fields), skill/mcp link tables, and local projections of the
// catalog's skill/MCP definitions used to validate attachments within a
// workspace (no service-to-service sync calls).
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

// whereScopedAt returns `AND a.workspace_id = ANY($start)` scoping plus args.
func whereScopedAt(start int, ws []contracts.ID) (string, []any) {
	if len(ws) == 0 {
		return " AND false", nil
	}
	ids := make([]string, len(ws))
	for i, id := range ws {
		ids[i] = string(id)
	}
	return fmt.Sprintf(" AND a.workspace_id = ANY($%d::uuid[])", start), []any{ids}
}

// agentSelect returns one agent row including aggregated skill/mcp id arrays
// and the builder fields.
const agentSelect = `
SELECT a.id, a.workspace_id, a.name, a.role, a.system_prompt, a.default_model, a.allowed_tools,
       a.status, a.load, a.current_task_id::text, a.created_at,
       a.role_title, a.provider, a.temperature, a.max_output_tokens, a.autonomy_mode,
       a.user_prompt_template, a.knowledge_source_ids, a.guardrails,
       COALESCE((SELECT array_agg(skill_id) FROM agent_skills WHERE agent_id = a.id), '{}'),
       COALESCE((SELECT array_agg(mcp_id)   FROM agent_mcps   WHERE agent_id = a.id), '{}')
FROM agents a`

func scanAgent(row pgx.Row) (contracts.Agent, error) {
	var a contracts.Agent
	var load *int
	var currentTask *string
	var temperature *float64
	var guardrails []byte
	err := row.Scan(&a.ID, &a.WorkspaceID, &a.Name, &a.Role, &a.SystemPrompt, &a.DefaultModel, &a.AllowedTools,
		&a.Status, &load, &currentTask, &a.CreatedAt,
		&a.RoleTitle, &a.Provider, &temperature, &a.MaxOutputTokens, &a.AutonomyMode,
		&a.UserPromptTemplate, &a.KnowledgeSourceIDs, &guardrails,
		&a.SkillIDs, &a.McpIDs)
	if err != nil {
		return contracts.Agent{}, err
	}
	a.Load = load
	a.Temperature = temperature
	if currentTask != nil {
		id := contracts.ID(*currentTask)
		a.CurrentTaskID = &id
	}
	if len(guardrails) > 0 && string(guardrails) != "{}" && string(guardrails) != "null" {
		var g contracts.Guardrails
		if err := json.Unmarshal(guardrails, &g); err == nil {
			a.Guardrails = &g
		}
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
	if a.KnowledgeSourceIDs == nil {
		a.KnowledgeSourceIDs = []contracts.ID{}
	}
	return a, nil
}

func (s *Store) List(ctx context.Context, ws []contracts.ID) ([]contracts.Agent, error) {
	where, args := whereScopedAt(1, ws)
	rows, err := s.pool.Query(ctx, agentSelect+` WHERE 1=1`+where+` ORDER BY a.created_at DESC`, args...)
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

func (s *Store) Get(ctx context.Context, id contracts.ID, ws []contracts.ID) (contracts.Agent, error) {
	where, args := whereScopedAt(2, ws)
	row := s.pool.QueryRow(ctx, agentSelect+` WHERE a.id = $1`+where, append([]any{id}, args...)...)
	a, err := scanAgent(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return contracts.Agent{}, ErrAgentNotFound
	}
	return a, err
}

// GetUnscoped returns an agent without a workspace filter. Used only by the
// saga consumers and attachment validation where the agent is already trusted.
func (s *Store) GetUnscoped(ctx context.Context, id contracts.ID) (contracts.Agent, error) {
	row := s.pool.QueryRow(ctx, agentSelect+` WHERE a.id = $1`, id)
	a, err := scanAgent(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return contracts.Agent{}, ErrAgentNotFound
	}
	return a, err
}

type CreateInput struct {
	Name               string               `json:"name"`
	Role               string               `json:"role"`
	SystemPrompt       string               `json:"system_prompt,omitempty"`
	DefaultModel       string               `json:"default_model,omitempty"`
	AllowedTools       []string             `json:"allowed_tools,omitempty"`
	RoleTitle          string               `json:"role_title,omitempty"`
	Provider           contracts.Provider   `json:"provider,omitempty"`
	Temperature        *float64             `json:"temperature,omitempty"`
	MaxOutputTokens    *int                 `json:"max_output_tokens,omitempty"`
	AutonomyMode       contracts.AutonomyMode `json:"autonomy_mode,omitempty"`
	UserPromptTemplate string               `json:"user_prompt_template,omitempty"`
	KnowledgeSourceIDs []contracts.ID       `json:"knowledge_source_ids,omitempty"`
	Guardrails         *contracts.Guardrails `json:"guardrails,omitempty"`
}

func (s *Store) Create(ctx context.Context, workspaceID contracts.ID, in CreateInput) (contracts.Agent, error) {
	if in.AllowedTools == nil {
		in.AllowedTools = []string{}
	}
	if in.KnowledgeSourceIDs == nil {
		in.KnowledgeSourceIDs = []contracts.ID{}
	}
	gr, err := json.Marshal(in.Guardrails)
	if err != nil {
		return contracts.Agent{}, err
	}
	row := s.pool.QueryRow(ctx, `
		INSERT INTO agents (workspace_id, name, role, system_prompt, default_model, allowed_tools,
			role_title, provider, temperature, max_output_tokens, autonomy_mode,
			user_prompt_template, knowledge_source_ids, guardrails)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING id`,
		workspaceID, in.Name, in.Role, in.SystemPrompt, in.DefaultModel, in.AllowedTools,
		in.RoleTitle, in.Provider, in.Temperature, in.MaxOutputTokens, in.AutonomyMode,
		in.UserPromptTemplate, in.KnowledgeSourceIDs, gr)
	var id contracts.ID
	if err := row.Scan(&id); err != nil {
		return contracts.Agent{}, err
	}
	return s.Get(ctx, id, []contracts.ID{workspaceID})
}

type UpdateInput struct {
	Name               *string                 `json:"name,omitempty"`
	Role               *string                 `json:"role,omitempty"`
	SystemPrompt       *string                 `json:"system_prompt,omitempty"`
	DefaultModel       *string                 `json:"default_model,omitempty"`
	AllowedTools       *[]string               `json:"allowed_tools,omitempty"`
	Status             *string                 `json:"status,omitempty"`
	CurrentTaskID      *string                 `json:"current_task_id,omitempty"`
	RoleTitle          *string                 `json:"role_title,omitempty"`
	Provider           *contracts.Provider     `json:"provider,omitempty"`
	Temperature        *float64                `json:"temperature,omitempty"`
	MaxOutputTokens    *int                    `json:"max_output_tokens,omitempty"`
	AutonomyMode       *contracts.AutonomyMode `json:"autonomy_mode,omitempty"`
	UserPromptTemplate *string                 `json:"user_prompt_template,omitempty"`
	KnowledgeSourceIDs *[]contracts.ID         `json:"knowledge_source_ids,omitempty"`
	Guardrails         *contracts.Guardrails   `json:"guardrails,omitempty"`
}

func (s *Store) Update(ctx context.Context, id contracts.ID, ws []contracts.ID, in UpdateInput) (contracts.Agent, error) {
	where, scopeArgs := whereScopedAt(2, ws)
	if len(scopeArgs) == 0 {
		return contracts.Agent{}, ErrAgentNotFound
	}
	sets, args := []string{}, append([]any{id}, scopeArgs...)
	idx := 2 + len(scopeArgs)
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
	if in.CurrentTaskID != nil {
		if *in.CurrentTaskID == "" {
			add("current_task_id", nil)
		} else {
			add("current_task_id", *in.CurrentTaskID)
		}
	}
	if in.RoleTitle != nil {
		add("role_title", *in.RoleTitle)
	}
	if in.Provider != nil {
		add("provider", *in.Provider)
	}
	if in.Temperature != nil {
		add("temperature", *in.Temperature)
	}
	if in.MaxOutputTokens != nil {
		add("max_output_tokens", *in.MaxOutputTokens)
	}
	if in.AutonomyMode != nil {
		add("autonomy_mode", *in.AutonomyMode)
	}
	if in.UserPromptTemplate != nil {
		add("user_prompt_template", *in.UserPromptTemplate)
	}
	if in.KnowledgeSourceIDs != nil {
		add("knowledge_source_ids", *in.KnowledgeSourceIDs)
	}
	if in.Guardrails != nil {
		gr, err := json.Marshal(in.Guardrails)
		if err != nil {
			return contracts.Agent{}, err
		}
		add("guardrails", gr)
	}
	if len(sets) > 0 {
		// SET placeholders may be non-sequential; Postgres binds by number.
		q := `UPDATE agents SET ` + strings.Join(sets, ", ") + ` WHERE id = $1` + where
		tag, err := s.pool.Exec(ctx, q, args...)
		if err != nil {
			return contracts.Agent{}, err
		}
		if tag.RowsAffected() == 0 {
			return contracts.Agent{}, ErrAgentNotFound
		}
	}
	return s.Get(ctx, id, ws)
}

func (s *Store) Delete(ctx context.Context, id contracts.ID, ws []contracts.ID) error {
	where, args := whereScopedAt(2, ws)
	if len(args) == 0 {
		return ErrAgentNotFound
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM agents WHERE id = $1`+where, append([]any{id}, args...)...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrAgentNotFound
	}
	return nil
}

// CountByWorkspace counts agents in a workspace (Gateway workspace-stats composition).
func (s *Store) CountByWorkspace(ctx context.Context, workspaceID contracts.ID) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM agents WHERE workspace_id = $1`, workspaceID).Scan(&n)
	return n, err
}

// ── Attachments (validated against the agent's workspace) ──────────────────

// ErrCrossWorkspace is returned when an attachment references a skill/MCP
// definition from another workspace.
var ErrCrossWorkspace = errors.New("skill or mcp belongs to another workspace")

// AttachSkill adds a skill to the agent; rejects definitions from another
// workspace (spec: cross-workspace attachment rejected).
func (s *Store) AttachSkill(ctx context.Context, agentID, skillID contracts.ID) error {
	a, err := s.GetUnscoped(ctx, agentID)
	if err != nil {
		return err
	}
	ws, err := s.skillWorkspace(ctx, skillID)
	if err != nil {
		return err // unknown skill: reject
	}
	if ws != a.WorkspaceID {
		return ErrCrossWorkspace
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO agent_skills (agent_id, skill_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, agentID, skillID)
	return err
}

func (s *Store) DetachSkill(ctx context.Context, agentID, skillID contracts.ID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM agent_skills WHERE agent_id = $1 AND skill_id = $2`, agentID, skillID)
	return err
}

// AttachMcp adds an MCP server to the agent; rejects definitions from another
// workspace.
func (s *Store) AttachMcp(ctx context.Context, agentID, mcpID contracts.ID) error {
	a, err := s.GetUnscoped(ctx, agentID)
	if err != nil {
		return err
	}
	ws, err := s.mcpWorkspace(ctx, mcpID)
	if err != nil {
		return err
	}
	if ws != a.WorkspaceID {
		return ErrCrossWorkspace
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO agent_mcps (agent_id, mcp_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, agentID, mcpID)
	return err
}

func (s *Store) DetachMcp(ctx context.Context, agentID, mcpID contracts.ID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM agent_mcps WHERE agent_id = $1 AND mcp_id = $2`, agentID, mcpID)
	return err
}
