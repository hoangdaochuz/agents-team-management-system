// Package repository implements the Settings domain repository port on Postgres
// (Ports & Adapters: the adapter side of the hexagon). Provider keys are
// stored as opaque ciphertext; the crypto port lives in infrastructure/crypto.
package repository

import (
	"context"
	"embed"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/platform/db"
	"github.com/aaks/server/services/settings/internal/domain"
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

// Store owns the settings Postgres pool and exposes the pool-backed adapter
// for the domain port. It is the composition-root entrypoint of this package.
type Store struct {
	pool *pgxpool.Pool
	log  *slog.Logger
	Keys domain.ProviderKeyRepository
}

// New opens the settings database and runs migrations.
func New(ctx context.Context, dsn string, log *slog.Logger) (*Store, error) {
	pool, err := db.Pool(ctx, dsn, log)
	if err != nil {
		return nil, err
	}
	if err := db.Migrate(ctx, pool, migrations, "migrations", log); err != nil {
		return nil, err
	}
	s := &Store{pool: pool, log: log}
	s.Keys = &keyRepo{q: pool}
	return s, nil
}

// Close releases the connection pool.
func (s *Store) Close() { s.pool.Close() }

// ── Provider keys ───────────────────────────────────────────────────────────

type keyRepo struct{ q querier }

func (r *keyRepo) List(ctx context.Context) ([]identity.ProviderKey, error) {
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
func (r *keyRepo) Upsert(ctx context.Context, provider identity.Provider, ciphertext []byte) (identity.ProviderKey, error) {
	var k identity.ProviderKey
	err := r.q.QueryRow(ctx, `
		INSERT INTO provider_keys (provider, ciphertext, updated_at)
		VALUES ($1, $2, now())
		ON CONFLICT (provider) DO UPDATE SET ciphertext = EXCLUDED.ciphertext, updated_at = now()
		RETURNING provider, created_at`, provider, ciphertext).Scan(&k.Provider, &k.CreatedAt)
	return k, err
}

func (r *keyRepo) Delete(ctx context.Context, provider identity.Provider) error {
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
func (r *keyRepo) Ciphertext(ctx context.Context, provider identity.Provider) ([]byte, error) {
	var ct []byte
	err := r.q.QueryRow(ctx, `SELECT ciphertext FROM provider_keys WHERE provider = $1`, provider).Scan(&ct)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return ct, err
}
