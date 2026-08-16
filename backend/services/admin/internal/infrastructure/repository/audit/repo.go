// Package audit implements the Audit aggregate's Postgres adapter.
package audit

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/services/admin/internal/domain"
)

// querier is satisfied by both *pgxpool.Pool and pgx.Tx, letting one adapter
// implementation serve plain and transactional access.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Repo is the pool-backed adapter for domain.AuditRepository.
type Repo struct{ q querier }

// New wraps the given querier (pool or tx) as an audit repository.
func New(q querier) *Repo { return &Repo{q: q} }

const auditCols = `id, workspace_id, actor_name, actor_id, action, action_kind, target, ip, created_at`

func scanAuditRow(row pgx.Row) (domain.AuditRow, error) {
	var a domain.AuditRow
	err := row.Scan(&a.ID, &a.WorkspaceID, &a.Actor.Name, &a.ActorID, &a.Action, &a.ActionKind, &a.Target, &a.IP, &a.CreatedAt)
	return a, err
}

func (r *Repo) List(ctx context.Context, workspaceID identity.ID, kind string) ([]domain.AuditRow, error) {
	rows, err := r.q.Query(ctx, `
		SELECT `+auditCols+` FROM audit_entries
		WHERE workspace_id = $1 AND ($2 = '' OR action_kind = $2)
		ORDER BY created_at DESC LIMIT 200`, workspaceID, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.AuditRow{}
	for rows.Next() {
		a, err := scanAuditRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *Repo) ListSystem(ctx context.Context, limit int) ([]domain.AuditRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := r.q.Query(ctx, `
		SELECT `+auditCols+` FROM audit_entries ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.AuditRow{}
	for rows.Next() {
		a, err := scanAuditRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *Repo) Append(ctx context.Context, workspaceID identity.ID, actorName string, actorID identity.ID, action, kind, target, ip string) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO audit_entries (workspace_id, actor_name, actor_id, action, action_kind, target, ip)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`, workspaceID, actorName, actorID, action, kind, target, ip)
	return err
}

func (r *Repo) Count24h(ctx context.Context, workspaceID identity.ID) (int, error) {
	var n int
	err := r.q.QueryRow(ctx, `
		SELECT count(*) FROM audit_entries
		WHERE workspace_id = $1 AND created_at > now() - interval '24 hours'`, workspaceID).Scan(&n)
	return n, err
}
