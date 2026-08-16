// Package domain holds the Task bounded-context entities and repository ports.
// It imports nothing infrastructural (enforced by the import-direction lint);
// wire DTOs from the shared kernel are used as domain types where they do not
// diverge.
package domain

import "errors"

// Sentinels shared by the application and interface layers. The HTTP layer maps
// these to status codes; application handlers never touch net/http.
var (
	// ErrNotFound is returned when a queried task does not exist.
	ErrNotFound = errors.New("task not found")
	// ErrNoAgent is returned when a re-run is requested for an unassigned task.
	ErrNoAgent = errors.New("task has no assigned agent")
	// ErrNotDone is returned when open-pr is requested for a non-done task.
	ErrNotDone = errors.New("open-pr is only allowed on done tasks")
)