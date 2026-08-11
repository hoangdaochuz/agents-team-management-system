package httputil

import (
	"net/http"
	"strings"

	"github.com/aaks/server/internal/contracts"
)

// Workspace-scoping contract (design D8): the Gateway resolves the session's
// workspace context and forwards it on every internal call:
//
//	X-Workspace-ID   single workspace context (explicit path or single-workspace session)
//	X-Workspace-IDs  comma-separated union of workspaces the session can access
//	X-User-ID        the authenticated user ("" when unauthenticated)
//	X-User-Role      the user's role in the resolved workspace ("" when none)
//
// Services enforce the boundary independently: lists filter by the workspace
// set, get/update/delete/mutations reject rows outside it (404), and creates
// inherit the workspace context. When no workspace header is present a service
// MUST return an empty result set for lists (fail closed) and 400 for creates.

// WorkspaceID returns the single workspace context, or "".
func WorkspaceID(r *http.Request) contracts.ID {
	return r.Header.Get("X-Workspace-ID")
}

// WorkspaceIDs returns the full workspace set the session can access
// (single-workspace sessions get exactly one id; unions are comma-separated).
func WorkspaceIDs(r *http.Request) []contracts.ID {
	if v := r.Header.Get("X-Workspace-ID"); v != "" {
		return []contracts.ID{v}
	}
	ids := []contracts.ID{}
	for _, p := range strings.Split(r.Header.Get("X-Workspace-IDs"), ",") {
		if p = strings.TrimSpace(p); p != "" {
			ids = append(ids, p)
		}
	}
	return ids
}

// UserID returns the authenticated user id forwarded by the Gateway.
func UserID(r *http.Request) contracts.ID {
	return r.Header.Get("X-User-ID")
}

// UserRole returns the user's role in the resolved workspace context.
func UserRole(r *http.Request) contracts.Role {
	return contracts.Role(r.Header.Get("X-User-Role"))
}

// UserSuperadmin reports whether the Gateway marked the session superadmin.
func UserSuperadmin(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("X-User-Superadmin"), "true")
}
