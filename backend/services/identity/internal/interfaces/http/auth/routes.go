// Package http exposes the Auth use cases as thin HTTP handlers: decode →
// call application handler → encode. All business rules live in application.
//
// Routes match frontend/src/api/auth.ts: login, me, logout, signup (join/create
// modes), signup-status, resend, and the SSO begin stub.
//
// The Auth service owns users, sessions, and signup requests. Sessions are
// httpOnly cookies (the SPA sends no auth header). Workspace memberships live
// in the Orgs service, so the Gateway assembles the full Session; Auth returns
// the user identity and the Gateway composes memberships.
package http

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/workspaces"
	httputil "github.com/aaks/server/internal/platform/http"
	"github.com/aaks/server/services/identity/internal/application/auth"
	"github.com/aaks/server/services/identity/internal/domain/auth"
)

const (
	sessionCookie = "aaks_session"
	signupCookie  = "aaks_signup"
)

// WorkspaceLister lists a user's workspace union (the orgs plane of this
// service), used to compose the Gateway session in one call.
type WorkspaceLister interface {
	InternalWorkspaces(ctx context.Context, userID identity.ID) ([]workspaces.Workspace, error)
}

// Server wires the Auth routes to the application service.
type Server struct {
	app    *application.App
	log    *slog.Logger
	ssoCfg map[string]string // provider -> redirect url
	ws     WorkspaceLister   // orgs plane (nil = empty workspace union)
}

// New builds the HTTP adapter.
func New(app *application.App, log *slog.Logger, ssoCfg map[string]string, ws WorkspaceLister) *Server {
	return &Server{app: app, log: log, ssoCfg: ssoCfg, ws: ws}
}

// Register mounts all Auth routes on mux.
func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /auth/login", s.login)
	mux.HandleFunc("POST /auth/logout", s.logout)
	mux.HandleFunc("GET /auth/me", s.me)
	mux.HandleFunc("POST /auth/signup", s.signup)
	mux.HandleFunc("GET /auth/signup-status", s.signupStatus)
	mux.HandleFunc("POST /auth/signup-status/resend", s.resend)
	mux.HandleFunc("POST /auth/sso/begin", s.ssoBegin)
	// Internal surface used only by the Gateway (KPIs + identity composition).
	mux.HandleFunc("GET /internal/active-users-24h", s.activeUsers24h)
	mux.HandleFunc("GET /internal/identity", s.identity)
}

// ── Cookie / client helpers ─────────────────────────────────────────────────

// sessionToken extracts the session cookie value.
func sessionToken(r *http.Request) string {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return ""
	}
	return c.Value
}

// setSessionCookie writes the session cookie on the response. Secure is only
// set when the request arrived over TLS (direct or via X-Forwarded-Proto), so
// the cookie never rides plaintext on a downgrade path.
func setSessionCookie(w http.ResponseWriter, r *http.Request, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{ // nosemgrep: go.lang.security.audit.net.cookie-missing-secure.cookie-missing-secure
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecure(r),
		MaxAge:   maxAge,
	})
}

// isSecure reports whether the request arrived over TLS.
func isSecure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func clearCookie(w http.ResponseWriter, r *http.Request, name string) {
	// Mirror the session cookie flags so browsers accept the deletion.
	// Secure is dynamic via isSecure (TLS-or-forwarded-https), which the
	// static rule below cannot see — hence the suppression.
	http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: isSecure(r), MaxAge: -1}) // nosemgrep: go.lang.security.audit.net.cookie-missing-secure.cookie-missing-secure
}

// clientIP extracts the client address, honoring X-Forwarded-For (the gateway
// terminates TLS/connects, so the real client is in the forwarded header).
func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		return strings.TrimSpace(strings.Split(fwd, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ── Sessions ────────────────────────────────────────────────────────────────

// login validates credentials and issues a session cookie. Returns the user
// identity; the Gateway assembles the full Session (auth + memberships).
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if s.app.LoginGate(ip) {
		httputil.Error(w, http.StatusTooManyRequests, "too many login attempts, try again later")
		return
	}
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Remember *bool  `json:"remember,omitempty"`
	}
	if httputil.Decode(w, r, &body) {
		return
	}
	if body.Email == "" || body.Password == "" {
		httputil.Error(w, http.StatusBadRequest, "email and password are required")
		return
	}
	res, err := s.app.Login(r.Context(), body.Email, body.Password, ip, body.Remember != nil && *body.Remember)
	switch {
	case errors.Is(err, domain.ErrThrottled):
		httputil.Error(w, http.StatusTooManyRequests, "too many login attempts, try again later")
	case errors.Is(err, domain.ErrBadPassword):
		// Generic 401 without leaking which field was wrong.
		httputil.Error(w, http.StatusUnauthorized, "invalid email or password")
	case errors.Is(err, domain.ErrPending):
		// Unapproved signup: 403 with a pending state so the SPA routes to the
		// awaiting-approval screen (spec: unapproved user scenario).
		httputil.WriteJSON(w, http.StatusForbidden, map[string]any{
			"error": "account awaiting approval", "state": "pending",
		})
	case err != nil:
		httputil.ServerError(w, s.log, "auth.Login", err)
	default:
		setSessionCookie(w, r, res.Token, res.MaxAge)
		httputil.WriteJSON(w, http.StatusOK, res.User)
	}
}

// logout invalidates the session and clears the cookie.
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if token := sessionToken(r); token != "" {
		_ = s.app.Logout(r.Context(), token)
	}
	clearCookie(w, r, sessionCookie)
	w.WriteHeader(http.StatusNoContent)
}

// me returns the session user or 401.
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	u, err := s.currentUser(r)
	if err != nil {
		httputil.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, u.User)
}

// identity resolves the session cookie to a lightweight identity for the
// Gateway's scoping headers (internal-only).
func (s *Server) identity(w http.ResponseWriter, r *http.Request) {
	u, err := s.currentUser(r)
	if err != nil {
		httputil.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	// One call composes the full Gateway session: user identity + workspace
	// union (the former Auth→Orgs fan-out is a local read in this service).
	// A failed union is non-fatal (valid session, empty union) — but it is
	// logged: a silently-empty union looks exactly like "user lost access to
	// everything" from the outside.
	wss := []workspaces.Workspace{}
	if s.ws != nil {
		if list, err := s.ws.InternalWorkspaces(r.Context(), u.ID); err == nil && list != nil {
			wss = list
		} else if err != nil {
			s.log.Error("identity composition: workspace union failed", "user_id", u.ID, "error", err)
		}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"user_id":       u.ID,
		"name":          u.Name,
		"email":         u.Email,
		"is_superadmin": u.IsSuperadmin != nil && *u.IsSuperadmin,
		"workspaces":    wss,
	})
}

// currentUser resolves the session user from the cookie.
func (s *Server) currentUser(r *http.Request) (domain.User, error) {
	token := sessionToken(r)
	if token == "" {
		return domain.User{}, domain.ErrNotFound
	}
	return s.app.SessionUser(r.Context(), token)
}

// ── Signup ──────────────────────────────────────────────────────────────────

// signup records a pending access request (join or create mode), sets the
// signup cookie for status polling, and emits signup.requested.
func (s *Server) signup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		FullName         string `json:"full_name"`
		Email            string `json:"email"`
		Password         string `json:"password"`
		StartMode        string `json:"start_mode"`
		InviteCode       string `json:"invite_code,omitempty"`
		OrganizationName string `json:"organization_name,omitempty"`
	}
	if httputil.Decode(w, r, &body) {
		return
	}
	reqID, err := s.app.Signup(r.Context(), application.SignupInput{
		FullName: body.FullName, Email: body.Email, Password: body.Password,
		StartMode: body.StartMode, InviteCode: body.InviteCode, OrganizationName: body.OrganizationName,
	})
	switch {
	case errors.Is(err, application.ErrFieldsRequired):
		httputil.Error(w, http.StatusBadRequest, "full_name, email and password are required")
	case errors.Is(err, application.ErrPasswordTooShort):
		httputil.Error(w, http.StatusBadRequest, "password must be at least 8 characters")
	case errors.Is(err, application.ErrStartMode):
		httputil.Error(w, http.StatusBadRequest, "start_mode must be 'join' or 'create'")
	case errors.Is(err, application.ErrInviteCodeRequired):
		httputil.Error(w, http.StatusBadRequest, "invite_code is required for join mode")
	case errors.Is(err, application.ErrUnknownInviteCode):
		httputil.Error(w, http.StatusBadRequest, "unknown invite code")
	case errors.Is(err, application.ErrOrganizationRequired):
		httputil.Error(w, http.StatusBadRequest, "organization_name is required for create mode")
	case errors.Is(err, domain.ErrEmailTaken):
		httputil.Error(w, http.StatusBadRequest, "an account with this email already exists")
	case err != nil:
		httputil.ServerError(w, s.log, "auth.Signup", err)
	default:
		clearCookie(w, r, sessionCookie)
		http.SetCookie(w, &http.Cookie{Name: signupCookie, Value: reqID, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: isSecure(r), MaxAge: 7 * 86400}) // nosemgrep: go.lang.security.audit.net.cookie-missing-secure.cookie-missing-secure
		httputil.WriteJSON(w, http.StatusCreated, map[string]any{"request_id": reqID})
	}
}

// signupStatus returns the pending request state for the signup cookie.
func (s *Server) signupStatus(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(signupCookie)
	if err != nil || c.Value == "" {
		// Fall back to the most recent request for the email, if a cookie is absent.
		if email := r.URL.Query().Get("email"); email != "" {
			req, err := s.app.SignupStatusByEmail(r.Context(), email)
			if err != nil {
				httputil.Error(w, http.StatusNotFound, "no signup request found")
				return
			}
			s.writeSignupStatus(w, req)
			return
		}
		httputil.Error(w, http.StatusNotFound, "no signup request found")
		return
	}
	req, err := s.app.SignupStatus(r.Context(), identity.ID(c.Value))
	if err != nil {
		httputil.Error(w, http.StatusNotFound, "no signup request found")
		return
	}
	s.writeSignupStatus(w, req)
}

func (s *Server) writeSignupStatus(w http.ResponseWriter, req domain.SignupRequest) {
	out := map[string]any{
		"state": req.Status, "email": req.Email,
	}
	if req.WorkspaceName != "" {
		out["workspace_name"] = req.WorkspaceName
	}
	if req.Status == identity.SignupApproved {
		out["admin_name"] = "Admin"
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

// resend is idempotent (spec): notifications are out of scope for the MVP.
func (s *Server) resend(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(signupCookie)
	if err != nil || c.Value == "" {
		httputil.Error(w, http.StatusNotFound, "no signup request found")
		return
	}
	if _, err := s.app.SignupStatus(r.Context(), identity.ID(c.Value)); err != nil {
		httputil.Error(w, http.StatusNotFound, "no signup request found")
		return
	}
	s.log.Info("signup notification resent (stub)", "request_id", c.Value)
	w.WriteHeader(http.StatusNoContent)
}

// ── SSO (stub) ──────────────────────────────────────────────────────────────

// ssoBegin returns a configured redirect_url or a documented error.
func (s *Server) ssoBegin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Provider string `json:"provider"`
	}
	if httputil.Decode(w, r, &body) {
		return
	}
	url, ok := s.ssoCfg[strings.ToLower(body.Provider)]
	if !ok || url == "" {
		httputil.Error(w, http.StatusBadRequest, "sso provider is not configured")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"redirect_url": url})
}

// ── Internal (Gateway composition) ──────────────────────────────────────────

// activeUsers24h serves the Gateway's KPI composition.
func (s *Server) activeUsers24h(w http.ResponseWriter, r *http.Request) {
	n, err := s.app.ActiveUsers24h(r.Context())
	if err != nil {
		httputil.ServerError(w, s.log, "auth.ActiveUsers", err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]int{"active_users_24h": n})
}
