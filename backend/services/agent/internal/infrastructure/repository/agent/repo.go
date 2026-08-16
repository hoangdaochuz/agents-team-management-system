// Package agent implements the agent aggregate repository adapter on Postgres
// (Ports & Adapters: the adapter side of the hexagon): CRUD including the
// agent-builder fields, the skill/mcp link tables, and the runtime status
// mutations driven by the run-lifecycle consumers.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/aaks/server/internal/contracts/agentexec"
	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/services/agent/internal/domain"
)

// querier is satisfied by both *pgxpool.Pool and pgx.Tx, letting one adapter
// implementation serve plain and transactional access.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Repo is the pool- or tx-backed adapter for the agent aggregate.
type Repo struct{ q querier }

// New wraps a querier (pool or tx) as the agent adapter.
func New(q querier) *Repo { return &Repo{q: q} }

// whereScopedAt returns `AND a.workspace_id = ANY($start)` scoping plus args.
func whereScopedAt(start int, ws []identity.ID) (string, []any) {
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

func scanAgent(row pgx.Row) (agentexec.Agent, error) {
	var a agentexec.Agent
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
		return agentexec.Agent{}, err
	}
	a.Load = load
	a.Temperature = temperature
	if currentTask != nil {
		id := identity.ID(*currentTask)
		a.CurrentTaskID = &id
	}
	if len(guardrails) > 0 && string(guardrails) != "{}" && string(guardrails) != "null" {
		var g agentexec.Guardrails
		if err := json.Unmarshal(guardrails, &g); err == nil {
			a.Guardrails = &g
		}
	}
	if a.AllowedTools == nil {
		a.AllowedTools = []string{}
	}
	if a.SkillIDs == nil {
		a.SkillIDs = []identity.ID{}
	}
	if a.McpIDs == nil {
		a.McpIDs = []identity.ID{}
	}
	if a.KnowledgeSourceIDs == nil {
		a.KnowledgeSourceIDs = []identity.ID{}
	}
	return a, nil
}

func (r *Repo) List(ctx context.Context, ws []identity.ID) ([]agentexec.Agent, error) {
	where, args := whereScopedAt(1, ws)
	rows, err := r.q.Query(ctx, agentSelect+` WHERE 1=1`+where+` ORDER BY a.created_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []agentexec.Agent{}
	for rows.Next() {
		a, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *Repo) Get(ctx context.Context, id identity.ID, ws []identity.ID) (agentexec.Agent, error) {
	where, args := whereScopedAt(2, ws)
	row := r.q.QueryRow(ctx, agentSelect+` WHERE a.id = $1`+where, append([]any{id}, args...)...)
	a, err := scanAgent(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return agentexec.Agent{}, domain.ErrAgentNotFound
	}
	return a, err
}

// GetUnscoped returns an agent without a workspace filter. Used only by the
// attachment validation and the MCP hydration path where the agent is already
// trusted.
func (r *Repo) GetUnscoped(ctx context.Context, id identity.ID) (agentexec.Agent, error) {
	row := r.q.QueryRow(ctx, agentSelect+` WHERE a.id = $1`, id)
	a, err := scanAgent(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return agentexec.Agent{}, domain.ErrAgentNotFound
	}
	return a, err
}

func (r *Repo) Create(ctx context.Context, workspaceID identity.ID, in domain.AgentCreate) (agentexec.Agent, error) {
	if in.AllowedTools == nil {
		in.AllowedTools = []string{}
	}
	if in.KnowledgeSourceIDs == nil {
		in.KnowledgeSourceIDs = []identity.ID{}
	}
	gr, err := json.Marshal(in.Guardrails)
	if err != nil {
		return agentexec.Agent{}, err
	}
	row := r.q.QueryRow(ctx, `
		INSERT INTO agents (workspace_id, name, role, system_prompt, default_model, allowed_tools,
			role_title, provider, temperature, max_output_tokens, autonomy_mode,
			user_prompt_template, knowledge_source_ids, guardrails)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING id`,
		workspaceID, in.Name, in.Role, in.SystemPrompt, in.DefaultModel, in.AllowedTools,
		in.RoleTitle, in.Provider, in.Temperature, in.MaxOutputTokens, in.AutonomyMode,
		in.UserPromptTemplate, in.KnowledgeSourceIDs, gr)
	var id identity.ID
	if err := row.Scan(&id); err != nil {
		return agentexec.Agent{}, err
	}
	return r.Get(ctx, id, []identity.ID{workspaceID})
}

func (r *Repo) Update(ctx context.Context, id identity.ID, ws []identity.ID, in domain.AgentUpdate) (agentexec.Agent, error) {
	where, scopeArgs := whereScopedAt(2, ws)
	if len(scopeArgs) == 0 {
		return agentexec.Agent{}, domain.ErrAgentNotFound
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
			return agentexec.Agent{}, err
		}
		add("guardrails", gr)
	}
	if len(sets) > 0 {
		// SET placeholders may be non-sequential; Postgres binds by number.
		q := `UPDATE agents SET ` + strings.Join(sets, ", ") + ` WHERE id = $1` + where
		tag, err := r.q.Exec(ctx, q, args...)
		if err != nil {
			return agentexec.Agent{}, err
		}
		if tag.RowsAffected() == 0 {
			return agentexec.Agent{}, domain.ErrAgentNotFound
		}
	}
	return r.Get(ctx, id, ws)
}

func (r *Repo) Delete(ctx context.Context, id identity.ID, ws []identity.ID) error {
	where, args := whereScopedAt(2, ws)
	if len(args) == 0 {
		return domain.ErrAgentNotFound
	}
	tag, err := r.q.Exec(ctx, `DELETE FROM agents WHERE id = $1`+where, append([]any{id}, args...)...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrAgentNotFound
	}
	return nil
}

// CountByWorkspace counts agents in a workspace (Gateway workspace-stats composition).
func (r *Repo) CountByWorkspace(ctx context.Context, workspaceID identity.ID) (int, error) {
	var n int
	err := r.q.QueryRow(ctx, `SELECT count(*) FROM agents WHERE workspace_id = $1`, workspaceID).Scan(&n)
	return n, err
}

// ── Attachments (link tables) ───────────────────────────────────────────────

// LinkSkill adds a skill to an agent (idempotent). Workspace validation is the
// application's job; this adapter only writes the link.
func (r *Repo) LinkSkill(ctx context.Context, agentID, skillID identity.ID) error {
	_, err := r.q.Exec(ctx, `INSERT INTO agent_skills (agent_id, skill_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, agentID, skillID)
	return err
}

func (r *Repo) UnlinkSkill(ctx context.Context, agentID, skillID identity.ID) error {
	_, err := r.q.Exec(ctx, `DELETE FROM agent_skills WHERE agent_id = $1 AND skill_id = $2`, agentID, skillID)
	return err
}

// LinkMcp adds an MCP server to an agent (idempotent).
func (r *Repo) LinkMcp(ctx context.Context, agentID, mcpID identity.ID) error {
	_, err := r.q.Exec(ctx, `INSERT INTO agent_mcps (agent_id, mcp_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, agentID, mcpID)
	return err
}

func (r *Repo) UnlinkMcp(ctx context.Context, agentID, mcpID identity.ID) error {
	_, err := r.q.Exec(ctx, `DELETE FROM agent_mcps WHERE agent_id = $1 AND mcp_id = $2`, agentID, mcpID)
	return err
}

// ── Runtime status mutations (run-lifecycle + admin consumers) ──────────────
// These write agent status derived from events; the consumers that call them
// arrive with the messaging conversion.

// SetAgentRunning marks an agent as executing a task (run.started consumer).
func (r *Repo) SetAgentRunning(ctx context.Context, agentID, taskID identity.ID) error {
	_, err := r.q.Exec(ctx, `UPDATE agents SET status = 'running', current_task_id = $2 WHERE id = $1`, agentID, taskID)
	return err
}

// SetAgentIdle clears an agent's running state (run.completed consumer).
func (r *Repo) SetAgentIdle(ctx context.Context, agentID identity.ID) error {
	_, err := r.q.Exec(ctx, `UPDATE agents SET status = 'idle', current_task_id = NULL, load = NULL WHERE id = $1 AND status = 'running'`, agentID)
	return err
}

// SetAgentPaused pauses an agent (admin action).
func (r *Repo) SetAgentPaused(ctx context.Context, agentID identity.ID) error {
	_, err := r.q.Exec(ctx, `UPDATE agents SET status = 'paused' WHERE id = $1`, agentID)
	return err
}