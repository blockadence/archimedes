package gitutil_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blockadence/archimedes/cli/internal/gitutil"
)

// makeRepo sets up a clone of a bare "origin" with one commit, mirroring
// the fixture tests/spawn_materializes_context.sh uses for the shell
// version. Tests that only need a remote URL to parse use makeRemote.
func makeRepo(t *testing.T, tmp string) string {
	t.Helper()
	origin := filepath.Join(tmp, "origin.git")
	clone := filepath.Join(tmp, "clone")

	mustGit(t, "", "init", "-q", "--bare", "-b", "main", origin)
	mustGit(t, "", "clone", "-q", origin, clone)
	if err := os.WriteFile(filepath.Join(clone, "README.md"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, clone, "add", "-A")
	mustGit(t, clone, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "-m", "init")
	mustGit(t, clone, "push", "-q", "origin", "main")
	return clone
}

// makeRemote returns an empty repo whose origin remote is originURL —
// everything GHSlug needs, without the cost of a real clone.
func makeRemote(t *testing.T, originURL string) string {
	t.Helper()
	dir := t.TempDir()
	mustGit(t, dir, "init", "-q")
	mustGit(t, dir, "remote", "add", "origin", originURL)
	return dir
}

// mustGit runs git in dir and fails the test if it doesn't succeed.
func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// gitOutput runs git in dir and returns its stdout.
func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return string(out)
}

func TestGHSlug(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"ssh", "git@github.com:acme/widgets.git", "acme/widgets"},
		{"https", "https://github.com/acme/widgets.git", "acme/widgets"},
		{"non-github unchanged", "git@gitlab.com:acme/widgets.git", "git@gitlab.com:acme/widgets.git"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := gitutil.GHSlug(makeRemote(t, tt.url))
			if err != nil {
				t.Fatalf("GHSlug returned error: %v", err)
			}
			if got != tt.want {
				t.Errorf("GHSlug(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestGHSlugNoRemoteErrors(t *testing.T) {
	dir := t.TempDir()
	mustGit(t, dir, "init", "-q")

	if _, err := gitutil.GHSlug(dir); err == nil {
		t.Fatal("expected error when origin remote is missing, got nil")
	}
}

func TestRemoveWorktreeAndBranch(t *testing.T) {
	tmp := t.TempDir()
	repo := makeRepo(t, tmp)

	wt := filepath.Join(tmp, "widget-fix-worktree")
	mustGit(t, repo, "worktree", "add", wt, "-b", "widget-fix", "main")

	if err := gitutil.RemoveWorktree(repo, wt); err != nil {
		t.Fatalf("RemoveWorktree: %v", err)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Errorf("expected worktree dir to be gone, stat err = %v", err)
	}

	if err := gitutil.RemoveBranch(repo, "widget-fix"); err != nil {
		t.Fatalf("RemoveBranch: %v", err)
	}
	out := strings.TrimSpace(gitOutput(t, repo, "branch", "--list", "widget-fix"))
	if out != "" {
		t.Errorf("expected branch widget-fix to be gone, git branch --list returned %q", out)
	}
}

func TestRemoveBranchToleratesAlreadyGone(t *testing.T) {
	repo := makeRepo(t, t.TempDir())

	if err := gitutil.RemoveBranch(repo, "never-existed"); err != nil {
		t.Errorf("expected RemoveBranch to tolerate a missing branch, got: %v", err)
	}
}
