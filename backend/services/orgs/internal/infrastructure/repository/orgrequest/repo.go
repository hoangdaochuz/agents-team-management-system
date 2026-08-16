// Package orgrequest adapts the OrgRequest (org-signup-request projection)
// repository port to Postgres (Ports & Adapters: the adapter side of the
// hexagon). Instances serve both plain-pool and tx-scoped access via the
// shared querier.
package orgrequest

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

// Repo is the Postgres adapter for the OrgRequest aggregate.
type Repo struct{ q querier }

// New builds the adapter on a pool or transaction querier.
func New(q querier) *Repo { return &Repo{q: q} }

func (r *Repo) ListPending(ctx context.Context) ([]domain.OrgRequest, error) {
	rows, err := r.q.Query(ctx, `
		SELECT request_id, user_id, name, email, organization_name, requested_role, status, created_at
		FROM org_requests WHERE status = 'pending' ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.OrgRequest{}
	for rows.Next() {
		var o domain.OrgRequest
		if err := rows.Scan(&o.ID, &o.UserID, &o.Name, &o.Email, &o.OrganizationName, &o.RequestedRole, &o.Status, &o.RequestedAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (r *Repo) Get(ctx context.Context, requestID identity.ID) (domain.OrgRequest, error) {
	var o domain.OrgRequest
	err := r.q.QueryRow(ctx, `
		SELECT request_id, user_id, name, email, organization_name, requested_role, status, created_at
		FROM org_requests WHERE request_id = $1`, requestID).
		Scan(&o.ID, &o.UserID, &o.Name, &o.Email, &o.OrganizationName, &o.RequestedRole, &o.Status, &o.RequestedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.OrgRequest{}, domain.ErrNotFound
	}
	return o, err
}

func (r *Repo) Upsert(ctx context.Context, req events.SignupRequestedData) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO org_requests (request_id, user_id, name, email, organization_name, requested_role)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (request_id) DO NOTHING`,
		req.RequestID, req.UserID, req.Name, req.Email, req.OrganizationName, req.RequestedRole)
	return err
}

func (r *Repo) SetStatus(ctx context.Context, requestID identity.ID, status identity.SignupState) error {
	_, err := r.q.Exec(ctx, `UPDATE org_requests SET status = $2 WHERE request_id = $1`, requestID, status)
	return err
}