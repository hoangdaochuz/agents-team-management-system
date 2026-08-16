// Package store is the Resources service persistence layer: knowledge sources,
// plugins, rules, and MCP connections, all scoped by workspace_id.
package store

import (
	"context"
	"embed"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aaks/server/internal/contracts"
	"github.com/aaks/server/internal/platform/db"
)

//go:embed migrations/*.sql
var migrations embed.FS

var ErrNotFound = errors.New("not found")

// Store owns Resources persistence.
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

// ── Knowledge sources ───────────────────────────────────────────────────────

// ListKnowledge returns the workspace's knowledge sources, newest first.
func (s *Store) ListKnowledge(ctx context.Context, workspaceID contracts.ID) ([]contracts.KnowledgeSource, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, title, kind, chunks, pages, status FROM knowledge_sources
		WHERE workspace_id = $1 ORDER BY created_at DESC`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []contracts.KnowledgeSource{}
	for rows.Next() {
		var k contracts.KnowledgeSource
		if err := rows.Scan(&k.ID, &k.Title, &k.Kind, &k.Chunks, &k.Pages, &k.Status); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// CreateKnowledge inserts a knowledge source (async indexing status pending;
// the runner/indexer flips status later).
func (s *Store) CreateKnowledge(ctx context.Context, workspaceID contracts.ID, title, kind string) (contracts.KnowledgeSource, error) {
	var k contracts.KnowledgeSource
	err := s.pool.QueryRow(ctx, `
		INSERT INTO knowledge_sources (workspace_id, title, kind)
		VALUES ($1, $2, $3)
		RETURNING id, title, kind, chunks, pages, status`, workspaceID, title, kind).
		Scan(&k.ID, &k.Title, &k.Kind, &k.Chunks, &k.Pages, &k.Status)
	return k, err
}

// ── Plugins ─────────────────────────────────────────────────────────────────

// ListPlugins returns the workspace's plugins.
func (s *Store) ListPlugins(ctx context.Context, workspaceID contracts.ID) ([]contracts.Plugin, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, version, capabilities, scopes, enabled FROM plugins
		WHERE workspace_id = $1 ORDER BY name`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []contracts.Plugin{}
	for rows.Next() {
		var p contracts.Plugin
		if err := rows.Scan(&p.ID, &p.Name, &p.Version, &p.Capabilities, &p.Scopes, &p.Enabled); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// SetPluginEnabled toggles a plugin.
func (s *Store) SetPluginEnabled(ctx context.Context, workspaceID, id contracts.ID, enabled bool) (contracts.Plugin, error) {
	row := s.pool.QueryRow(ctx, `
		UPDATE plugins SET enabled = $3 WHERE workspace_id = $1 AND id = $2
		RETURNING id, name, version, capabilities, scopes, enabled`, workspaceID, id, enabled)
	var p contracts.Plugin
	err := row.Scan(&p.ID, &p.Name, &p.Version, &p.Capabilities, &p.Scopes, &p.Enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return contracts.Plugin{}, ErrNotFound
	}
	return p, err
}

// ── Rules ───────────────────────────────────────────────────────────────────

// ListRules returns the workspace's rules.
func (s *Store) ListRules(ctx context.Context, workspaceID contracts.ID) ([]contracts.Rule, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, description, enabled FROM rules
		WHERE workspace_id = $1 ORDER BY name`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []contracts.Rule{}
	for rows.Next() {
		var r contracts.Rule
		if err := rows.Scan(&r.ID, &r.Name, &r.Description, &r.Enabled); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// CreateRule inserts a rule (idempotent by workspace+name).
func (s *Store) CreateRule(ctx context.Context, workspaceID contracts.ID, name, description string, enabled bool) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO rules (workspace_id, name, description, enabled)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT DO NOTHING`, workspaceID, name, description, enabled)
	return err
}

// SetRuleEnabled toggles a rule.
func (s *Store) SetRuleEnabled(ctx context.Context, workspaceID, id contracts.ID, enabled bool) (contracts.Rule, error) {
	row := s.pool.QueryRow(ctx, `
		UPDATE rules SET enabled = $3 WHERE workspace_id = $1 AND id = $2
		RETURNING id, name, description, enabled`, workspaceID, id, enabled)
	var r contracts.Rule
	err := row.Scan(&r.ID, &r.Name, &r.Description, &r.Enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return contracts.Rule{}, ErrNotFound
	}
	return r, err
}

// EnabledRules returns the enforced (enabled) rules for the Runner's guardrails.
func (s *Store) EnabledRules(ctx context.Context, workspaceID contracts.ID) ([]contracts.Rule, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, description, enabled FROM rules
		WHERE workspace_id = $1 AND enabled = true ORDER BY name`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []contracts.Rule{}
	for rows.Next() {
		var r contracts.Rule
		if err := rows.Scan(&r.ID, &r.Name, &r.Description, &r.Enabled); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ── MCP connections ─────────────────────────────────────────────────────────

// ListMcpConnections returns the workspace's MCP connections.
func (s *Store) ListMcpConnections(ctx context.Context, workspaceID contracts.ID) ([]contracts.McpConnection, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, transport, tool_count, tool_names, status FROM mcp_connections
		WHERE workspace_id = $1 ORDER BY name`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []contracts.McpConnection{}
	for rows.Next() {
		var m contracts.McpConnection
		if err := rows.Scan(&m.ID, &m.Name, &m.Transport, &m.ToolCount, &m.ToolNames, &m.Status); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// UpsertMcpConnection projects a catalog mcp-created event.
func (s *Store) UpsertMcpConnection(ctx context.Context, mcpID, workspaceID contracts.ID, name string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO mcp_connections (workspace_id, mcp_server_id, name)
		VALUES ($1, $2, $3)
		ON CONFLICT (workspace_id, mcp_server_id) DO UPDATE SET name = EXCLUDED.name`,
		workspaceID, mcpID, name)
	return err
}

// DeleteMcpConnection projects a catalog mcp-deleted event.
func (s *Store) DeleteMcpConnection(ctx context.Context, workspaceID, mcpID contracts.ID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM mcp_connections WHERE workspace_id = $1 AND mcp_server_id = $2`, workspaceID, mcpID)
	return err
}

// ReconnectMcpConnection marks a connection online (tool discovery is the
// runner's job at run time).
func (s *Store) ReconnectMcpConnection(ctx context.Context, workspaceID, id contracts.ID) (contracts.McpConnection, error) {
	row := s.pool.QueryRow(ctx, `
		UPDATE mcp_connections SET status = 'connected' WHERE workspace_id = $1 AND id = $2
		RETURNING id, name, transport, tool_count, tool_names, status`, workspaceID, id)
	var m contracts.McpConnection
	err := row.Scan(&m.ID, &m.Name, &m.Transport, &m.ToolCount, &m.ToolNames, &m.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return contracts.McpConnection{}, ErrNotFound
	}
	return m, err
}
