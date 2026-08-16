// Package signup adapts the Auth signup-request aggregate on Postgres.
package signup

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

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

// Repo adapts the domain SignupRequestRepository port to Postgres.
type Repo struct {
	q    querier
	pool *pgxpool.Pool // for the multi-row Create transaction
}

// New builds the adapter from the pool, which doubles as the querier for plain
// access and as the transaction source for the multi-row Create.
func New(pool *pgxpool.Pool) *Repo { return &Repo{q: pool, pool: pool} }

// Create records a pending signup and its (inactive) user atomically.
func (r *Repo) Create(ctx context.Context, name, email, passwordHash, mode, inviteCode, workspaceName, organizationName string, workspaceID identity.ID, role identity.Role) (domain.SignupRequest, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.SignupRequest{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var userID identity.ID
	if err := tx.QueryRow(ctx, `
		INSERT INTO users (name, email, password_hash) VALUES ($1, $2, $3)
		RETURNING id`, name, email, passwordHash).Scan(&userID); err != nil {
		if isUniqueViolation(err) {
			return domain.SignupRequest{}, domain.ErrEmailTaken
		}
		return domain.SignupRequest{}, err
	}

	row := tx.QueryRow(ctx, `
		INSERT INTO signup_requests (user_id, email, mode, invite_code, workspace_id, workspace_name, organization_name, requested_role)
		VALUES ($1, $2, $3, $4, NULLIF($5, '')::uuid, $6, $7, $8)
		RETURNING id, user_id, email, mode, invite_code, workspace_id, workspace_name, organization_name, requested_role, status, created_at`,
		userID, email, mode, inviteCode, workspaceID, workspaceName, organizationName, role)
	rq, err := scanSignupRequest(row)
	if err != nil {
		return domain.SignupRequest{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.SignupRequest{}, err
	}
	return rq, nil
}

func scanSignupRequest(row pgx.Row) (domain.SignupRequest, error) {
	var r domain.SignupRequest
	var wsID *string
	err := row.Scan(&r.ID, &r.UserID, &r.Email, &r.Mode, &r.InviteCode, &wsID, &r.WorkspaceName,
		&r.OrganizationName, &r.RequestedRole, &r.Status, &r.RequestedAt)
	if err != nil {
		return domain.SignupRequest{}, err
	}
	if wsID != nil {
		r.WorkspaceID = *wsID
	}
	return r, nil
}

// Get returns a request by id.
func (r *Repo) Get(ctx context.Context, id identity.ID) (domain.SignupRequest, error) {
	row := r.q.QueryRow(ctx, `
		SELECT id, user_id, email, mode, invite_code, workspace_id, workspace_name, organization_name, requested_role, status, created_at
		FROM signup_requests WHERE id = $1`, id)
	rq, err := scanSignupRequest(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.SignupRequest{}, domain.ErrNotFound
	}
	return rq, err
}

// GetByEmail returns the most recent request for an email.
func (r *Repo) GetByEmail(ctx context.Context, email string) (domain.SignupRequest, error) {
	row := r.q.QueryRow(ctx, `
		SELECT id, user_id, email, mode, invite_code, workspace_id, workspace_name, organization_name, requested_role, status, created_at
		FROM signup_requests WHERE lower(email) = lower($1) ORDER BY created_at DESC LIMIT 1`, email)
	rq, err := scanSignupRequest(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.SignupRequest{}, domain.ErrNotFound
	}
	return rq, err
}

// SetStatus transitions a request's state.
func (r *Repo) SetStatus(ctx context.Context, id identity.ID, status identity.SignupState) error {
	_, err := r.q.Exec(ctx, `UPDATE signup_requests SET status = $2 WHERE id = $1`, id, status)
	return err
}

// isUniqueViolation detects Postgres unique constraint violations.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}