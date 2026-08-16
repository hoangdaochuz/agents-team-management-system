// Package skill implements the Catalog Skills aggregate repository port on
// Postgres (Ports & Adapters: the adapter side of the hexagon). The same
// adapter serves plain pool access and tx-scoped access via the UnitOfWork.
package skill

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/resources"
	"github.com/aaks/server/services/catalog/internal/domain"
)

// querier is satisfied by both *pgxpool.Pool and pgx.Tx, letting one adapter
// implementation serve plain and transactional access.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Repo implements domain.SkillRepository on Postgres.
type Repo struct{ q querier }

// New builds the adapter over a pool or transaction.
func New(q querier) *Repo { return &Repo{q: q} }

// whereScopedAt returns `AND workspace_id = ANY($start)` scoping plus args.
func whereScopedAt(start int, ws []identity.ID) (string, []any) {
	if len(ws) == 0 {
		return " AND false", nil
	}
	ids := make([]string, len(ws))
	for i, id := range ws {
		ids[i] = string(id)
	}
	return fmt.Sprintf(" AND workspace_id = ANY($%d::uuid[])", start), []any{ids}
}

const skillCols = `id, workspace_id, name, description, body_md, resources_path, enabled, trigger, step_count, created_at`

func scanSkill(row pgx.Row) (resources.Skill, error) {
	var sk resources.Skill
	var enabled *bool
	var stepCount *int
	err := row.Scan(&sk.ID, &sk.WorkspaceID, &sk.Name, &sk.Description, &sk.BodyMd, &sk.ResourcesPath,
		&enabled, &sk.Trigger, &stepCount, &sk.CreatedAt)
	if err != nil {
		return resources.Skill{}, err
	}
	sk.Enabled = enabled
	sk.StepCount = stepCount
	return sk, nil
}

func (r *Repo) List(ctx context.Context, ws []identity.ID) ([]resources.Skill, error) {
	where, args := whereScopedAt(1, ws)
	rows, err := r.q.Query(ctx, `SELECT `+skillCols+` FROM skills WHERE 1=1`+where+` ORDER BY created_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []resources.Skill{}
	for rows.Next() {
		sk, err := scanSkill(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sk)
	}
	return out, rows.Err()
}

func (r *Repo) Get(ctx context.Context, id identity.ID, ws []identity.ID) (resources.Skill, error) {
	where, args := whereScopedAt(2, ws)
	row := r.q.QueryRow(ctx, `SELECT `+skillCols+` FROM skills WHERE id = $1`+where, append([]any{id}, args...)...)
	sk, err := scanSkill(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return resources.Skill{}, domain.ErrSkillNotFound
	}
	return sk, err
}

func (r *Repo) Create(ctx context.Context, workspaceID identity.ID, in domain.SkillCreate) (resources.Skill, error) {
	row := r.q.QueryRow(ctx, `
		INSERT INTO skills (workspace_id, name, description, body_md, trigger)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+skillCols, workspaceID, in.Name, in.Description, in.BodyMd, in.Trigger)
	return scanSkill(row)
}

func (r *Repo) Update(ctx context.Context, id identity.ID, ws []identity.ID, in domain.SkillUpdate) (resources.Skill, error) {
	where, scopeArgs := whereScopedAt(2, ws)
	if len(scopeArgs) == 0 {
		return resources.Skill{}, domain.ErrSkillNotFound
	}
	sets, args := []string{}, append([]any{id}, scopeArgs...)
	idx := 2 + len(scopeArgs)
	add := func(col string, v any) {
		sets = append(sets, fmt.Sprintf("%s = $%d", col, idx))
		args = append(args, v)
		idx++
	}
	if in.Name != nil {
		add("name", *in.Name)
	}
	if in.Description != nil {
		add("description", *in.Description)
	}
	if in.BodyMd != nil {
		add("body_md", *in.BodyMd)
	}
	if in.ResourcesPath != nil {
		add("resources_path", *in.ResourcesPath)
	}
	if in.Enabled != nil {
		add("enabled", *in.Enabled)
	}
	if in.Trigger != nil {
		add("trigger", *in.Trigger)
	}
	if in.StepCount != nil {
		add("step_count", *in.StepCount)
	}
	if len(sets) == 0 {
		return r.Get(ctx, id, ws)
	}
	q := `UPDATE skills SET ` + strings.Join(sets, ", ") + ` WHERE id = $1` + where + ` RETURNING ` + skillCols
	row := r.q.QueryRow(ctx, q, args...)
	sk, err := scanSkill(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return resources.Skill{}, domain.ErrSkillNotFound
	}
	return sk, err
}

func (r *Repo) Delete(ctx context.Context, id identity.ID, ws []identity.ID) error {
	where, args := whereScopedAt(2, ws)
	if len(args) == 0 {
		return domain.ErrSkillNotFound
	}
	tag, err := r.q.Exec(ctx, `DELETE FROM skills WHERE id = $1`+where, append([]any{id}, args...)...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrSkillNotFound
	}
	return nil
}

func (r *Repo) ListByWorkspace(ctx context.Context, workspaceID identity.ID) ([]resources.Skill, error) {
	rows, err := r.q.Query(ctx, `SELECT `+skillCols+` FROM skills WHERE workspace_id = $1 ORDER BY created_at DESC`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []resources.Skill{}
	for rows.Next() {
		sk, err := scanSkill(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sk)
	}
	return out, rows.Err()
}

func (r *Repo) SetEnabled(ctx context.Context, workspaceID, id identity.ID, enabled bool) (resources.Skill, error) {
	row := r.q.QueryRow(ctx, `
		UPDATE skills SET enabled = $3 WHERE id = $1 AND workspace_id = $2
		RETURNING `+skillCols, id, workspaceID, enabled)
	sk, err := scanSkill(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return resources.Skill{}, domain.ErrSkillNotFound
	}
	return sk, err
}
