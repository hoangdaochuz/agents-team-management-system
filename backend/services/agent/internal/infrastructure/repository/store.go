// Package repository implements the Agent domain repository ports on Postgres
// (Ports & Adapters: the adapter side of the hexagon). Each aggregate has its
// own adapter: agents (including the agent-builder fields), the skill/mcp link
// tables, and local projections of the catalog's skill/MCP definitions used to
// validate attachments within a workspace (no service-to-service sync calls).
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

	"github.com/aaks/server/internal/contracts/agentexec"
	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/platform/db"
	"github.com/aaks/server/services/agent/internal/domain"
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

// Store owns the agent Postgres pool and exposes pool-backed adapters for the
// domain ports. It is the composition-root entrypoint of this package.
type Store struct {
	pool        *pgxpool.Pool
	log         *slog.Logger
	Agents      domain.AgentRepository
	Projections domain.CatalogProjectionRepository
}

// New opens the agent database and runs migrations.
func New(ctx context.Context, dsn string, log *slog.Logger) (*Store, error) {
	pool, err := db.Pool(ctx, dsn, log)
	if err != nil {
		return nil, err
	}
	if err := db.Migrate(ctx, pool, migrations, "migrations", log); err != nil {
		return nil, err
	}
	s := &Store{pool: pool, log: log}
	s.Agents = &agentRepo{q: pool}
	s.Projections = &projectionRepo{q: pool}
	return s, nil
}

// Close releases the connection pool.
func (s *Store) Close() { s.pool.Close() }

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

// ── Agents ──────────────────────────────────────────────────────────────────

type agentRepo struct{ q querier }

func (r *agentRepo) List(ctx context.Context, ws []identity.ID) ([]agentexec.Agent, error) {
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

func (r *agentRepo) Get(ctx context.Context, id identity.ID, ws []identity.ID) (agentexec.Agent, error) {
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
func (r *agentRepo) GetUnscoped(ctx context.Context, id identity.ID) (agentexec.Agent, error) {
	row := r.q.QueryRow(ctx, agentSelect+` WHERE a.id = $1`, id)
	a, err := scanAgent(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return agentexec.Agent{}, domain.ErrAgentNotFound
	}
	return a, err
}

func (r *agentRepo) Create(ctx context.Context, workspaceID identity.ID, in domain.AgentCreate) (agentexec.Agent, error) {
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

func (r *agentRepo) Update(ctx context.Context, id identity.ID, ws []identity.ID, in domain.AgentUpdate) (agentexec.Agent, error) {
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

func (r *agentRepo) Delete(ctx context.Context, id identity.ID, ws []identity.ID) error {
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
func (r *agentRepo) CountByWorkspace(ctx context.Context, workspaceID identity.ID) (int, error) {
	var n int
	err := r.q.QueryRow(ctx, `SELECT count(*) FROM agents WHERE workspace_id = $1`, workspaceID).Scan(&n)
	return n, err
}

// ── Attachments (link tables) ───────────────────────────────────────────────

// LinkSkill adds a skill to an agent (idempotent). Workspace validation is the
// application's job; this adapter only writes the link.
func (r *agentRepo) LinkSkill(ctx context.Context, agentID, skillID identity.ID) error {
	_, err := r.q.Exec(ctx, `INSERT INTO agent_skills (agent_id, skill_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, agentID, skillID)
	return err
}

func (r *agentRepo) UnlinkSkill(ctx context.Context, agentID, skillID identity.ID) error {
	_, err := r.q.Exec(ctx, `DELETE FROM agent_skills WHERE agent_id = $1 AND skill_id = $2`, agentID, skillID)
	return err
}

// LinkMcp adds an MCP server to an agent (idempotent).
func (r *agentRepo) LinkMcp(ctx context.Context, agentID, mcpID identity.ID) error {
	_, err := r.q.Exec(ctx, `INSERT INTO agent_mcps (agent_id, mcp_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, agentID, mcpID)
	return err
}

func (r *agentRepo) UnlinkMcp(ctx context.Context, agentID, mcpID identity.ID) error {
	_, err := r.q.Exec(ctx, `DELETE FROM agent_mcps WHERE agent_id = $1 AND mcp_id = $2`, agentID, mcpID)
	return err
}

// ── Catalog projections (skill/mcp → workspace) ─────────────────────────────

type projectionRepo struct{ q querier }

// SkillWorkspace returns the workspace a skill belongs to, or ErrUnknownDefinition.
func (r *projectionRepo) SkillWorkspace(ctx context.Context, skillID identity.ID) (identity.ID, error) {
	var ws identity.ID
	err := r.q.QueryRow(ctx, `SELECT workspace_id FROM known_skills WHERE skill_id = $1`, skillID).Scan(&ws)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrUnknownDefinition
	}
	return ws, err
}

// McpWorkspace returns the workspace an MCP definition belongs to, or ErrUnknownDefinition.
func (r *projectionRepo) McpWorkspace(ctx context.Context, mcpID identity.ID) (identity.ID, error) {
	var ws identity.ID
	err := r.q.QueryRow(ctx, `SELECT workspace_id FROM known_mcps WHERE mcp_id = $1`, mcpID).Scan(&ws)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrUnknownDefinition
	}
	return ws, err
}

// ── Projection/status mutations (run-lifecycle + catalog consumers) ─────────
// These write the local projections and runtime status derived from events;
// the consumers that call them arrive with the messaging conversion.

// UpsertSkillProjection records a catalog skill's workspace.
func (s *Store) UpsertSkillProjection(ctx context.Context, skillID, workspaceID identity.ID) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO known_skills (skill_id, workspace_id) VALUES ($1, $2)
		ON CONFLICT (skill_id) DO UPDATE SET workspace_id = EXCLUDED.workspace_id`, skillID, workspaceID)
	return err
}

// DeleteSkillProjection forgets a deleted catalog skill.
func (s *Store) DeleteSkillProjection(ctx context.Context, skillID identity.ID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM known_skills WHERE skill_id = $1`, skillID)
	return err
}

// UpsertMcpProjection records a catalog MCP definition's workspace.
func (s *Store) UpsertMcpProjection(ctx context.Context, mcpID, workspaceID identity.ID) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO known_mcps (mcp_id, workspace_id) VALUES ($1, $2)
		ON CONFLICT (mcp_id) DO UPDATE SET workspace_id = EXCLUDED.workspace_id`, mcpID, workspaceID)
	return err
}

// DeleteMcpProjection forgets a deleted catalog MCP definition.
func (s *Store) DeleteMcpProjection(ctx context.Context, mcpID identity.ID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM known_mcps WHERE mcp_id = $1`, mcpID)
	return err
}

// SetAgentRunning marks an agent as executing a task (run.started consumer).
func (s *Store) SetAgentRunning(ctx context.Context, agentID, taskID identity.ID) error {
	_, err := s.pool.Exec(ctx, `UPDATE agents SET status = 'running', current_task_id = $2 WHERE id = $1`, agentID, taskID)
	return err
}

// SetAgentIdle clears an agent's running state (run.completed consumer).
func (s *Store) SetAgentIdle(ctx context.Context, agentID identity.ID) error {
	_, err := s.pool.Exec(ctx, `UPDATE agents SET status = 'idle', current_task_id = NULL, load = NULL WHERE id = $1 AND status = 'running'`, agentID)
	return err
}

// SetAgentPaused pauses an agent (admin action).
func (s *Store) SetAgentPaused(ctx context.Context, agentID identity.ID) error {
	_, err := s.pool.Exec(ctx, `UPDATE agents SET status = 'paused' WHERE id = $1`, agentID)
	return err
}
