// Package config loads runtime configuration from the environment.
package config

import (
	"fmt"
	"os"
)

// Config holds all process configuration. Fields are added as subsystems come
// online; validation for a field is enforced by the subsystem that consumes it
// (e.g. the secrets package enforces ENCRYPTION_KEY in Phase 1).
type Config struct {
	HTTPAddr      string // HTTP listen address
	DBDSN         string // Postgres connection string
	EncryptionKey string // envelope key for at-rest secret encryption
	GitKeyPath    string // path to the shared SSH/deploy key
	GitHubToken   string // token for gh / GitHub API (PR creation)
	ReposRoot     string // managed clone storage root
	WorktreesRoot string // per-task worktree root
}

// Load reads configuration from the environment.
func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:      getenv("HTTP_ADDR", ":8080"),
		DBDSN:         getenv("DB_DSN", ""),
		EncryptionKey: getenv("ENCRYPTION_KEY", ""),
		GitKeyPath:    getenv("GIT_KEY_PATH", ""),
		GitHubToken:   getenv("GITHUB_TOKEN", ""),
		ReposRoot:     getenv("REPOS_ROOT", "/var/lib/aaks/repos"),
		WorktreesRoot: getenv("WORKTREES_ROOT", "/var/lib/aaks/worktrees"),
	}
	if cfg.HTTPAddr == "" {
		return Config{}, fmt.Errorf("HTTP_ADDR must not be empty")
	}
	return cfg, nil
}

func getenv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}
