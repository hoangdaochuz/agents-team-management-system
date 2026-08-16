// Package store is the Runner service persistence layer: runs, steps,
// findings, and artifacts.
package store

import (
	"context"
	"embed"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aaks/server/internal/contracts"
	"github.com/aaks/server/internal/platform/db"
)

//go:embed migrations/*.sql
var migrations embed.FS

var ErrNotFound = errors.New("not found")

// Store owns Runner persistence.
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

// ── Runs ───────────────────────────────────────────────────────────────────

// CreateRun inserts a run and returns its id + started time.
func (s *Store) CreateRun(ctx context.Context, taskID contracts.ID, role contracts.RunRole, agentID contracts.ID, model string, roundNo int) (contracts.ID, error) {
	var id contracts.ID
	err := s.pool.QueryRow(ctx, `
		INSERT INTO runs (task_id, role, agent_id, model, round_no)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		taskID, role, agentID, model, roundNo).Scan(&id)
	return id, err
}

// ListRunsByTask returns a task's runs, newest first.
func (s *Store) ListRunsByTask(ctx context.Context, taskID contracts.ID) ([]contracts.Run, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, task_id, role, agent_id, model, status, round_no, started_at, ended_at, token_usage, error
		FROM runs WHERE task_id = $1 ORDER BY started_at DESC`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []contracts.Run{}
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetRun returns one run.
func (s *Store) GetRun(ctx context.Context, runID contracts.ID) (contracts.Run, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, task_id, role, agent_id, model, status, round_no, started_at, ended_at, token_usage, error
		FROM runs WHERE id = $1`, runID)
	return scanRun(row)
}

// LatestRun returns the most recent run for a task (PR/artifact wiring).
func (s *Store) LatestRun(ctx context.Context, taskID contracts.ID) (contracts.Run, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, task_id, role, agent_id, model, status, round_no, started_at, ended_at, token_usage, error
		FROM runs WHERE task_id = $1 ORDER BY started_at DESC LIMIT 1`, taskID)
	return scanRun(row)
}

func scanRun(row interface{ Scan(...any) error }) (contracts.Run, error) {
	var r contracts.Run
	var ended *time.Time
	var agent contracts.ID
	var errMsg string
	err := row.Scan(&r.ID, &r.TaskID, &r.Role, &agent, &r.Model, &r.Status, &r.RoundNo, &r.StartedAt, &ended, &r.TokenUsage, &errMsg)
	if err != nil {
		return contracts.Run{}, err
	}
	r.AgentID = agent
	r.EndedAt = ended
	r.Error = errMsg
	return r, nil
}

// FinishRun marks a run done/aborted/stopped with token usage + error.
func (s *Store) FinishRun(ctx context.Context, runID contracts.ID, status contracts.RunStatus, tokenUsage int, errMsg string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE runs SET status = $2, token_usage = $3, error = $4, ended_at = now()
		WHERE id = $1`, runID, status, tokenUsage, errMsg)
	return err
}

// ── Steps ──────────────────────────────────────────────────────────────────

// AppendStep persists a step (idempotent by run_id+seq).
func (s *Store) AppendStep(ctx context.Context, runID contracts.ID, seq int, kind contracts.StepKind, payload []byte) (contracts.Step, error) {
	var st contracts.Step
	err := s.pool.QueryRow(ctx, `
		INSERT INTO steps (run_id, seq, kind, payload) VALUES ($1, $2, $3, $4)
		ON CONFLICT (run_id, seq) DO NOTHING
		RETURNING id, run_id, seq, kind, payload, created_at`,
		runID, seq, kind, payload).
		Scan(&st.ID, &st.RunID, &st.Seq, &st.Kind, &st.Payload, &st.CreatedAt)
	return st, err
}

// ListSteps returns a run's steps in sequence order.
func (s *Store) ListSteps(ctx context.Context, runID contracts.ID) ([]contracts.Step, error) {	rows, err := s.pool.Query(ctx, `
		SELECT id, run_id, seq, kind, payload, created_at FROM steps
		WHERE run_id = $1 ORDER BY seq`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []contracts.Step{}
	for rows.Next() {
		var st contracts.Step
		if err := rows.Scan(&st.ID, &st.RunID, &st.Seq, &st.Kind, &st.Payload, &st.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// ListStepsByTask returns all steps for a task's runs, ordered by run start
// then seq (SSE replay).
func (s *Store) ListStepsByTask(ctx context.Context, taskID contracts.ID) ([]contracts.Step, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT st.id, st.run_id, st.seq, st.kind, st.payload, st.created_at
		FROM steps st JOIN runs r ON r.id = st.run_id
		WHERE r.task_id = $1 ORDER BY r.started_at, st.seq`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []contracts.Step{}
	for rows.Next() {
		var st contracts.Step
		if err := rows.Scan(&st.ID, &st.RunID, &st.Seq, &st.Kind, &st.Payload, &st.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// MaxStepSeq returns the last seq persisted for a run (replay resume).
func (s *Store) MaxStepSeq(ctx context.Context, runID contracts.ID) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT coalesce(max(seq), 0) FROM steps WHERE run_id = $1`, runID).Scan(&n)
	return n, err
}

// ── Findings ────────────────────────────────────────────────────────────────

// AddFinding persists a reviewer finding.
func (s *Store) AddFinding(ctx context.Context, runID contracts.ID, f contracts.Finding) (contracts.Finding, error) {
	var out contracts.Finding
	err := s.pool.QueryRow(ctx, `
		INSERT INTO findings (run_id, file, line, severity, issue, recommendation, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, run_id, file, line, severity, issue, recommendation, status`,
		runID, f.File, f.Line, f.Severity, f.Issue, f.Recommendation, f.Status).
		Scan(&out.ID, &out.RunID, &out.File, &out.Line, &out.Severity, &out.Issue, &out.Recommendation, &out.Status)
	return out, err
}

// ListFindings returns a run's findings.
func (s *Store) ListFindings(ctx context.Context, runID contracts.ID) ([]contracts.Finding, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, run_id, file, line, severity, issue, recommendation, status FROM findings
		WHERE run_id = $1 ORDER BY created_at`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []contracts.Finding{}
	for rows.Next() {
		var f contracts.Finding
		if err := rows.Scan(&f.ID, &f.RunID, &f.File, &f.Line, &f.Severity, &f.Issue, &f.Recommendation, &f.Status); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// ── Artifacts ───────────────────────────────────────────────────────────────

// AddArtifact persists a run artifact.
func (s *Store) AddArtifact(ctx context.Context, taskID contracts.ID, runID *contracts.ID, filename, kind, summary string, additions, deletions int) (contracts.Artifact, error) {
	var a contracts.Artifact
	err := s.pool.QueryRow(ctx, `
		INSERT INTO artifacts (task_id, run_id, filename, kind, summary, additions, deletions)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, task_id, run_id, filename, kind, summary, additions, deletions, created_at`,
		taskID, runID, filename, kind, summary, additions, deletions).
		Scan(&a.ID, &a.TaskID, &a.RunID, &a.Filename, &a.Kind, &a.Summary, &a.Additions, &a.Deletions, &a.CreatedAt)
	return a, err
}

// ListArtifactsByTask returns a task's artifacts, newest first.
func (s *Store) ListArtifactsByTask(ctx context.Context, taskID contracts.ID) ([]contracts.Artifact, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, task_id, run_id, filename, kind, summary, additions, deletions, created_at
		FROM artifacts WHERE task_id = $1 ORDER BY created_at DESC`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []contracts.Artifact{}
	for rows.Next() {
		var a contracts.Artifact
		if err := rows.Scan(&a.ID, &a.TaskID, &a.RunID, &a.Filename, &a.Kind, &a.Summary, &a.Additions, &a.Deletions, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
