// Package gitutil runs git as a subprocess on behalf of other packages that
// need repository state or need to mutate a working tree. It has no
// knowledge of Archimedes' own conventions (repos.yaml, work/, .archimedes/)
// — see internal/spawn for that.
package gitutil

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
)

// Run executes git in dir and returns its trimmed stdout. On failure the
// returned error includes stderr.
func Run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// RunOut executes git in dir with its output streamed to progress rather
// than captured, for commands whose progress is meant to reach the user
// directly (fetch, pull, worktree add). git writes progress to stderr, so
// both streams go to the one writer the caller designated for it — keeping
// them off the caller's own stdout.
func RunOut(dir string, progress io.Writer, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stdout = progress
	cmd.Stderr = progress
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

// CommonDir returns the absolute path of repoPath's shared (commondir) git
// directory — the same directory across every worktree of that repo.
func CommonDir(repoPath string) (string, error) {
	out, err := Run(repoPath, "rev-parse", "--git-common-dir")
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(out) {
		return out, nil
	}
	return filepath.Join(repoPath, out), nil
}
