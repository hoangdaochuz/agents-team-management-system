// Package repository implements the Runner domain repository ports on Postgres.
// Each aggregate has its own adapter satisfying the domain port (Ports &
// Adapters: the adapter side of the hexagon).
package repository

import (
	"context"
	"embed"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aaks/server/internal/contracts/agentexec"
	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/platform/db"
	"github.com/aaks/server/services/runner/internal/domain"
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

// Store owns the runner Postgres pool and exposes pool-backed adapters.
type Store struct {
	pool *pgxpool.Pool
	log  *slog.Logger

	Runs      domain.RunRepository
	Steps     domain.StepRepository
	Findings  domain.FindingRepository
	Artifacts domain.ArtifactRepository
}

// New opens the runner database and runs migrations.
func New(ctx context.Context, dsn string, log *slog.Logger) (*Store, error) {
	pool, err := db.Pool(ctx, dsn, log)
	if err != nil {
		return nil, err
	}
	if err := db.Migrate(ctx, pool, migrations, "migrations", log); err != nil {
		return nil, err
	}
	s := &Store{pool: pool, log: log}
	s.Runs = &runRepo{q: pool}
	s.Steps = &stepRepo{q: pool}
	s.Findings = &findingRepo{q: pool}
	s.Artifacts = &artifactRepo{q: pool}
	return s, nil
}

// Close releases the connection pool.
func (s *Store) Close() { s.pool.Close() }

// ── Runs ───────────────────────────────────────────────────────────────────

type runRepo struct{ q querier }

func scanRun(row pgx.Row) (agentexec.Run, error) {
	var r agentexec.Run
	var ended *time.Time
	var agent identity.ID
	var errMsg string
	err := row.Scan(&r.ID, &r.TaskID, &r.Role, &agent, &r.Model, &r.Status, &r.RoundNo, &r.StartedAt, &ended, &r.TokenUsage, &errMsg)
	if err != nil {
		return agentexec.Run{}, err
	}
	r.AgentID = agent
	r.EndedAt = ended
	r.Error = errMsg
	return r, nil
}

func (r *runRepo) CreateRun(ctx context.Context, taskID identity.ID, role agentexec.RunRole, agentID identity.ID, model string, roundNo int) (identity.ID, error) {
	var id identity.ID
	err := r.q.QueryRow(ctx, `
		INSERT INTO runs (task_id, role, agent_id, model, round_no)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		taskID, role, agentID, model, roundNo).Scan(&id)
	return id, err
}

func (r *runRepo) ListRunsByTask(ctx context.Context, taskID identity.ID) ([]agentexec.Run, error) {
	rows, err := r.q.Query(ctx, `
		SELECT id, task_id, role, agent_id, model, status, round_no, started_at, ended_at, token_usage, error
		FROM runs WHERE task_id = $1 ORDER BY started_at DESC`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []agentexec.Run{}
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

func (r *runRepo) GetRun(ctx context.Context, runID identity.ID) (agentexec.Run, error) {
	row := r.q.QueryRow(ctx, `
		SELECT id, task_id, role, agent_id, model, status, round_no, started_at, ended_at, token_usage, error
		FROM runs WHERE id = $1`, runID)
	return scanRun(row)
}

func (r *runRepo) LatestRun(ctx context.Context, taskID identity.ID) (agentexec.Run, error) {
	row := r.q.QueryRow(ctx, `
		SELECT id, task_id, role, agent_id, model, status, round_no, started_at, ended_at, token_usage, error
		FROM runs WHERE task_id = $1 ORDER BY started_at DESC LIMIT 1`, taskID)
	return scanRun(row)
}

func (r *runRepo) FinishRun(ctx context.Context, runID identity.ID, status agentexec.RunStatus, tokenUsage int, errMsg string) error {
	_, err := r.q.Exec(ctx, `
		UPDATE runs SET status = $2, token_usage = $3, error = $4, ended_at = now()
		WHERE id = $1`, runID, status, tokenUsage, errMsg)
	return err
}

// ── Steps ─────────────────────────────────────────────────────────────────

type stepRepo struct{ q querier }

func (r *stepRepo) AppendStep(ctx context.Context, runID identity.ID, seq int, kind agentexec.StepKind, payload []byte) (agentexec.Step, error) {
	var st agentexec.Step
	err := r.q.QueryRow(ctx, `
		INSERT INTO steps (run_id, seq, kind, payload) VALUES ($1, $2, $3, $4)
		ON CONFLICT (run_id, seq) DO NOTHING
		RETURNING id, run_id, seq, kind, payload, created_at`,
		runID, seq, kind, payload).
		Scan(&st.ID, &st.RunID, &st.Seq, &st.Kind, &st.Payload, &st.CreatedAt)
	return st, err
}

func (r *stepRepo) ListSteps(ctx context.Context, runID identity.ID) ([]agentexec.Step, error) {
	rows, err := r.q.Query(ctx, `
		SELECT id, run_id, seq, kind, payload, created_at FROM steps
		WHERE run_id = $1 ORDER BY seq`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []agentexec.Step{}
	for rows.Next() {
		var st agentexec.Step
		if err := rows.Scan(&st.ID, &st.RunID, &st.Seq, &st.Kind, &st.Payload, &st.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

func (r *stepRepo) ListStepsByTask(ctx context.Context, taskID identity.ID) ([]agentexec.Step, error) {
	rows, err := r.q.Query(ctx, `
		SELECT st.id, st.run_id, st.seq, st.kind, st.payload, st.created_at
		FROM steps st JOIN runs r ON r.id = st.run_id
		WHERE r.task_id = $1 ORDER BY r.started_at, st.seq`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []agentexec.Step{}
	for rows.Next() {
		var st agentexec.Step
		if err := rows.Scan(&st.ID, &st.RunID, &st.Seq, &st.Kind, &st.Payload, &st.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

func (r *stepRepo) MaxStepSeq(ctx context.Context, runID identity.ID) (int, error) {
	var n int
	err := r.q.QueryRow(ctx, `SELECT coalesce(max(seq), 0) FROM steps WHERE run_id = $1`, runID).Scan(&n)
	return n, err
}

// ── Findings ───────────────────────────────────────────────────────────────

type findingRepo struct{ q querier }

func (r *findingRepo) AddFinding(ctx context.Context, runID identity.ID, f agentexec.Finding) (agentexec.Finding, error) {
	var out agentexec.Finding
	err := r.q.QueryRow(ctx, `
		INSERT INTO findings (run_id, file, line, severity, issue, recommendation, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, run_id, file, line, severity, issue, recommendation, status`,
		runID, f.File, f.Line, f.Severity, f.Issue, f.Recommendation, f.Status).
		Scan(&out.ID, &out.RunID, &out.File, &out.Line, &out.Severity, &out.Issue, &out.Recommendation, &out.Status)
	return out, err
}

func (r *findingRepo) ListFindings(ctx context.Context, runID identity.ID) ([]agentexec.Finding, error) {
	rows, err := r.q.Query(ctx, `
		SELECT id, run_id, file, line, severity, issue, recommendation, status FROM findings
		WHERE run_id = $1 ORDER BY created_at`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []agentexec.Finding{}
	for rows.Next() {
		var f agentexec.Finding
		if err := rows.Scan(&f.ID, &f.RunID, &f.File, &f.Line, &f.Severity, &f.Issue, &f.Recommendation, &f.Status); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// ── Artifacts ──────────────────────────────────────────────────────────────

type artifactRepo struct{ q querier }

func (r *artifactRepo) AddArtifact(ctx context.Context, taskID identity.ID, runID *identity.ID, filename, kind, summary string, additions, deletions int) (agentexec.Artifact, error) {
	var a agentexec.Artifact
	err := r.q.QueryRow(ctx, `
		INSERT INTO artifacts (task_id, run_id, filename, kind, summary, additions, deletions)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, task_id, run_id, filename, kind, summary, additions, deletions, created_at`,
		taskID, runID, filename, kind, summary, additions, deletions).
		Scan(&a.ID, &a.TaskID, &a.RunID, &a.Filename, &a.Kind, &a.Summary, &a.Additions, &a.Deletions, &a.CreatedAt)
	return a, err
}

func (r *artifactRepo) ListArtifactsByTask(ctx context.Context, taskID identity.ID) ([]agentexec.Artifact, error) {
	rows, err := r.q.Query(ctx, `
		SELECT id, task_id, run_id, filename, kind, summary, additions, deletions, created_at
		FROM artifacts WHERE task_id = $1 ORDER BY created_at DESC`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []agentexec.Artifact{}
	for rows.Next() {
		var a agentexec.Artifact
		if err := rows.Scan(&a.ID, &a.TaskID, &a.RunID, &a.Filename, &a.Kind, &a.Summary, &a.Additions, &a.Deletions, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
