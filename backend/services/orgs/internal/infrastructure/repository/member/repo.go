// Package member adapts the Membership aggregate repository port to Postgres
// (Ports & Adapters: the adapter side of the hexagon). Instances serve both
// plain-pool and tx-scoped access via the shared querier.
package member

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/services/orgs/internal/domain"
)

// querier is satisfied by both *pgxpool.Pool and pgx.Tx, letting one adapter
// implementation serve plain and transactional access.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Repo is the Postgres adapter for the Membership aggregate.
type Repo struct{ q querier }

// New builds the adapter on a pool or transaction querier.
func New(q querier) *Repo { return &Repo{q: q} }

const memberCols = `id, workspace_id, user_id, user_name, user_email, role, status, last_active_at, is_service_account`

func scanMember(row pgx.Row) (domain.Member, error) {
	var m domain.Member
	var lastActive *time.Time
	var isService bool
	err := row.Scan(&m.ID, &m.WorkspaceID, &m.UserID, &m.User.Name, &m.User.Email, &m.Role, &m.Status, &lastActive, &isService)
	if err != nil {
		return domain.Member{}, err
	}
	m.User.ID = m.UserID
	if lastActive != nil {
		m.LastActiveAt = (*identity.ISOTime)(lastActive)
	}
	m.IsServiceAccount = &isService
	return m, nil
}

func (r *Repo) List(ctx context.Context, workspaceID identity.ID) ([]domain.Member, error) {
	rows, err := r.q.Query(ctx, `
		SELECT `+memberCols+` FROM memberships WHERE workspace_id = $1 ORDER BY created_at`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Member{}
	for rows.Next() {
		m, err := scanMember(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *Repo) Add(ctx context.Context, workspaceID, userID identity.ID, userName, userEmail string, role identity.Role) (domain.Member, error) {
	row := r.q.QueryRow(ctx, `
		INSERT INTO memberships (workspace_id, user_id, user_name, user_email, role, status)
		VALUES ($1, $2, $3, $4, $5, 'active')
		ON CONFLICT (workspace_id, user_id) DO UPDATE SET
			role = EXCLUDED.role, user_name = EXCLUDED.user_name, user_email = EXCLUDED.user_email,
			status = 'active'
		RETURNING `+memberCols, workspaceID, userID, userName, userEmail, role)
	return scanMember(row)
}

func (r *Repo) SetRole(ctx context.Context, workspaceID, memberID identity.ID, role identity.Role) (domain.Member, error) {
	row := r.q.QueryRow(ctx, `
		UPDATE memberships SET role = $3 WHERE workspace_id = $1 AND id = $2
		RETURNING `+memberCols, workspaceID, memberID, role)
	m, err := scanMember(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Member{}, domain.ErrNotFound
	}
	return m, err
}

func (r *Repo) Remove(ctx context.Context, workspaceID, memberID identity.ID) error {
	tag, err := r.q.Exec(ctx, `DELETE FROM memberships WHERE workspace_id = $1 AND id = $2`, workspaceID, memberID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *Repo) OwnerCount(ctx context.Context, workspaceID identity.ID) (int, error) {
	var n int
	err := r.q.QueryRow(ctx, `SELECT count(*) FROM memberships WHERE workspace_id = $1 AND role = 'owner'`, workspaceID).Scan(&n)
	return n, err
}

func (r *Repo) UserRoleIn(ctx context.Context, userID, workspaceID identity.ID) (identity.Role, error) {
	var role string
	err := r.q.QueryRow(ctx, `
		SELECT role FROM memberships WHERE user_id = $1 AND workspace_id = $2 AND status = 'active'`,
		userID, workspaceID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrNotFound
	}
	return identity.Role(role), err
}