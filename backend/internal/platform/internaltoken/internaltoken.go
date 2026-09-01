// Package internaltoken guards service-to-service endpoints. Every service
// exposes /internal/* routes that trust the caller (session composition, task
// ownership, stats, key decrypt, provisioning); on a shared compose network
// "trust the network" means "trust every container". A shared secret in the
// INTERNAL_TOKEN env var restores an authenticated boundary: when set, every
// /internal/* request must carry it in the X-Internal-Token header
// (constant-time compared). When unset the guard is disabled so local dev and
// httptest-based tests keep working — production (compose) always sets it.
package internaltoken

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// Header carries the shared internal token on service-to-service calls.
const Header = "X-Internal-Token"

// Wrap gates every /internal/* path behind the shared token and passes
// everything else through untouched. Mount it at the composition root around
// the fully registered mux. An empty token disables the guard.
func Wrap(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/internal/") {
			got := r.Header.Get(Header)
			if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
