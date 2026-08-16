// Package domain holds the Settings bounded-context entities and repository
// ports. It imports nothing infrastructural (enforced by the import lint).
package domain

import "errors"

// Sentinels shared by the application and interface layers. The HTTP layer maps
// these to status codes; application handlers never touch net/http.
var (
	ErrNotFound  = errors.New("not found")
	ErrForbidden = errors.New("superadmin or workspace owner/admin required")
)
