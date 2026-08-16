// Package session adapts the Auth session aggregate on Postgres.
package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

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

// Repo adapts the domain SessionRepository port to Postgres.
type Repo struct{ q querier }

// New builds the adapter from a pool or transaction.
func New(q querier) *Repo { return &Repo{q: q} }

// randomToken returns n random bytes hex-encoded (session tokens).
func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Create inserts a session and returns its token.
func (r *Repo) Create(ctx context.Context, userID identity.ID, ttl time.Duration) (string, error) {
	token, err := randomToken(32)
	if err != nil {
		return "", err
	}
	_, err = r.q.Exec(ctx, `
		INSERT INTO sessions (token, user_id, expires_at) VALUES ($1, $2, now() + $3::interval)`,
		token, userID, ttl.String())
	return token, err
}

// User returns the user for a session token, or domain.ErrNotFound.
func (r *Repo) User(ctx context.Context, token string) (domain.User, error) {
	var u domain.User
	var super bool
	err := r.q.QueryRow(ctx, `
		SELECT u.id, u.name, u.email, u.is_active, u.is_superadmin, u.created_at
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token = $1 AND s.expires_at > now()`, token).
		Scan(&u.ID, &u.Name, &u.Email, &u.Active, &super, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrNotFound
	}
	u.IsSuperadmin = &super
	u.Role = identity.RoleMember
	return u, err
}

// Delete invalidates a session token.
func (r *Repo) Delete(ctx context.Context, token string) error {
	_, err := r.q.Exec(ctx, `DELETE FROM sessions WHERE token = $1`, token)
	return err
}

// CountActiveUsers24h counts distinct users with a session created in the last 24h.
func (r *Repo) CountActiveUsers24h(ctx context.Context) (int, error) {
	var n int
	err := r.q.QueryRow(ctx,
		`SELECT count(DISTINCT user_id) FROM sessions WHERE created_at > now() - interval '24 hours'`).Scan(&n)
	return n, err
}