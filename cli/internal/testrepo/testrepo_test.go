package testrepo_test

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/blockadence/archimedes/cli/internal/testrepo"
)

const modulePath = "github.com/blockadence/archimedes/cli"

// moduleRoot locates the module from this file's own path, so the guard
// below doesn't depend on the working directory the test binary is run in.
func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate this test's own source file")
	}
	return filepath.Join(filepath.Dir(file), "..", "..")
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v (in %s): %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestNewBuildsACloneOfABareOriginWithOneCommitPushed(t *testing.T) {
	dir := t.TempDir()

	repo := testrepo.New(t, testrepo.Spec{Dir: dir, Name: "app"})

	if want := filepath.Join(dir, "app"); repo.Clone != want {
		t.Errorf("Clone = %q, want %q", repo.Clone, want)
	}
	if want := filepath.Join(dir, "app.git"); repo.Origin != want {
		t.Errorf("Origin = %q, want %q", repo.Origin, want)
	}
	if repo.Branch != "main" {
		t.Errorf("Branch = %q, want the default main", repo.Branch)
	}
	if got := git(t, repo.Origin, "rev-parse", "--is-bare-repository"); got != "true" {
		t.Errorf("origin is-bare-repository = %q, want true", got)
	}
	if got := git(t, repo.Clone, "rev-parse", "--abbrev-ref", "HEAD"); got != "main" {
		t.Errorf("clone is on %q, want main", got)
	}
	// The commit is on the remote, not just locally: the pass under test
	// is usually fetching and diffing against origin/<base branch>.
	if local, remote := git(t, repo.Clone, "rev-parse", "HEAD"), git(t, repo.Clone, "rev-parse", "origin/main"); local != remote {
		t.Errorf("HEAD %s != origin/main %s, want the initial commit pushed", local, remote)
	}
	if got := git(t, repo.Clone, "show", "--name-only", "--format=", "HEAD"); got != "README.md" {
		t.Errorf("initial commit touched %q, want a README.md seed", got)
	}
	if got := git(t, repo.Clone, "status", "--porcelain"); got != "" {
		t.Errorf("clone has uncommitted changes:\n%s", got)
	}
}

func TestNewHonorsTheBranchTheSpecAsksFor(t *testing.T) {
	dir := t.TempDir()

	repo := testrepo.New(t, testrepo.Spec{Dir: dir, Name: "app", Branch: "trunk"})

	if repo.Branch != "trunk" {
		t.Errorf("Branch = %q, want trunk", repo.Branch)
	}
	if got := git(t, repo.Clone, "rev-parse", "--abbrev-ref", "HEAD"); got != "trunk" {
		t.Errorf("clone is on %q, want trunk", got)
	}
	if got := git(t, repo.Origin, "rev-parse", "--abbrev-ref", "HEAD"); got != "trunk" {
		t.Errorf("origin HEAD is %q, want trunk", got)
	}
}

func TestNewSeedsTheFilesTheSpecNames(t *testing.T) {
	dir := t.TempDir()

	repo := testrepo.New(t, testrepo.Spec{Dir: dir, Name: "app", Files: map[string]string{
		".gitignore":  "*.log\n",
		"docs/why.md": "because\n",
	}})

	tracked := strings.Split(git(t, repo.Clone, "ls-tree", "-r", "--name-only", "HEAD"), "\n")
	want := []string{".gitignore", "docs/why.md"}
	if len(tracked) != len(want) {
		t.Fatalf("tracked files = %v, want exactly %v", tracked, want)
	}
	for i, name := range want {
		if tracked[i] != name {
			t.Errorf("tracked files = %v, want %v", tracked, want)
		}
	}
	if got := git(t, repo.Clone, "show", "HEAD:docs/why.md"); got != "because" {
		t.Errorf("docs/why.md = %q, want the seeded content", got)
	}
}

func TestNewPutsEachHalfWhereTheSpecSays(t *testing.T) {
	dir := t.TempDir()

	// bootstrap's tests need a seed clone that does *not* sit at the path
	// the code under test is about to clone into.
	repo := testrepo.New(t, testrepo.Spec{Dir: dir, Name: "app", Clone: "app-seed", Origin: "app-origin.git"})

	if want := filepath.Join(dir, "app-seed"); repo.Clone != want {
		t.Errorf("Clone = %q, want %q", repo.Clone, want)
	}
	if want := filepath.Join(dir, "app-origin.git"); repo.Origin != want {
		t.Errorf("Origin = %q, want %q", repo.Origin, want)
	}
	if got := git(t, repo.Clone, "rev-parse", "--show-toplevel"); !strings.HasSuffix(got, "app-seed") {
		t.Errorf("clone toplevel = %q, want it at app-seed", got)
	}
}

func TestNewLeavesTheCloneCommittableWithoutTheMachinesGitConfig(t *testing.T) {
	dir := t.TempDir()

	repo := testrepo.New(t, testrepo.Spec{Dir: dir, Name: "app"})

	if got := git(t, repo.Clone, "log", "-1", "--format=%ae"); got != "t@t" {
		t.Errorf("initial commit author = %q, want the fixture's own identity", got)
	}
	// A test that commits more on top must not have to repeat -c user.email.
	git(t, repo.Clone, "commit", "-q", "--allow-empty", "-m", "more")
	if got := git(t, repo.Clone, "log", "-1", "--format=%ae"); got != "t@t" {
		t.Errorf("follow-up commit author = %q, want the identity the fixture configured", got)
	}
}

// The fixture is test-only scaffolding: no production package may end up
// depending on it, since that would pull `testing` into the shipped binary.
// cmd/archimedes is the module's only main package, so its dependency
// closure is the whole shipped surface.
func TestTheFixtureIsNotInTheProductionBinarysDependencies(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", modulePath+"/cmd/archimedes")
	cmd.Dir = moduleRoot(t)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -deps: %v", err)
	}
	for _, pkg := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if pkg == modulePath+"/internal/testrepo" || pkg == "testing" {
			t.Errorf("the archimedes binary depends on %s; the test fixture has leaked into production code", pkg)
		}
	}
}
