// Package domain holds the Auth bounded-context entities and repository ports.
// It imports nothing infrastructural (enforced by the import-direction lint).
package domain

import "errors"

// Sentinels shared by the application and interface layers. The HTTP layer maps
// these to status codes; application handlers never touch net/http.
var (
	ErrNotFound   = errors.New("not found")
	ErrEmailTaken = errors.New("email already registered")
	ErrBadPassword = errors.New("invalid credentials")
	ErrPending     = errors.New("account awaiting approval")
	ErrThrottled   = errors.New("too many login attempts")
)