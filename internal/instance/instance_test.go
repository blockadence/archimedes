package instance_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/blockadence/gh-archimedes"
	"github.com/blockadence/gh-archimedes/internal/instance"
	"github.com/blockadence/gh-archimedes/internal/testrepo"
)

// Create's caller is a person on a machine that may have no clone of this
// repo, so the fixtures here are filesystems rather than checkouts: a small
// hand-built one for the rules Create enforces, and the real embedded
// template for the promise that what it produces is a working instance.

// fakeTemplate is the shape Create cares about: data, a dotfile, and the
// empty marker that gives an instance one of its directories.
func fakeTemplate() fstest.MapFS {
	return fstest.MapFS{
		"repos.yaml":        &fstest.MapFile{Data: []byte("repos: []\n")},
		".gitignore":        &fstest.MapFile{Data: []byte("/tmp/\n")},
		"work/.gitkeep":     &fstest.MapFile{},
		"drivers/README.md": &fstest.MapFile{Data: []byte("# Drivers\n")},
	}
}

// unreadable is a template with one file that cannot be read: a failure
// part-way through writing an instance, which is the only interesting one
// — everything before it has already landed on disk.
type unreadable struct {
	fsys fstest.MapFS
	path string
}

func (u unreadable) Open(name string) (fs.File, error) {
	if name == u.path {
		return nil, errors.New("simulated I/O failure")
	}
	return u.fsys.Open(name)
}

func TestCreateWritesTheTemplateTreeIntoANewDirectory(t *testing.T) {
	testrepo.IsolateGit(t)
	parent := t.TempDir()

	res, err := instance.Create(fakeTemplate(), "widgets", parent)
	if err != nil {
		t.Fatal(err)
	}
	dest := res.Path

	if want := filepath.Join(parent, "widgets"); dest != want {
		t.Errorf("dest = %q, want %q", dest, want)
	}
	for path, want := range map[string]string{
		"repos.yaml":        "repos: []\n",
		".gitignore":        "/tmp/\n",
		"drivers/README.md": "# Drivers\n",
	} {
		got, err := os.ReadFile(filepath.Join(dest, path))
		if err != nil {
			t.Errorf("reading %s: %v", path, err)
			continue
		}
		if string(got) != want {
			t.Errorf("%s = %q, want %q", path, got, want)
		}
	}
	if _, err := os.Stat(filepath.Join(dest, "work", ".gitkeep")); err != nil {
		t.Errorf("the empty marker that gives the instance its work/ is missing: %v", err)
	}
}

func TestCreateStartsTheInstanceOnItsOwnFreshHistory(t *testing.T) {
	testrepo.IsolateGit(t)
	parent := t.TempDir()

	res, err := instance.Create(fakeTemplate(), "widgets", parent)
	if err != nil {
		t.Fatal(err)
	}
	dest := res.Path

	if !res.Committed {
		t.Error("Committed = false on a machine that has an identity to commit under")
	}
	if got := testrepo.GitOut(t, dest, "rev-list", "--count", "HEAD"); got != "1" {
		t.Errorf("commit count = %s, want 1: an instance starts on its own history", got)
	}
	if got := testrepo.GitOut(t, dest, "log", "-1", "--format=%s"); !strings.Contains(got, "widgets") {
		t.Errorf("commit subject = %q, want it to name the instance", got)
	}
	// Everything the template carried is in that commit, dotfiles included.
	if got := testrepo.GitOut(t, dest, "status", "--porcelain"); got != "" {
		t.Errorf("a fresh instance is not clean:\n%s", got)
	}
}

// The machine the tool meets first: a fresh laptop, a container, a CI
// runner, where git has no identity and every commit dies with "Please tell
// me who you are". The files are the valuable half of what init does, and
// they land; the commit is the half that needs an operator git does not
// have yet, and it waits for them.
func TestCreateScaffoldsWithoutCommittingWhenGitHasNoIdentity(t *testing.T) {
	testrepo.StripGitIdentity(t)
	parent := t.TempDir()

	res, err := instance.Create(fakeTemplate(), "widgets", parent)
	if err != nil {
		t.Fatalf("no identity is not a failure, it is a machine: %v", err)
	}

	if res.Committed {
		t.Error("Committed = true, but there was no identity to commit under")
	}
	// The instance itself is all there. A caller told the operator this
	// worked, so it has to have.
	for _, path := range []string{"repos.yaml", ".gitignore", "drivers/README.md", "work/.gitkeep"} {
		if _, err := os.Stat(filepath.Join(res.Path, path)); err != nil {
			t.Errorf("the instance is missing %s: %v", path, err)
		}
	}
	// git init still ran, so the operator's own first commit is one command
	// away rather than two.
	if _, err := os.Stat(filepath.Join(res.Path, ".git")); err != nil {
		t.Errorf("the instance is not a git repository: %v", err)
	}
	if out := testrepo.GitOut(t, res.Path, "rev-list", "--all", "--count"); out != "0" {
		t.Errorf("commit count = %s, want 0: nothing may be committed without an identity", out)
	}

	// Nothing is staged either. What the operator is told to run is `git add
	// -A && git commit`, and a half-staged index would make that line quietly
	// wrong about what it was committing.
	if out := testrepo.GitOut(t, res.Path, "diff", "--cached", "--name-only"); out != "" {
		t.Errorf("the index is not empty:\n%s", out)
	}
}

// The second criterion of the issue this came from, asserted rather than
// argued: an instance is the operator's own repository and its first commit
// is in its history forever, so no identity of ours may ever appear in one.
func TestCreateNeverCommitsUnderAnIdentityItInvented(t *testing.T) {
	testrepo.StripGitIdentity(t)

	res, err := instance.Create(fakeTemplate(), "widgets", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	if out := testrepo.GitOut(t, res.Path, "log", "--all", "--format=%an <%ae>"); out != "" {
		t.Errorf("an instance carries an author nobody configured: %s", out)
	}
}

func TestCreateRefusesADestinationThatAlreadyExists(t *testing.T) {
	testrepo.IsolateGit(t)
	parent := t.TempDir()
	existing := filepath.Join(parent, "widgets")
	if err := os.MkdirAll(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(existing, "notes.md")
	if err := os.WriteFile(keep, []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := instance.Create(fakeTemplate(), "widgets", parent); err == nil {
		t.Fatal("expected an error, got none")
	}

	// Refusing is only worth anything if it refuses before writing.
	got, err := os.ReadFile(keep)
	if err != nil || string(got) != "mine\n" {
		t.Errorf("the existing directory was disturbed: %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(existing, "repos.yaml")); !os.IsNotExist(err) {
		t.Error("the template was written into a directory Create said it would not touch")
	}
}

func TestCreateLeavesNothingBehindWhenItFails(t *testing.T) {
	testrepo.IsolateGit(t)
	parent := t.TempDir()
	broken := unreadable{fsys: fakeTemplate(), path: "work/.gitkeep"}

	if _, err := instance.Create(broken, "widgets", parent); err == nil {
		t.Fatal("expected an error, got none")
	}

	// A half-written instance would make the retry fail for the wrong
	// reason — "already exists" instead of what actually went wrong.
	if _, err := os.Stat(filepath.Join(parent, "widgets")); !os.IsNotExist(err) {
		t.Errorf("a partial instance was left behind: %v", err)
	}
}

// The other half of the cleanup path, and the half no test reached: a git
// step that genuinely fails, after `git init` has already made dest a
// repository. It is neither of the two cases beside it — materialize failing
// happens before git is involved at all, and a missing identity is now a
// success — so it is the one a future edit could quietly turn into a partial
// instance while both of those kept passing.
func TestCreateLeavesNothingBehindWhenAGitStepFails(t *testing.T) {
	testrepo.IsolateGit(t)
	refuseCommits(t)
	parent := t.TempDir()

	if _, err := instance.Create(fakeTemplate(), "widgets", parent); err == nil {
		t.Fatal("expected an error, got none")
	}

	if _, err := os.Stat(filepath.Join(parent, "widgets")); !os.IsNotExist(err) {
		t.Errorf("a partial instance was left behind: %v", err)
	}
}

// refuseCommits makes `git commit` fail wherever this test runs one, by way
// of a pre-commit hook that says no. It leaves the identity alone on
// purpose: the failure being staged is the one that survives a machine with
// everything configured, so that a skipped commit and a failed one stay
// visibly different things.
func refuseCommits(t *testing.T) {
	t.Helper()
	hooks := t.TempDir()
	if err := os.WriteFile(filepath.Join(hooks, "pre-commit"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Layered over the config IsolateGit already wrote rather than replacing
	// it, which is what this environment triple is for.
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "core.hooksPath")
	t.Setenv("GIT_CONFIG_VALUE_0", hooks)
}

func TestCreateScaffoldsTheTemplateThisRepoShips(t *testing.T) {
	testrepo.IsolateGit(t)

	res, err := instance.Create(archimedes.Template(), "widgets", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dest := res.Path

	for _, path := range []string{"repos.yaml", "WORKSPACE-MAP.md", "AGENTS.md", ".gitignore"} {
		if _, err := os.Stat(filepath.Join(dest, path)); err != nil {
			t.Errorf("the instance is missing %s: %v", path, err)
		}
	}
	for _, dir := range []string{"repos", "work", "drivers", "scaffolding", "convention-packs"} {
		info, err := os.Stat(filepath.Join(dest, dir))
		if err != nil || !info.IsDir() {
			t.Errorf("the instance is missing %s/: %v", dir, err)
		}
	}
	if got := testrepo.GitOut(t, dest, "status", "--porcelain"); got != "" {
		t.Errorf("a fresh instance is not clean:\n%s", got)
	}

	// Nothing in an instance is executable, because nothing in an instance
	// is a program: the drivers ride in the binary, and everything here is
	// data. This is the guarantee that decays quietly — one convenience
	// script back in the template and instances start carrying tooling
	// again, which is the thing that had to be retired for a fix to be able
	// to reach them at all.
	if stray := executableFiles(t, dest); len(stray) != 0 {
		t.Errorf("a fresh instance carries executable files: %v", stray)
	}

	// Its drivers/ is the operator's own empty shelf, ready for a driver
	// they write or adopt.
	entries, err := os.ReadDir(filepath.Join(dest, "drivers"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() {
			t.Errorf("a fresh instance already holds a driver (%s)", e.Name())
		}
	}
}

func executableFiles(t *testing.T, dest string) []string {
	t.Helper()
	var found []string
	err := filepath.WalkDir(dest, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&0o111 != 0 {
			found = append(found, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}
