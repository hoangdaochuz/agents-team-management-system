// Package joinrequest adapts the JoinRequest (signup-request projection)
// repository port to Postgres (Ports & Adapters: the adapter side of the
// hexagon). Instances serve both plain-pool and tx-scoped access via the
// shared querier.
package joinrequest

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/aaks/server/internal/contracts/events"
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

// Repo is the Postgres adapter for the JoinRequest aggregate.
type Repo struct{ q querier }

// New builds the adapter on a pool or transaction querier.
func New(q querier) *Repo { return &Repo{q: q} }

func scanJoinRequest(row pgx.Row) (domain.JoinRequest, error) {
	var j domain.JoinRequest
	err := row.Scan(&j.ID, &j.UserID, &j.Name, &j.Email, &j.WorkspaceID, &j.RequestedRole, &j.Status, &j.RequestedAt)
	return j, err
}

func (r *Repo) ListPending(ctx context.Context, workspaceID identity.ID) ([]domain.JoinRequest, error) {
	rows, err := r.q.Query(ctx, `
		SELECT request_id, user_id, name, email, workspace_id, requested_role, status, created_at
		FROM join_requests WHERE workspace_id = $1 AND status = 'pending' ORDER BY created_at`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.JoinRequest{}
	for rows.Next() {
		jr, err := scanJoinRequest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, jr)
	}
	return out, rows.Err()
}

func (r *Repo) Get(ctx context.Context, requestID identity.ID) (domain.JoinRequest, error) {
	row := r.q.QueryRow(ctx, `
		SELECT request_id, user_id, name, email, workspace_id, requested_role, status, created_at
		FROM join_requests WHERE request_id = $1`, requestID)
	j, err := scanJoinRequest(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.JoinRequest{}, domain.ErrNotFound
	}
	return j, err
}

func (r *Repo) Upsert(ctx context.Context, req events.SignupRequestedData) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO join_requests (request_id, user_id, name, email, workspace_id, requested_role)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (request_id) DO NOTHING`,
		req.RequestID, req.UserID, req.Name, req.Email, req.WorkspaceID, req.RequestedRole)
	return err
}

func (r *Repo) SetStatus(ctx context.Context, requestID identity.ID, status identity.SignupState) error {
	_, err := r.q.Exec(ctx, `UPDATE join_requests SET status = $2 WHERE request_id = $1`, requestID, status)
	return err
}