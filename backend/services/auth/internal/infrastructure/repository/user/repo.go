// Package user adapts the Auth User aggregate on Postgres.
package user

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

// Repo adapts the domain UserRepository port to Postgres.
type Repo struct{ q querier }

// New builds the adapter from a pool or transaction.
func New(q querier) *Repo { return &Repo{q: q} }

const userCols = `id, name, email, is_active, is_superadmin, created_at`

func scanUser(row pgx.Row) (domain.User, error) {
	var u domain.User
	var super bool
	err := row.Scan(&u.ID, &u.Name, &u.Email, &u.Active, &super, &u.CreatedAt)
	if err != nil {
		return domain.User{}, err
	}
	u.IsSuperadmin = &super
	u.Role = identity.RoleMember // role is per-workspace; the Gateway overrides
	return u, nil
}

// GetByEmail returns a user with its password hash (for login).
func (r *Repo) GetByEmail(ctx context.Context, email string) (domain.User, error) {
	var u domain.User
	var super bool
	err := r.q.QueryRow(ctx, `
		SELECT id, name, email, is_active, is_superadmin, created_at, password_hash
		FROM users WHERE lower(email) = lower($1)`, email).
		Scan(&u.ID, &u.Name, &u.Email, &u.Active, &super, &u.CreatedAt, &u.PasswordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrNotFound
	}
	u.IsSuperadmin = &super
	u.Role = identity.RoleMember
	return u, err
}

// Get returns a user by id.
func (r *Repo) Get(ctx context.Context, id identity.ID) (domain.User, error) {
	u, err := scanUser(r.q.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrNotFound
	}
	return u, err
}

// Create inserts an inactive user.
func (r *Repo) Create(ctx context.Context, name, email, passwordHash string, superadmin bool) (identity.User, error) {
	row := r.q.QueryRow(ctx, `
		INSERT INTO users (name, email, password_hash, is_active, is_superadmin)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+userCols,
		name, email, passwordHash, false, superadmin)
	u, err := scanUser(row)
	if err != nil {
		if isUniqueViolation(err) {
			return identity.User{}, domain.ErrEmailTaken
		}
		return identity.User{}, err
	}
	return u.User, nil
}

// Activate marks a user active (signup.approved consumer).
func (r *Repo) Activate(ctx context.Context, id identity.ID) error {
	_, err := r.q.Exec(ctx, `UPDATE users SET is_active = true WHERE id = $1`, id)
	return err
}

// ActivateByEmail marks a user active by email.
func (r *Repo) ActivateByEmail(ctx context.Context, email string) error {
	_, err := r.q.Exec(ctx, `UPDATE users SET is_active = true WHERE lower(email) = lower($1)`, email)
	return err
}

// isUniqueViolation detects Postgres unique constraint violations.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}