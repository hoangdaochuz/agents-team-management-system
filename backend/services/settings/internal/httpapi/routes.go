// Package httpapi registers the Settings service routes, matching
// frontend/src/api/providerKeys.ts (list/set/update/delete), plus the internal
// decrypt endpoint that is the ONLY path that returns plaintext keys — mTLS
// (client cert signed by deploy/certs CA) + a service token check.
package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"

	"github.com/aaks/server/internal/contracts"
	"github.com/aaks/server/internal/platform/http"
	"github.com/aaks/server/internal/platform/tenancy"
	"github.com/aaks/server/services/settings/internal/store"
)

// App holds the Settings service dependencies.
type App struct {
	store *store.Store
	log   *slog.Logger
	// token gates the internal decrypt endpoint (shared with the Runner).
	token string
}

// Register wires settings routes. SETTINGS_MASTER_KEY is required; the decrypt
// endpoint additionally requires the shared SETTINGS_INTERNAL_TOKEN.
func Register(mux *http.ServeMux, log *slog.Logger) error {
	dsn := os.Getenv("SETTINGS_DB_DSN")
	if dsn == "" {
		return errors.New("SETTINGS_DB_DSN is not set")
	}
	st, err := store.New(context.Background(), dsn, log)
	if err != nil {
		return err
	}
	app := &App{store: st, log: log, token: os.Getenv("SETTINGS_INTERNAL_TOKEN")}

	mux.HandleFunc("GET /provider-keys", app.listKeys)
	mux.HandleFunc("POST /provider-keys", app.setKey)
	mux.HandleFunc("PUT /provider-keys/{provider}", app.updateKey)
	mux.HandleFunc("DELETE /provider-keys/{provider}", app.deleteKey)

	// Internal surface: the runner fetches plaintext per run over mTLS + token.
	mux.HandleFunc("GET /internal/keys/{provider}", app.internalPlaintext)

	log.Info("settings routes registered", "endpoints", 5, "mtls", os.Getenv("SETTINGS_MTLS") == "on")
	return nil
}

// ── Handlers ────────────────────────────────────────────────────────────────

func (a *App) listKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := a.store.ListKeys(r.Context())
	if err != nil {
		httputil.ServerError(w, a.log, "settings.ListKeys", err)
		return
	}
	out := make([]contracts.ProviderKey, 0, len(keys))
	for _, k := range keys {
		out = append(out, contracts.ProviderKey{Provider: k.Provider, CreatedAt: k.CreatedAt})
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

func (a *App) setKey(w http.ResponseWriter, r *http.Request) {
	if !a.allowWrite(w, r) {
		return
	}
	var body struct {
		Provider string `json:"provider"`
		APIKey   string `json:"api_key"`
	}
	if httputil.Decode(w, r, &body) {
		return
	}
	if !validProvider(body.Provider) || body.APIKey == "" {
		httputil.Error(w, http.StatusBadRequest, "valid provider and api_key are required")
		return
	}
	k, err := a.store.UpsertKey(r.Context(), contracts.Provider(body.Provider), body.APIKey)
	if err != nil {
		httputil.ServerError(w, a.log, "settings.SetKey", err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, contracts.ProviderKey{Provider: k.Provider, CreatedAt: k.CreatedAt})
}

func (a *App) updateKey(w http.ResponseWriter, r *http.Request) {
	if !a.allowWrite(w, r) {
		return
	}
	var body struct {
		APIKey string `json:"api_key"`
	}
	if httputil.Decode(w, r, &body) {
		return
	}
	provider := r.PathValue("provider")
	if !validProvider(provider) || body.APIKey == "" {
		httputil.Error(w, http.StatusBadRequest, "valid provider and api_key are required")
		return
	}
	k, err := a.store.UpsertKey(r.Context(), contracts.Provider(provider), body.APIKey)
	if err != nil {
		httputil.ServerError(w, a.log, "settings.UpdateKey", err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, contracts.ProviderKey{Provider: k.Provider, CreatedAt: k.CreatedAt})
}

func (a *App) deleteKey(w http.ResponseWriter, r *http.Request) {
	if !a.allowWrite(w, r) {
		return
	}
	provider := r.PathValue("provider")
	if !validProvider(provider) {
		httputil.Error(w, http.StatusBadRequest, "invalid provider")
		return
	}
	err := a.store.DeleteKey(r.Context(), contracts.Provider(provider))
	if errors.Is(err, store.ErrNotFound) {
		httputil.Error(w, http.StatusNotFound, "key not found")
		return
	}
	if err != nil {
		httputil.ServerError(w, a.log, "settings.DeleteKey", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// internalPlaintext returns the plaintext key. Gate: X-Settings-Token must
// match SETTINGS_INTERNAL_TOKEN (the runner holds it; dev compose shares it).
// When mTLS is enabled (SETTINGS_MTLS=on) the TLS peer must also present a
// client cert signed by the dev CA (handled by the listener in cmd).
func (a *App) internalPlaintext(w http.ResponseWriter, r *http.Request) {
	if a.token == "" || r.Header.Get("X-Settings-Token") != a.token {
		httputil.Error(w, http.StatusForbidden, "forbidden")
		return
	}
	provider := r.PathValue("provider")
	if !validProvider(provider) {
		httputil.Error(w, http.StatusBadRequest, "invalid provider")
		return
	}
	pt, err := a.store.GetPlaintext(r.Context(), contracts.Provider(provider))
	if errors.Is(err, store.ErrNotFound) {
		httputil.Error(w, http.StatusNotFound, "no key for provider")
		return
	}
	if err != nil {
		httputil.ServerError(w, a.log, "settings.Plaintext", err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"provider": provider, "api_key": pt})
}

func validProvider(p string) bool {
	switch p {
	case "openai", "anthropic", "gemini", "glm":
		return true
	}
	return false
}

// allowWrite gates the provider-key write path. Keys are a single global row
// set scoped by neither workspace nor org, so only superadmins (or an
// owner/admin of any workspace) may set/update/delete them — otherwise any
// authenticated user could overwrite or delete the deployment's provider keys.
func (a *App) allowWrite(w http.ResponseWriter, r *http.Request) bool {
	if tenancy.UserSuperadmin(r) {
		return true
	}
	switch contracts.Role(tenancy.UserRole(r)) {
	case contracts.RoleOwner, contracts.RoleAdmin:
		return true
	}
	httputil.Error(w, http.StatusForbidden, "superadmin or workspace owner/admin required")
	return false
}

