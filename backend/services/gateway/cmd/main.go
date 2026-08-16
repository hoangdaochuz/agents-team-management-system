// Package main is the API Gateway / BFF entrypoint. The gateway is the
// frontend's sole HTTP entrypoint: it routes the full /api surface, composes
// cross-service reads via synchronous fan-out, and serves the SSE stream.
package main

import (
	"os"

	"github.com/aaks/server/internal/platform/svcrun"
	"github.com/aaks/server/services/gateway/internal/httpapi"
)

func main() {
	svcrun.Run("gateway", getenv("HTTP_ADDR", ":8080"), httpapi.Register)
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
