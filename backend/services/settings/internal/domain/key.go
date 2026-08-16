package domain

import (
	"context"

	"github.com/aaks/server/internal/contracts/identity"
)

// ProviderKey is the provider-key aggregate's public view (wire DTO as domain
// type, D7). The API key itself never appears here — only ciphertext.
type ProviderKey = identity.ProviderKey

// ProviderKeyRepository is the provider-key aggregate port. It stores and
// returns ciphertext only; encryption/decryption lives behind the KeyCipher
// port in the application layer.
type ProviderKeyRepository interface {
	List(ctx context.Context) ([]ProviderKey, error)
	Upsert(ctx context.Context, provider identity.Provider, ciphertext []byte) (ProviderKey, error)
	Delete(ctx context.Context, provider identity.Provider) error
	Ciphertext(ctx context.Context, provider identity.Provider) ([]byte, error)
}