// Package mcp implements the MCP-connection aggregate repository adapter on
// Postgres (Ports & Adapters: the adapter side of the hexagon).
package mcp

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/resources"
	"github.com/aaks/server/services/resources/internal/domain"
)

// querier is satisfied by both *pgxpool.Pool and pgx.Tx, letting one adapter
// implementation serve plain and transactional access.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Repo is the pool- or tx-backed adapter for the MCP-connection aggregate.
type Repo struct{ q querier }

// New wraps a querier (pool or tx) as the MCP-connection adapter.
func New(q querier) *Repo { return &Repo{q: q} }

const mcpCols = `id, name, transport, tool_count, tool_names, status`

func scanMcpConnection(row pgx.Row) (resources.McpConnection, error) {
	var m resources.McpConnection
	err := row.Scan(&m.ID, &m.Name, &m.Transport, &m.ToolCount, &m.ToolNames, &m.Status)
	return m, err
}

func (r *Repo) List(ctx context.Context, workspaceID identity.ID) ([]resources.McpConnection, error) {
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

func (r *Repo) Upsert(ctx context.Context, mcpID, workspaceID identity.ID, name string) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO mcp_connections (workspace_id, mcp_server_id, name)
		VALUES ($1, $2, $3)
		ON CONFLICT (workspace_id, mcp_server_id) DO UPDATE SET name = EXCLUDED.name`,
		workspaceID, mcpID, name)
	return err
}

func (r *Repo) Delete(ctx context.Context, workspaceID, mcpID identity.ID) error {
	_, err := r.q.Exec(ctx, `DELETE FROM mcp_connections WHERE workspace_id = $1 AND mcp_server_id = $2`, workspaceID, mcpID)
	return err
}

func (r *Repo) Reconnect(ctx context.Context, workspaceID, id identity.ID) (resources.McpConnection, error) {
	row := r.q.QueryRow(ctx, `
		UPDATE mcp_connections SET status = 'connected' WHERE workspace_id = $1 AND id = $2
		RETURNING `+mcpCols, workspaceID, id)
	m, err := scanMcpConnection(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return resources.McpConnection{}, domain.ErrNotFound
	}
	return m, err
}