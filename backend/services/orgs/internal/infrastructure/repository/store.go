// Package repository implements the Orgs domain repository ports on Postgres
// (Ports & Adapters: the adapter side of the hexagon). Each aggregate has its
// own adapter; pool-backed instances serve single-aggregate use cases and
// tx-scoped instances are constructed by the UnitOfWork for multi-aggregate
// mutations.
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

	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/workspaces"
	"github.com/aaks/server/internal/platform/db"
	"github.com/aaks/server/services/orgs/internal/domain"
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

// Store owns the orgs Postgres pool and exposes pool-backed adapters for the
// domain ports. It is the composition-root entrypoint of this package.
type Store struct {
	pool *pgxpool.Pool
	log  *slog.Logger

	Organizations domain.OrganizationRepository
	Workspaces    domain.WorkspaceRepository
	Members       domain.MembershipRepository
	Invites       domain.InviteRepository
	JoinRequests  domain.JoinRequestRepository
	OrgRequests   domain.OrgRequestRepository
}

// New opens the orgs database and runs migrations.
func New(ctx context.Context, dsn string, log *slog.Logger) (*Store, error) {
	pool, err := db.Pool(ctx, dsn, log)
	if err != nil {
		return nil, err
	}
	if err := db.Migrate(ctx, pool, migrations, "migrations", log); err != nil {
		return nil, err
	}
	s := &Store{pool: pool, log: log}
	s.Organizations = &orgRepo{q: pool}
	s.Workspaces = &workspaceRepo{q: pool}
	s.Members = &memberRepo{q: pool}
	s.Invites = &inviteRepo{q: pool}
	s.JoinRequests = &joinRequestRepo{q: pool}
	s.OrgRequests = &orgRequestRepo{q: pool}
	return s, nil
}

// Close releases the connection pool.
func (s *Store) Close() { s.pool.Close() }

// randomToken returns n random bytes hex-encoded (invite codes, session ids).
func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// ── Organizations ───────────────────────────────────────────────────────────

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

type orgRepo struct{ q querier }

func (r *orgRepo) List(ctx context.Context) ([]workspaces.Organization, error) {
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

func (r *orgRepo) Get(ctx context.Context, id identity.ID) (workspaces.Organization, error) {
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

func (r *orgRepo) Create(ctx context.Context, ownerID identity.ID, name string, plan identity.Plan) (workspaces.Organization, error) {
	if plan == "" {
		plan = identity.PlanFree
	}
	row := r.q.QueryRow(ctx, `
		INSERT INTO organizations (owner_id, name, plan) VALUES ($1, $2, $3)
		RETURNING `+orgCols, ownerID, name, plan)
	return scanOrg(row)
}

func (r *orgRepo) SetStatus(ctx context.Context, id identity.ID, status identity.OrgStatus) (workspaces.Organization, error) {
	row := r.q.QueryRow(ctx, `UPDATE organizations SET status = $2 WHERE id = $1 RETURNING `+orgCols, id, status)
	o, err := scanOrg(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return workspaces.Organization{}, domain.ErrNotFound
	}
	return o, err
}

func (r *orgRepo) ByUser(ctx context.Context, userID identity.ID) (workspaces.Organization, error) {
	row := r.q.QueryRow(ctx, `SELECT `+orgCols+` FROM organizations WHERE owner_id = $1 ORDER BY created_at LIMIT 1`, userID)
	o, err := scanOrg(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return workspaces.Organization{}, domain.ErrNoOrg
	}
	return o, err
}

func (r *orgRepo) Stats(ctx context.Context) (organizations, workspaces, openSeats int, err error) {
	err = r.q.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM organizations),
		       (SELECT count(*) FROM workspaces),
		       (SELECT coalesce(sum(greatest(0, seats_total -
		           (SELECT count(*) FROM memberships m JOIN workspaces w ON w.id = m.workspace_id
		            WHERE w.organization_id = o.id))), 0) FROM organizations o)`).
		Scan(&organizations, &workspaces, &openSeats)
	return
}

// ── Workspaces ──────────────────────────────────────────────────────────────

const wsCols = `id, organization_id, name, repo_source, default_branch, glyph, description, created_at`

func scanWorkspace(row pgx.Row) (workspaces.Workspace, error) {
	var w workspaces.Workspace
	var org identity.ID
	err := row.Scan(&w.ID, &org, &w.Name, &w.RepoSource, &w.DefaultBranch, &w.Glyph, &w.Description, &w.CreatedAt)
	if err != nil {
		return workspaces.Workspace{}, err
	}
	return w, err
}

type workspaceRepo struct{ q querier }

func (r *workspaceRepo) Create(ctx context.Context, orgID identity.ID, name, repoSource, defaultBranch, glyph, description string) (workspaces.Workspace, error) {
	if defaultBranch == "" {
		defaultBranch = "main"
	}
	row := r.q.QueryRow(ctx, `
		INSERT INTO workspaces (organization_id, name, repo_source, default_branch, glyph, description)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+wsCols, orgID, name, repoSource, defaultBranch, glyph, description)
	return scanWorkspace(row)
}

func (r *workspaceRepo) ByID(ctx context.Context, id identity.ID) (workspaces.Workspace, error) {
	row := r.q.QueryRow(ctx, `SELECT `+wsCols+` FROM workspaces WHERE id = $1`, id)
	w, err := scanWorkspace(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return workspaces.Workspace{}, domain.ErrNotFound
	}
	return w, err
}

func (r *workspaceRepo) ListByUser(ctx context.Context, userID identity.ID) ([]workspaces.Workspace, error) {
	rows, err := r.q.Query(ctx, `
		SELECT w.id, w.organization_id, w.name, w.repo_source, w.default_branch, w.glyph, w.description, w.created_at, m.role
		FROM memberships m JOIN workspaces w ON w.id = m.workspace_id
		WHERE m.user_id = $1 AND m.status = 'active'
		ORDER BY w.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []workspaces.Workspace{}
	for rows.Next() {
		var w workspaces.Workspace
		var org, role string
		if err := rows.Scan(&w.ID, &org, &w.Name, &w.RepoSource, &w.DefaultBranch, &w.Glyph, &w.Description, &w.CreatedAt, &role); err != nil {
			return nil, err
		}
		w.Role = identity.Role(role)
		out = append(out, w)
	}
	return out, rows.Err()
}

func (r *workspaceRepo) GetByUser(ctx context.Context, userID, workspaceID identity.ID) (workspaces.Workspace, error) {
	row := r.q.QueryRow(ctx, `
		SELECT w.id, w.organization_id, w.name, w.repo_source, w.default_branch, w.glyph, w.description, w.created_at, m.role
		FROM memberships m JOIN workspaces w ON w.id = m.workspace_id
		WHERE m.user_id = $1 AND w.id = $2 AND m.status = 'active'`, userID, workspaceID)
	var w workspaces.Workspace
	var org, role string
	err := row.Scan(&w.ID, &org, &w.Name, &w.RepoSource, &w.Glyph, &w.Description, &w.CreatedAt, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		return workspaces.Workspace{}, domain.ErrNotFound
	}
	if err != nil {
		return workspaces.Workspace{}, err
	}
	w.Role = identity.Role(role)
	return w, nil
}

// ── Memberships ─────────────────────────────────────────────────────────────

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

type memberRepo struct{ q querier }

func (r *memberRepo) List(ctx context.Context, workspaceID identity.ID) ([]domain.Member, error) {
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

func (r *memberRepo) Add(ctx context.Context, workspaceID, userID identity.ID, userName, userEmail string, role identity.Role) (domain.Member, error) {
	row := r.q.QueryRow(ctx, `
		INSERT INTO memberships (workspace_id, user_id, user_name, user_email, role, status)
		VALUES ($1, $2, $3, $4, $5, 'active')
		ON CONFLICT (workspace_id, user_id) DO UPDATE SET
			role = EXCLUDED.role, user_name = EXCLUDED.user_name, user_email = EXCLUDED.user_email,
			status = 'active'
		RETURNING `+memberCols, workspaceID, userID, userName, userEmail, role)
	return scanMember(row)
}

func (r *memberRepo) SetRole(ctx context.Context, workspaceID, memberID identity.ID, role identity.Role) (domain.Member, error) {
	row := r.q.QueryRow(ctx, `
		UPDATE memberships SET role = $3 WHERE workspace_id = $1 AND id = $2
		RETURNING `+memberCols, workspaceID, memberID, role)
	m, err := scanMember(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Member{}, domain.ErrNotFound
	}
	return m, err
}

func (r *memberRepo) Remove(ctx context.Context, workspaceID, memberID identity.ID) error {
	tag, err := r.q.Exec(ctx, `DELETE FROM memberships WHERE workspace_id = $1 AND id = $2`, workspaceID, memberID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *memberRepo) OwnerCount(ctx context.Context, workspaceID identity.ID) (int, error) {
	var n int
	err := r.q.QueryRow(ctx, `SELECT count(*) FROM memberships WHERE workspace_id = $1 AND role = 'owner'`, workspaceID).Scan(&n)
	return n, err
}

func (r *memberRepo) UserRoleIn(ctx context.Context, userID, workspaceID identity.ID) (identity.Role, error) {
	var role string
	err := r.q.QueryRow(ctx, `
		SELECT role FROM memberships WHERE user_id = $1 AND workspace_id = $2 AND status = 'active'`,
		userID, workspaceID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrNotFound
	}
	return identity.Role(role), err
}

// ── Invites ─────────────────────────────────────────────────────────────────

type inviteRepo struct{ q querier }

func (r *inviteRepo) Create(ctx context.Context, workspaceID identity.ID, emails []string, role identity.Role) ([]domain.Invite, error) {
	out := []domain.Invite{}
	for _, email := range emails {
		code, err := randomToken(16)
		if err != nil {
			return nil, err
		}
		row := r.q.QueryRow(ctx, `
			INSERT INTO invites (workspace_id, email, role, invite_code)
			VALUES ($1, $2, $3, $4)
			RETURNING id, email, name, role, invite_code, created_at`,
			workspaceID, email, role, code)
		var i domain.Invite
		if err := row.Scan(&i.ID, &i.Email, &i.Name, &i.Role, &i.InviteCode, &i.RequestedAt); err != nil {
			return nil, err
		}
		i.WorkspaceID = workspaceID
		out = append(out, i)
	}
	return out, nil
}

// ── Signup-request projections ──────────────────────────────────────────────

type joinRequestRepo struct{ q querier }

func scanJoinRequest(row pgx.Row) (domain.JoinRequest, error) {
	var j domain.JoinRequest
	err := row.Scan(&j.ID, &j.UserID, &j.Name, &j.Email, &j.WorkspaceID, &j.RequestedRole, &j.Status, &j.RequestedAt)
	return j, err
}

func (r *joinRequestRepo) ListPending(ctx context.Context, workspaceID identity.ID) ([]domain.JoinRequest, error) {
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

func (r *joinRequestRepo) Get(ctx context.Context, requestID identity.ID) (domain.JoinRequest, error) {
	row := r.q.QueryRow(ctx, `
		SELECT request_id, user_id, name, email, workspace_id, requested_role, status, created_at
		FROM join_requests WHERE request_id = $1`, requestID)
	j, err := scanJoinRequest(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.JoinRequest{}, domain.ErrNotFound
	}
	return j, err
}

func (r *joinRequestRepo) Upsert(ctx context.Context, req events.SignupRequestedData) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO join_requests (request_id, user_id, name, email, workspace_id, requested_role)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (request_id) DO NOTHING`,
		req.RequestID, req.UserID, req.Name, req.Email, req.WorkspaceID, req.RequestedRole)
	return err
}

func (r *joinRequestRepo) SetStatus(ctx context.Context, requestID identity.ID, status identity.SignupState) error {
	_, err := r.q.Exec(ctx, `UPDATE join_requests SET status = $2 WHERE request_id = $1`, requestID, status)
	return err
}

type orgRequestRepo struct{ q querier }

func (r *orgRequestRepo) ListPending(ctx context.Context) ([]domain.OrgRequest, error) {
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

func (r *orgRequestRepo) Get(ctx context.Context, requestID identity.ID) (domain.OrgRequest, error) {
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

func (r *orgRequestRepo) Upsert(ctx context.Context, req events.SignupRequestedData) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO org_requests (request_id, user_id, name, email, organization_name, requested_role)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (request_id) DO NOTHING`,
		req.RequestID, req.UserID, req.Name, req.Email, req.OrganizationName, req.RequestedRole)
	return err
}

func (r *orgRequestRepo) SetStatus(ctx context.Context, requestID identity.ID, status identity.SignupState) error {
	_, err := r.q.Exec(ctx, `UPDATE org_requests SET status = $2 WHERE request_id = $1`, requestID, status)
	return err
}
