package application

import (
	"context"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/services/settings/internal/domain"
)

// ListKeys returns the providers that have a key (public view only — the
// ciphertext never leaves the service).
func (a *App) ListKeys(ctx context.Context) ([]identity.ProviderKey, error) {
	keys, err := a.repo.Keys.List(ctx)
	if err != nil {
		return nil, err
	}
	if keys == nil {
		keys = []identity.ProviderKey{}
	}
	return keys, nil
}

// SetKey encrypts apiKey with the master key and stores the ciphertext
// (upsert). The plaintext exists only in memory for the duration of the call.
func (a *App) SetKey(ctx context.Context, provider identity.Provider, apiKey string) (identity.ProviderKey, error) {
	ct, err := a.cipher.Encrypt([]byte(apiKey))
	if err != nil {
		return identity.ProviderKey{}, err
	}
	return a.repo.Keys.Upsert(ctx, provider, ct)
}

// DeleteKey removes a provider key.
func (a *App) DeleteKey(ctx context.Context, provider identity.Provider) error {
	return a.repo.Keys.Delete(ctx, provider)
}

// Plaintext returns the decrypted key. This is the ONLY path that yields
// plaintext; the interface layer gates it with the shared service token (the
// Runner's mTLS + token channel).
func (a *App) Plaintext(ctx context.Context, provider identity.Provider) (string, error) {
	ct, err := a.repo.Keys.Ciphertext(ctx, provider)
	if err != nil {
		return "", err
	}
	pt, err := a.cipher.Decrypt(ct)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

// AllowWrite gates the provider-key write path. Keys are a single global row
// set scoped by neither workspace nor org, so only superadmins (or an
// owner/admin of any workspace) may set/update/delete them — otherwise any
// authenticated user could overwrite or delete the deployment's provider keys.
func (a *App) AllowWrite(superadmin bool, role identity.Role) error {
	if superadmin {
		return nil
	}
	switch role {
	case identity.RoleOwner, identity.RoleAdmin:
		return nil
	}
	return domain.ErrForbidden
}
