// Package testrepo builds the throwaway git repository nearly every test in
// this module needs: a bare "origin" plus a clone of it carrying one commit,
// already pushed to the base branch. That shape is what makes the fetch,
// rev-parse origin/<base>, worktree, and push paths testable against real
// git without touching the network. Two variants cover what that shape
// can't: Reclone, a second checkout of one origin, for staging a remote
// that has moved on behind a stale one; and Init, a repository with no
// origin, for the code paths that only read the checkout in front of them.
// Every one of them carries a fixed commit identity, so no test has to
// spell one out to commit; IsolateGit gives the same guarantee to a test
// whose subject builds the repository itself, where there is no fixture
// checkout to configure.
//
// It also provides the runner that building such a fixture needs anyway —
// Git and GitOut, which run one git command and fail the test if it doesn't
// succeed — so a test package that drives git a step further than New does
// has one place to get that from rather than its own copy.
//
// It is test-only scaffolding. Nothing under cmd/ or the production side of
// internal/ may import it — see the guard test in this package.
//
// Like the fixtures it replaces, it drives git directly rather than through
// internal/gitutil, so a bug in gitutil can't hide itself by also breaking
// the fixture.
package testrepo

import (
	"cmp"
	"os"
	"path/filepath"
	"testing"
)

// defaultBranch is the base branch a repo gets when a Spec doesn't name one.
const defaultBranch = "main"

// Spec describes the repository to build. Only Dir and Name are required;
// everything else has a default that suits the common case.
type Spec struct {
	// Dir is the directory both halves are created under. Required.
	Dir string

	// Name is the repository's name, and the basename of both halves.
	// Required.
	Name string

	// Clone overrides the clone's directory name within Dir. Defaults to
	// Name. Set it when the code under test is itself going to clone into
	// Dir/Name and must not find anything already there.
	Clone string

	// Origin overrides the bare repo's directory name within Dir.
	// Defaults to Name + ".git".
	Origin string

	// Branch is the branch the initial commit lands on, in the origin as
	// well as the clone. Defaults to "main".
	Branch string

	// Files are the seed files of the initial commit, keyed by path
	// relative to the clone. Defaults to a single README.md. Parent
	// directories are created as needed.
	Files map[string]string
}

// Repo is a built fixture: where its two halves live, and the branch they
// share.
type Repo struct {
	// Origin is the path of the bare repo the clone's origin remote points at.
	Origin string
	// Clone is the path of the working checkout.
	Clone string
	// Branch is the base branch the initial commit is on.
	Branch string
}

// New builds the repository spec describes and returns where it landed,
// failing the test if any git step does.
func New(t testing.TB, spec Spec) Repo {
	t.Helper()

	if spec.Dir == "" || spec.Name == "" {
		t.Fatalf("testrepo.New: Spec needs both Dir and Name, got %+v", spec)
	}

	repo := Repo{
		Origin: filepath.Join(spec.Dir, cmp.Or(spec.Origin, spec.Name+".git")),
		Clone:  filepath.Join(spec.Dir, cmp.Or(spec.Clone, spec.Name)),
		Branch: cmp.Or(spec.Branch, defaultBranch),
	}
	if err := os.MkdirAll(spec.Dir, 0o755); err != nil {
		t.Fatal(err)
	}

	Git(t, "", "init", "-q", "--bare", "-b", repo.Branch, repo.Origin)
	Git(t, "", "clone", "-q", repo.Origin, repo.Clone)
	configureIdentity(t, repo.Clone)

	seed(t, repo.Clone, spec.Name, spec.Files)
	Git(t, repo.Clone, "push", "-q", "origin", repo.Branch)

	return repo
}

// Init builds a repository with no origin at all: a checkout at path, on
// the default branch, carrying one seed commit and the fixed identity. Use
// it where the code under test only reads and writes the checkout in front
// of it, so a bare origin it never fetches from or pushes to would be
// nothing but a slower fixture. The directory is created if it isn't there.
//
// It hands back the path it was given rather than a Repo, so it can be
// used inline where one is wanted. A Repo would have to carry an empty
// Origin, and the half of Repo's interface that reads that field — Reclone
// above all — has no answer for a repository that was never cloned.
func Init(t testing.TB, path string) string {
	t.Helper()

	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	Git(t, path, "init", "-q", "-b", defaultBranch)
	configureIdentity(t, path)
	// Seeded, because the branch and worktree commands a test reaches for
	// next have nothing to start from until HEAD exists.
	seed(t, path, filepath.Base(path), nil)

	return path
}

// Reclone builds a second working checkout of r's origin, named name and
// sitting beside the origin, and returns it as a fixture in its own right.
// It is how a test moves the remote on behind a stale checkout's back: the
// state a fetch-first code path exists to cope with, which needs two
// checkouts of one origin and can't be staged with a single one.
func (r Repo) Reclone(t testing.TB, name string) Repo {
	t.Helper()

	other := Repo{
		Origin: r.Origin,
		Clone:  filepath.Join(filepath.Dir(r.Origin), name),
		Branch: r.Branch,
	}
	Git(t, "", "clone", "-q", other.Origin, other.Clone)
	configureIdentity(t, other.Clone)

	return other
}

// seed writes files into the checkout at dir and makes them its first
// commit, defaulting to a lone README.md named after the repository when
// the caller has no files of its own to plant.
func seed(t testing.TB, dir, name string, files map[string]string) {
	t.Helper()

	if len(files) == 0 {
		files = map[string]string{"README.md": "# " + name + "\n"}
	}
	for file, content := range files {
		path := filepath.Join(dir, file)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	Git(t, dir, "add", "-A")
	Git(t, dir, "commit", "-q", "-m", "init")
}

// configureIdentity gives the checkout at dir a fixed commit identity and
// no signing, so committing there — in the fixture's own steps and in
// whatever the test commits on top — doesn't depend on the machine's git
// config.
func configureIdentity(t testing.TB, dir string) {
	t.Helper()
	Git(t, dir, "config", "user.email", "t@t")
	Git(t, dir, "config", "user.name", "t")
	Git(t, dir, "config", "commit.gpgsign", "false")
}

// IsolateGit is the same guarantee for the tests configureIdentity can't
// reach: those whose subject creates the repository itself, so there is no
// checkout to configure before the code under test commits in it. It gives
// git a global config of this test's own, holding an identity and nothing
// else — which is also what keeps a machine's commit signing out of the way,
// since replacing the global config replaces that too.
//
// Prefer the fixtures above wherever the test owns the repository. Reach for
// this only when it doesn't.
func IsolateGit(t testing.TB) {
	t.Helper()
	cfg := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(cfg, []byte("[user]\n\tname = t\n\temail = t@t\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
}
