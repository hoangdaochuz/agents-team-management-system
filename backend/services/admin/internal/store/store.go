// Package store is the Admin service persistence layer: audit entries and
// feature flags.
package store

import (
	"context"
	"embed"
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

var ErrNotFound = errors.New("not found")

// Store owns Admin persistence.
type Store struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, dsn string, log *slog.Logger) (*Store, error) {
	pool, err := db.Pool(ctx, dsn, log)
	if err != nil {
		return nil, err
	}
	if err := db.MigrateFS(ctx, pool, migrations, "migrations", log); err != nil {
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

// ── Audit ───────────────────────────────────────────────────────────────────

// AuditRow is an audit entry with its raw actor id (unexposed in the DTO).
type AuditRow struct {
	contracts.AuditEntry
	ActorID     contracts.ID
	WorkspaceID contracts.ID
}

const auditCols = `id, workspace_id, actor_name, actor_id, action, action_kind, target, ip, created_at`

// ListAudit returns a workspace's audit entries, newest first, optionally
// filtered by action kind.
func (s *Store) ListAudit(ctx context.Context, workspaceID contracts.ID, kind string) ([]AuditRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+auditCols+` FROM audit_entries
		WHERE workspace_id = $1 AND ($2 = '' OR action_kind = $2)
		ORDER BY created_at DESC LIMIT 200`, workspaceID, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditRow{}
	for rows.Next() {
		var a AuditRow
		if err := rows.Scan(&a.ID, &a.WorkspaceID, &a.Actor.Name, &a.ActorID, &a.Action, &a.ActionKind, &a.Target, &a.IP, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ListSystemAudit returns the most recent audit entries across workspaces
// (superadmin surface).
func (s *Store) ListSystemAudit(ctx context.Context, limit int) ([]AuditRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.pool.Query(ctx, `
		SELECT `+auditCols+` FROM audit_entries ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditRow{}
	for rows.Next() {
		var a AuditRow
		if err := rows.Scan(&a.ID, &a.WorkspaceID, &a.Actor.Name, &a.ActorID, &a.Action, &a.ActionKind, &a.Target, &a.IP, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// AppendAudit records a workspace admin action (from audit.recorded events).
func (s *Store) AppendAudit(ctx context.Context, workspaceID contracts.ID, actorName string, actorID contracts.ID, action, kind, target, ip string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO audit_entries (workspace_id, actor_name, actor_id, action, action_kind, target, ip)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`, workspaceID, actorName, actorID, action, kind, target, ip)
	return err
}

// CountAudit24h counts entries in a workspace over the last 24h.
func (s *Store) CountAudit24h(ctx context.Context, workspaceID contracts.ID) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM audit_entries
		WHERE workspace_id = $1 AND created_at > now() - interval '24 hours'`, workspaceID).Scan(&n)
	return n, err
}

// ── Feature flags ───────────────────────────────────────────────────────────

// ListFlags returns all feature flags.
func (s *Store) ListFlags(ctx context.Context) ([]contracts.FeatureFlag, error) {
	rows, err := s.pool.Query(ctx, `SELECT key, label, description, enabled FROM feature_flags ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []contracts.FeatureFlag{}
	for rows.Next() {
		var f contracts.FeatureFlag
		if err := rows.Scan(&f.Key, &f.Label, &f.Description, &f.Enabled); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// SetFlagEnabled toggles a feature flag.
func (s *Store) SetFlagEnabled(ctx context.Context, key string, enabled bool) (contracts.FeatureFlag, error) {
	row := s.pool.QueryRow(ctx, `
		UPDATE feature_flags SET enabled = $2, updated_at = $3 WHERE key = $1
		RETURNING key, label, description, enabled`, key, enabled, time.Now())
	var f contracts.FeatureFlag
	err := row.Scan(&f.Key, &f.Label, &f.Description, &f.Enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return contracts.FeatureFlag{}, ErrNotFound
	}
	return f, err
}
