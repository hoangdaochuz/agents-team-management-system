// Package store is the Settings service persistence layer: provider keys
// stored as AES-GCM ciphertext. The master key lives in the process (env), so
// a DB dump leaks nothing usable.
package store

import (
	"context"
	"crypto/aes"
	"crypto/sha256"
	"crypto/cipher"
	"crypto/rand"
	"embed"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aaks/server/internal/contracts"
	"github.com/aaks/server/internal/platform/db"
)

//go:embed migrations/*.sql
var migrations embed.FS

var (
	ErrNotFound  = errors.New("not found")
	ErrNoKey     = errors.New("SETTINGS_MASTER_KEY not set")
)

// Store owns Settings persistence.
type Store struct {
	pool *pgxpool.Pool
	aead cipher.AEAD
}

// New opens the pool, migrates, and loads the master key (32 bytes, hex or raw).
func New(ctx context.Context, dsn string, log *slog.Logger) (*Store, error) {
	pool, err := db.Pool(ctx, dsn, log)
	if err != nil {
		return nil, err
	}
	if err := db.MigrateFS(ctx, pool, migrations, "migrations", log); err != nil {
		return nil, err
	}
	key := os.Getenv("SETTINGS_MASTER_KEY")
	if key == "" {
		return nil, ErrNoKey
	}
	if len(key) < 16 {
		return nil, fmt.Errorf("SETTINGS_MASTER_KEY must be at least 16 bytes")
	}
	// Derive a fixed 32-byte AES key from the master key (SHA-256), so any
	// passphrase length works and the key is stable across restarts.
	sum := sha256.Sum256([]byte(key))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, err
	}
	return &Store{pool: pool, aead: newGCM(block)}, nil
}

func newGCM(block cipher.Block) cipher.AEAD {
	aead, err := cipher.NewGCM(block)
	if err != nil {
		panic("settings: gcm: " + err.Error())
	}
	return aead
}

func (s *Store) Close() { s.pool.Close() }

// KeyRow is a provider key's public view.
type KeyRow struct {
	Provider  contracts.Provider
	CreatedAt contracts.ISOTime
}

// ListKeys returns the providers that have a key (public view only).
func (s *Store) ListKeys(ctx context.Context) ([]KeyRow, error) {
	rows, err := s.pool.Query(ctx, `SELECT provider, created_at FROM provider_keys ORDER BY provider`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []KeyRow{}
	for rows.Next() {
		var k KeyRow
		if err := rows.Scan(&k.Provider, &k.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// UpsertKey encrypts apiKey and stores it for the provider.
func (s *Store) UpsertKey(ctx context.Context, provider contracts.Provider, apiKey string) (KeyRow, error) {
	ct, err := s.encrypt([]byte(apiKey))
	if err != nil {
		return KeyRow{}, err
	}
	var k KeyRow
	err = s.pool.QueryRow(ctx, `
		INSERT INTO provider_keys (provider, ciphertext, updated_at)
		VALUES ($1, $2, now())
		ON CONFLICT (provider) DO UPDATE SET ciphertext = EXCLUDED.ciphertext, updated_at = now()
		RETURNING provider, created_at`, provider, ct).Scan(&k.Provider, &k.CreatedAt)
	return k, err
}

// DeleteKey removes a provider key.
func (s *Store) DeleteKey(ctx context.Context, provider contracts.Provider) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM provider_keys WHERE provider = $1`, provider)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// GetPlaintext returns the decrypted key (runner mTLS path only).
func (s *Store) GetPlaintext(ctx context.Context, provider contracts.Provider) (string, error) {
	var ct []byte
	err := s.pool.QueryRow(ctx, `SELECT ciphertext FROM provider_keys WHERE provider = $1`, provider).Scan(&ct)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	pt, err := s.decrypt(ct)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

// encrypt seals plaintext as nonce|ciphertext (AES-GCM).
func (s *Store) encrypt(pt []byte) ([]byte, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return s.aead.Seal(nonce, nonce, pt, nil), nil
}

func (s *Store) decrypt(ct []byte) ([]byte, error) {
	if len(ct) < s.aead.NonceSize() {
		return nil, errors.New("settings: ciphertext too short")
	}
	nonce, body := ct[:s.aead.NonceSize()], ct[s.aead.NonceSize():]
	return s.aead.Open(nil, nonce, body, nil)
}

