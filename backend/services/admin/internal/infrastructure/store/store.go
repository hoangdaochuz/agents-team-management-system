// Package store implements the Admin domain repository ports on Postgres
// (Ports & Adapters: the adapter side of the hexagon). Each aggregate has its
// own adapter; admin use cases are single-aggregate writes, so pool-backed
// adapters are sufficient.
package store

import (
	"context"
	"embed"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aaks/server/internal/contracts/admin"
	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/platform/db"
	"github.com/aaks/server/services/admin/internal/domain"
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

// Store owns the admin Postgres pool and exposes pool-backed adapters for the
// domain ports. It is the composition-root entrypoint of this package.
type Store struct {
	pool *pgxpool.Pool
	log  *slog.Logger

	Audit domain.AuditRepository
	Flags domain.FlagRepository
}

// New opens the admin database and runs migrations.
func New(ctx context.Context, dsn string, log *slog.Logger) (*Store, error) {
	pool, err := db.Pool(ctx, dsn, log)
	if err != nil {
		return nil, err
	}
	if err := db.Migrate(ctx, pool, migrations, "migrations", log); err != nil {
		return nil, err
	}
	s := &Store{pool: pool, log: log}
	s.Audit = &auditRepo{q: pool}
	s.Flags = &flagRepo{q: pool}
	return s, nil
}

// Close releases the connection pool.
func (s *Store) Close() { s.pool.Close() }

// ── Audit ───────────────────────────────────────────────────────────────────

const auditCols = `id, workspace_id, actor_name, actor_id, action, action_kind, target, ip, created_at`

func scanAuditRow(row pgx.Row) (domain.AuditRow, error) {
	var a domain.AuditRow
	err := row.Scan(&a.ID, &a.WorkspaceID, &a.Actor.Name, &a.ActorID, &a.Action, &a.ActionKind, &a.Target, &a.IP, &a.CreatedAt)
	return a, err
}

type auditRepo struct{ q querier }

func (r *auditRepo) List(ctx context.Context, workspaceID identity.ID, kind string) ([]domain.AuditRow, error) {
	rows, err := r.q.Query(ctx, `
		SELECT `+auditCols+` FROM audit_entries
		WHERE workspace_id = $1 AND ($2 = '' OR action_kind = $2)
		ORDER BY created_at DESC LIMIT 200`, workspaceID, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.AuditRow{}
	for rows.Next() {
		a, err := scanAuditRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *auditRepo) ListSystem(ctx context.Context, limit int) ([]domain.AuditRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := r.q.Query(ctx, `
		SELECT `+auditCols+` FROM audit_entries ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.AuditRow{}
	for rows.Next() {
		a, err := scanAuditRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *auditRepo) Append(ctx context.Context, workspaceID identity.ID, actorName string, actorID identity.ID, action, kind, target, ip string) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO audit_entries (workspace_id, actor_name, actor_id, action, action_kind, target, ip)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`, workspaceID, actorName, actorID, action, kind, target, ip)
	return err
}

func (r *auditRepo) Count24h(ctx context.Context, workspaceID identity.ID) (int, error) {
	var n int
	err := r.q.QueryRow(ctx, `
		SELECT count(*) FROM audit_entries
		WHERE workspace_id = $1 AND created_at > now() - interval '24 hours'`, workspaceID).Scan(&n)
	return n, err
}

// ── Feature flags ───────────────────────────────────────────────────────────

const flagCols = `key, label, description, enabled`

func scanFlag(row pgx.Row) (admin.FeatureFlag, error) {
	var f admin.FeatureFlag
	err := row.Scan(&f.Key, &f.Label, &f.Description, &f.Enabled)
	return f, err
}

type flagRepo struct{ q querier }

func (r *flagRepo) List(ctx context.Context) ([]admin.FeatureFlag, error) {
	rows, err := r.q.Query(ctx, `SELECT `+flagCols+` FROM feature_flags ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []admin.FeatureFlag{}
	for rows.Next() {
		f, err := scanFlag(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (r *flagRepo) SetEnabled(ctx context.Context, key string, enabled bool) (admin.FeatureFlag, error) {
	row := r.q.QueryRow(ctx, `
		UPDATE feature_flags SET enabled = $2, updated_at = $3 WHERE key = $1
		RETURNING `+flagCols, key, enabled, time.Now())
	f, err := scanFlag(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return admin.FeatureFlag{}, domain.ErrNotFound
	}
	return f, err
}