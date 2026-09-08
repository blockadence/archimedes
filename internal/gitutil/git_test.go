package gitutil_test

import (
	"os"
	"os/exec"
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

// HasConfiguredIdentity guards a commit nobody typed, so what it has to
// agree with is git's own answer to "is there an identity somebody set on
// purpose" — asserted here by committing, or failing to, in the repository
// it was asked about, on the three machines where git's answer to that and
// its answer to "can I commit at all" are the same one.
func TestHasConfiguredIdentity(t *testing.T) {
	tests := []struct {
		name  string
		setup func(testing.TB)
		want  bool
	}{
		{"configured in git's config", testrepo.IsolateGit, true},
		// The environment is configuration too, and the case that would be
		// lost by reading user.name and user.email: a CI system that sets an
		// identity there set it deliberately, and a commit under it is one
		// somebody asked for.
		{"configured in the environment", environmentIdentity, true},
		{"nothing at all", testrepo.StripGitIdentity, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup(t)
			dir := t.TempDir()
			testrepo.Git(t, dir, "init", "-q")

			if got := gitutil.HasConfiguredIdentity(dir); got != tt.want {
				t.Errorf("HasConfiguredIdentity = %v, want %v", got, tt.want)
			}
			if got := canCommit(t, dir); got != tt.want {
				t.Errorf("git itself commits = %v, so HasConfiguredIdentity's %v is the wrong answer", got, tt.want)
			}
		})
	}
}

// The fourth machine, and the decision this function carries: nothing
// configured, but an OS account git can guess a usable identity from, so git
// commits and this says no anyway. Whether the guess succeeds is a property
// of the box — a full name on a developer's macOS one, nothing on a CI
// runner — so where there is nothing to guess there is nothing here to
// assert, and the test skips rather than passing quietly on the machine that
// cannot exercise it.
func TestHasConfiguredIdentityRefusesTheOneGitGuesses(t *testing.T) {
	testrepo.UnconfigureGitIdentity(t)
	dir := t.TempDir()
	testrepo.Git(t, dir, "init", "-q")

	if !canCommit(t, dir) {
		t.Skip("this machine's account carries no name for git to guess an identity from")
	}
	if gitutil.HasConfiguredIdentity(dir) {
		t.Error("HasConfiguredIdentity = true for an identity git guessed from the account, which nobody configured")
	}
}

// Half configured is not configured. An operator who has set a user.name and
// no user.email has supplied one of the two things a commit needs, and git
// fills the other from the account — so the author would be half theirs and
// half a hostname nobody chose, which is the same objection in miniature.
//
// This one is portable where the test above is not: what git has left to
// guess here is only ever half the ident, so the answer is the same on a
// developer's box and on a runner, and the rule cannot rot into an "either
// one will do" that passes for the wrong reason.
func TestHasConfiguredIdentityRefusesAHalfConfiguredOne(t *testing.T) {
	tests := []struct{ name, key, value string }{
		{"name configured, address left to git", "user.name", "t"},
		{"address configured, name left to git", "user.email", "t@t"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testrepo.UnconfigureGitIdentity(t)
			dir := t.TempDir()
			testrepo.Git(t, dir, "init", "-q")
			testrepo.Git(t, dir, "config", tt.key, tt.value)

			if gitutil.HasConfiguredIdentity(dir) {
				t.Errorf("HasConfiguredIdentity = true with only %s set, so the other half of the author is still whoever git guessed", tt.key)
			}
		})
	}
}

// environmentIdentity is the machine whose identity is in GIT_AUTHOR_* and
// GIT_COMMITTER_* and nowhere else: a CI system that sets it there on
// purpose. Unconfigured first, so what the test binary inherited cannot be
// what it ends up measuring.
func environmentIdentity(t testing.TB) {
	t.Helper()
	testrepo.UnconfigureGitIdentity(t)
	for _, name := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(name, "t")
	}
	for _, name := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(name, "t@t")
	}
}

// canCommit reports whether git will actually make a commit in dir. It is
// what HasConfiguredIdentity used to be a prediction of and deliberately no
// longer is: on the machine above the two diverge, and that divergence is
// the decision. Everywhere else they must still agree. It runs git itself
// rather than going through testrepo's runner, which is the one place in
// this module's tests that is the right way round: the runner fails the test
// when git does, and here git failing is the answer being asked for.
func canCommit(t *testing.T, dir string) bool {
	t.Helper()
	cmd := exec.Command("git", "commit", "-q", "--allow-empty", "-m", "probe")
	cmd.Dir = dir
	return cmd.Run() == nil
}
