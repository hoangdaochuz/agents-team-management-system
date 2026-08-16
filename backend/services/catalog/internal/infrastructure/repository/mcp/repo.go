// Package mcp implements the Catalog McpServer aggregate repository port on
// Postgres (Ports & Adapters: the adapter side of the hexagon). The same
// adapter serves plain pool access and tx-scoped access via the UnitOfWork.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/resources"
	"github.com/aaks/server/services/catalog/internal/domain"
)

// querier is satisfied by both *pgxpool.Pool and pgx.Tx, letting one adapter
// implementation serve plain and transactional access.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Repo implements domain.McpRepository on Postgres.
type Repo struct{ q querier }

// New builds the adapter over a pool or transaction.
func New(q querier) *Repo { return &Repo{q: q} }

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

func (r *Repo) List(ctx context.Context, ws []identity.ID) ([]resources.McpServer, error) {
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

func (r *Repo) Get(ctx context.Context, id identity.ID, ws []identity.ID) (resources.McpServer, error) {
	where, args := whereScopedAt(2, ws)
	row := r.q.QueryRow(ctx, `SELECT `+mcpCols+` FROM mcp_servers WHERE id = $1`+where, append([]any{id}, args...)...)
	m, err := scanMcp(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return resources.McpServer{}, domain.ErrMcpNotFound
	}
	return m, err
}

func (r *Repo) Create(ctx context.Context, workspaceID identity.ID, in domain.McpCreate) (resources.McpServer, error) {
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

func (r *Repo) Update(ctx context.Context, id identity.ID, ws []identity.ID, in domain.McpUpdate) (resources.McpServer, error) {
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

func (r *Repo) Delete(ctx context.Context, id identity.ID, ws []identity.ID) error {
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
func (r *Repo) ListByIDs(ctx context.Context, ids []identity.ID) ([]resources.McpServer, error) {
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
