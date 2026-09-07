// Package gitutil runs git as a subprocess on behalf of other packages that
// need repository state or need to mutate a working tree. It has no
// knowledge of Archimedes' own conventions (repos.yaml, work/, .archimedes/)
// — see internal/spawn for that. It does know the one thing every caller
// needs from a remote URL: the GitHub "owner/name" slug gh addresses repos
// by (GHSlug).
package gitutil

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
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

// Clone clones url into dest, with git's progress streamed to progress
// rather than captured — a clone is slow enough that the caller's user
// wants to see it happening.
func Clone(url, dest string, progress io.Writer) error {
	return RunOut("", progress, "clone", url, dest)
}

// RemoveWorktree force-removes the git worktree at wt from the repo
// checked out at repoPath, mirroring `git worktree remove --force`.
func RemoveWorktree(repoPath, wt string) error {
	_, err := Run(repoPath, "worktree", "remove", wt, "--force")
	return err
}

// RemoveBranch deletes branch from repoPath. An already-gone branch is not
// an error, matching prune.sh's `git branch -D "$slug" 2>/dev/null || true`.
func RemoveBranch(repoPath, branch string) error {
	_, _ = Run(repoPath, "branch", "-D", branch)
	return nil
}

// githubRemoteRE extracts "owner/name" from a github.com origin remote URL,
// SSH or HTTPS. Mirrors lib.sh's gh_slug():
// sed -E 's#.*github\.com[:/](.+)\.git#\1#'
var githubRemoteRE = regexp.MustCompile(`github\.com[:/](.+)\.git$`)

// GHSlug derives the "owner/name" slug gh needs from repoPath's origin
// remote. If the remote URL doesn't match the expected github.com/...git
// shape, it's returned unchanged, same as the sed pass it replaces (which
// leaves non-matching input untouched rather than erroring).
func GHSlug(repoPath string) (string, error) {
	url, err := Run(repoPath, "remote", "get-url", "origin")
	if err != nil {
		return "", fmt.Errorf("reading origin remote for %s: %w", repoPath, err)
	}

	if m := githubRemoteRE.FindStringSubmatch(url); m != nil {
		return m[1], nil
	}
	return url, nil
}

// HasRef reports whether ref names a commit in the repo at repoPath. Any
// failure to resolve it — a deleted branch, a remote-tracking ref that was
// never fetched, a broken repo — reads as absent, since every caller is
// asking "is this still here?" rather than "why not?".
func HasRef(repoPath, ref string) bool {
	_, err := Run(repoPath, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	return err == nil
}

// IsAncestor reports whether ancestor is reachable from descendant, i.e.
// whether descendant's history already contains it (`git merge-base
// --is-ancestor`). A ref that doesn't resolve is neither an ancestor nor a
// descendant of anything, so an unknown ref reads as false rather than as
// an error — callers use this to decide whether a branch has been left
// behind, and "can't tell" must never masquerade as "yes".
func IsAncestor(repoPath, ancestor, descendant string) bool {
	if !HasRef(repoPath, ancestor) || !HasRef(repoPath, descendant) {
		return false
	}
	_, err := Run(repoPath, "merge-base", "--is-ancestor", ancestor, descendant)
	return err == nil
}
