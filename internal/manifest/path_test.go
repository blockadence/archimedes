package manifest_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/blockadence/gh-archimedes/internal/manifest"
)

// The layout spelled out rather than built from the code under test: this
// is the one place that says where an instance's manifest is, so the test
// that holds it has to state the answer itself.
func TestPathIsReposYAMLUnderTheInstanceRoot(t *testing.T) {
	got := manifest.Path("/Users/someone/Code/widgets")
	want := "/Users/someone/Code/widgets/repos.yaml"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// The root is answered as the operator typed it, as in workdir.Path and
// dossier.Dir: `--root .` is the ordinary way to name an instance, and a
// caller that needs an absolute answer absolutizes the root first — which
// is what LoadInstance does below.
func TestPathKeepsARelativeRootRelative(t *testing.T) {
	if got := manifest.Path("instance"); got != "instance/repos.yaml" {
		t.Errorf("got %q, want %q", got, "instance/repos.yaml")
	}
}

// LoadInstance is Path's first caller rather than a ninth site that knows
// the layout: the file it fails to read is the file Path names, under the
// absolute root it resolves. The expected name is spelled out here for the
// same reason as above.
func TestLoadInstanceFailsNamingTheManifestFileUnderTheAbsoluteRoot(t *testing.T) {
	dir := t.TempDir()

	_, _, err := manifest.LoadInstance(dir)
	if err == nil {
		t.Fatal("expected an error for an instance with no manifest, got nil")
	}
	if want := filepath.Join(dir, "repos.yaml"); !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want it to name %q", err, want)
	}
}
