package application

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/services/settings/internal/domain"
)

// ── Fakes ───────────────────────────────────────────────────────────────────

// fakeKeys is the in-memory ProviderKeyRepository adapter. It stores only
// what the application hands it (ciphertext), mirroring the pgx adapter.
type fakeKeys struct {
	ct map[identity.Provider][]byte
}

func (f *fakeKeys) List(context.Context) ([]identity.ProviderKey, error) {
	out := []identity.ProviderKey{}
	for p := range f.ct {
		out = append(out, identity.ProviderKey{Provider: p, CreatedAt: time.Now()})
	}
	return out, nil
}
func (f *fakeKeys) Upsert(_ context.Context, provider identity.Provider, ciphertext []byte) (identity.ProviderKey, error) {
	f.ct[provider] = ciphertext
	return identity.ProviderKey{Provider: provider, CreatedAt: time.Now()}, nil
}
func (f *fakeKeys) Delete(_ context.Context, provider identity.Provider) error {
	if _, ok := f.ct[provider]; !ok {
		return domain.ErrNotFound
	}
	delete(f.ct, provider)
	return nil
}
func (f *fakeKeys) Ciphertext(_ context.Context, provider identity.Provider) ([]byte, error) {
	ct, ok := f.ct[provider]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return ct, nil
}

// fakeCipher "encrypts" by prefixing and "decrypts" by stripping the prefix.
// The application must store the cipher output, never the plaintext.
type fakeCipher struct{}

func (fakeCipher) Encrypt(pt []byte) ([]byte, error) { return []byte("ct:" + string(pt)), nil }
func (fakeCipher) Decrypt(ct []byte) ([]byte, error) {
	return []byte(strings.TrimPrefix(string(ct), "ct:")), nil
}

func newTestApp() (*App, *fakeKeys) {
	f := &fakeKeys{ct: map[identity.Provider][]byte{}}
	app := New(&Repository{Keys: f}, fakeCipher{}, slog.New(slog.DiscardHandler))
	return app, f
}

// ── Set / get key (encrypted at rest) ───────────────────────────────────────

func TestSetKeyEncryptsAtRest(t *testing.T) {
	app, f := newTestApp()

	k, err := app.SetKey(context.Background(), identity.Provider("openai"), "sk-secret-123")
	if err != nil {
		t.Fatalf("set key: %v", err)
	}
	if k.Provider != identity.Provider("openai") {
		t.Fatalf("unexpected key row: %+v", k)
	}
	// The stored value must be the cipher output, never the plaintext.
	stored, ok := f.ct[identity.Provider("openai")]
	if !ok {
		t.Fatal("key must be persisted")
	}
	if string(stored) != "ct:sk-secret-123" {
		t.Fatalf("key must be the cipher output, got %q", stored)
	}
}

func TestSetKeyUpserts(t *testing.T) {
	app, f := newTestApp()

	if _, err := app.SetKey(context.Background(), "anthropic", "k1"); err != nil {
		t.Fatalf("first set: %v", err)
	}
	if _, err := app.SetKey(context.Background(), "anthropic", "k2"); err != nil {
		t.Fatalf("second set: %v", err)
	}
	if string(f.ct["anthropic"]) != "ct:k2" {
		t.Fatalf("upsert must replace the ciphertext, got %q", f.ct["anthropic"])
	}
}

// ── Decrypt flow (internal endpoint path) ───────────────────────────────────

func TestPlaintextDecryptFlow(t *testing.T) {
	app, f := newTestApp()

	// Seed the store with ciphertext, as SetKey would have produced.
	f.ct[identity.Provider("gemini")] = []byte("ct:gm-key")

	pt, err := app.Plaintext(context.Background(), identity.Provider("gemini"))
	if err != nil {
		t.Fatalf("plaintext: %v", err)
	}
	if pt != "gm-key" {
		t.Fatalf("expected decrypted key, got %q", pt)
	}
}

func TestPlaintextUnknownProvider(t *testing.T) {
	app, _ := newTestApp()

	_, err := app.Plaintext(context.Background(), "glm")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown provider must surface ErrNotFound, got %v", err)
	}
}

// ── List / delete ───────────────────────────────────────────────────────────

func TestListAndDeleteKeys(t *testing.T) {
	app, f := newTestApp()

	if _, err := app.SetKey(context.Background(), "openai", "k1"); err != nil {
		t.Fatalf("set openai: %v", err)
	}
	if _, err := app.SetKey(context.Background(), "glm", "k2"); err != nil {
		t.Fatalf("set glm: %v", err)
	}

	keys, err := app.ListKeys(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 providers, got %d (%+v)", len(keys), keys)
	}
	for _, k := range keys {
		if k.Provider != identity.Provider("openai") && k.Provider != identity.Provider("glm") {
			t.Fatalf("unexpected key row: %+v", k)
		}
	}

	if err := app.DeleteKey(context.Background(), "openai"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok := f.ct["openai"]; ok {
		t.Fatal("key must be removed")
	}
	if err := app.DeleteKey(context.Background(), "openai"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("deleting a missing key must surface ErrNotFound, got %v", err)
	}
}

// ── Write gating ────────────────────────────────────────────────────────────

func TestAllowWrite(t *testing.T) {
	app, _ := newTestApp()

	if err := app.AllowWrite(true, identity.RoleMember); err != nil {
		t.Fatalf("superadmin must be allowed: %v", err)
	}
	if err := app.AllowWrite(false, identity.RoleOwner); err != nil {
		t.Fatalf("owner must be allowed: %v", err)
	}
	if err := app.AllowWrite(false, identity.RoleAdmin); err != nil {
		t.Fatalf("admin must be allowed: %v", err)
	}
	if err := app.AllowWrite(false, identity.RoleMember); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("member must be rejected, got %v", err)
	}
	if err := app.AllowWrite(false, ""); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("unknown role must be rejected, got %v", err)
	}
}
