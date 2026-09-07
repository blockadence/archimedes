package status

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// squashMergedStack builds a checkout in the state a squash-merged base
// leaves behind: "auth-api" landed on main as one new commit, its own
// branch is still around (prune refuses to delete a base something is
// stacked on), and "auth-ui" is still sitting on auth-api's original
// commits.
func squashMergedStack(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	origin := filepath.Join(tmp, "origin.git")
	clone := filepath.Join(tmp, "service-a")

	git(t, "", "init", "-q", "--bare", "-b", "main", origin)
	git(t, "", "clone", "-q", origin, clone)
	git(t, clone, "config", "user.email", "t@t")
	git(t, clone, "config", "user.name", "t")
	commit(t, clone, "README.md", "hello\n", "init")
	git(t, clone, "push", "-q", "origin", "main")

	git(t, clone, "checkout", "-q", "-b", "auth-api")
	commit(t, clone, "api.go", "api\n", "add the api")
	git(t, clone, "checkout", "-q", "-b", "auth-ui")
	commit(t, clone, "ui.go", "ui\n", "add the ui")

	// The squash: the same file content lands on main under a new commit
	// message — and so a new SHA — leaving auth-api's own commit nowhere
	// in main's history.
	git(t, clone, "checkout", "-q", "main")
	commit(t, clone, "api.go", "api\n", "add the api (#7)")
	git(t, clone, "push", "-q", "origin", "main")
	git(t, clone, "fetch", "-q", "origin")

	return clone
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func commit(t *testing.T, repo, name, body, message string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-q", "-m", message)
}

// stackedRow runs a one-row report over repo against real git, with note
// as the row's status.md note and mergedState standing in for gh.
func stackedRow(repo, note string, mergedState MergedLookup) Row {
	entries := []Entry{{Slug: "auth-ui", Repo: "service-a", Branch: "auth-ui", Note: note}}
	return BuildReport(entries, Sources{
		Repos:  func(string) (RepoRef, error) { return RepoRef{Path: repo, BaseBranch: "main"}, nil },
		PR:     func(string, string) (PR, error) { return noPR, nil },
		Refs:   LocalRefs{},
		Merged: mergedState,
	}, 3).Rows[0]
}

const stackedOnAuthAPI = "stacked on service-a:auth-api"

func TestLocalRefsFlagsAndClearsAStackedRebase(t *testing.T) {
	repo := squashMergedStack(t)

	row := stackedRow(repo, stackedOnAuthAPI, merged)
	if !row.NeedsRebase {
		t.Fatalf("expected a rebase flag while auth-ui still carries the squashed base's commits, got: %#v", row)
	}
	if row.RebaseOnto != "origin/main" {
		t.Errorf("expected rebase target origin/main, got %q", row.RebaseOnto)
	}

	git(t, repo, "checkout", "-q", "auth-ui")
	git(t, repo, "rebase", "-q", "origin/main")

	if row := stackedRow(repo, stackedOnAuthAPI, merged); row.NeedsRebase {
		t.Fatalf("expected the flag to clear once auth-ui was rebased, got: %#v", row)
	}

	// Someone else lands something. Being behind main is not what this
	// flag reports, so it must stay clear.
	git(t, repo, "checkout", "-q", "main")
	commit(t, repo, "unrelated.go", "unrelated\n", "something else")
	git(t, repo, "push", "-q", "origin", "main")
	git(t, repo, "fetch", "-q", "origin")

	if row := stackedRow(repo, stackedOnAuthAPI, merged); row.NeedsRebase {
		t.Errorf("expected the flag to stay clear after origin/main moved on, got: %#v", row)
	}
}

func TestLocalRefsLeavesAnUnstackedBranchAlone(t *testing.T) {
	repo := squashMergedStack(t)

	if row := stackedRow(repo, "based on main", merged); row.NeedsRebase {
		t.Errorf("expected no flag for a branch that was never stacked, got: %#v", row)
	}
}

func TestLocalRefsLeavesAnUnmergedBaseAlone(t *testing.T) {
	repo := squashMergedStack(t)

	if row := stackedRow(repo, stackedOnAuthAPI, notMerged); row.NeedsRebase {
		t.Errorf("expected no flag while the base's pull request is still open, got: %#v", row)
	}
}

func TestLocalRefsLeavesABaseThatLandedAsItselfAlone(t *testing.T) {
	repo := squashMergedStack(t)
	// Undo the squash and merge auth-api into main under its own commits
	// instead, the way a merge commit or fast-forward would.
	git(t, repo, "checkout", "-q", "main")
	git(t, repo, "reset", "-q", "--hard", "origin/main~1")
	git(t, repo, "merge", "-q", "--no-ff", "-m", "merge auth-api", "auth-api")
	git(t, repo, "push", "-q", "--force", "origin", "main")
	git(t, repo, "fetch", "-q", "origin")

	if row := stackedRow(repo, stackedOnAuthAPI, merged); row.NeedsRebase {
		t.Errorf("expected no flag: auth-api's commits are on origin/main as themselves, so auth-ui isn't duplicating anything; got: %#v", row)
	}
}
