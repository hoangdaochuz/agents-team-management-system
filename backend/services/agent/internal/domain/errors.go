// Package domain holds the Agent bounded-context entities and repository
// ports. It imports nothing infrastructural (enforced by the import lint).
package domain

import "errors"

// Sentinels shared by the application and interface layers. The HTTP layer maps
// these to status codes; application handlers never touch net/http.
var (
	ErrAgentNotFound = errors.New("agent not found")
	// ErrCrossWorkspace is returned when an attachment references a skill/MCP
	// definition from another workspace (spec: cross-workspace attachment
	// rejected).
	ErrCrossWorkspace = errors.New("skill or mcp belongs to another workspace")
	// ErrUnknownDefinition reports an attachment referencing a definition the
	// catalog has not projected into this workspace.
	ErrUnknownDefinition = errors.New("skill or mcp definition is unknown in this workspace")
)