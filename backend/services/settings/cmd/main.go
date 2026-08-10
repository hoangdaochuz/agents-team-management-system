// Package main is the Settings service entrypoint: sole decryptor of provider
// keys + git credentials, exposed to the runner over mTLS only.
package main

import (
	"os"

	"github.com/aaks/server/internal/svcrun"
	"github.com/aaks/server/services/settings/internal/httpapi"
)

func main() {
	svcrun.Run("settings", getenv("HTTP_ADDR", ":8085"), httpapi.Register)
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
