// Package invite adapts the Auth invite-code projection on Postgres.
package invite

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/services/auth/internal/domain"
)

// querier is satisfied by both *pgxpool.Pool and pgx.Tx, letting one adapter
// implementation serve plain and transactional access.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Repo adapts the domain InviteRepository port to Postgres.
type Repo struct{ q querier }

// New builds the adapter from a pool or transaction.
func New(q querier) *Repo { return &Repo{q: q} }

// Lookup resolves a join-mode invite code.
func (r *Repo) Lookup(ctx context.Context, code string) (domain.InviteCode, error) {
	row := r.q.QueryRow(ctx, `
		SELECT invite_code, email, role, workspace_id, workspace_name FROM invite_codes WHERE invite_code = $1`, code)
	var i domain.InviteCode
	err := row.Scan(&i.Code, &i.Email, &i.Role, &i.WorkspaceID, &i.WorkspaceName)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.InviteCode{}, domain.ErrNotFound
	}
	return i, err
}

// Upsert records an invite.created event.
func (r *Repo) Upsert(ctx context.Context, code, email string, role identity.Role, workspaceID identity.ID, workspaceName string) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO invite_codes (invite_code, email, role, workspace_id, workspace_name)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (invite_code) DO UPDATE SET
			email = EXCLUDED.email, role = EXCLUDED.role,
			workspace_id = EXCLUDED.workspace_id, workspace_name = EXCLUDED.workspace_name`,
		code, email, role, workspaceID, workspaceName)
	return err
}