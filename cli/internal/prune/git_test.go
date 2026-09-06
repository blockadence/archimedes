package prune_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blockadence/archimedes/cli/internal/prune"
)

// makeTargetRepo sets up a bare "origin" plus a clone with the given
// originURL, mirroring the fixture tests/spawn_materializes_context.sh
// uses for the shell version.
func makeTargetRepo(t *testing.T, tmp, originURL string) string {
	t.Helper()
	origin := filepath.Join(tmp, "origin.git")
	clone := filepath.Join(tmp, "clone")

	run(t, "", "git", "init", "-q", "--bare", "-b", "main", origin)
	run(t, "", "git", "clone", "-q", origin, clone)
	if err := os.WriteFile(filepath.Join(clone, "README.md"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, clone, "git", "add", "-A")
	run(t, clone, "git", "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "-m", "init")
	run(t, clone, "git", "push", "-q", "origin", "main")
	if originURL != "" {
		run(t, clone, "git", "remote", "set-url", "origin", originURL)
	}
	return clone
}

func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}

func TestGHSlugFromSSHRemote(t *testing.T) {
	tmp := t.TempDir()
	repo := makeTargetRepo(t, tmp, "git@github.com:acme/widgets.git")

	slug, err := prune.GHSlug(repo)
	if err != nil {
		t.Fatal(err)
	}
	if slug != "acme/widgets" {
		t.Errorf("got %q, want acme/widgets", slug)
	}
}

func TestGHSlugFromHTTPSRemote(t *testing.T) {
	tmp := t.TempDir()
	repo := makeTargetRepo(t, tmp, "https://github.com/acme/widgets.git")

	slug, err := prune.GHSlug(repo)
	if err != nil {
		t.Fatal(err)
	}
	if slug != "acme/widgets" {
		t.Errorf("got %q, want acme/widgets", slug)
	}
}

func TestRemoveWorktreeAndBranch(t *testing.T) {
	tmp := t.TempDir()
	repo := makeTargetRepo(t, tmp, "")

	wt := filepath.Join(tmp, "widget-fix-worktree")
	run(t, repo, "git", "worktree", "add", wt, "-b", "widget-fix", "main")

	if err := prune.RemoveWorktree(repo, wt); err != nil {
		t.Fatalf("RemoveWorktree: %v", err)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Errorf("expected worktree dir to be gone, stat err = %v", err)
	}

	if err := prune.RemoveBranch(repo, "widget-fix"); err != nil {
		t.Fatalf("RemoveBranch: %v", err)
	}
	out := strings.TrimSpace(runOut(t, repo, "git", "branch", "--list", "widget-fix"))
	if out != "" {
		t.Errorf("expected branch widget-fix to be gone, git branch --list returned %q", out)
	}
}

func TestRemoveBranchToleratesAlreadyGone(t *testing.T) {
	tmp := t.TempDir()
	repo := makeTargetRepo(t, tmp, "")

	if err := prune.RemoveBranch(repo, "never-existed"); err != nil {
		t.Errorf("expected RemoveBranch to tolerate a missing branch, got: %v", err)
	}
}

func runOut(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%s %v: %v", name, args, err)
	}
	return string(out)
}
