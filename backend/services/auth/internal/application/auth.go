package application

import (
	"context"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/services/auth/internal/domain"
)

// loginLimit is the max allowed login failures for an IP+email (and per IP)
// within loginWindow before the client is throttled (credential-stuffing
// hardening).
const (
	loginLimit  = 5
	loginWindow = 15 * time.Minute
)

// loginRateEntry tracks consecutive failures for a throttle key.
type loginRateEntry struct {
	first time.Time
	count int
}

// loginRate is a concurrency-safe failure counter keyed by client IP and
// IP+email. sync.Map matches the pre-refactor throttling implementation.
type loginRate struct {
	sync.Map
}

// LoginResult is the outcome of a successful login.
type LoginResult struct {
	User   identity.User
	Token  string
	MaxAge int
}

// LoginGate reports whether the client IP is currently throttled. It is the
// pre-body per-IP gate: the handler checks it before decoding the request.
func (a *App) LoginGate(ip string) bool {
	return a.throttled(loginKey(ip, ""))
}

// Login validates credentials and issues a session token. Failures are
// recorded for throttling; a success clears the throttle keys. The active
// check rejects unapproved signups (spec: unapproved user scenario).
func (a *App) Login(ctx context.Context, email, password, ip string, remember bool) (LoginResult, error) {
	if a.throttled(loginKey(ip, "")) {
		return LoginResult{}, domain.ErrThrottled
	}
	u, err := a.repo.Users.GetByEmail(ctx, email)
	if err != nil {
		a.failLogin(ip, email)
		return LoginResult{}, domain.ErrBadPassword
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		a.failLogin(ip, email)
		return LoginResult{}, domain.ErrBadPassword
	}
	if !u.Active {
		return LoginResult{}, domain.ErrPending
	}
	ttl := 24 * time.Hour
	maxAge := 86400
	if remember {
		ttl = 30 * 24 * time.Hour
		maxAge = 30 * 86400
	}
	token, err := a.repo.Sessions.Create(ctx, u.ID, ttl)
	if err != nil {
		return LoginResult{}, err
	}
	a.loginRate.Delete(loginKey(ip, email))
	a.loginRate.Delete(loginKey(ip, ""))
	return LoginResult{User: u.User, Token: token, MaxAge: maxAge}, nil
}

// SessionUser resolves the session token to its user, or domain.ErrNotFound.
func (a *App) SessionUser(ctx context.Context, token string) (domain.User, error) {
	return a.repo.Sessions.User(ctx, token)
}

// Logout invalidates a session token (best-effort; the caller ignores errors).
func (a *App) Logout(ctx context.Context, token string) error {
	return a.repo.Sessions.Delete(ctx, token)
}

// ActiveUsers24h counts distinct users with a session created in the last 24h
// (Gateway KPI composition).
func (a *App) ActiveUsers24h(ctx context.Context) (int, error) {
	return a.repo.Sessions.CountActiveUsers24h(ctx)
}

// SeedSuperadmin creates the configured superadmin if it does not exist.
func (a *App) SeedSuperadmin(ctx context.Context, email, password string) {
	if _, err := a.repo.Users.GetByEmail(ctx, email); err == nil {
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		a.log.Error("seed superadmin: hash failed", "error", err)
		return
	}
	if _, err := a.repo.Users.Create(ctx, "Superadmin", email, string(hash), true); err != nil {
		a.log.Error("seed superadmin: create failed", "error", err)
		return
	}
	if err := a.repo.Users.ActivateByEmail(ctx, email); err != nil {
		a.log.Error("seed superadmin: activate failed", "error", err)
		return
	}
	a.log.Info("seeded superadmin", "email", email)
}

// failLogin records the failure for throttling.
func (a *App) failLogin(ip, email string) {
	a.recordFailure(loginKey(ip, email))
	a.recordFailure(loginKey(ip, ""))
}

// throttled reports whether the given key has hit the failure limit within the
// window.
func (a *App) throttled(key string) bool {
	if v, ok := a.loginRate.Load(key); ok {
		if e, ok := v.(loginRateEntry); ok && time.Since(e.first) < loginWindow && e.count >= loginLimit {
			return true
		}
	}
	return false
}

// recordFailure bumps the failure count for a key, resetting the window when it
// has expired.
func (a *App) recordFailure(key string) {
	v, _ := a.loginRate.LoadOrStore(key, loginRateEntry{first: time.Now(), count: 0})
	entry := v.(loginRateEntry)
	if time.Since(entry.first) >= loginWindow {
		a.loginRate.Store(key, loginRateEntry{first: time.Now(), count: 1})
		return
	}
	entry.count++
	a.loginRate.Store(key, entry)
}

// loginKey keys the throttle by client IP + attempt email. The email-less form
// (pre-body) throttles per IP only.
func loginKey(ip, email string) string {
	if email == "" {
		return "ip:" + ip
	}
	return "ip:" + ip + "|email:" + strings.ToLower(email)
}