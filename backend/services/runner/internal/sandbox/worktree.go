package sandbox

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Worktree is a per-task git worktree on a dedicated agent branch.
type Worktree struct {
	Path   string // absolute path bind-mounted into the sandbox
	Branch string // agent/<task-id>-<slug>
}

// worktreeDir is the on-disk location of a task's worktree under the clone root.
func worktreeDir(cloneRoot, taskID string) string {
	return filepath.Join(cloneRoot, ".aaks-worktrees", taskID)
}

// addWorktree creates `git worktree add -b <branch> <path>` under the managed
// clone. The branch is `agent/<task-id>-<slug>`.
func addWorktree(ctx context.Context, cloneRoot, taskID, slug string) (*Worktree, error) {
	branch := AgentBranch(taskID, slug)
	dir := worktreeDir(cloneRoot, taskID)
	if err := runGit(ctx, cloneRoot, "worktree", "add", "-b", branch, dir); err != nil {
		return nil, fmt.Errorf("git worktree add: %w", err)
	}
	return &Worktree{Path: dir, Branch: branch}, nil
}

// removeWorktree removes a task's worktree (force, so partial branches clear).
func removeWorktree(ctx context.Context, cloneRoot, taskID string) error {
	dir := worktreeDir(cloneRoot, taskID)
	return runGit(ctx, cloneRoot, "worktree", "remove", "--force", dir)
}

// AgentBranch returns the canonical per-task agent branch name.
func AgentBranch(taskID, slug string) string {
	return "agent/" + taskID + "-" + slugify(slug)
}

// Slug returns a branch-safe slug for an arbitrary label (exported so callers
// building a branch from a prompt can share the exact normalization).
func Slug(s string) string { return slugify(s) }

var unsafeBranch = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// slugify turns an arbitrary title/slug into a git-branch-safe token.
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = unsafeBranch.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-_.")
	if s == "" {
		s = "task"
	}
	if len(s) > 48 {
		s = strings.TrimRight(s[:48], "-_.")
	}
	return s
}

// runGit runs git in dir, surfacing stderr on failure.
func runGit(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
