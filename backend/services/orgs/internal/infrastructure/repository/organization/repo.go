// Package organization adapts the Organization aggregate repository port to
// Postgres (Ports & Adapters: the adapter side of the hexagon). Instances
// serve both plain-pool and tx-scoped access via the shared querier.
package organization

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/workspaces"
	"github.com/aaks/server/services/orgs/internal/domain"
)

// querier is satisfied by both *pgxpool.Pool and pgx.Tx, letting one adapter
// implementation serve plain and transactional access.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Repo is the Postgres adapter for the Organization aggregate.
type Repo struct{ q querier }

// New builds the adapter on a pool or transaction querier.
func New(q querier) *Repo { return &Repo{q: q} }

const orgCols = `id, owner_id, name, subdomain, plan, seats_total, status, created_at`

func scanOrg(row pgx.Row) (workspaces.Organization, error) {
	var o workspaces.Organization
	var owner identity.ID
	err := row.Scan(&o.ID, &owner, &o.Name, &o.Subdomain, &o.Plan, &o.SeatsTotal, &o.Status, &o.CreatedAt)
	if err != nil {
		return workspaces.Organization{}, err
	}
	return o, err
}

func (r *Repo) List(ctx context.Context) ([]workspaces.Organization, error) {
	rows, err := r.q.Query(ctx, `
		SELECT o.id, o.owner_id, o.name, o.subdomain, o.plan, o.seats_total, o.status, o.created_at,
		       (SELECT count(*) FROM workspaces w WHERE w.organization_id = o.id) AS ws_count,
		       (SELECT count(*) FROM memberships m JOIN workspaces w ON w.id = m.workspace_id WHERE w.organization_id = o.id) AS seats
		FROM organizations o ORDER BY o.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []workspaces.Organization{}
	for rows.Next() {
		var o workspaces.Organization
		var owner identity.ID
		var wsCount, seats int
		if err := rows.Scan(&o.ID, &owner, &o.Name, &o.Subdomain, &o.Plan, &o.SeatsTotal, &o.Status, &o.CreatedAt, &wsCount, &seats); err != nil {
			return nil, err
		}
		o.WorkspaceCount = wsCount
		o.SeatsUsed = seats
		out = append(out, o)
	}
	return out, rows.Err()
}

func (r *Repo) Get(ctx context.Context, id identity.ID) (workspaces.Organization, error) {
	row := r.q.QueryRow(ctx, `
		SELECT o.id, o.owner_id, o.name, o.subdomain, o.plan, o.seats_total, o.status, o.created_at,
		       (SELECT count(*) FROM workspaces w WHERE w.organization_id = o.id) AS ws_count,
		       (SELECT count(*) FROM memberships m JOIN workspaces w ON w.id = m.workspace_id WHERE w.organization_id = o.id) AS seats
		FROM organizations o WHERE o.id = $1`, id)
	var o workspaces.Organization
	var owner identity.ID
	var wsCount, seats int
	err := row.Scan(&o.ID, &owner, &o.Name, &o.Subdomain, &o.Plan, &o.SeatsTotal, &o.Status, &o.CreatedAt, &wsCount, &seats)
	if errors.Is(err, pgx.ErrNoRows) {
		return workspaces.Organization{}, domain.ErrNotFound
	}
	if err != nil {
		return workspaces.Organization{}, err
	}
	o.WorkspaceCount = wsCount
	o.SeatsUsed = seats
	return o, nil
}

func (r *Repo) Create(ctx context.Context, ownerID identity.ID, name string, plan identity.Plan) (workspaces.Organization, error) {
	if plan == "" {
		plan = identity.PlanFree
	}
	row := r.q.QueryRow(ctx, `
		INSERT INTO organizations (owner_id, name, plan) VALUES ($1, $2, $3)
		RETURNING `+orgCols, ownerID, name, plan)
	return scanOrg(row)
}

func (r *Repo) SetStatus(ctx context.Context, id identity.ID, status identity.OrgStatus) (workspaces.Organization, error) {
	row := r.q.QueryRow(ctx, `UPDATE organizations SET status = $2 WHERE id = $1 RETURNING `+orgCols, id, status)
	o, err := scanOrg(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return workspaces.Organization{}, domain.ErrNotFound
	}
	return o, err
}

func (r *Repo) ByUser(ctx context.Context, userID identity.ID) (workspaces.Organization, error) {
	row := r.q.QueryRow(ctx, `SELECT `+orgCols+` FROM organizations WHERE owner_id = $1 ORDER BY created_at LIMIT 1`, userID)
	o, err := scanOrg(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return workspaces.Organization{}, domain.ErrNoOrg
	}
	return o, err
}

func (r *Repo) Stats(ctx context.Context) (organizations, workspaces, openSeats int, err error) {
	err = r.q.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM organizations),
		       (SELECT count(*) FROM workspaces),
		       (SELECT coalesce(sum(greatest(0, seats_total -
		           (SELECT count(*) FROM memberships m JOIN workspaces w ON w.id = m.workspace_id
		            WHERE w.organization_id = o.id))), 0) FROM organizations o)`).
		Scan(&organizations, &workspaces, &openSeats)
	return
}