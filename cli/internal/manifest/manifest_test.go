package manifest_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/blockadence/archimedes/cli/internal/manifest"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "repos.yaml")
	content := `driver: openspec
repos:
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
    driver: pocock
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := manifest.Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if m.Driver != "openspec" {
		t.Errorf("instance-wide driver = %q, want %q", m.Driver, "openspec")
	}

	want := []manifest.Repo{
		{Name: "service-a", Path: "../service-a", BaseBranch: "main", DependsOn: []string{}},
		{
			Name:              "service-b",
			Path:              "../service-b",
			BaseBranch:        "develop",
			DependsOn:         []string{"service-a"},
			ContextModeledSHA: "abc123",
			Driver:            "pocock",
		},
	}
	if len(m.Repos) != len(want) {
		t.Fatalf("got %d repos, want %d", len(m.Repos), len(want))
	}
	for i := range want {
		if !reflect.DeepEqual(m.Repos[i], want[i]) {
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

func TestRepoPath(t *testing.T) {
	m := &manifest.Manifest{Repos: []manifest.Repo{
		{Name: "service-a", Path: "../service-a", BaseBranch: "main"},
	}}

	got, err := m.RepoPath("/instances/demo", "service-a")
	if err != nil {
		t.Fatalf("RepoPath returned error: %v", err)
	}
	if want := filepath.Join("/instances/demo", "../service-a"); got != want {
		t.Errorf("RepoPath() = %q, want %q", got, want)
	}
}

func TestRepoPathUnknownRepoErrors(t *testing.T) {
	m := &manifest.Manifest{Repos: []manifest.Repo{
		{Name: "service-a", Path: "../service-a", BaseBranch: "main"},
	}}

	if _, err := m.RepoPath("/instances/demo", "service-z"); err == nil {
		t.Fatal("expected error for unknown repo, got nil")
	}
}
