// Package httpapi registers the Auth (identity) service routes, matching
// frontend/src/api/auth.ts: login, me, logout, signup (join/create modes),
// signup-status, resend, and the SSO begin stub.
//
// The Auth service owns users, sessions, and signup requests. Sessions are
// httpOnly cookies (the SPA sends no auth header). Workspace memberships live
// in the Orgs service, so the Gateway assembles the full Session; Auth returns
// the user identity and the Gateway composes memberships.
//
// Consumers: signup.approved/declined (approval transitions), invite.created
// (join-mode invite-code projection).
package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/IBM/sarama"
	"golang.org/x/crypto/bcrypt"

	"github.com/aaks/server/internal/contracts"
	"github.com/aaks/server/internal/httputil"
	"github.com/aaks/server/internal/kafka"
	"github.com/aaks/server/services/auth/internal/store"
)

const (
	sessionCookie = "aaks_session"
	signupCookie  = "aaks_signup"
)

// App holds the Auth service dependencies.
type App struct {
	store  *store.Store
	prod   sarama.SyncProducer
	log    *slog.Logger
	ssoCfg map[string]string // provider -> redirect url
}

// Register wires auth routes + the signup/invite Kafka consumers.
func Register(mux *http.ServeMux, log *slog.Logger) error {
	dsn := os.Getenv("AUTH_DB_DSN")
	if dsn == "" {
		return errors.New("AUTH_DB_DSN is not set")
	}
	st, err := store.New(context.Background(), dsn, log)
	if err != nil {
		return err
	}
	app := &App{store: st, log: log, ssoCfg: map[string]string{
		"google": os.Getenv("SSO_GOOGLE_REDIRECT_URL"),
		"saml":   os.Getenv("SSO_SAML_REDIRECT_URL"),
	}}
	if brokers := os.Getenv("KAFKA_BROKERS"); brokers != "" {
		if p, err := kafka.NewProducer(kafka.Brokers(strings.Split(brokers, ",")), log); err != nil {
			log.Warn("kafka producer unavailable; auth emits no signup events", "error", err)
		} else {
			app.prod = p
		}
	}

	if email, pass := os.Getenv("AUTH_SEED_SUPERADMIN_EMAIL"), os.Getenv("AUTH_SEED_SUPERADMIN_PASSWORD"); email != "" && pass != "" {
		app.seedSuperadmin(email, pass)
	}

	mux.HandleFunc("POST /auth/login", app.login)
	mux.HandleFunc("POST /auth/logout", app.logout)
	mux.HandleFunc("GET /auth/me", app.me)
	mux.HandleFunc("POST /auth/signup", app.signup)
	mux.HandleFunc("GET /auth/signup-status", app.signupStatus)
	mux.HandleFunc("POST /auth/signup-status/resend", app.resend)
	mux.HandleFunc("POST /auth/sso/begin", app.ssoBegin)
	// Internal surface used only by the Gateway (KPIs + identity composition).
	mux.HandleFunc("GET /internal/active-users-24h", app.activeUsers24h)
	mux.HandleFunc("GET /internal/identity", app.identity)

	app.startConsumers()

	log.Info("auth routes registered", "endpoints", 9)
	return nil
}

// seedSuperadmin creates the configured superadmin if it does not exist.
func (a *App) seedSuperadmin(email, password string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := a.store.GetUserByEmail(ctx, email); err == nil {
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		a.log.Error("seed superadmin: hash failed", "error", err)
		return
	}
	if _, err := a.store.CreateUser(ctx, "Superadmin", email, string(hash), true); err != nil {
		a.log.Error("seed superadmin: create failed", "error", err)
		return
	}
	if err := a.store.ActivateUserByEmail(ctx, email); err != nil {
		a.log.Error("seed superadmin: activate failed", "error", err)
		return
	}
	a.log.Info("seeded superadmin", "email", email)
}

// startConsumers subscribes to signup approval + invite events (best-effort).
func (a *App) startConsumers() {
	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers == "" {
		return
	}
	bs := kafka.Brokers(strings.Split(brokers, ","))
	cg, err := kafka.NewConsumerGroup(bs, "auth-signup", a.log)
	if err != nil {
		a.log.Warn("auth consumers unavailable", "error", err)
		return
	}
	topics := []string{contracts.TopicSignupApproved, contracts.TopicSignupDeclined, contracts.TopicInviteCreated}
	go func() {
		if err := cg.Run(context.Background(), topics, a.consume); err != nil {
			a.log.Error("auth consumer stopped", "error", err)
		}
	}()
}

// consume dispatches signup approval/decline + invite projection events.
func (a *App) consume(ctx context.Context, env contracts.EventEnvelope) error {
	switch env.EventType {
	case contracts.TopicSignupApproved:
		var d contracts.SignupApprovedData
		if err := env.DecodeData(&d); err != nil {
			return err
		}
		if err := a.store.SetSignupStatus(ctx, d.RequestID, contracts.SignupApproved); err != nil {
			return err
		}
		if d.UserID != "" {
			return a.store.ActivateUser(ctx, d.UserID)
		}
		return a.store.ActivateUserByEmail(ctx, d.Email)
	case contracts.TopicSignupDeclined:
		var d contracts.SignupDeclinedData
		if err := env.DecodeData(&d); err != nil {
			return err
		}
		return a.store.SetSignupStatus(ctx, d.RequestID, contracts.SignupDeclined)
	case contracts.TopicInviteCreated:
		var d contracts.InviteCreatedData
		if err := env.DecodeData(&d); err != nil {
			return err
		}
		return a.store.UpsertInviteCode(ctx, d.InviteCode, d.Email, d.Role, d.WorkspaceID, d.WorkspaceName)
	}
	return nil
}

// ── Handlers ────────────────────────────────────────────────────────────────

// sessionToken extracts the session cookie value.
func sessionToken(r *http.Request) string {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return ""
	}
	return c.Value
}

// setSessionCookie writes the session cookie on the response.
func setSessionCookie(w http.ResponseWriter, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	})
}

func clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", HttpOnly: true, MaxAge: -1})
}

// login validates credentials and issues a session cookie. Returns the user
// identity; the Gateway assembles the full Session (auth + memberships).
func (a *App) login(w http.ResponseWriter, r *http.Request) {
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
	u, err := a.store.GetUserByEmail(r.Context(), body.Email)
	if err != nil {
		a.failLogin(w)
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(body.Password)) != nil {
		a.failLogin(w)
		return
	}
	if !u.Active {
		// Unapproved signup: 403 with a pending state so the SPA routes to the
		// awaiting-approval screen (spec: unapproved user scenario).
		httputil.WriteJSON(w, http.StatusForbidden, map[string]any{
			"error": "account awaiting approval", "state": "pending",
		})
		return
	}
	ttl := 24 * time.Hour
	maxAge := 86400
	if body.Remember != nil && *body.Remember {
		ttl = 30 * 24 * time.Hour
		maxAge = 30 * 86400
	}
	token, err := a.store.CreateSession(r.Context(), u.ID, ttl)
	if err != nil {
		httputil.ServerError(w, a.log, "auth.Login", err)
		return
	}
	setSessionCookie(w, token, maxAge)
	httputil.WriteJSON(w, http.StatusOK, u.User)
}

// failLogin returns a generic 401 without leaking which field was wrong.
func (a *App) failLogin(w http.ResponseWriter) {
	httputil.Error(w, http.StatusUnauthorized, "invalid email or password")
}

// me returns the session user or 401.
func (a *App) me(w http.ResponseWriter, r *http.Request) {
	u, err := a.currentUser(r)
	if err != nil {
		httputil.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, u.User)
}

// currentUser resolves the session user from the cookie.
func (a *App) currentUser(r *http.Request) (store.UserRow, error) {
	token := sessionToken(r)
	if token == "" {
		return store.UserRow{}, store.ErrNotFound
	}
	return a.store.SessionUser(r.Context(), token)
}

// identity resolves the session cookie to a lightweight identity for the
// Gateway's scoping headers (internal-only).
func (a *App) identity(w http.ResponseWriter, r *http.Request) {
	u, err := a.currentUser(r)
	if err != nil {
		httputil.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"user_id":       u.ID,
		"name":          u.Name,
		"email":         u.Email,
		"is_superadmin": u.IsSuperadmin != nil && *u.IsSuperadmin,
	})
}

// logout invalidates the session and clears the cookie.
func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	if token := sessionToken(r); token != "" {
		_ = a.store.DeleteSession(r.Context(), token)
	}
	clearCookie(w, sessionCookie)
	w.WriteHeader(http.StatusNoContent)
}

// signup records a pending access request (join or create mode), sets the
// signup cookie for status polling, and emits signup.requested.
func (a *App) signup(w http.ResponseWriter, r *http.Request) {
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
	if body.FullName == "" || body.Email == "" || body.Password == "" {
		httputil.Error(w, http.StatusBadRequest, "full_name, email and password are required")
		return
	}
	if len(body.Password) < 8 {
		httputil.Error(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	if body.StartMode != "join" && body.StartMode != "create" {
		httputil.Error(w, http.StatusBadRequest, "start_mode must be 'join' or 'create'")
		return
	}

	workspaceID := contracts.ID("")
	workspaceName := ""
	role := contracts.RoleMember
	if body.StartMode == "join" {
		if body.InviteCode == "" {
			httputil.Error(w, http.StatusBadRequest, "invite_code is required for join mode")
			return
		}
		inv, err := a.store.LookupInviteCode(r.Context(), body.InviteCode)
		if err != nil {
			httputil.Error(w, http.StatusBadRequest, "unknown invite code")
			return
		}
		workspaceID = inv.WorkspaceID
		workspaceName = inv.WorkspaceName
		role = inv.Role
	} else {
		if body.OrganizationName == "" {
			httputil.Error(w, http.StatusBadRequest, "organization_name is required for create mode")
			return
		}
		role = contracts.RoleOwner
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		httputil.ServerError(w, a.log, "auth.Signup", err)
		return
	}
	req, err := a.store.CreateSignupRequest(r.Context(), body.FullName, body.Email, string(hash),
		body.StartMode, body.InviteCode, workspaceName, body.OrganizationName, workspaceID, role)
	if errors.Is(err, store.ErrEmailTaken) {
		httputil.Error(w, http.StatusBadRequest, "an account with this email already exists")
		return
	}
	if err != nil {
		httputil.ServerError(w, a.log, "auth.Signup", err)
		return
	}

	clearCookie(w, sessionCookie)
	http.SetCookie(w, &http.Cookie{Name: signupCookie, Value: req.ID, Path: "/", HttpOnly: true, MaxAge: 7 * 86400})
	a.publish(r.Context(), contracts.TopicSignupRequested, contracts.SignupRequestedData{
		RequestID: req.ID, UserID: req.UserID, Name: body.FullName, Email: body.Email,
		Mode: body.StartMode, InviteCode: body.InviteCode, WorkspaceID: req.WorkspaceID,
		OrganizationName: body.OrganizationName, RequestedRole: role,
	}, "")
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"request_id": req.ID})
}

// publish emits an event; non-fatal when the producer is unavailable.
func (a *App) publish(ctx context.Context, topic string, data any, key contracts.ID) {
	if a.prod == nil {
		return
	}
	env := contracts.EventEnvelope{TaskID: key, Data: data}
	if err := kafka.Publish(ctx, a.prod, topic, env, a.log); err != nil {
		a.log.Error("publish event failed", "topic", topic, "error", err)
	}
}

// signupStatus returns the pending request state for the signup cookie.
func (a *App) signupStatus(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(signupCookie)
	if err != nil || c.Value == "" {
		// Fall back to the most recent request for the email, if a cookie is absent.
		if email := r.URL.Query().Get("email"); email != "" {
			req, err := a.store.GetSignupRequestByEmail(r.Context(), email)
			if err != nil {
				httputil.Error(w, http.StatusNotFound, "no signup request found")
				return
			}
			a.writeSignupStatus(w, req)
			return
		}
		httputil.Error(w, http.StatusNotFound, "no signup request found")
		return
	}
	req, err := a.store.GetSignupRequest(r.Context(), c.Value)
	if err != nil {
		httputil.Error(w, http.StatusNotFound, "no signup request found")
		return
	}
	a.writeSignupStatus(w, req)
}

func (a *App) writeSignupStatus(w http.ResponseWriter, req store.SignupRequestRow) {
	out := map[string]any{
		"state": req.Status, "email": req.Email,
	}
	if req.WorkspaceName != "" {
		out["workspace_name"] = req.WorkspaceName
	}
	if req.Status == contracts.SignupApproved {
		out["admin_name"] = "Admin"
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

// resend is idempotent (spec): notifications are out of scope for the MVP.
func (a *App) resend(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(signupCookie)
	if err != nil || c.Value == "" {
		httputil.Error(w, http.StatusNotFound, "no signup request found")
		return
	}
	if _, err := a.store.GetSignupRequest(r.Context(), c.Value); err != nil {
		httputil.Error(w, http.StatusNotFound, "no signup request found")
		return
	}
	a.log.Info("signup notification resent (stub)", "request_id", c.Value)
	w.WriteHeader(http.StatusNoContent)
}

// ssoBegin returns a configured redirect_url or a documented error.
func (a *App) ssoBegin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Provider string `json:"provider"`
	}
	if httputil.Decode(w, r, &body) {
		return
	}
	url, ok := a.ssoCfg[strings.ToLower(body.Provider)]
	if !ok || url == "" {
		httputil.Error(w, http.StatusBadRequest, "sso provider is not configured")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"redirect_url": url})
}

// internalIdentity resolves the session cookie to the identity the Gateway
// forwards as the X-User-* scoping headers (workspace-scoping contract).
func (a *App) internalIdentity(w http.ResponseWriter, r *http.Request) {
	u, err := a.currentUser(r)
	if err != nil {
		httputil.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	super := u.IsSuperadmin != nil && *u.IsSuperadmin
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"user_id":       u.ID,
		"name":          u.Name,
		"email":         u.Email,
		"is_superadmin": super,
	})
}

// activeUsers24h serves the Gateway's KPI composition.
func (a *App) activeUsers24h(w http.ResponseWriter, r *http.Request) {
	n, err := a.store.CountActiveUsers24h(r.Context())
	if err != nil {
		httputil.ServerError(w, a.log, "auth.ActiveUsers", err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]int{"active_users_24h": n})
}
