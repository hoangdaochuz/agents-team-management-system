// Package application holds the Settings use-case handlers. It depends only on
// domain ports plus the abstractions declared here (DIP: no pgx, no AES-GCM,
// no net/http). The plaintext API key crosses the application boundary only as
// a function argument and is never persisted.
package application

import (
	"log/slog"

	"github.com/aaks/server/services/settings/internal/domain"
)

// KeyCipher encrypts and decrypts provider keys (DIP: the AES-GCM machinery
// lives in infrastructure; the application depends on the port).
type KeyCipher interface {
	Encrypt(plaintext []byte) ([]byte, error)
	Decrypt(ciphertext []byte) ([]byte, error)
}

// Repository is the store of aggregate ports (plain pool).
type Repository struct {
	Keys domain.ProviderKeyRepository
}

// App is the Settings application service: the composition root injects the
// concrete repository and the key cipher.
type App struct {
	repo   *Repository
	cipher KeyCipher
	log    *slog.Logger
}

// New builds the application service with its injected dependencies.
func New(repo *Repository, cipher KeyCipher, log *slog.Logger) *App {
	return &App{repo: repo, cipher: cipher, log: log}
}