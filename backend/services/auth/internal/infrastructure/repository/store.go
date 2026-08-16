// Package repository implements the Auth domain repository ports on Postgres
// (Ports & Adapters: the adapter side of the hexagon). Each aggregate has its
// own adapter; pool-backed instances serve the single-aggregate use cases.
package repository

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/platform/db"
	"github.com/aaks/server/services/auth/internal/domain"
)

//go:embed migrations/*.sql
var migrations embed.FS

// querier is satisfied by both *pgxpool.Pool and pgx.Tx, letting one adapter
// implementation serve plain and transactional access.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Store owns the auth Postgres pool and exposes pool-backed adapters for the
// domain ports. It is the composition-root entrypoint of this package.
type Store struct {
	pool *pgxpool.Pool
	log  *slog.Logger

	Users          domain.UserRepository
	Sessions       domain.SessionRepository
	SignupRequests domain.SignupRequestRepository
	Invites        domain.InviteRepository
}

// New opens the auth database and runs migrations.
func New(ctx context.Context, dsn string, log *slog.Logger) (*Store, error) {
	pool, err := db.Pool(ctx, dsn, log)
	if err != nil {
		return nil, err
	}
	if err := db.Migrate(ctx, pool, migrations, "migrations", log); err != nil {
		return nil, err
	}
	s := &Store{pool: pool, log: log}
	s.Users = &userRepo{q: pool}
	s.Sessions = &sessionRepo{q: pool}
	s.SignupRequests = &signupRepo{q: pool, pool: pool}
	s.Invites = &inviteRepo{q: pool}
	return s, nil
}

// Close releases the connection pool.
func (s *Store) Close() { s.pool.Close() }

// randomToken returns n random bytes hex-encoded (session tokens).
func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// ── Users / sessions ────────────────────────────────────────────────────────

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

type userRepo struct{ q querier }

// GetByEmail returns a user with its password hash (for login).
func (r *userRepo) GetByEmail(ctx context.Context, email string) (domain.User, error) {
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
func (r *userRepo) Get(ctx context.Context, id identity.ID) (domain.User, error) {
	u, err := scanUser(r.q.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrNotFound
	}
	return u, err
}

// Create inserts an inactive user.
func (r *userRepo) Create(ctx context.Context, name, email, passwordHash string, superadmin bool) (identity.User, error) {
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
func (r *userRepo) Activate(ctx context.Context, id identity.ID) error {
	_, err := r.q.Exec(ctx, `UPDATE users SET is_active = true WHERE id = $1`, id)
	return err
}

// ActivateByEmail marks a user active by email.
func (r *userRepo) ActivateByEmail(ctx context.Context, email string) error {
	_, err := r.q.Exec(ctx, `UPDATE users SET is_active = true WHERE lower(email) = lower($1)`, email)
	return err
}

type sessionRepo struct{ q querier }

// Create inserts a session and returns its token.
func (r *sessionRepo) Create(ctx context.Context, userID identity.ID, ttl time.Duration) (string, error) {
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
func (r *sessionRepo) User(ctx context.Context, token string) (domain.User, error) {
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
func (r *sessionRepo) Delete(ctx context.Context, token string) error {
	_, err := r.q.Exec(ctx, `DELETE FROM sessions WHERE token = $1`, token)
	return err
}

// CountActiveUsers24h counts distinct users with a session created in the last 24h.
func (r *sessionRepo) CountActiveUsers24h(ctx context.Context) (int, error) {
	var n int
	err := r.q.QueryRow(ctx,
		`SELECT count(DISTINCT user_id) FROM sessions WHERE created_at > now() - interval '24 hours'`).Scan(&n)
	return n, err
}

// ── Signup requests ─────────────────────────────────────────────────────────

type signupRepo struct {
	q    querier
	pool *pgxpool.Pool // for the multi-row Create transaction
}

// Create records a pending signup and its (inactive) user atomically.
func (r *signupRepo) Create(ctx context.Context, name, email, passwordHash, mode, inviteCode, workspaceName, organizationName string, workspaceID identity.ID, role identity.Role) (domain.SignupRequest, error) {
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
func (r *signupRepo) Get(ctx context.Context, id identity.ID) (domain.SignupRequest, error) {
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
func (r *signupRepo) GetByEmail(ctx context.Context, email string) (domain.SignupRequest, error) {
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
func (r *signupRepo) SetStatus(ctx context.Context, id identity.ID, status identity.SignupState) error {
	_, err := r.q.Exec(ctx, `UPDATE signup_requests SET status = $2 WHERE id = $1`, id, status)
	return err
}

// ── Invite-code projection ──────────────────────────────────────────────────

type inviteRepo struct{ q querier }

// Lookup resolves a join-mode invite code.
func (r *inviteRepo) Lookup(ctx context.Context, code string) (domain.InviteCode, error) {
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
func (r *inviteRepo) Upsert(ctx context.Context, code, email string, role identity.Role, workspaceID identity.ID, workspaceName string) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO invite_codes (invite_code, email, role, workspace_id, workspace_name)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (invite_code) DO UPDATE SET
			email = EXCLUDED.email, role = EXCLUDED.role,
			workspace_id = EXCLUDED.workspace_id, workspace_name = EXCLUDED.workspace_name`,
		code, email, role, workspaceID, workspaceName)
	return err
}

// isUniqueViolation detects Postgres unique constraint violations.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
