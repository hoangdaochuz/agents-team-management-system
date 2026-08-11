package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Add Login Page":         "add-login-page",
		"Fix CVE-2024-1234!!!":   "fix-cve-2024-1234",
		"  trailing   spaces  ":  "trailing-spaces",
		"":                       "task",
		"@@@":                    "task",
		"MixedCASE Title":        "mixedcase-title",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAgentBranch(t *testing.T) {
	got := AgentBranch("task_42", "Add Login Page")
	want := "agent/task_42-add-login-page"
	if got != want {
		t.Errorf("AgentBranch = %q, want %q", got, want)
	}
}

func TestNewDefaults(t *testing.T) {
	m := New(Config{Kind: "docker"}, nil)
	if m.cfg.Image != "aaks-runner:latest" {
		t.Errorf("default image = %q", m.cfg.Image)
	}
	if m.cfg.Socket != "/var/run/docker.sock" {
		t.Errorf("default socket = %q", m.cfg.Socket)
	}
	if !m.Enabled() {
		t.Error("docker kind should be enabled")
	}
	if New(Config{Kind: "none"}, nil).Enabled() {
		t.Error("none kind should not be enabled")
	}
	if New(Config{}, nil).Enabled() {
		t.Error("empty kind should not be enabled")
	}
}

func TestSetupDisabledReturnsNil(t *testing.T) {
	m := New(Config{}, nil)
	env, err := m.Setup(context.Background(), "task_1", "x")
	if err != nil {
		t.Fatalf("disabled setup errored: %v", err)
	}
	if env != nil {
		t.Fatal("disabled setup should return nil env")
	}
}

// TestLocalEnvFileOps exercises the host file ops + exec against a temp dir
// acting as a worktree — no Docker, no git required.
func TestLocalEnvFileOps(t *testing.T) {
	dir := t.TempDir()
	e := &localEnv{wt: &Worktree{Path: dir, Branch: "agent/test"}}

	ctx := context.Background()
	if err := e.WriteFile(ctx, "sub/hello.txt", "hi"); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := e.ReadFile(ctx, "sub/hello.txt")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if got != "hi" {
		t.Errorf("ReadFile = %q, want %q", got, "hi")
	}
	names, err := e.ListFiles(ctx, "sub")
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(names) != 1 || names[0] != "hello.txt" {
		t.Errorf("ListFiles = %v, want [hello.txt]", names)
	}

	// Exec: list the directory on whatever platform we're on.
	var cmd, arg string
	if runtime.GOOS == "windows" {
		cmd, arg = "cmd", "/c dir sub"
	} else {
		cmd, arg = "ls", "sub"
	}
	res, err := e.Exec(ctx, cmd, arg)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.ExitCode != 0 {
		t.Errorf("Exec exit = %d, stderr=%q", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "hello.txt") {
		t.Errorf("Exec stdout = %q, want it to contain hello.txt", res.Stdout)
	}
}

// TestLocalEnvNonZeroExit verifies a failing command is a normal result, not an
// error (the agent loop reports build/test failures via step output).
func TestLocalEnvNonZeroExit(t *testing.T) {
	dir := t.TempDir()
	e := &localEnv{wt: &Worktree{Path: dir, Branch: "agent/test"}}
	cmd := "false"
	if runtime.GOOS == "windows" {
		cmd = "cmd"
	}
	args := []string{}
	if runtime.GOOS == "windows" {
		args = []string{"/c", "exit", "1"}
	}
	res, err := e.Exec(context.Background(), cmd, args...)
	if err != nil {
		t.Fatalf("Exec err: %v", err)
	}
	if res.ExitCode == 0 {
		t.Error("expected non-zero exit")
	}
}

// TestHostFileOpsRoundTrip guards the shared helpers used by both drivers.
func TestHostFileOpsRoundTrip(t *testing.T) {
	root := t.TempDir()
	if err := writeHostFile(root, "a/b/c.txt", "z"); err != nil {
		t.Fatal(err)
	}
	got, err := readHostFile(root, "a/b/c.txt")
	if err != nil || got != "z" {
		t.Fatalf("readHostFile = %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(root, "a", "b")); err != nil {
		t.Errorf("dirs not created: %v", err)
	}
}
