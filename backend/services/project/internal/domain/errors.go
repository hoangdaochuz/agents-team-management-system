// Package domain holds the Project bounded-context entities and repository
// ports. It imports nothing infrastructural (enforced by the import-direction
// lint); wire DTOs from the shared kernel are used as domain types where they
// do not diverge.
package domain

import "errors"

// Sentinels shared by the application and interface layers. The HTTP layer maps
// these to status codes; application handlers never touch net/http.
var ErrNotFound = errors.New("project not found")
