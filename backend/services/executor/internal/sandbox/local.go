package sandbox

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"
)

// localEnv is the host fallback for environments without a Docker daemon: the
// worktree is real (disjoint per task) but commands run on the host in the
// worktree directory. It honours the same Env contract but is NOT a credential
// isolation boundary — use only for local dev without secrets in scope.
type localEnv struct {
	wt  *Worktree
	log *slog.Logger
}

func (e *localEnv) WorktreeBranch() string { return e.wt.Branch }

func (e *localEnv) Exec(ctx context.Context, cmd string, args ...string) (ExecResult, error) {
	c := exec.CommandContext(ctx, cmd, args...)
	c.Dir = e.wt.Path
	var stdout, stderr strings.Builder
	c.Stdout = &stdout
	c.Stderr = &stderr
	err := c.Run()
	res := ExecResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			res.ExitCode = ee.ExitCode()
			return res, nil // non-zero exit is a normal sandbox result, not an error
		}
		return res, fmt.Errorf("run %q: %w", cmd, err)
	}
	return res, nil
}

func (e *localEnv) ReadFile(_ context.Context, path string) (string, error) {
	return readHostFile(e.wt.Path, path)
}
func (e *localEnv) WriteFile(_ context.Context, path, content string) error {
	return writeHostFile(e.wt.Path, path, content)
}
func (e *localEnv) ListFiles(_ context.Context, path string) ([]string, error) {
	return listHostDir(e.wt.Path, path)
}

func (e *localEnv) Close(ctx context.Context) error {
	return removeWorktree(ctx, filepath.Dir(filepath.Dir(e.wt.Path)), filepath.Base(e.wt.Path))
}
