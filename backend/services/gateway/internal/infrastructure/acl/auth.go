package acl

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/aaks/server/internal/contracts/workspaces"
	"github.com/aaks/server/internal/platform/internaltoken"
	"github.com/aaks/server/services/gateway/internal/application"
)

// IdentityClient resolves a session token to the full identity view — user
// plus workspace union — in one call against the Identity service's internal
// endpoint. The session cookie name is part of the Identity protocol.
type IdentityClient struct {
	url    string
	cookie string
	token  string
	hc     *http.Client
	log    *slog.Logger
}

// NewIdentityClient builds the Identity session client.
func NewIdentityClient(url, sessionCookie, internalToken string, log *slog.Logger) *IdentityClient {
	return &IdentityClient{
		url: strings.TrimSuffix(url, "/"), cookie: sessionCookie, token: internalToken,
		hc: &http.Client{Timeout: 5 * time.Second}, log: log,
	}
}

// Resolve implements application.IdentityClient: GET /internal/identity with
// the session cookie. An empty user_id is treated as an unresolvable session.
// A failed workspace union is non-fatal: the identity stays valid with an
// empty union (matching the previous Auth+Orgs behavior).
func (c *IdentityClient) Resolve(ctx context.Context, token string) (application.Identity, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url+"/internal/identity", nil)
	if err != nil {
		return application.Identity{}, err
	}
	req.Header.Set("Cookie", c.cookie+"="+token)
	if c.token != "" {
		req.Header.Set(internaltoken.Header, c.token)
	}
	var u struct {
		UserID       string                 `json:"user_id"`
		Name         string                 `json:"name"`
		Email        string                 `json:"email"`
		IsSuperadmin bool                   `json:"is_superadmin"`
		Workspaces   []workspaces.Workspace `json:"workspaces"`
	}
	if err := doGet(c.hc, c.log, ctx, req.URL.String(), req.Header, &u); err != nil {
		var se *StatusError
		if errors.As(err, &se) && (se.Code == http.StatusUnauthorized || se.Code == http.StatusForbidden) {
			// The Identity service saw the token and rejected it — definitive.
			return application.Identity{}, fmt.Errorf("%w: %s", application.ErrSessionRejected, se.Error())
		}
		return application.Identity{}, err
	}
	if u.UserID == "" {
		// Identity answered but resolved no session — definitive rejection.
		return application.Identity{}, fmt.Errorf("%w: empty user", application.ErrSessionRejected)
	}
	if u.Workspaces == nil {
		u.Workspaces = []workspaces.Workspace{}
	}
	return application.Identity{
		UserID: u.UserID, Name: u.Name, Email: u.Email, Superadmin: u.IsSuperadmin,
		Workspaces: u.Workspaces,
	}, nil
}
