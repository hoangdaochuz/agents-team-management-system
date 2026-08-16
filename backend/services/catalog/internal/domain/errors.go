// Package domain holds the Catalog bounded-context entities and repository
// ports. It imports nothing infrastructural (enforced by the import lint).
package domain

import "errors"

// Sentinels shared by the application and interface layers. The HTTP layer maps
// these to status codes; application handlers never touch net/http.
var (
	ErrSkillNotFound = errors.New("skill not found")
	ErrMcpNotFound   = errors.New("mcp server not found")
)