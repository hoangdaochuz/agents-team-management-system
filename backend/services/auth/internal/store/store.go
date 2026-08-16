// Package store is the Auth service persistence layer: users, sessions,
// signup requests, and the invite-code projection.
package store

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

	"github.com/aaks/server/internal/contracts"
	"github.com/aaks/server/internal/platform/db"
)

//go:embed migrations/*.sql
var migrations embed.FS

var (
	ErrNotFound    = errors.New("not found")
	ErrEmailTaken  = errors.New("email already registered")
	ErrBadPassword = errors.New("invalid credentials")
	ErrPending     = errors.New("account awaiting approval")
)

// Store owns Auth persistence.
type Store struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, dsn string, log *slog.Logger) (*Store, error) {
	pool, err := db.Pool(ctx, dsn, log)
	if err != nil {
		return nil, err
	}
	if err := db.Migrate(ctx, pool, migrations, "migrations", log); err != nil {
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

// ── Users / sessions ────────────────────────────────────────────────────────

// UserRow is a user with its password hash (internal only).
type UserRow struct {
	contracts.User
	PasswordHash string
	Active       bool
}

const userCols = `id, name, email, is_active, is_superadmin, created_at`

func scanUser(row pgx.Row) (UserRow, error) {
	var u UserRow
	var super bool
	err := row.Scan(&u.ID, &u.Name, &u.Email, &u.Active, &super, &u.CreatedAt)
	if err != nil {
		return UserRow{}, err
	}
	u.IsSuperadmin = &super
	u.Role = contracts.RoleMember // role is per-workspace; the Gateway overrides
	return u, nil
}

// GetUserByEmail returns a user with its password hash (for login).
func (s *Store) GetUserByEmail(ctx context.Context, email string) (UserRow, error) {
	var u UserRow
	var super bool
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, email, is_active, is_superadmin, created_at, password_hash
		FROM users WHERE lower(email) = lower($1)`, email).
		Scan(&u.ID, &u.Name, &u.Email, &u.Active, &super, &u.CreatedAt, &u.PasswordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return UserRow{}, ErrNotFound
	}
	u.IsSuperadmin = &super
	u.Role = contracts.RoleMember
	return u, err
}

// GetUser returns a user by id.
func (s *Store) GetUser(ctx context.Context, id contracts.ID) (UserRow, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE id = $1`, id)
	u, err := scanUser(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return UserRow{}, ErrNotFound
	}
	return u, err
}

// CreateUser inserts an inactive user.
func (s *Store) CreateUser(ctx context.Context, name, email, passwordHash string, superadmin bool) (contracts.User, error) {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO users (name, email, password_hash, is_active, is_superadmin)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+userCols,
		name, email, passwordHash, false, superadmin)
	u, err := scanUser(row)
	if err != nil {
		if isUniqueViolation(err) {
			return contracts.User{}, ErrEmailTaken
		}
		return contracts.User{}, err
	}
	return u.User, nil
}

// ActivateUser marks a user active (signup.approved consumer).
func (s *Store) ActivateUser(ctx context.Context, id contracts.ID) error {
	_, err := s.pool.Exec(ctx, `UPDATE users SET is_active = true WHERE id = $1`, id)
	return err
}

// ActivateUserByEmail marks a user active by email.
func (s *Store) ActivateUserByEmail(ctx context.Context, email string) error {
	_, err := s.pool.Exec(ctx, `UPDATE users SET is_active = true WHERE lower(email) = lower($1)`, email)
	return err
}

// CreateSession inserts a session and returns its token.
func (s *Store) CreateSession(ctx context.Context, userID contracts.ID, ttl time.Duration) (string, error) {
	token, err := randomToken(32)
	if err != nil {
		return "", err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO sessions (token, user_id, expires_at) VALUES ($1, $2, now() + $3::interval)`,
		token, userID, ttl.String())
	return token, err
}

// SessionUser returns the user for a session token, or ErrNotFound.
func (s *Store) SessionUser(ctx context.Context, token string) (UserRow, error) {
	var u UserRow
	var super bool
	err := s.pool.QueryRow(ctx, `
		SELECT u.id, u.name, u.email, u.is_active, u.is_superadmin, u.created_at
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token = $1 AND s.expires_at > now()`, token).
		Scan(&u.ID, &u.Name, &u.Email, &u.Active, &super, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return UserRow{}, ErrNotFound
	}
	u.IsSuperadmin = &super
	u.Role = contracts.RoleMember
	return u, err
}

// DeleteSession invalidates a session token.
func (s *Store) DeleteSession(ctx context.Context, token string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE token = $1`, token)
	return err
}

// CountActiveUsers24h counts distinct users with a session created in the last 24h.
func (s *Store) CountActiveUsers24h(ctx context.Context) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT count(DISTINCT user_id) FROM sessions WHERE created_at > now() - interval '24 hours'`).Scan(&n)
	return n, err
}

// ── Signup requests ─────────────────────────────────────────────────────────

// SignupRequestRow mirrors contracts.SignupRequest plus internal fields.
type SignupRequestRow struct {
	ID               contracts.ID `json:"id"`
	UserID           contracts.ID
	Email            string
	Mode             string
	InviteCode       string
	WorkspaceID      contracts.ID
	WorkspaceName    string
	OrganizationName string
	RequestedRole    contracts.Role
	Status           contracts.SignupState
	RequestedAt      time.Time
}

// CreateSignupRequest records a pending signup and its (inactive) user.
func (s *Store) CreateSignupRequest(ctx context.Context, name, email, passwordHash, mode, inviteCode, workspaceName, organizationName string, workspaceID contracts.ID, role contracts.Role) (SignupRequestRow, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return SignupRequestRow{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var userID contracts.ID
	if err := tx.QueryRow(ctx, `
		INSERT INTO users (name, email, password_hash) VALUES ($1, $2, $3)
		RETURNING id`, name, email, passwordHash).Scan(&userID); err != nil {
		if isUniqueViolation(err) {
			return SignupRequestRow{}, ErrEmailTaken
		}
		return SignupRequestRow{}, err
	}

	row := tx.QueryRow(ctx, `
		INSERT INTO signup_requests (user_id, email, mode, invite_code, workspace_id, workspace_name, organization_name, requested_role)
		VALUES ($1, $2, $3, $4, NULLIF($5, '')::uuid, $6, $7, $8)
		RETURNING id, user_id, email, mode, invite_code, workspace_id, workspace_name, organization_name, requested_role, status, created_at`,
		userID, email, mode, inviteCode, workspaceID, workspaceName, organizationName, role)
	r, err := scanSignupRequest(row)
	if err != nil {
		return SignupRequestRow{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SignupRequestRow{}, err
	}
	return r, nil
}

func scanSignupRequest(row pgx.Row) (SignupRequestRow, error) {
	var r SignupRequestRow
	var wsID *string
	err := row.Scan(&r.ID, &r.UserID, &r.Email, &r.Mode, &r.InviteCode, &wsID, &r.WorkspaceName,
		&r.OrganizationName, &r.RequestedRole, &r.Status, &r.RequestedAt)
	if err != nil {
		return SignupRequestRow{}, err
	}
	if wsID != nil {
		r.WorkspaceID = *wsID
	}
	return r, nil
}

// GetSignupRequest returns a request by id.
func (s *Store) GetSignupRequest(ctx context.Context, id contracts.ID) (SignupRequestRow, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, user_id, email, mode, invite_code, workspace_id, workspace_name, organization_name, requested_role, status, created_at
		FROM signup_requests WHERE id = $1`, id)
	r, err := scanSignupRequest(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return SignupRequestRow{}, ErrNotFound
	}
	return r, err
}

// GetSignupRequestByEmail returns the most recent request for an email.
func (s *Store) GetSignupRequestByEmail(ctx context.Context, email string) (SignupRequestRow, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, user_id, email, mode, invite_code, workspace_id, workspace_name, organization_name, requested_role, status, created_at
		FROM signup_requests WHERE lower(email) = lower($1) ORDER BY created_at DESC LIMIT 1`, email)
	r, err := scanSignupRequest(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return SignupRequestRow{}, ErrNotFound
	}
	return r, err
}

// SetSignupStatus transitions a request's state.
func (s *Store) SetSignupStatus(ctx context.Context, id contracts.ID, status contracts.SignupState) error {
	_, err := s.pool.Exec(ctx, `UPDATE signup_requests SET status = $2 WHERE id = $1`, id, status)
	return err
}

// ── Invite-code projection ──────────────────────────────────────────────────

// LookupInviteCode resolves a join-mode invite code.
func (s *Store) LookupInviteCode(ctx context.Context, code string) (inviteCodeRow, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT invite_code, email, role, workspace_id, workspace_name FROM invite_codes WHERE invite_code = $1`, code)
	var i inviteCodeRow
	err := row.Scan(&i.Code, &i.Email, &i.Role, &i.WorkspaceID, &i.WorkspaceName)
	if errors.Is(err, pgx.ErrNoRows) {
		return inviteCodeRow{}, ErrNotFound
	}
	return i, err
}

type inviteCodeRow struct {
	Code          string
	Email         string
	Role          contracts.Role
	WorkspaceID   contracts.ID
	WorkspaceName string
}

// UpsertInviteCode records an invite.created event.
func (s *Store) UpsertInviteCode(ctx context.Context, code string, email string, role contracts.Role, workspaceID, workspaceName contracts.ID) error {
	_, err := s.pool.Exec(ctx, `
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

// randomToken returns n random bytes hex-encoded.
func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
