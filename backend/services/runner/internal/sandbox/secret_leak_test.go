package sandbox

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestDockerSandboxNoSecrets is task 7.4: the credential-less sandbox must not
// leak provider keys / git credentials into the container env, its filesystem,
// or its logs. It runs a real container, so it requires a docker daemon and the
// sandbox image. Opt in with AAKS_SANDBOX_TEST_DOCKER=1 (matches the repo-wide
// convention of infra-gated integration tests); `go test ./...` stays green
// without docker.
func TestDockerSandboxNoSecrets(t *testing.T) {
	if os.Getenv("AAKS_SANDBOX_TEST_DOCKER") == "" {
		t.Skip("set AAKS_SANDBOX_TEST_DOCKER=1 to run the docker sandbox secret-leak test")
	}
	ctx := context.Background()
	if _, err := os.Stat("/var/run/docker.sock"); err != nil {
		t.Fatalf("docker daemon not reachable: %v", err)
	}
	ensureSandboxImage(t)

	cloneRoot := makeGitRepo(t)
	cfg := Config{Kind: "docker", CloneRoot: cloneRoot}
	m := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	env, err := m.Setup(ctx, "task_secretcheck", "secret-leak-test")
	if err != nil {
		t.Fatalf("sandbox setup: %v", err)
	}
	closed := false
	t.Cleanup(func() {
		if !closed {
			if err := env.Close(ctx); err != nil {
				t.Errorf("sandbox close: %v", err)
			}
		}
	})

	// Secrets the Runner holds in memory (dev values + a fake provider key).
	// None may appear inside the container.
	probes := []string{
		"dev-master-key-0123456789abcdef", // SETTINGS_MASTER_KEY
		"dev-internal-token",              // SETTINGS_INTERNAL_TOKEN
		"sk-test-secret-provider-key-123456",
		"ghp_fake_git_token_9876543210",
	}
	// 1. Container env.
	out, err := env.Exec(ctx, "env")
	if err != nil {
		t.Fatalf("exec env: %v", err)
	}
	envOut := out.Stdout
	for _, p := range probes {
		if strings.Contains(envOut, p) {
			t.Errorf("secret %q leaked into container env:\n%s", p, envOut)
		}
	}
	// No variable NAMED like a secret either (case-insensitive).
	nameRe := regexp.MustCompile(`(?i)^([A-Z0-9_]+)=`)
	for _, line := range strings.Split(envOut, "\n") {
		m2 := nameRe.FindStringSubmatch(line)
		if m2 == nil {
			continue
		}
		if strings.Contains(m2[1], "KEY") || strings.Contains(m2[1], "TOKEN") ||
			strings.Contains(m2[1], "SECRET") || strings.Contains(m2[1], "PASSWORD") {
			t.Errorf("secret-named env var present in container: %s", line)
		}
	}

	// 2. Filesystem: the bind-mounted worktree is visible inside the container
	// (host file appears at /workspace), and no secret text lives anywhere we
	// could put it (worktree, /tmp, /root).
	if err := env.WriteFile(ctx, "notes.txt", "hello from host"); err != nil {
		t.Fatalf("write file: %v", err)
	}
	cat, err := env.Exec(ctx, "cat", "notes.txt")
	if err != nil || !strings.Contains(cat.Stdout, "hello from host") {
		t.Errorf("bind-mount round-trip failed: out=%q err=%v", cat.Stdout, err)
	}
	grep, err := env.Exec(ctx, "sh", "-c", "grep -rIl "+grepPattern(probes)+" /workspace /tmp /root 2>/dev/null || true")
	if err != nil {
		t.Fatalf("exec grep: %v", err)
	}
	if strings.TrimSpace(grep.Stdout) != "" {
		t.Errorf("secret text found in container filesystem: %s", grep.Stdout)
	}

	// 3. Container logs.
	denv, ok := env.(interface{ Logs(context.Context) (string, error) })
	if !ok {
		t.Fatal("docker env does not expose Logs")
	}
	logs, err := denv.Logs(ctx)
	if err != nil {
		t.Fatalf("docker logs: %v", err)
	}
	for _, p := range probes {
		if strings.Contains(logs, p) {
			t.Errorf("secret %q leaked into container logs: %s", p, logs)
		}
	}

	// 4. Cleanup really removed the container and worktree.
	if err := env.Close(ctx); err != nil {
		t.Fatalf("close: %v", err)
	}
	closed = true
	if _, err := os.Stat(filepath.Join(cloneRoot, ".aaks-worktrees", "task_secretcheck")); !os.IsNotExist(err) {
		t.Errorf("worktree not removed after close")
	}
}

// grepPattern builds a grep -e list of fixed strings.
func grepPattern(probes []string) string {
	parts := make([]string, 0, len(probes))
	for _, p := range probes {
		parts = append(parts, "-e "+p)
	}
	return strings.Join(parts, " ")
}

// ensureSandboxImage builds backend/runner/Dockerfile as aaks-runner:latest
// when it is not already present locally.
func ensureSandboxImage(t *testing.T) {
	t.Helper()
	if _, err := exec.Command("docker", "image", "inspect", "aaks-runner:latest").CombinedOutput(); err == nil {
		return
	}
	dfDir, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "runner"))
	if err != nil {
		t.Fatalf("locate runner Dockerfile: %v", err)
	}
	if out, err := exec.Command("docker", "build", "-t", "aaks-runner:latest", dfDir).CombinedOutput(); err != nil {
		t.Fatalf("docker build sandbox image: %v\n%s", err, out)
	}
}

// makeGitRepo initializes a git repo with one commit (worktree add requires
// at least one commit on the main branch).
func makeGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustCmd(t, dir, "git", "init", "-q", "-b", "main")
	mustCmd(t, dir, "git", "config", "user.email", "test@aaks.dev")
	mustCmd(t, dir, "git", "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("test repo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustCmd(t, dir, "git", "add", "README.md")
	mustCmd(t, dir, "git", "commit", "-q", "-m", "init")
	return dir
}

func mustCmd(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}
