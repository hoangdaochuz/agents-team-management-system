// Package tenancy carries the workspace-scoping contract (design D8) shared by
// the Gateway (which injects the headers) and every downstream service (which
// reads them):
//
//	X-Workspace-ID   single workspace context (explicit path or single-workspace session)
//	X-Workspace-IDs  comma-separated union of workspaces the session can access
//	X-User-ID        the authenticated user ("" when unauthenticated)
//	X-User-Name      the authenticated user's display name
//	X-User-Email     the authenticated user's email
//	X-User-Role      the user's strongest role across the workspace union
//	X-User-Superadmin "true" only when the Gateway marked the session superadmin
//
// Services enforce the boundary independently: lists filter by the workspace
// set, get/update/delete/mutations reject rows outside it (404), and creates
// inherit the workspace context. When no workspace header is present a service
// MUST return an empty result set for lists (fail closed) and 400 for creates.
//
// Only the Gateway may populate these headers; inbound values are stripped
// before proxying (forgery boundary).
package tenancy

import (
	"net/http"
	"strings"
)

// Identity and scoping headers injected by the Gateway. These are the single
// source of truth for both the injector and the readers.
const (
	HeaderWorkspaceID    = "X-Workspace-ID"
	HeaderWorkspaceIDs   = "X-Workspace-IDs"
	HeaderUserID         = "X-User-ID"
	HeaderUserName       = "X-User-Name"
	HeaderUserEmail      = "X-User-Email"
	HeaderUserRole       = "X-User-Role"
	HeaderUserSuperadmin = "X-User-Superadmin"
)

// IdentityHeaders lists every identity/scoping header, for wholesale stripping
// of inbound (possibly forged) values.
var IdentityHeaders = []string{
	HeaderWorkspaceID,
	HeaderWorkspaceIDs,
	HeaderUserID,
	HeaderUserName,
	HeaderUserEmail,
	HeaderUserRole,
	HeaderUserSuperadmin,
}

// WorkspaceID returns the single workspace context, or "".
func WorkspaceID(r *http.Request) string {
	return r.Header.Get(HeaderWorkspaceID)
}

// WorkspaceIDs returns the full workspace set the session can access
// (single-workspace sessions get exactly one id; unions are comma-separated).
func WorkspaceIDs(r *http.Request) []string {
	if v := r.Header.Get(HeaderWorkspaceID); v != "" {
		return []string{v}
	}
	ids := []string{}
	for _, p := range strings.Split(r.Header.Get(HeaderWorkspaceIDs), ",") {
		if p = strings.TrimSpace(p); p != "" {
			ids = append(ids, p)
		}
	}
	return ids
}

// UserID returns the authenticated user id forwarded by the Gateway.
func UserID(r *http.Request) string {
	return r.Header.Get(HeaderUserID)
}

// UserRole returns the user's strongest role across the workspace union.
func UserRole(r *http.Request) string {
	return r.Header.Get(HeaderUserRole)
}

// UserSuperadmin reports whether the Gateway marked the session superadmin.
func UserSuperadmin(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get(HeaderUserSuperadmin), "true")
}
