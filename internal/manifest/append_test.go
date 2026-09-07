package manifest_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blockadence/gh-archimedes/internal/manifest"
)

func writeManifest(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "repos.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// The scaffolded entry spells out every per-repo knob, including the ones
// nothing sets yet: an operator declaring a convention pack or a driver
// edits a key that's already there rather than having to remember its name.
func TestAppendRepoScaffoldsEveryPerRepoField(t *testing.T) {
	path := writeManifest(t, t.TempDir(), "repos: []\n")

	added, err := manifest.AppendRepo(path, manifest.Repo{Name: "service-a", Path: "../service-a", BaseBranch: "main"})
	if err != nil {
		t.Fatalf("AppendRepo: %v", err)
	}
	if !added {
		t.Error("AppendRepo reported no addition for a repo not yet listed")
	}

	want := `repos:
  - name: service-a
    path: ../service-a
    base_branch: main
    depends_on: []
    context_modeled_sha: null
    convention_pack: null
    driver: null
`
	if got := readFile(t, path); got != want {
		t.Errorf("repos.yaml mismatch\n got: %q\nwant: %q", got, want)
	}
}

// repos.yaml carries hand-written commentary about the fields it holds.
// Rewriting the file to add an entry must not cost the operator that.
func TestAppendRepoPreservesComments(t *testing.T) {
	path := writeManifest(t, t.TempDir(), "# instance-wide default driver goes here\nrepos: []\n")

	if _, err := manifest.AppendRepo(path, manifest.Repo{Name: "service-a", Path: "../service-a", BaseBranch: "main"}); err != nil {
		t.Fatalf("AppendRepo: %v", err)
	}

	if got := readFile(t, path); got[:len("# instance-wide default driver goes here\n")] != "# instance-wide default driver goes here\n" {
		t.Errorf("header comment lost:\n%s", got)
	}
}

// Bootstrap re-walks every repo the org has on every run, so all but the
// first run is almost entirely repeats.
func TestAppendRepoIsANoOpForAnAlreadyListedRepo(t *testing.T) {
	existing := "repos:\n  - name: service-a\n    path: ../service-a\n    base_branch: main\n    convention_pack: java-gradle\n"
	path := writeManifest(t, t.TempDir(), existing)

	added, err := manifest.AppendRepo(path, manifest.Repo{Name: "service-a", Path: "../service-a", BaseBranch: "main"})
	if err != nil {
		t.Fatalf("AppendRepo: %v", err)
	}
	if added {
		t.Error("AppendRepo reported an addition for an already-listed repo")
	}
	if got := readFile(t, path); got != existing {
		t.Errorf("an already-listed repo must leave the file untouched\n got: %q\nwant: %q", got, existing)
	}
}

// Hand-declared values — a convention pack, a driver, a dependency edge —
// are the whole point of the manifest. Appending a sibling must not disturb
// them.
func TestAppendRepoPreservesExistingEntries(t *testing.T) {
	path := writeManifest(t, t.TempDir(),
		"repos:\n  - name: service-a\n    path: ../service-a\n    base_branch: main\n"+
			"    depends_on: [shared-lib]\n    convention_pack: java-gradle\n    driver: openspec\n")

	if _, err := manifest.AppendRepo(path, manifest.Repo{Name: "service-b", Path: "../service-b", BaseBranch: "develop"}); err != nil {
		t.Fatalf("AppendRepo: %v", err)
	}

	m, err := manifest.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(m.Repos) != 2 {
		t.Fatalf("got %d repos, want 2", len(m.Repos))
	}

	a := m.Repos[0]
	if a.ConventionPack != "java-gradle" || a.Driver != "openspec" || len(a.DependsOn) != 1 || a.DependsOn[0] != "shared-lib" {
		t.Errorf("existing entry was disturbed: %+v", a)
	}
	if b := m.Repos[1]; b.Name != "service-b" || b.Path != "../service-b" || b.BaseBranch != "develop" {
		t.Errorf("appended entry wrong: %+v", b)
	}
}

// An instance whose repos.yaml is still the untouched template has no list
// to append to yet.
func TestAppendRepoHandlesAnEmptyRepoList(t *testing.T) {
	for _, tc := range []struct{ name, content string }{
		{"key with no value", "repos:\n"},
		{"no repos key at all", "driver: openspec\n"},
		{"empty file", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := writeManifest(t, t.TempDir(), tc.content)

			added, err := manifest.AppendRepo(path, manifest.Repo{Name: "service-a", Path: "../service-a", BaseBranch: "main"})
			if err != nil {
				t.Fatalf("AppendRepo: %v", err)
			}
			if !added {
				t.Error("AppendRepo reported no addition")
			}

			m, err := manifest.Load(path)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if len(m.Repos) != 1 || m.Repos[0].Name != "service-a" {
				t.Errorf("got %+v, want one service-a entry", m.Repos)
			}
		})
	}
}

func TestAppendRepoMissingFileErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repos.yaml")

	if _, err := manifest.AppendRepo(path, manifest.Repo{Name: "r"}); err == nil {
		t.Fatal("expected an error appending to a nonexistent repos.yaml, got nil")
	}
}
