package instance_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
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

// fakeTemplate is the shape Create cares about: some data, a driver whose
// manifest names its command, and — beside it — a file the driver sources
// rather than runs.
func fakeTemplate() fstest.MapFS {
	return fstest.MapFS{
		"repos.yaml":               &fstest.MapFile{Data: []byte("repos: []\n")},
		".gitignore":               &fstest.MapFile{Data: []byte("/tmp/\n")},
		"work/.gitkeep":            &fstest.MapFile{},
		"drivers/README.md":        &fstest.MapFile{Data: []byte("# Drivers\n")},
		"drivers/demo/driver.yaml": &fstest.MapFile{Data: []byte("name: demo\noutput_mode: path-parameterized\ncommand: run.sh\n")},
		"drivers/demo/run.sh":      &fstest.MapFile{Data: []byte("#!/usr/bin/env bash\necho hi\n")},
		"drivers/demo/lib.sh":      &fstest.MapFile{Data: []byte("# sourced, never run\n")},
		"drivers/notes/scratch.md": &fstest.MapFile{Data: []byte("not a driver\n")},
	}
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

func TestCreateMakesEachDriversCommandExecutable(t *testing.T) {
	testrepo.IsolateGit(t)

	dest, err := instance.Create(fakeTemplate(), "widgets", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	assertExecutable(t, filepath.Join(dest, "drivers/demo/run.sh"), true)
	// The mode is restored from what a driver's manifest names, not from
	// what a filename looks like: a driver's sourced helper is not a
	// program and must not be turned into one.
	assertExecutable(t, filepath.Join(dest, "drivers/demo/lib.sh"), false)
	assertExecutable(t, filepath.Join(dest, "repos.yaml"), false)
	// A directory under drivers/ that declares no driver is carried like
	// anything else rather than treated as a broken one.
	assertExecutable(t, filepath.Join(dest, "drivers/notes/scratch.md"), false)
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
	broken := fakeTemplate()
	broken["drivers/demo/driver.yaml"] = &fstest.MapFile{Data: []byte("name: [unterminated\n")}

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

	// Every driver that ships is runnable, and nothing else in the instance
	// is executable at all — the guarantee the scaffolding has always made.
	commands := driverCommands(t, dest)
	if len(commands) == 0 {
		t.Fatal("the template shipped no drivers")
	}
	for _, cmd := range commands {
		assertExecutable(t, cmd, true)
	}
	for _, stray := range executableFiles(t, dest) {
		if !slices.Contains(commands, stray) {
			t.Errorf("%s is executable but is no driver's command", stray)
		}
	}
}

func assertExecutable(t *testing.T, path string, want bool) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode()&0o111 != 0; got != want {
		t.Errorf("%s executable = %v, want %v (mode %v)", path, got, want, info.Mode())
	}
}

// driverCommands returns the absolute path of the command each driver in
// the instance declares.
func driverCommands(t *testing.T, dest string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dest, "drivers"))
	if err != nil {
		t.Fatal(err)
	}
	var cmds []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dest, "drivers", e.Name(), "driver.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(data), "\n") {
			if rest, ok := strings.CutPrefix(line, "command:"); ok {
				cmds = append(cmds, filepath.Join(dest, "drivers", e.Name(), strings.TrimSpace(rest)))
			}
		}
	}
	return cmds
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
