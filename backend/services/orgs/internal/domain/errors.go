// Package domain holds the Orgs bounded-context entities and repository ports.
// It imports nothing infrastructural (enforced by the import-direction lint).
package domain

import "errors"

// Sentinels shared by the application and interface layers. The HTTP layer maps
// these to status codes; application handlers never touch net/http.
var (
	ErrNotFound   = errors.New("not found")
	ErrNoOrg      = errors.New("no organization for user")
	ErrLastOwner  = errors.New("cannot demote or remove the last owner")
	ErrNotMember  = errors.New("not a member of this workspace")
	ErrForbidden  = errors.New("admin role required")
	ErrNotPending = errors.New("request is not pending")
)