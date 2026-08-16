// Package knowledge implements the knowledge-source aggregate repository
// adapter on Postgres (Ports & Adapters: the adapter side of the hexagon).
package knowledge

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/resources"
)

// querier is satisfied by both *pgxpool.Pool and pgx.Tx, letting one adapter
// implementation serve plain and transactional access.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Repo is the pool- or tx-backed adapter for the knowledge-source aggregate.
type Repo struct{ q querier }

// New wraps a querier (pool or tx) as the knowledge-source adapter.
func New(q querier) *Repo { return &Repo{q: q} }

const knowledgeCols = `id, title, kind, chunks, pages, status`

func scanKnowledge(row pgx.Row) (resources.KnowledgeSource, error) {
	var k resources.KnowledgeSource
	err := row.Scan(&k.ID, &k.Title, &k.Kind, &k.Chunks, &k.Pages, &k.Status)
	return k, err
}

func (r *Repo) List(ctx context.Context, workspaceID identity.ID) ([]resources.KnowledgeSource, error) {
	rows, err := r.q.Query(ctx, `
		SELECT `+knowledgeCols+` FROM knowledge_sources
		WHERE workspace_id = $1 ORDER BY created_at DESC`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []resources.KnowledgeSource{}
	for rows.Next() {
		k, err := scanKnowledge(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (r *Repo) Create(ctx context.Context, workspaceID identity.ID, title, kind string) (resources.KnowledgeSource, error) {
	row := r.q.QueryRow(ctx, `
		INSERT INTO knowledge_sources (workspace_id, title, kind)
		VALUES ($1, $2, $3)
		RETURNING `+knowledgeCols, workspaceID, title, kind)
	return scanKnowledge(row)
}