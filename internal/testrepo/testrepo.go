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
// checkout to configure. StripGitIdentity and UnconfigureGitIdentity are the
// deliberate absence of one, for the tests whose subject is a machine that
// has not got an identity: the first where git itself will not commit, the
// second where nothing is configured but git will guess.
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
	// The environment outranks every config file, so an identity the test
	// binary inherited would beat the one just written and make this a
	// no-op. Setenv first and Unsetenv second: Setenv is what registers the
	// restore at the end of the test, and Unsetenv is what Setenv cannot do.
	for _, name := range []string{"GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(name, "")
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}
}

// StripGitIdentity is IsolateGit's opposite, for the tests whose subject is
// the machine where git itself will not commit: a fresh container, a CI
// runner, where any commit dies with "empty ident name ... not allowed". It
// is a fixture in its own right because that machine is the one no developer
// has — the identity in a developer's own git config is exactly what hid
// this case until a runner without one ran the suite.
//
// The name is emptied rather than unset, and that is what makes this that
// machine on every machine. Unset, git derives a name from the account, and
// whether the derivation comes back usable is a property of the box: empty
// on a CI runner, a full name on a developer's macOS one. An empty
// GIT_AUTHOR_NAME reaches git's own refusal by the route the runner takes,
// everywhere. For the other machine with nothing configured — the one where
// git guesses and commits — see UnconfigureGitIdentity.
//
// Only the names, deliberately, though IsolateGit clears the addresses too.
// That asymmetry is the runner: git derives an address from the account and
// the login it finds there is a real one, so what a machine with no identity
// actually has is the shape in git's own complaint — "empty ident name (for
// <runner@...>)". Emptying the addresses as well would test a machine that
// does not exist.
func StripGitIdentity(t testing.TB) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	for _, name := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(name, "")
	}
}

// RefusedCommitMessage is what the hook RefuseCommits installs writes on its
// way out. It is exported because the tests that use that fixture assert
// git's words reached an operator, and a copy of the string in each of them
// would be two things to keep in step instead of one.
const RefusedCommitMessage = "pre-commit hook said no"

// RefuseCommits is the machine none of the three above are: an identity
// configured, correct, and git's to use, and git refusing the commit all the
// same. What operators actually meet there is commit signing configured with
// no key that works on the box — a dotfile copied to a new laptop, a
// container with no keyring — which a test cannot stage without a gpg to
// break, so a `pre-commit` hook that says no stands in for it. Any of them
// reaches the code under test the same way: git was asked, and would not.
//
// It layers one setting over whatever config the caller already arranged
// rather than replacing it, which is what git's environment triple is for,
// so it composes with IsolateGit and must be called after it — the identity
// is what makes this machine the one it is. It owns that triple for the
// length of the test; a caller with a second setting to layer has to write
// both itself.
func RefuseCommits(t testing.TB) {
	t.Helper()
	hooks := t.TempDir()
	hook := "#!/bin/sh\necho '" + RefusedCommitMessage + "' >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(hooks, "pre-commit"), []byte(hook), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "core.hooksPath")
	t.Setenv("GIT_CONFIG_VALUE_0", hooks)
}

// UnconfigureGitIdentity is the machine between those two: nothing
// configured anywhere, and git left free to guess an identity from the OS
// account the way it does in any repository on that box. It is the machine
// an operator who has never run `git config --global user.name` is actually
// on, which is most of them, and it is the fixture for anything whose answer
// must not depend on what the account happens to carry.
//
// What the guess comes back with is a property of the box — a full name on a
// developer's macOS one, nothing on a CI runner — so git will commit here in
// one place and refuse in the other. A test using this fixture asserts what
// is true of both: that an identity nobody chose is not one to commit an
// operator's repository under. See gitutil.HasConfiguredIdentity.
//
// Nothing is emptied here, only removed, EMAIL included — git reads that one
// as a configured address in its own right, so leaving a machine's EMAIL
// standing would make this fixture a different machine on that machine.
// Emptying is StripGitIdentity's job, and makes a different machine again.
func UnconfigureGitIdentity(t testing.TB) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	// Setenv first and Unsetenv second, as in IsolateGit: Setenv is what
	// registers the restore at the end of the test, and Unsetenv is what
	// Setenv cannot do. Unset is what these have to be, since an empty
	// GIT_AUTHOR_NAME is the other machine.
	for _, name := range []string{"GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL", "EMAIL"} {
		t.Setenv(name, "")
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}
}
