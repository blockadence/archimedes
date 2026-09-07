package manifest_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blockadence/archimedes/cli/internal/manifest"
)

// instance writes a repos.yaml listing one repo at repos/service-a and
// returns the instance root it lives in, along with the parsed manifest.
func instance(t *testing.T) (string, *manifest.Manifest) {
	t.Helper()
	root := t.TempDir()
	content := "repos:\n  - name: service-a\n    path: repos/service-a\n    base_branch: main\n"
	if err := os.WriteFile(filepath.Join(root, "repos.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := manifest.Load(filepath.Join(root, "repos.yaml"))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	return root, m
}

func TestCheckoutClonedRepo(t *testing.T) {
	root, m := instance(t)
	if err := os.MkdirAll(filepath.Join(root, "repos", "service-a"), 0o755); err != nil {
		t.Fatal(err)
	}

	c := m.Checkout(root, "service-a")
	if !c.Listed || !c.Cloned || !c.Ready() {
		t.Fatalf("a listed, cloned repo reported as %+v", c)
	}
	if want := filepath.Join(root, "repos", "service-a"); c.Path != want {
		t.Errorf("Path = %q, want %q", c.Path, want)
	}
	if c.BaseBranch != "main" {
		t.Errorf("BaseBranch = %q, want %q", c.BaseBranch, "main")
	}
}

func TestCheckoutUnlistedRepo(t *testing.T) {
	root, m := instance(t)

	c := m.Checkout(root, "service-z")
	if c.Listed {
		t.Error("a repo repos.yaml never listed reported as listed")
	}
	if c.Ready() {
		t.Error("an unlisted repo reported as ready to work in")
	}
}

// The two shortfalls stay apart: a caller wording one message for "not in
// repos.yaml" and another for "listed but not cloned" can tell which it
// got, and can still name the path the clone is missing from.
func TestCheckoutListedButNotCloned(t *testing.T) {
	root, m := instance(t)

	c := m.Checkout(root, "service-a")
	if !c.Listed {
		t.Error("a repo repos.yaml lists reported as unlisted")
	}
	if c.Cloned || c.Ready() {
		t.Error("a repo with nothing on disk reported as cloned")
	}
	if want := filepath.Join(root, "repos", "service-a"); c.Path != want {
		t.Errorf("Path = %q, want %q; an uncloned repo still resolves to where its clone belongs", c.Path, want)
	}
}

// A file where the checkout should be is not a checkout.
func TestCheckoutFileAtRepoPath(t *testing.T) {
	root, m := instance(t)
	if err := os.MkdirAll(filepath.Join(root, "repos"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "repos", "service-a"), []byte("not a checkout\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if c := m.Checkout(root, "service-a"); c.Cloned {
		t.Error("a plain file at the repo's path reported as a checkout")
	}
}

func TestCheckoutOfEntryInHand(t *testing.T) {
	root, m := instance(t)
	if err := os.MkdirAll(filepath.Join(root, "repos", "service-a"), 0o755); err != nil {
		t.Fatal(err)
	}

	entry, ok := m.Find("service-a")
	if !ok {
		t.Fatal("Find did not return the entry repos.yaml lists")
	}
	c := manifest.CheckoutOf(root, entry)
	if !c.Ready() {
		t.Fatalf("an entry in hand resolved to %+v, want it ready", c)
	}
	if want := filepath.Join(root, "repos", "service-a"); c.Path != want {
		t.Errorf("Path = %q, want %q", c.Path, want)
	}
}
