package manifest_test

import (
	"os"
	"strings"
	"testing"

	"github.com/blockadence/gh-archimedes/internal/manifest"
)

func TestSetRepoFieldUpdatesExistingField(t *testing.T) {
	path := writeManifest(t, t.TempDir(), `repos:
  - name: service-a
    path: ../service-a
    base_branch: main
    context_modeled_sha: null
  - name: service-b
    path: ../service-b
    base_branch: main
    context_modeled_sha: oldsha
`)

	if err := manifest.SetRepoField(path, "service-b", "context_modeled_sha", "newsha"); err != nil {
		t.Fatalf("SetRepoField returned error: %v", err)
	}

	m, err := manifest.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := m.Find("service-b")
	if b.ContextModeledSHA != "newsha" {
		t.Errorf("service-b sha = %q, want %q", b.ContextModeledSHA, "newsha")
	}
	a, _ := m.Find("service-a")
	if a.ContextModeledSHA != "" {
		t.Errorf("service-a sha = %q, want it untouched", a.ContextModeledSHA)
	}
	if a.Path != "../service-a" {
		t.Errorf("service-a path = %q, want it untouched", a.Path)
	}
}

func TestSetRepoFieldAddsMissingField(t *testing.T) {
	path := writeManifest(t, t.TempDir(), `repos:
  - name: service-a
    path: ../service-a
    base_branch: main
`)

	if err := manifest.SetRepoField(path, "service-a", "context_modeled_sha", "abc123"); err != nil {
		t.Fatalf("SetRepoField returned error: %v", err)
	}

	m, err := manifest.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := m.Find("service-a")
	if a.ContextModeledSHA != "abc123" {
		t.Errorf("sha = %q, want %q", a.ContextModeledSHA, "abc123")
	}
}

// The instance's repos.yaml is a hand-edited file: it carries the
// instance-wide driver comment the template ships with, and operators add
// their own. Recording a mapped SHA must not eat them.
func TestSetRepoFieldPreservesComments(t *testing.T) {
	path := writeManifest(t, t.TempDir(), `# driver: <name>   # optional instance-wide default driver
driver: openspec
repos:
  # the one everything else depends on
  - name: service-a
    path: ../service-a
    base_branch: main
    depends_on: []
    context_modeled_sha: null
`)

	if err := manifest.SetRepoField(path, "service-a", "context_modeled_sha", "abc123"); err != nil {
		t.Fatalf("SetRepoField returned error: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"# driver: <name>",
		"# the one everything else depends on",
		"driver: openspec",
		"context_modeled_sha: abc123",
	} {
		if !strings.Contains(string(got), want) {
			t.Errorf("rewritten manifest missing %q:\n%s", want, got)
		}
	}
}

func TestSetRepoFieldUnknownRepoErrors(t *testing.T) {
	path := writeManifest(t, t.TempDir(), "repos: []\n")

	if err := manifest.SetRepoField(path, "nope", "context_modeled_sha", "x"); err == nil {
		t.Fatal("expected an error for an unknown repo, got nil")
	}
}
