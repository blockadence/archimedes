package prune

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// RemoveWorktree force-removes the git worktree at wt from the repo
// checked out at repoPath, mirroring `git worktree remove --force`.
func RemoveWorktree(repoPath, wt string) error {
	cmd := exec.Command("git", "-C", repoPath, "worktree", "remove", wt, "--force")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree remove %s: %w: %s", wt, err, out)
	}
	return nil
}

// RemoveBranch deletes branch from repoPath. An already-gone branch is not
// an error, matching prune.sh's `git branch -D "$slug" 2>/dev/null || true`.
func RemoveBranch(repoPath, branch string) error {
	_ = exec.Command("git", "-C", repoPath, "branch", "-D", branch).Run()
	return nil
}

// githubRemoteRE mirrors lib.sh's gh_slug():
// sed -E 's#.*github\.com[:/](.+)\.git#\1#'
var githubRemoteRE = regexp.MustCompile(`github\.com[:/](.+)\.git$`)

// GHSlug derives the "owner/repo" slug gh needs from repoPath's origin
// remote. If the remote URL doesn't match the expected github.com/...git
// shape, it's returned unchanged, same as the sed pass it replaces (which
// leaves non-matching input untouched rather than erroring).
func GHSlug(repoPath string) (string, error) {
	out, err := exec.Command("git", "-C", repoPath, "remote", "get-url", "origin").Output()
	if err != nil {
		return "", fmt.Errorf("reading origin remote for %s: %w", repoPath, err)
	}

	url := strings.TrimSpace(string(out))
	if m := githubRemoteRE.FindStringSubmatch(url); m != nil {
		return m[1], nil
	}
	return url, nil
}
