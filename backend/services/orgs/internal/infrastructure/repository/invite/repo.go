// Package invite adapts the Invite aggregate repository port to Postgres
// (Ports & Adapters: the adapter side of the hexagon). Instances serve both
// plain-pool and tx-scoped access via the shared querier.
package invite

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/services/orgs/internal/domain"
)

// querier is satisfied by both *pgxpool.Pool and pgx.Tx, letting one adapter
// implementation serve plain and transactional access.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Repo is the Postgres adapter for the Invite aggregate.
type Repo struct{ q querier }

// New builds the adapter on a pool or transaction querier.
func New(q querier) *Repo { return &Repo{q: q} }

// randomToken returns n random bytes hex-encoded (invite codes, session ids).
func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (r *Repo) Create(ctx context.Context, workspaceID identity.ID, emails []string, role identity.Role) ([]domain.Invite, error) {
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