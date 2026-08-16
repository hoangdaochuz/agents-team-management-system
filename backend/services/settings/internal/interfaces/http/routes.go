// Package http exposes the Settings use cases as thin HTTP handlers: decode →
// call application handler → encode. The internal decrypt endpoint is the ONLY
// path that returns plaintext keys — gated by the shared service token (the
// Runner holds it; mTLS, when enabled, is enforced by the listener).
package http

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/aaks/server/internal/contracts/identity"
	httputil "github.com/aaks/server/internal/platform/http"
	"github.com/aaks/server/internal/platform/tenancy"
	"github.com/aaks/server/services/settings/internal/application"
	"github.com/aaks/server/services/settings/internal/domain"
)

// Server wires the Settings routes to the application service.
type Server struct {
	app *application.App
	log *slog.Logger
	// token gates the internal decrypt endpoint (shared with the Runner).
	token string
}

// New builds the HTTP adapter. token is SETTINGS_INTERNAL_TOKEN; the decrypt
// endpoint rejects requests when it is unset or mismatched.
func New(app *application.App, log *slog.Logger, token string) *Server {
	return &Server{app: app, log: log, token: token}
}

// Register mounts all Settings routes on mux, matching
// frontend/src/api/providerKeys.ts (list/set/update/delete) plus the internal
// decrypt endpoint.
func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /provider-keys", s.listKeys)
	mux.HandleFunc("POST /provider-keys", s.setKey)
	mux.HandleFunc("PUT /provider-keys/{provider}", s.updateKey)
	mux.HandleFunc("DELETE /provider-keys/{provider}", s.deleteKey)

	// Internal surface: the runner fetches plaintext per run over mTLS + token.
	mux.HandleFunc("GET /internal/keys/{provider}", s.internalPlaintext)
}

func (s *Server) listKeys(w http.ResponseWriter, r *http.Request) {
	out, err := s.app.ListKeys(r.Context())
	if err != nil {
		httputil.ServerError(w, s.log, "settings.ListKeys", err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

func (s *Server) setKey(w http.ResponseWriter, r *http.Request) {
	if !s.allowWrite(w, r) {
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
	k, err := s.app.SetKey(r.Context(), identity.Provider(body.Provider), body.APIKey)
	if err != nil {
		httputil.ServerError(w, s.log, "settings.SetKey", err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, k)
}

func (s *Server) updateKey(w http.ResponseWriter, r *http.Request) {
	if !s.allowWrite(w, r) {
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
	k, err := s.app.SetKey(r.Context(), identity.Provider(provider), body.APIKey)
	if err != nil {
		httputil.ServerError(w, s.log, "settings.UpdateKey", err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, k)
}

func (s *Server) deleteKey(w http.ResponseWriter, r *http.Request) {
	if !s.allowWrite(w, r) {
		return
	}
	provider := r.PathValue("provider")
	if !validProvider(provider) {
		httputil.Error(w, http.StatusBadRequest, "invalid provider")
		return
	}
	err := s.app.DeleteKey(r.Context(), identity.Provider(provider))
	if errors.Is(err, domain.ErrNotFound) {
		httputil.Error(w, http.StatusNotFound, "key not found")
		return
	}
	if err != nil {
		httputil.ServerError(w, s.log, "settings.DeleteKey", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// internalPlaintext returns the plaintext key. Gate: X-Settings-Token must
// match SETTINGS_INTERNAL_TOKEN (the runner holds it; dev compose shares it).
// When mTLS is enabled (SETTINGS_MTLS=on) the TLS peer must also present a
// client cert signed by the dev CA (handled by the listener in cmd).
func (s *Server) internalPlaintext(w http.ResponseWriter, r *http.Request) {
	if s.token == "" || r.Header.Get("X-Settings-Token") != s.token {
		httputil.Error(w, http.StatusForbidden, "forbidden")
		return
	}
	provider := r.PathValue("provider")
	if !validProvider(provider) {
		httputil.Error(w, http.StatusBadRequest, "invalid provider")
		return
	}
	pt, err := s.app.Plaintext(r.Context(), identity.Provider(provider))
	if errors.Is(err, domain.ErrNotFound) {
		httputil.Error(w, http.StatusNotFound, "no key for provider")
		return
	}
	if err != nil {
		httputil.ServerError(w, s.log, "settings.Plaintext", err)
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

// allowWrite gates the provider-key write path via the application rule
// (superadmin or workspace owner/admin); the tenancy headers are extracted
// here, at the HTTP boundary.
func (s *Server) allowWrite(w http.ResponseWriter, r *http.Request) bool {
	if err := s.app.AllowWrite(tenancy.UserSuperadmin(r), identity.Role(tenancy.UserRole(r))); err != nil {
		httputil.Error(w, http.StatusForbidden, err.Error())
		return false
	}
	return true
}
