package manifest_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blockadence/archimedes/cli/internal/manifest"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "repos.yaml")
	content := `repos:
  - name: service-a
    path: ../service-a
    base_branch: main
    depends_on: []
    context_modeled_sha: null
  - name: service-b
    path: ../service-b
    base_branch: develop
    depends_on: [service-a]
    context_modeled_sha: abc123
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := manifest.Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	want := []manifest.Repo{
		{Name: "service-a", Path: "../service-a", BaseBranch: "main"},
		{Name: "service-b", Path: "../service-b", BaseBranch: "develop"},
	}
	if len(m.Repos) != len(want) {
		t.Fatalf("got %d repos, want %d", len(m.Repos), len(want))
	}
	for i := range want {
		if m.Repos[i] != want[i] {
			t.Errorf("repo %d: got %+v, want %+v", i, m.Repos[i], want[i])
		}
	}
}

func TestLoadEmptyRepos(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "repos.yaml")
	if err := os.WriteFile(path, []byte("repos: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := manifest.Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(m.Repos) != 0 {
		t.Fatalf("got %d repos, want 0", len(m.Repos))
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := manifest.Load(filepath.Join(t.TempDir(), "missing.yaml"))
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

// loadSample writes a two-repo repos.yaml and loads it.
func loadSample(t *testing.T) *manifest.Manifest {
	t.Helper()
	path := filepath.Join(t.TempDir(), "repos.yaml")
	content := "repos:\n  - name: service-a\n    path: repos/service-a\n    base_branch: main\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := manifest.Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	return m
}

func TestResolve(t *testing.T) {
	got, err := loadSample(t).Resolve("/instances/demo", "service-a")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if want := "/instances/demo/repos/service-a"; got.Path != want {
		t.Errorf("Resolve().Path = %q, want %q", got.Path, want)
	}
	if got.BaseBranch != "main" {
		t.Errorf("Resolve().BaseBranch = %q, want %q", got.BaseBranch, "main")
	}
}

func TestResolveUnknownRepoErrors(t *testing.T) {
	if _, err := loadSample(t).Resolve("/instances/demo", "service-z"); err == nil {
		t.Fatal("expected error for unknown repo, got nil")
	}
}
