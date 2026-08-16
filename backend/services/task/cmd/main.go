// Package main is the Task service entrypoint: owns Task CRUD, feedback, and the
// task-lifecycle saga (run/review/stop/open-pr) coordinated over Kafka.
package main

import (
	"os"

	"github.com/aaks/server/internal/platform/svcrun"
	"github.com/aaks/server/services/task/internal/httpapi"
)

func main() {
	svcrun.Run("task", getenv("HTTP_ADDR", ":8082"), httpapi.Register)
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
