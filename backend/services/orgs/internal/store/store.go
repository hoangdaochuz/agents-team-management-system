// Package store is the Orgs/Workspaces service persistence layer: organizations,
// workspaces, memberships, invites, and the signup-request projections.
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
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aaks/server/internal/contracts"
	"github.com/aaks/server/internal/platform/db"
)

//go:embed migrations/*.sql
var migrations embed.FS

var (
	ErrNotFound          = errors.New("not found")
	ErrForbidden         = errors.New("forbidden")
	ErrLastOwner         = errors.New("cannot demote or remove the last owner")
	ErrNoOrg             = errors.New("no organization for user")
	ErrDuplicateMember   = errors.New("user is already a member")
	ErrDuplicateInvite   = errors.New("an invite for this email already exists")
)

// Store owns Orgs persistence.
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

// ── Organizations ───────────────────────────────────────────────────────────

const orgCols = `id, owner_id, name, subdomain, plan, seats_total, status, created_at`

func scanOrg(row pgx.Row) (contracts.Organization, error) {
	var o contracts.Organization
	var owner contracts.ID
	err := row.Scan(&o.ID, &owner, &o.Name, &o.Subdomain, &o.Plan, &o.SeatsTotal, &o.Status, &o.CreatedAt)
	if err != nil {
		return contracts.Organization{}, err
	}
	o.SeatsUsed, _ = 0, 0 // filled by ListOrgs/GetOrg
	return o, err
}

// OrgWithCounts is an Organization plus seats/workspace counts.
type OrgWithCounts struct {
	contracts.Organization
}

// ListOrgs returns all organizations (sysadmin), newest first.
func (s *Store) ListOrgs(ctx context.Context) ([]contracts.Organization, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT o.id, o.owner_id, o.name, o.subdomain, o.plan, o.seats_total, o.status, o.created_at,
		       (SELECT count(*) FROM workspaces w WHERE w.organization_id = o.id) AS ws_count,
		       (SELECT count(*) FROM memberships m JOIN workspaces w ON w.id = m.workspace_id WHERE w.organization_id = o.id) AS seats
		FROM organizations o ORDER BY o.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []contracts.Organization{}
	for rows.Next() {
		var o contracts.Organization
		var owner contracts.ID
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

// GetOrg returns one organization by id.
func (s *Store) GetOrg(ctx context.Context, id contracts.ID) (contracts.Organization, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT o.id, o.owner_id, o.name, o.subdomain, o.plan, o.seats_total, o.status, o.created_at,
		       (SELECT count(*) FROM workspaces w WHERE w.organization_id = o.id) AS ws_count,
		       (SELECT count(*) FROM memberships m JOIN workspaces w ON w.id = m.workspace_id WHERE w.organization_id = o.id) AS seats
		FROM organizations o WHERE o.id = $1`, id)
	var o contracts.Organization
	var owner contracts.ID
	var wsCount, seats int
	err := row.Scan(&o.ID, &owner, &o.Name, &o.Subdomain, &o.Plan, &o.SeatsTotal, &o.Status, &o.CreatedAt, &wsCount, &seats)
	if errors.Is(err, pgx.ErrNoRows) {
		return contracts.Organization{}, ErrNotFound
	}
	if err != nil {
		return contracts.Organization{}, err
	}
	o.WorkspaceCount = wsCount
	o.SeatsUsed = seats
	return o, err
}

// CreateOrg inserts an organization.
func (s *Store) CreateOrg(ctx context.Context, ownerID contracts.ID, name string, plan contracts.Plan) (contracts.Organization, error) {
	if plan == "" {
		plan = contracts.PlanFree
	}
	row := s.pool.QueryRow(ctx, `
		INSERT INTO organizations (owner_id, name, plan) VALUES ($1, $2, $3)
		RETURNING `+orgCols, ownerID, name, plan)
	return scanOrg(row)
}

// SetOrgStatus suspends/restores an organization.
func (s *Store) SetOrgStatus(ctx context.Context, id contracts.ID, status contracts.OrgStatus) (contracts.Organization, error) {
	row := s.pool.QueryRow(ctx, `UPDATE organizations SET status = $2 WHERE id = $1 RETURNING `+orgCols, id, status)
	o, err := scanOrg(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return contracts.Organization{}, ErrNotFound
	}
	return o, err
}

// OrgForUser returns the organization owned by a user (created on signup or first workspace).
func (s *Store) OrgForUser(ctx context.Context, userID contracts.ID) (contracts.Organization, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+orgCols+` FROM organizations WHERE owner_id = $1 ORDER BY created_at LIMIT 1`, userID)
	o, err := scanOrg(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return contracts.Organization{}, ErrNoOrg
	}
	return o, err
}

// OrgsStats returns cross-org KPIs for the Gateway composition.
func (s *Store) OrgsStats(ctx context.Context) (organizations, workspaces, openSeats int, err error) {
	err = s.pool.QueryRow(ctx, `
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

func scanWorkspace(row pgx.Row) (contracts.Workspace, error) {
	var w contracts.Workspace
	var org contracts.ID
	err := row.Scan(&w.ID, &org, &w.Name, &w.RepoSource, &w.DefaultBranch, &w.Glyph, &w.Description, &w.CreatedAt)
	if err != nil {
		return contracts.Workspace{}, err
	}
	return w, err
}

// CreateWorkspace inserts a workspace (optionally ensuring a membership).
func (s *Store) CreateWorkspace(ctx context.Context, orgID contracts.ID, name, repoSource, defaultBranch, glyph, description string) (contracts.Workspace, error) {
	if defaultBranch == "" {
		defaultBranch = "main"
	}
	row := s.pool.QueryRow(ctx, `
		INSERT INTO workspaces (organization_id, name, repo_source, default_branch, glyph, description)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+wsCols, orgID, name, repoSource, defaultBranch, glyph, description)
	return scanWorkspace(row)
}

// ListUserWorkspaces returns the workspaces a user belongs to with their role.
func (s *Store) ListUserWorkspaces(ctx context.Context, userID contracts.ID) ([]contracts.Workspace, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT w.id, w.organization_id, w.name, w.repo_source, w.default_branch, w.glyph, w.description, w.created_at, m.role
		FROM memberships m JOIN workspaces w ON w.id = m.workspace_id
		WHERE m.user_id = $1 AND m.status = 'active'
		ORDER BY w.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []contracts.Workspace{}
	for rows.Next() {
		var w contracts.Workspace
		var org, role string
		if err := rows.Scan(&w.ID, &org, &w.Name, &w.RepoSource, &w.DefaultBranch, &w.Glyph, &w.Description, &w.CreatedAt, &role); err != nil {
			return nil, err
		}
		w.Role = contracts.Role(role)
		out = append(out, w)
	}
	return out, rows.Err()
}

// GetUserWorkspace returns one workspace if the user is an active member.
func (s *Store) GetUserWorkspace(ctx context.Context, userID, workspaceID contracts.ID) (contracts.Workspace, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT w.id, w.organization_id, w.name, w.repo_source, w.default_branch, w.glyph, w.description, w.created_at, m.role
		FROM memberships m JOIN workspaces w ON w.id = m.workspace_id
		WHERE m.user_id = $1 AND w.id = $2 AND m.status = 'active'`, userID, workspaceID)
	var w contracts.Workspace
	var org, role string
	err := row.Scan(&w.ID, &org, &w.Name, &w.RepoSource, &w.Glyph, &w.Description, &w.CreatedAt, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		return contracts.Workspace{}, ErrNotFound
	}
	if err != nil {
		return contracts.Workspace{}, err
	}
	w.Role = contracts.Role(role)
	return w, nil
}

// UserRoleIn returns the user's role in a workspace ("" if not a member).
func (s *Store) UserRoleIn(ctx context.Context, userID, workspaceID contracts.ID) (contracts.Role, error) {
	var role string
	err := s.pool.QueryRow(ctx, `
		SELECT role FROM memberships WHERE user_id = $1 AND workspace_id = $2 AND status = 'active'`,
		userID, workspaceID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return contracts.Role(role), err
}

// WorkspaceUserIDs returns the members of a workspace (Gateway/agent composition).
func (s *Store) WorkspaceUserIDs(ctx context.Context, workspaceID contracts.ID) ([]contracts.ID, error) {
	rows, err := s.pool.Query(ctx, `SELECT user_id FROM memberships WHERE workspace_id = $1`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []contracts.ID{}
	for rows.Next() {
		var id contracts.ID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ── Memberships ─────────────────────────────────────────────────────────────

// MemberRow is a membership with its user identity snapshot.
type MemberRow struct {
	contracts.Member
	WorkspaceID contracts.ID
	UserID      contracts.ID
}

const memberCols = `id, workspace_id, user_id, user_name, user_email, role, status, last_active_at, is_service_account`

func scanMember(row pgx.Row) (MemberRow, error) {
	var m MemberRow
	var lastActive *time.Time
	var isService bool
	err := row.Scan(&m.ID, &m.WorkspaceID, &m.UserID, &m.User.Name, &m.User.Email, &m.Role, &m.Status, &lastActive, &isService)
	if err != nil {
		return MemberRow{}, err
	}
	m.User.ID = m.UserID
	m.LastActiveAt = lastActive
	m.IsServiceAccount = &isService
	return m, nil
}

// ListMembers returns the active + invited members of a workspace.
func (s *Store) ListMembers(ctx context.Context, workspaceID contracts.ID) ([]MemberRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+memberCols+` FROM memberships WHERE workspace_id = $1 ORDER BY created_at`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MemberRow{}
	for rows.Next() {
		m, err := scanMember(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// AddMembership inserts a membership (upsert from invited → active).
func (s *Store) AddMembership(ctx context.Context, workspaceID, userID contracts.ID, userName, userEmail string, role contracts.Role) (MemberRow, error) {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO memberships (workspace_id, user_id, user_name, user_email, role, status)
		VALUES ($1, $2, $3, $4, $5, 'active')
		ON CONFLICT (workspace_id, user_id) DO UPDATE SET
			role = EXCLUDED.role, user_name = EXCLUDED.user_name, user_email = EXCLUDED.user_email,
			status = 'active'
		RETURNING `+memberCols, workspaceID, userID, userName, userEmail, role)
	return scanMember(row)
}

// SetMemberRole changes a member's role.
func (s *Store) SetMemberRole(ctx context.Context, workspaceID, memberID contracts.ID, role contracts.Role) (MemberRow, error) {
	row := s.pool.QueryRow(ctx, `
		UPDATE memberships SET role = $3 WHERE workspace_id = $1 AND id = $2
		RETURNING `+memberCols, workspaceID, memberID, role)
	m, err := scanMember(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return MemberRow{}, ErrNotFound
	}
	return m, err
}

// RemoveMember deletes a membership.
func (s *Store) RemoveMember(ctx context.Context, workspaceID, memberID contracts.ID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM memberships WHERE workspace_id = $1 AND id = $2`, workspaceID, memberID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// OwnerCount counts owners in a workspace (last-owner protection).
func (s *Store) OwnerCount(ctx context.Context, workspaceID contracts.ID) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM memberships WHERE workspace_id = $1 AND role = 'owner'`, workspaceID).Scan(&n)
	return n, err
}

// ── Invites ─────────────────────────────────────────────────────────────────

// InviteRow mirrors contracts.Invite plus the invite code (internal only).
type InviteRow struct {
	contracts.Invite
	WorkspaceID contracts.ID
	InviteCode  string
}

// CreateInvites inserts invites for the given emails (one code each).
func (s *Store) CreateInvites(ctx context.Context, workspaceID contracts.ID, emails []string, role contracts.Role) ([]InviteRow, error) {
	out := []InviteRow{}
	for _, email := range emails {
		code, err := randomToken(16)
		if err != nil {
			return nil, err
		}
		row := s.pool.QueryRow(ctx, `
			INSERT INTO invites (workspace_id, email, role, invite_code)
			VALUES ($1, $2, $3, $4)
			RETURNING id, email, name, role, invite_code, created_at`,
			workspaceID, email, role, code)
		var i InviteRow
		if err := row.Scan(&i.ID, &i.Email, &i.Name, &i.Role, &i.InviteCode, &i.RequestedAt); err != nil {
			return nil, err
		}
		i.WorkspaceID = workspaceID
		out = append(out, i)
	}
	return out, nil
}

// ── Signup-request projections ──────────────────────────────────────────────

// JoinRequest mirrors contracts.SignupRequest for a workspace.
type JoinRequest struct {
	contracts.SignupRequest
	UserID contracts.ID
	Status contracts.SignupState
}

// ListPendingJoinRequests returns pending join requests for a workspace.
func (s *Store) ListPendingJoinRequests(ctx context.Context, workspaceID contracts.ID) ([]JoinRequest, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT request_id, user_id, name, email, workspace_id, requested_role, status, created_at
		FROM join_requests WHERE workspace_id = $1 AND status = 'pending' ORDER BY created_at`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []JoinRequest{}
	for rows.Next() {
		jr, err := scanJoinRequest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, jr)
	}
	return out, rows.Err()
}

func scanJoinRequest(row pgx.Row) (JoinRequest, error) {
	var j JoinRequest
	err := row.Scan(&j.ID, &j.UserID, &j.Name, &j.Email, &j.WorkspaceID, &j.RequestedRole, &j.Status, &j.RequestedAt)
	return j, err
}

// GetJoinRequest returns one join request.
func (s *Store) GetJoinRequest(ctx context.Context, requestID contracts.ID) (JoinRequest, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT request_id, user_id, name, email, workspace_id, requested_role, status, created_at
		FROM join_requests WHERE request_id = $1`, requestID)
	j, err := scanJoinRequest(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return JoinRequest{}, ErrNotFound
	}
	return j, err
}

// UpsertJoinRequest projects a signup.requested (join mode).
func (s *Store) UpsertJoinRequest(ctx context.Context, req contracts.SignupRequestedData) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO join_requests (request_id, user_id, name, email, workspace_id, requested_role)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (request_id) DO NOTHING`,
		req.RequestID, req.UserID, req.Name, req.Email, req.WorkspaceID, req.RequestedRole)
	return err
}

// SetJoinRequestStatus transitions a join request.
func (s *Store) SetJoinRequestStatus(ctx context.Context, requestID contracts.ID, status contracts.SignupState) error {
	_, err := s.pool.Exec(ctx, `UPDATE join_requests SET status = $2 WHERE request_id = $1`, requestID, status)
	return err
}

// OrgRequest mirrors contracts.SignupRequest for create-mode requests.
type OrgRequest struct {
	contracts.SignupRequest
	UserID           contracts.ID
	OrganizationName string
	Status           contracts.SignupState
}

// ListPendingOrgRequests returns pending create-mode requests (sysadmin).
func (s *Store) ListPendingOrgRequests(ctx context.Context) ([]OrgRequest, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT request_id, user_id, name, email, organization_name, requested_role, status, created_at
		FROM org_requests WHERE status = 'pending' ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []OrgRequest{}
	for rows.Next() {
		var o OrgRequest
		if err := rows.Scan(&o.ID, &o.UserID, &o.Name, &o.Email, &o.OrganizationName, &o.RequestedRole, &o.Status, &o.RequestedAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// GetOrgRequest returns one create-mode request.
func (s *Store) GetOrgRequest(ctx context.Context, requestID contracts.ID) (OrgRequest, error) {
	var o OrgRequest
	err := s.pool.QueryRow(ctx, `
		SELECT request_id, user_id, name, email, organization_name, requested_role, status, created_at
		FROM org_requests WHERE request_id = $1`, requestID).
		Scan(&o.ID, &o.UserID, &o.Name, &o.Email, &o.OrganizationName, &o.RequestedRole, &o.Status, &o.RequestedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return OrgRequest{}, ErrNotFound
	}
	return o, err
}

// UpsertOrgRequest projects a signup.requested (create mode).
func (s *Store) UpsertOrgRequest(ctx context.Context, req contracts.SignupRequestedData) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO org_requests (request_id, user_id, name, email, organization_name, requested_role)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (request_id) DO NOTHING`,
		req.RequestID, req.UserID, req.Name, req.Email, req.OrganizationName, req.RequestedRole)
	return err
}

// SetOrgRequestStatus transitions a create-mode request.
func (s *Store) SetOrgRequestStatus(ctx context.Context, requestID contracts.ID, status contracts.SignupState) error {
	_, err := s.pool.Exec(ctx, `UPDATE org_requests SET status = $2 WHERE request_id = $1`, requestID, status)
	return err
}

// WorkspaceByID returns a workspace regardless of membership (orgs-internal).
func (s *Store) WorkspaceByID(ctx context.Context, id contracts.ID) (contracts.Workspace, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+wsCols+` FROM workspaces WHERE id = $1`, id)
	w, err := scanWorkspace(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return contracts.Workspace{}, ErrNotFound
	}
	return w, err
}

// randomToken returns n random bytes hex-encoded (invite codes, session ids).
func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
