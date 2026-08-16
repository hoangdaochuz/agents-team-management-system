// Package key implements the ProviderKey aggregate's Postgres adapter.
package key

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/services/settings/internal/domain"
)

// querier is satisfied by both *pgxpool.Pool and pgx.Tx, letting one adapter
// implementation serve plain and transactional access.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Repo is the pool-backed adapter for domain.ProviderKeyRepository.
type Repo struct{ q querier }

// New wraps the given querier (pool or tx) as a provider key repository.
func New(q querier) *Repo { return &Repo{q: q} }

func (r *Repo) List(ctx context.Context) ([]identity.ProviderKey, error) {
	rows, err := r.q.Query(ctx, `SELECT provider, created_at FROM provider_keys ORDER BY provider`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []identity.ProviderKey{}
	for rows.Next() {
		var k identity.ProviderKey
		if err := rows.Scan(&k.Provider, &k.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// Upsert stores the ciphertext for the provider (upsert on the provider key).
func (r *Repo) Upsert(ctx context.Context, provider identity.Provider, ciphertext []byte) (identity.ProviderKey, error) {
	var k identity.ProviderKey
	err := r.q.QueryRow(ctx, `
		INSERT INTO provider_keys (provider, ciphertext, updated_at)
		VALUES ($1, $2, now())
		ON CONFLICT (provider) DO UPDATE SET ciphertext = EXCLUDED.ciphertext, updated_at = now()
		RETURNING provider, created_at`, provider, ciphertext).Scan(&k.Provider, &k.CreatedAt)
	return k, err
}

func (r *Repo) Delete(ctx context.Context, provider identity.Provider) error {
	tag, err := r.q.Exec(ctx, `DELETE FROM provider_keys WHERE provider = $1`, provider)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// Ciphertext returns the stored ciphertext; the caller (application) decrypts
// it through the KeyCipher port.
func (r *Repo) Ciphertext(ctx context.Context, provider identity.Provider) ([]byte, error) {
	var ct []byte
	err := r.q.QueryRow(ctx, `SELECT ciphertext FROM provider_keys WHERE provider = $1`, provider).Scan(&ct)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return ct, err
}
