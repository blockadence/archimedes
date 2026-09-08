package bootstrap

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blockadence/gh-archimedes/internal/manifest"
	"github.com/blockadence/gh-archimedes/internal/testrepo"
)

// newOrigin creates a bare repo with one commit on branch, standing in for
// a repo the org's forge would hand back. Cloning it needs no network. Its
// seed clone is kept out of dir/<name>, the path bootstrap itself clones
// into — finding one already there is a different case entirely.
func newOrigin(t *testing.T, dir, name, branch string) string {
	t.Helper()
	return testrepo.New(t, testrepo.Spec{
		Dir:    dir,
		Name:   name,
		Origin: name + "-origin.git",
		Clone:  name + "-seed",
		Branch: branch,
	}).Origin
}

// newInstance lays out an instance root beside the org's origins, the
// "../<name>" sibling arrangement bootstrap clones into.
func newInstance(t *testing.T, dir string) string {
	t.Helper()
	root := filepath.Join(dir, "instance")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func lister(repos ...OrgRepo) RepoLister {
	return func(string) ([]OrgRepo, error) { return repos, nil }
}

func runBootstrap(t *testing.T, root string, list RepoLister) string {
	t.Helper()
	var out bytes.Buffer
	if err := Run(Options{Root: root, Org: "acme", List: list}, &out, io.Discard); err != nil {
		t.Fatalf("Run: %v", err)
	}
	return out.String()
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(data)
}

func TestRunClonesAndScaffoldsADiscoveredOrg(t *testing.T) {
	dir := t.TempDir()
	root := newInstance(t, dir)
	originA := newOrigin(t, dir, "service-a", "main")
	originB := newOrigin(t, dir, "service-b", "trunk")

	out := runBootstrap(t, root, lister(
		OrgRepo{Name: "service-a", SSHURL: originA},
		OrgRepo{Name: "service-b", SSHURL: originB, DefaultBranchRef: &branchRef{Name: "trunk"}},
	))

	if !strings.Contains(out, "New repos added: 2") {
		t.Errorf("expected a count of 2 new repos, got:\n%s", out)
	}

	for _, name := range []string{"service-a", "service-b"} {
		if _, err := os.Stat(filepath.Join(dir, name, "README.md")); err != nil {
			t.Errorf("%s was not cloned as a sibling of the instance: %v", name, err)
		}
		if _, err := os.Stat(filepath.Join(root, "repos", name+".md")); err != nil {
			t.Errorf("%s got no dossier stub: %v", name, err)
		}
	}

	m, err := manifest.Load(filepath.Join(root, "repos.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []manifest.Repo{
		{Name: "service-a", Path: "../service-a", BaseBranch: "main", DependsOn: []string{}},
		{Name: "service-b", Path: "../service-b", BaseBranch: "trunk", DependsOn: []string{}},
	}
	if len(m.Repos) != len(want) {
		t.Fatalf("got %d repos, want %d", len(m.Repos), len(want))
	}
	for i := range want {
		if m.Repos[i].Name != want[i].Name || m.Repos[i].Path != want[i].Path || m.Repos[i].BaseBranch != want[i].BaseBranch {
			t.Errorf("repo %d: got %+v, want %+v", i, m.Repos[i], want[i])
		}
	}

	if got := read(t, filepath.Join(root, "WORKSPACE-MAP.md")); !strings.Contains(got, "- [service-b](../service-b) — base: `trunk`") {
		t.Errorf("WORKSPACE-MAP.md was not regenerated:\n%s", got)
	}
}

// The dossier a repo gets at bootstrap must already have the House rules
// section, so recording one is editing a section that's there rather than
// knowing to add it.
func TestRunScaffoldsDossiersWithAHouseRulesSection(t *testing.T) {
	dir := t.TempDir()
	root := newInstance(t, dir)
	origin := newOrigin(t, dir, "service-a", "main")

	runBootstrap(t, root, lister(OrgRepo{Name: "service-a", SSHURL: origin}))

	got := read(t, filepath.Join(root, "repos", "service-a.md"))
	if !strings.Contains(got, "## House rules") {
		t.Errorf("scaffolded dossier has no House rules section:\n%s", got)
	}
	if !strings.Contains(got, "## Known gotchas") {
		t.Errorf("scaffolded dossier has no Known gotchas section:\n%s", got)
	}
}

// A repo entry must carry the convention_pack key from the moment it's
// scaffolded, so declaring a pack is filling in a blank rather than
// inventing a field name.
func TestRunScaffoldsRepoEntriesReadyToDeclareAConventionPack(t *testing.T) {
	dir := t.TempDir()
	root := newInstance(t, dir)
	origin := newOrigin(t, dir, "service-a", "main")

	runBootstrap(t, root, lister(OrgRepo{Name: "service-a", SSHURL: origin}))

	if got := read(t, filepath.Join(root, "repos.yaml")); !strings.Contains(got, "convention_pack: null") {
		t.Errorf("scaffolded entry has no convention_pack key:\n%s", got)
	}

	// And what's declared there survives the next bootstrap.
	yaml := read(t, filepath.Join(root, "repos.yaml"))
	if err := os.WriteFile(filepath.Join(root, "repos.yaml"),
		[]byte(strings.Replace(yaml, "convention_pack: null", "convention_pack: java-gradle", 1)), 0o644); err != nil {
		t.Fatal(err)
	}

	runBootstrap(t, root, lister(OrgRepo{Name: "service-a", SSHURL: origin}))

	m, err := manifest.Load(filepath.Join(root, "repos.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if m.Repos[0].ConventionPack != "java-gradle" {
		t.Errorf("a declared convention pack was lost: %+v", m.Repos[0])
	}
}

// Re-running against an already-bootstrapped instance is the common case:
// it must add only what's missing and disturb nothing an operator has
// written since.
func TestRunAgainstABootstrappedInstanceAddsOnlyWhatsMissing(t *testing.T) {
	dir := t.TempDir()
	root := newInstance(t, dir)
	originA := newOrigin(t, dir, "service-a", "main")

	runBootstrap(t, root, lister(OrgRepo{Name: "service-a", SSHURL: originA}))

	dossierPath := filepath.Join(root, "repos", "service-a.md")
	if err := os.WriteFile(dossierPath, []byte("# service-a\n\nWritten by hand.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "service-a", "local-work.txt")
	if err := os.WriteFile(marker, []byte("uncommitted\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	originB := newOrigin(t, dir, "service-b", "main")
	out := runBootstrap(t, root, lister(
		OrgRepo{Name: "service-a", SSHURL: originA},
		OrgRepo{Name: "service-b", SSHURL: originB},
	))

	if !strings.Contains(out, "New repos added: 1") {
		t.Errorf("expected only service-b to count as new, got:\n%s", out)
	}
	if got := read(t, dossierPath); got != "# service-a\n\nWritten by hand.\n" {
		t.Errorf("an existing dossier was overwritten:\n%s", got)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("an existing checkout was re-cloned over: %v", err)
	}

	m, err := manifest.Load(filepath.Join(root, "repos.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Repos) != 2 {
		t.Fatalf("got %d repos, want 2 (no duplicate entry for service-a)", len(m.Repos))
	}
}

// An instance can also be half-set-up: an entry declared by hand, or left
// behind by a run that failed partway, with no checkout to go with it.
func TestRunClonesARepoAlreadyListedButNotCloned(t *testing.T) {
	dir := t.TempDir()
	root := newInstance(t, dir)
	origin := newOrigin(t, dir, "service-a", "main")

	if err := os.WriteFile(filepath.Join(root, "repos.yaml"),
		[]byte("repos:\n  - name: service-a\n    path: ../service-a\n    base_branch: main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := runBootstrap(t, root, lister(OrgRepo{Name: "service-a", SSHURL: origin}))

	if !strings.Contains(out, "New repos added: 0") {
		t.Errorf("an already-listed repo must not count as new, got:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "service-a", "README.md")); err != nil {
		t.Errorf("a listed-but-uncloned repo was not cloned: %v", err)
	}
}

func TestRunReportsACloneFailure(t *testing.T) {
	dir := t.TempDir()
	root := newInstance(t, dir)

	err := Run(Options{Root: root, Org: "acme", List: lister(
		OrgRepo{Name: "service-a", SSHURL: filepath.Join(dir, "nope-origin.git")},
	)}, io.Discard, io.Discard)
	if err == nil {
		t.Fatal("expected an error when a clone fails, got nil")
	}
}

// A mistyped --root should fail rather than quietly scaffold a second,
// empty instance in a directory the operator never meant to use.
func TestRunRejectsARootThatIsNotAnInstanceDirectory(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "typo")

	err := Run(Options{Root: missing, Org: "acme", List: lister()}, io.Discard, io.Discard)
	if err == nil {
		t.Fatal("expected an error for a root that doesn't exist, got nil")
	}
	if _, statErr := os.Stat(missing); !os.IsNotExist(statErr) {
		t.Errorf("a nonexistent root was created anyway: %v", statErr)
	}
}

func TestRunReportsADiscoveryFailure(t *testing.T) {
	root := newInstance(t, t.TempDir())

	failing := func(string) ([]OrgRepo, error) { return nil, os.ErrPermission }
	if err := Run(Options{Root: root, Org: "acme", List: failing}, io.Discard, io.Discard); err == nil {
		t.Fatal("expected an error when discovery fails, got nil")
	}
}

// The strongest form of "only adds what's missing": a run that discovers
// nothing new must leave the instance byte-for-byte as it found it, so an
// operator can re-run freely without generating diff noise.
func TestRunChangesNothingWhenNothingChanged(t *testing.T) {
	dir := t.TempDir()
	root := newInstance(t, dir)
	origin := newOrigin(t, dir, "service-a", "main")
	list := lister(OrgRepo{Name: "service-a", SSHURL: origin})

	runBootstrap(t, root, list)
	before := snapshot(t, root)

	runBootstrap(t, root, list)

	for path, want := range before {
		if got := snapshot(t, root)[path]; got != want {
			t.Errorf("%s changed on a no-op re-run\n got: %q\nwant: %q", path, got, want)
		}
	}
}

// snapshot reads every file under root, keyed by its path relative to root.
// An instance's own git history is excluded: what is being compared is what
// bootstrap wrote, and two runs that committed it would differ by hash for
// a reason that is not the one under test.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir():
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[rel] = read(t, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}
