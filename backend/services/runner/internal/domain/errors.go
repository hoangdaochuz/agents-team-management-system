// Package domain holds the Runner bounded-context repository ports. It imports
// nothing infrastructural (enforced by the import-direction lint); wire DTOs
// from the shared kernel are used as domain types where they do not diverge.
package domain

import "errors"

// ErrNotFound is returned when a queried entity does not exist.
var ErrNotFound = errors.New("not found")