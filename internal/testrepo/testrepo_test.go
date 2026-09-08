package testrepo_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/blockadence/gh-archimedes/internal/testrepo"
)

const modulePath = "github.com/blockadence/gh-archimedes"

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
	if got := testrepo.GitOut(t, repo.Origin, "rev-parse", "--is-bare-repository"); got != "true" {
		t.Errorf("origin is-bare-repository = %q, want true", got)
	}
	if got := testrepo.GitOut(t, repo.Clone, "rev-parse", "--abbrev-ref", "HEAD"); got != "main" {
		t.Errorf("clone is on %q, want main", got)
	}
	// The commit is on the remote, not just locally: the pass under test
	// is usually fetching and diffing against origin/<base branch>.
	if local, remote := testrepo.GitOut(t, repo.Clone, "rev-parse", "HEAD"), testrepo.GitOut(t, repo.Clone, "rev-parse", "origin/main"); local != remote {
		t.Errorf("HEAD %s != origin/main %s, want the initial commit pushed", local, remote)
	}
	if got := testrepo.GitOut(t, repo.Clone, "show", "--name-only", "--format=", "HEAD"); got != "README.md" {
		t.Errorf("initial commit touched %q, want a README.md seed", got)
	}
	if got := testrepo.GitOut(t, repo.Clone, "status", "--porcelain"); got != "" {
		t.Errorf("clone has uncommitted changes:\n%s", got)
	}
}

func TestNewHonorsTheBranchTheSpecAsksFor(t *testing.T) {
	dir := t.TempDir()

	repo := testrepo.New(t, testrepo.Spec{Dir: dir, Name: "app", Branch: "trunk"})

	if repo.Branch != "trunk" {
		t.Errorf("Branch = %q, want trunk", repo.Branch)
	}
	if got := testrepo.GitOut(t, repo.Clone, "rev-parse", "--abbrev-ref", "HEAD"); got != "trunk" {
		t.Errorf("clone is on %q, want trunk", got)
	}
	if got := testrepo.GitOut(t, repo.Origin, "rev-parse", "--abbrev-ref", "HEAD"); got != "trunk" {
		t.Errorf("origin HEAD is %q, want trunk", got)
	}
}

func TestNewSeedsTheFilesTheSpecNames(t *testing.T) {
	dir := t.TempDir()

	repo := testrepo.New(t, testrepo.Spec{Dir: dir, Name: "app", Files: map[string]string{
		".gitignore":  "*.log\n",
		"docs/why.md": "because\n",
	}})

	tracked := strings.Split(testrepo.GitOut(t, repo.Clone, "ls-tree", "-r", "--name-only", "HEAD"), "\n")
	want := []string{".gitignore", "docs/why.md"}
	if len(tracked) != len(want) {
		t.Fatalf("tracked files = %v, want exactly %v", tracked, want)
	}
	for i, name := range want {
		if tracked[i] != name {
			t.Errorf("tracked files = %v, want %v", tracked, want)
		}
	}
	if got := testrepo.GitOut(t, repo.Clone, "show", "HEAD:docs/why.md"); got != "because" {
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
	if got := testrepo.GitOut(t, repo.Clone, "rev-parse", "--show-toplevel"); !strings.HasSuffix(got, "app-seed") {
		t.Errorf("clone toplevel = %q, want it at app-seed", got)
	}
}

func TestNewLeavesTheCloneCommittableWithoutTheMachinesGitConfig(t *testing.T) {
	dir := t.TempDir()

	repo := testrepo.New(t, testrepo.Spec{Dir: dir, Name: "app"})

	if got := testrepo.GitOut(t, repo.Clone, "log", "-1", "--format=%ae"); got != "t@t" {
		t.Errorf("initial commit author = %q, want the fixture's own identity", got)
	}
	// A test that commits more on top must not have to repeat -c user.email.
	testrepo.Git(t, repo.Clone, "commit", "-q", "--allow-empty", "-m", "more")
	if got := testrepo.GitOut(t, repo.Clone, "log", "-1", "--format=%ae"); got != "t@t" {
		t.Errorf("follow-up commit author = %q, want the identity the fixture configured", got)
	}
}

func TestRecloneBuildsASecondCheckoutOfTheSameOriginBesideIt(t *testing.T) {
	dir := t.TempDir()
	repo := testrepo.New(t, testrepo.Spec{Dir: dir, Name: "app"})

	other := repo.Reclone(t, "other")

	if want := filepath.Join(dir, "other"); other.Clone != want {
		t.Errorf("Clone = %q, want the second checkout beside the origin at %q", other.Clone, want)
	}
	if other.Origin != repo.Origin {
		t.Errorf("Origin = %q, want the origin it was cloned from, %q", other.Origin, repo.Origin)
	}
	if other.Branch != repo.Branch {
		t.Errorf("Branch = %q, want %q", other.Branch, repo.Branch)
	}
	if got, want := testrepo.GitOut(t, other.Clone, "rev-parse", "HEAD"), testrepo.GitOut(t, repo.Clone, "rev-parse", "HEAD"); got != want {
		t.Errorf("second checkout is at %s, want the origin's %s", got, want)
	}
}

func TestRecloneLeavesTheSecondCheckoutCommittableWithoutTheMachinesGitConfig(t *testing.T) {
	repo := testrepo.New(t, testrepo.Spec{Dir: t.TempDir(), Name: "app"})

	other := repo.Reclone(t, "other")

	// The point of a second checkout is committing on it, so it must carry
	// the identity without the test repeating -c user.email.
	testrepo.Git(t, other.Clone, "commit", "-q", "--allow-empty", "-m", "second")
	if got := testrepo.GitOut(t, other.Clone, "log", "-1", "--format=%ae"); got != "t@t" {
		t.Errorf("commit author = %q, want the identity the fixture configured", got)
	}
}

func TestInitBuildsACommittedRepoWithNoOrigin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repo")

	got := testrepo.Init(t, path)

	if got != path {
		t.Errorf("Init returned %q, want the path it was given, %q", got, path)
	}
	if got := testrepo.GitOut(t, path, "remote"); got != "" {
		t.Errorf("remotes = %q, want none: this shape is the repo that stands alone", got)
	}
	if got := testrepo.GitOut(t, path, "rev-parse", "--abbrev-ref", "HEAD"); got != "main" {
		t.Errorf("repo is on %q, want the default main", got)
	}
	// One commit, so a branch or worktree can be based on it — an empty
	// repo has no HEAD to start one from.
	if got := testrepo.GitOut(t, path, "rev-list", "--count", "HEAD"); got != "1" {
		t.Errorf("commit count = %q, want the single seed commit", got)
	}
	if got := testrepo.GitOut(t, path, "status", "--porcelain"); got != "" {
		t.Errorf("repo has uncommitted changes:\n%s", got)
	}
}

func TestInitLeavesTheRepoCommittableWithoutTheMachinesGitConfig(t *testing.T) {
	path := testrepo.Init(t, filepath.Join(t.TempDir(), "repo"))

	testrepo.Git(t, path, "commit", "-q", "--allow-empty", "-m", "more")

	if got := testrepo.GitOut(t, path, "log", "-1", "--format=%ae"); got != "t@t" {
		t.Errorf("commit author = %q, want the identity the fixture configured", got)
	}
}

// The guard the three tests above cannot make on their own. Each of them
// asserts the fixture's identity on a machine whose environment carries none
// — which is every machine anyone has run this suite on, and so a passing
// test that would go on passing if the fixture wrote no config at all. Git
// reads GIT_AUTHOR_NAME and friends ahead of every config file, so a shell
// or a CI job that exports one is a machine where the fixtures would commit
// as somebody real, in every repository the module's tests build, and
// nothing would say so.
//
// Every shape the package builds is here because the clearing lives in
// configureIdentity and all three call it: what this pins is that all three
// keep calling it.
func TestTheFixturesCommitAsThemselvesOnAMachineCarryingAnIdentityInItsEnvironment(t *testing.T) {
	tests := []struct {
		name  string
		build func(t *testing.T) string
	}{
		{"New", func(t *testing.T) string {
			return testrepo.New(t, testrepo.Spec{Dir: t.TempDir(), Name: "app"}).Clone
		}},
		{"Init", func(t *testing.T) string {
			return testrepo.Init(t, filepath.Join(t.TempDir(), "repo"))
		}},
		{"Reclone", func(t *testing.T) string {
			return testrepo.New(t, testrepo.Spec{Dir: t.TempDir(), Name: "app"}).Reclone(t, "other").Clone
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inheritAnIdentity(t)

			dir := tt.build(t)

			// The commit the fixture made on its way out. Reclone has no
			// seed of its own, so the one it checks out is New's — which is
			// the same claim about the same guard.
			if got := headIdentity(t, dir); got != fixtureIdentity {
				t.Errorf("seed commit identity = %q, want the fixture's own %q", got, fixtureIdentity)
			}
			// And whatever the test commits on top, which is the half a
			// `-c user.email=` inside the fixture would not have covered.
			testrepo.Git(t, dir, "commit", "-q", "--allow-empty", "-m", "more")
			if got := headIdentity(t, dir); got != fixtureIdentity {
				t.Errorf("follow-up commit identity = %q, want the fixture's own %q", got, fixtureIdentity)
			}
			// Cleared out of the environment, rather than overridden on the
			// fixture's own git commands: what a -c user.email= could not
			// have reached is the subprocesses a test spawns, which is where
			// a driver under test does its committing. The same assertion
			// tests/gitfixture_repos.sh makes of the bash twin.
			for _, name := range []string{"GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL"} {
				if value, ok := os.LookupEnv(name); ok {
					t.Errorf("%s is still set, to %q: the fixture overrode the environment rather than clearing it", name, value)
				}
			}
		})
	}
}

// inheritAnIdentity is the machine the guard above exists for: an ordinary
// developer's shell, or a CI job, that exported an identity the test binary
// then inherited. All four variables, because git takes the author from two
// of them and the committer from the other two, and a fixture that cleared
// half would leave half a stranger's name in the log. Named after
// tests/gitfixture_repos.sh's inherit_an_identity, which stages the same
// machine for the bash twin.
func inheritAnIdentity(t testing.TB) {
	t.Helper()
	for _, name := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(name, "inherited")
	}
	for _, name := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(name, "inherited@example.com")
	}
}

// fixtureIdentity is what headIdentity reads back from a repo committing
// under the identity configureIdentity sets.
const fixtureIdentity = "t <t@t> / t <t@t>"

// headIdentity is who git recorded HEAD as, author and committer both — the
// four fields the environment overrides, in one string so a failure names
// which of them the machine won.
func headIdentity(t testing.TB, dir string) string {
	t.Helper()
	return testrepo.GitOut(t, dir, "log", "-1", "--format=%an <%ae> / %cn <%ce>")
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
