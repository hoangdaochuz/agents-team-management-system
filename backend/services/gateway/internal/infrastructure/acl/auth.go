package acl

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/aaks/server/services/gateway/internal/application"
)

// AuthClient resolves session tokens to identities against the Auth service's
// internal endpoint. The session cookie name is part of the Auth protocol.
type AuthClient struct {
	url    string
	cookie string
	hc     *http.Client
	log    *slog.Logger
}

// NewAuthClient builds the Auth session client.
func NewAuthClient(url, sessionCookie string, log *slog.Logger) *AuthClient {
	return &AuthClient{
		url: strings.TrimSuffix(url, "/"), cookie: sessionCookie,
		hc: &http.Client{Timeout: 5 * time.Second}, log: log,
	}
}

// Resolve implements application.SessionClient: GET /internal/identity with
// the session cookie. An empty user_id is treated as an unresolvable session.
func (c *AuthClient) Resolve(ctx context.Context, token string) (application.Session, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url+"/internal/identity", nil)
	if err != nil {
		return application.Session{}, err
	}
	req.Header.Set("Cookie", c.cookie+"="+token)
	var u struct {
		UserID       string `json:"user_id"`
		Name         string `json:"name"`
		Email        string `json:"email"`
		IsSuperadmin bool   `json:"is_superadmin"`
	}
	if err := doGet(c.hc, c.log, ctx, req.URL.String(), req.Header, &u); err != nil {
		return application.Session{}, err
	}
	if u.UserID == "" {
		return application.Session{}, errors.New("auth returned an empty identity")
	}
	return application.Session{UserID: u.UserID, Name: u.Name, Email: u.Email, Superadmin: u.IsSuperadmin}, nil
}
