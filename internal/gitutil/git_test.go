package gitutil_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blockadence/gh-archimedes/internal/gitutil"
	"github.com/blockadence/gh-archimedes/internal/testrepo"
)

// makeRepo is the full fixture, for the helpers that need real history to
// work against (worktrees, branches, merge state). Tests that only need a
// remote URL to parse use makeRemote, which skips the clone entirely.
func makeRepo(t *testing.T, tmp string) string {
	t.Helper()
	return testrepo.New(t, testrepo.Spec{Dir: tmp, Name: "app"}).Clone
}

// makeRemote returns an empty repo whose origin remote is originURL —
// everything GHSlug needs, without the cost of a real clone.
func makeRemote(t *testing.T, originURL string) string {
	t.Helper()
	dir := t.TempDir()
	testrepo.Git(t, dir, "init", "-q")
	testrepo.Git(t, dir, "remote", "add", "origin", originURL)
	return dir
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
	testrepo.Git(t, dir, "init", "-q")

	if _, err := gitutil.GHSlug(dir); err == nil {
		t.Fatal("expected error when origin remote is missing, got nil")
	}
}

func TestRemoveWorktreeAndBranch(t *testing.T) {
	tmp := t.TempDir()
	repo := makeRepo(t, tmp)

	wt := filepath.Join(tmp, "widget-fix-worktree")
	testrepo.Git(t, repo, "worktree", "add", wt, "-b", "widget-fix", "main")

	if err := gitutil.RemoveWorktree(repo, wt); err != nil {
		t.Fatalf("RemoveWorktree: %v", err)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Errorf("expected worktree dir to be gone, stat err = %v", err)
	}

	if err := gitutil.RemoveBranch(repo, "widget-fix"); err != nil {
		t.Fatalf("RemoveBranch: %v", err)
	}
	out := testrepo.GitOut(t, repo, "branch", "--list", "widget-fix")
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

// commitFile writes name into repo and commits it, moving the checked-out
// branch forward one commit.
func commitFile(t *testing.T, repo, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, name), []byte(name+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	testrepo.Git(t, repo, "add", "-A")
	testrepo.Git(t, repo, "commit", "-q", "-m", name)
}

func TestHasRef(t *testing.T) {
	repo := makeRepo(t, t.TempDir())
	testrepo.Git(t, repo, "branch", "feature", "main")

	tests := []struct {
		name string
		ref  string
		want bool
	}{
		{"local branch", "feature", true},
		{"remote-tracking branch", "origin/main", true},
		{"missing branch", "no-such-branch", false},
		{"missing remote-tracking branch", "origin/no-such-branch", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := gitutil.HasRef(repo, tt.ref); got != tt.want {
				t.Errorf("HasRef(%q) = %v, want %v", tt.ref, got, tt.want)
			}
		})
	}
}

func TestIsAncestor(t *testing.T) {
	repo := makeRepo(t, t.TempDir())
	testrepo.Git(t, repo, "checkout", "-q", "-b", "feature")
	commitFile(t, repo, "feature.txt")

	tests := []struct {
		name                 string
		ancestor, descendant string
		want                 bool
	}{
		{"base is reachable from branch built on it", "main", "feature", true},
		{"branch is not reachable from its base", "feature", "main", false},
		{"a ref is its own ancestor", "main", "main", true},
		{"an unresolvable ref is nobody's ancestor", "no-such-branch", "main", false},
		{"nothing is an ancestor of an unresolvable ref", "main", "no-such-branch", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := gitutil.IsAncestor(repo, tt.ancestor, tt.descendant); got != tt.want {
				t.Errorf("IsAncestor(%q, %q) = %v, want %v", tt.ancestor, tt.descendant, got, tt.want)
			}
		})
	}
}
