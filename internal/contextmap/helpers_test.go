package contextmap_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blockadence/archimedes/internal/manifest"
	"github.com/blockadence/archimedes/internal/testrepo"
)

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// instance is an Archimedes instance root plus the sibling target repos its
// repos.yaml points at, the "../<name>" layout bootstrap produces.
type instance struct {
	root       string
	repos      map[string]testrepo.Repo
	driversDir string
}

// newInstance clones one target repo per name and writes a repos.yaml with
// the given body (the caller owns its shape, since driver selection and
// depends_on are exactly what's under test).
func newInstance(t *testing.T, reposYAML string, names ...string) instance {
	t.Helper()
	tmp := t.TempDir()

	inst := instance{
		root:       filepath.Join(tmp, "instance"),
		repos:      map[string]testrepo.Repo{},
		driversDir: filepath.Join(tmp, "instance", "drivers"),
	}
	// A real bare "origin" plus a clone per repo, so the mapping pass's
	// fetch + rev-parse origin/main have remote state to read.
	for _, name := range names {
		inst.repos[name] = testrepo.New(t, testrepo.Spec{Dir: tmp, Name: name})
	}
	mustWriteFile(t, filepath.Join(inst.root, "repos.yaml"), reposYAML)

	return inst
}

// repoPath is where the named target repo is checked out.
func (i instance) repoPath(name string) string {
	return i.repos[name].Clone
}

// installDriver writes a driver manifest and executable under the
// instance's drivers/ directory.
func (i instance) installDriver(t *testing.T, name, manifestBody, command string) {
	t.Helper()
	mustWriteFile(t, filepath.Join(i.driversDir, name, "driver.yaml"), manifestBody)
	path := filepath.Join(i.driversDir, name, "run.sh")
	mustWriteFile(t, path, command)
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

// installPathDriver installs a well-behaved path-parameterized driver that
// records which driver ran, so tests can tell two configured drivers apart.
func (i instance) installPathDriver(t *testing.T, name string) {
	t.Helper()
	i.installDriver(t, name,
		"name: "+name+"\noutput_mode: path-parameterized\ncommand: run.sh\n",
		"#!/usr/bin/env bash\nset -euo pipefail\necho \""+name+" mapped $1\" > \"$2\"\n")
}

// sha reads the base-branch commit a repo's origin currently points at.
func (i instance) sha(t *testing.T, name string) string {
	t.Helper()
	return testrepo.GitOut(t, i.repoPath(name), "rev-parse", "origin/main")
}

// recordedSHA reads back what repos.yaml says a repo was last mapped at.
func (i instance) recordedSHA(t *testing.T, name string) string {
	t.Helper()
	m, err := manifest.Load(filepath.Join(i.root, "repos.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	r, ok := m.Find(name)
	if !ok {
		t.Fatalf("repo %s vanished from repos.yaml", name)
	}
	return r.ContextModeledSHA
}

func (i instance) contextFile(name string) string {
	return filepath.Join(i.repoPath(name), "CONTEXT.md")
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(data)
}

func assertNoFile(t *testing.T, path, what string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Errorf("%s: %s exists but should not", what, path)
	}
}

// setSHA records a repo as having been mapped at sha, the bookkeeping a
// previous pass would have left behind.
func setSHA(t *testing.T, i instance, name, sha string) {
	t.Helper()
	if err := manifest.SetRepoField(filepath.Join(i.root, "repos.yaml"), name, "context_modeled_sha", sha); err != nil {
		t.Fatal(err)
	}
}
