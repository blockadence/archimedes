package instance_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/blockadence/archimedes"
	"github.com/blockadence/archimedes/internal/instance"
	"github.com/blockadence/archimedes/internal/testrepo"
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

	dest, err := instance.Create(fakeTemplate(), "widgets", parent)
	if err != nil {
		t.Fatal(err)
	}

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

	dest, err := instance.Create(fakeTemplate(), "widgets", parent)
	if err != nil {
		t.Fatal(err)
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

func TestCreateScaffoldsTheTemplateThisRepoShips(t *testing.T) {
	testrepo.IsolateGit(t)

	dest, err := instance.Create(archimedes.Template(), "widgets", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

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
