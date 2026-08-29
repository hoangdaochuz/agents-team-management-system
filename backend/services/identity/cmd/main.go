// Package main is the Identity service entrypoint: the consolidated
// Auth + Orgs + Admin bounded context over a single identity_db. It serves
// user/session lifecycle, signup + approval, the orgs/workspace/member/invite
// surface, the workspace audit log, feature flags, and both halves of the
// sysadmin API. Intra-service events (signup flow, invite projection, audit
// recording) dispatch over the in-process event bus instead of Kafka.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"

	"github.com/aaks/server/internal/contracts/events"
	"github.com/aaks/server/internal/platform/svcrun"
	adminapp "github.com/aaks/server/services/identity/internal/application/admin"
	authapp "github.com/aaks/server/services/identity/internal/application/auth"
	orgsapp "github.com/aaks/server/services/identity/internal/application/orgs"
	"github.com/aaks/server/services/identity/internal/infrastructure/bus"
	"github.com/aaks/server/services/identity/internal/infrastructure/provision"
	"github.com/aaks/server/services/identity/internal/infrastructure/repository"
	adminhttp "github.com/aaks/server/services/identity/internal/interfaces/http/admin"
	authhttp "github.com/aaks/server/services/identity/internal/interfaces/http/auth"
	orgshttp "github.com/aaks/server/services/identity/internal/interfaces/http/orgs"
)

func main() {
	svcrun.Run("identity", getenv("HTTP_ADDR", ":8085"), register)
}

// register is the explicit composition root: config → platform deps →
// repositories → in-process event bus → application handlers → HTTP adapters.
func register(ctx context.Context, mux *http.ServeMux, log *slog.Logger) error {
	dsn := os.Getenv("IDENTITY_DB_DSN")
	if dsn == "" {
		return errors.New("IDENTITY_DB_DSN is not set")
	}
	st, err := repository.New(ctx, dsn, log)
	if err != nil {
		return err
	}

	// Event bus: the former auth↔orgs↔admin Kafka round-trips become
	// synchronous in-process dispatch — this service no longer touches Kafka.
	// The routes map is filled after the apps are built (they need pub; pub's
	// handlers need them) — the bus reads it per dispatch.
	routes := map[string][]bus.HandlerFunc{}
	pub := bus.NewPublisher("", log, bus.NewInProc(log, routes), nil)

	// Application handlers for the three merged planes. Workspace creation
	// provisions the Workspace service over a direct HTTP call (the former
	// workspace.created event).
	authApp := authapp.New(&authapp.Repository{
		Users:          st.Users,
		Sessions:       st.Sessions,
		SignupRequests: st.SignupRequests,
		Invites:        st.AuthInvites,
	}, pub, log)
	orgsApp := orgsapp.New(&orgsapp.Repository{
		Organizations: st.Organizations,
		Workspaces:    st.Workspaces,
		Members:       st.Members,
		Invites:       st.Invites,
		JoinRequests:  st.JoinRequests,
		OrgRequests:   st.OrgRequests,
	}, repository.NewUnitOfWork(st), pub, log, provision.New(os.Getenv("WORKSPACE_URL")))
	adminApp := adminapp.New(&adminapp.Repository{
		Audit: st.Audit,
		Flags: st.Flags,
	}, log)

	routes[events.TopicSignupRequested] = []bus.HandlerFunc{forward(orgsApp.ProjectSignupRequest)}
	routes[events.TopicSignupApproved] = []bus.HandlerFunc{forward(authApp.HandleSignupApproved)}
	routes[events.TopicSignupDeclined] = []bus.HandlerFunc{forward(authApp.HandleSignupDeclined)}
	routes[events.TopicInviteCreated] = []bus.HandlerFunc{forward(authApp.HandleInviteCreated)}
	routes[events.TopicAuditRecorded] = []bus.HandlerFunc{forward(adminApp.RecordAudit)}

	if email, pass := os.Getenv("AUTH_SEED_SUPERADMIN_EMAIL"), os.Getenv("AUTH_SEED_SUPERADMIN_PASSWORD"); email != "" && pass != "" {
		authApp.SeedSuperadmin(ctx, email, pass)
	}

	ssoCfg := map[string]string{
		"google": os.Getenv("SSO_GOOGLE_REDIRECT_URL"),
		"saml":   os.Getenv("SSO_SAML_REDIRECT_URL"),
	}
	authhttp.New(authApp, log, ssoCfg, orgsApp).Register(mux)
	orgshttp.New(orgsApp, log).Register(mux)
	adminhttp.New(adminApp, log).Register(mux)

	log.Info("identity routes registered", "endpoints", 34)
	return nil
}

// forward adapts a typed application handler to a bus.HandlerFunc via the
// shared envelope decoder.
func forward[T any](fn func(context.Context, T) error) bus.HandlerFunc {
	return func(ctx context.Context, msg events.EventEnvelope) error {
		return events.Forward(ctx, msg, fn)
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
