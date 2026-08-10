// Package main is the Auth (identity) service entrypoint: user/session lifecycle,
// signup + admin approval, SSO begin stub. Implemented in phase 10.
package main

import (
	"os"

	"github.com/aaks/server/internal/svcrun"
	"github.com/aaks/server/services/auth/internal/httpapi"
)

func main() {
	svcrun.Run("auth", getenv("HTTP_ADDR", ":8087"), httpapi.Register)
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
