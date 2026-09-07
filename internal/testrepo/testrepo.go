// Package testrepo builds the throwaway git repository nearly every test in
// this module needs: a bare "origin" plus a clone of it carrying one commit,
// already pushed to the base branch. That shape is what makes the fetch,
// rev-parse origin/<base>, worktree, and push paths testable against real
// git without touching the network.
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
	files := spec.Files
	if len(files) == 0 {
		files = map[string]string{"README.md": "# " + spec.Name + "\n"}
	}

	if err := os.MkdirAll(spec.Dir, 0o755); err != nil {
		t.Fatal(err)
	}

	Git(t, "", "init", "-q", "--bare", "-b", repo.Branch, repo.Origin)
	Git(t, "", "clone", "-q", repo.Origin, repo.Clone)
	// A fixed identity, and no signing, so committing here — now and in
	// whatever the test commits on top — doesn't depend on the machine's
	// git config.
	Git(t, repo.Clone, "config", "user.email", "t@t")
	Git(t, repo.Clone, "config", "user.name", "t")
	Git(t, repo.Clone, "config", "commit.gpgsign", "false")

	for name, content := range files {
		path := filepath.Join(repo.Clone, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	Git(t, repo.Clone, "add", "-A")
	Git(t, repo.Clone, "commit", "-q", "-m", "init")
	Git(t, repo.Clone, "push", "-q", "origin", repo.Branch)

	return repo
}
