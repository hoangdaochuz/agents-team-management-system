// Package store implements the Resources domain repository ports on Postgres
// (Ports & Adapters: the adapter side of the hexagon). Each aggregate has its
// own adapter; pool-backed instances serve single-aggregate use cases and
// tx-scoped instances are constructed by the UnitOfWork for the workspace
// bootstrap seed.
package store

import (
	"context"
	"embed"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/resources"
	"github.com/aaks/server/internal/platform/db"
	"github.com/aaks/server/services/resources/internal/domain"
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

// Store owns the resources Postgres pool and exposes pool-backed adapters for
// the domain ports. It is the composition-root entrypoint of this package.
type Store struct {
	pool *pgxpool.Pool
	log  *slog.Logger

	Knowledge domain.KnowledgeRepository
	Plugins   domain.PluginRepository
	Rules     domain.RuleRepository
	Mcp       domain.McpConnectionRepository
}

// New opens the resources database and runs migrations.
func New(ctx context.Context, dsn string, log *slog.Logger) (*Store, error) {
	pool, err := db.Pool(ctx, dsn, log)
	if err != nil {
		return nil, err
	}
	if err := db.Migrate(ctx, pool, migrations, "migrations", log); err != nil {
		return nil, err
	}
	s := &Store{pool: pool, log: log}
	s.Knowledge = &knowledgeRepo{q: pool}
	s.Plugins = &pluginRepo{q: pool}
	s.Rules = &ruleRepo{q: pool}
	s.Mcp = &mcpRepo{q: pool}
	return s, nil
}

// Close releases the connection pool.
func (s *Store) Close() { s.pool.Close() }

// ── Knowledge sources ───────────────────────────────────────────────────────

const knowledgeCols = `id, title, kind, chunks, pages, status`

func scanKnowledge(row pgx.Row) (resources.KnowledgeSource, error) {
	var k resources.KnowledgeSource
	err := row.Scan(&k.ID, &k.Title, &k.Kind, &k.Chunks, &k.Pages, &k.Status)
	return k, err
}

type knowledgeRepo struct{ q querier }

func (r *knowledgeRepo) List(ctx context.Context, workspaceID identity.ID) ([]resources.KnowledgeSource, error) {
	rows, err := r.q.Query(ctx, `
		SELECT `+knowledgeCols+` FROM knowledge_sources
		WHERE workspace_id = $1 ORDER BY created_at DESC`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []resources.KnowledgeSource{}
	for rows.Next() {
		k, err := scanKnowledge(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (r *knowledgeRepo) Create(ctx context.Context, workspaceID identity.ID, title, kind string) (resources.KnowledgeSource, error) {
	row := r.q.QueryRow(ctx, `
		INSERT INTO knowledge_sources (workspace_id, title, kind)
		VALUES ($1, $2, $3)
		RETURNING `+knowledgeCols, workspaceID, title, kind)
	return scanKnowledge(row)
}

// ── Plugins ─────────────────────────────────────────────────────────────────

const pluginCols = `id, name, version, capabilities, scopes, enabled`

func scanPlugin(row pgx.Row) (resources.Plugin, error) {
	var p resources.Plugin
	err := row.Scan(&p.ID, &p.Name, &p.Version, &p.Capabilities, &p.Scopes, &p.Enabled)
	return p, err
}

type pluginRepo struct{ q querier }

func (r *pluginRepo) List(ctx context.Context, workspaceID identity.ID) ([]resources.Plugin, error) {
	rows, err := r.q.Query(ctx, `
		SELECT `+pluginCols+` FROM plugins
		WHERE workspace_id = $1 ORDER BY name`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []resources.Plugin{}
	for rows.Next() {
		p, err := scanPlugin(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *pluginRepo) SetEnabled(ctx context.Context, workspaceID, id identity.ID, enabled bool) (resources.Plugin, error) {
	row := r.q.QueryRow(ctx, `
		UPDATE plugins SET enabled = $3 WHERE workspace_id = $1 AND id = $2
		RETURNING `+pluginCols, workspaceID, id, enabled)
	p, err := scanPlugin(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return resources.Plugin{}, domain.ErrNotFound
	}
	return p, err
}

// ── Rules ───────────────────────────────────────────────────────────────────

const ruleCols = `id, name, description, enabled`

func scanRule(row pgx.Row) (resources.Rule, error) {
	var r resources.Rule
	err := row.Scan(&r.ID, &r.Name, &r.Description, &r.Enabled)
	return r, err
}

type ruleRepo struct{ q querier }

func (r *ruleRepo) List(ctx context.Context, workspaceID identity.ID) ([]resources.Rule, error) {
	rows, err := r.q.Query(ctx, `
		SELECT `+ruleCols+` FROM rules
		WHERE workspace_id = $1 ORDER BY name`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []resources.Rule{}
	for rows.Next() {
		rule, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rule)
	}
	return out, rows.Err()
}

func (r *ruleRepo) Create(ctx context.Context, workspaceID identity.ID, name, description string, enabled bool) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO rules (workspace_id, name, description, enabled)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT DO NOTHING`, workspaceID, name, description, enabled)
	return err
}

func (r *ruleRepo) SetEnabled(ctx context.Context, workspaceID, id identity.ID, enabled bool) (resources.Rule, error) {
	row := r.q.QueryRow(ctx, `
		UPDATE rules SET enabled = $3 WHERE workspace_id = $1 AND id = $2
		RETURNING `+ruleCols, workspaceID, id, enabled)
	rule, err := scanRule(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return resources.Rule{}, domain.ErrNotFound
	}
	return rule, err
}

func (r *ruleRepo) Enabled(ctx context.Context, workspaceID identity.ID) ([]resources.Rule, error) {
	rows, err := r.q.Query(ctx, `
		SELECT `+ruleCols+` FROM rules
		WHERE workspace_id = $1 AND enabled = true ORDER BY name`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []resources.Rule{}
	for rows.Next() {
		rule, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rule)
	}
	return out, rows.Err()
}

// ── MCP connections ─────────────────────────────────────────────────────────

const mcpCols = `id, name, transport, tool_count, tool_names, status`

func scanMcpConnection(row pgx.Row) (resources.McpConnection, error) {
	var m resources.McpConnection
	err := row.Scan(&m.ID, &m.Name, &m.Transport, &m.ToolCount, &m.ToolNames, &m.Status)
	return m, err
}

type mcpRepo struct{ q querier }

func (r *mcpRepo) List(ctx context.Context, workspaceID identity.ID) ([]resources.McpConnection, error) {
	rows, err := r.q.Query(ctx, `
		SELECT `+mcpCols+` FROM mcp_connections
		WHERE workspace_id = $1 ORDER BY name`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []resources.McpConnection{}
	for rows.Next() {
		m, err := scanMcpConnection(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *mcpRepo) Upsert(ctx context.Context, mcpID, workspaceID identity.ID, name string) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO mcp_connections (workspace_id, mcp_server_id, name)
		VALUES ($1, $2, $3)
		ON CONFLICT (workspace_id, mcp_server_id) DO UPDATE SET name = EXCLUDED.name`,
		workspaceID, mcpID, name)
	return err
}

func (r *mcpRepo) Delete(ctx context.Context, workspaceID, mcpID identity.ID) error {
	_, err := r.q.Exec(ctx, `DELETE FROM mcp_connections WHERE workspace_id = $1 AND mcp_server_id = $2`, workspaceID, mcpID)
	return err
}

func (r *mcpRepo) Reconnect(ctx context.Context, workspaceID, id identity.ID) (resources.McpConnection, error) {
	row := r.q.QueryRow(ctx, `
		UPDATE mcp_connections SET status = 'connected' WHERE workspace_id = $1 AND id = $2
		RETURNING `+mcpCols, workspaceID, id)
	m, err := scanMcpConnection(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return resources.McpConnection{}, domain.ErrNotFound
	}
	return m, err
}