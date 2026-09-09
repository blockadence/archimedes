package dossier_test

import (
	"testing"

	"github.com/blockadence/gh-archimedes/internal/dossier"
)

// The layout spelled out rather than built from the code under test: this
// is the one place that says where an instance's dossiers are, so the test
// that holds it has to state the answer itself.
func TestDirIsTheReposDirectoryUnderTheInstanceRoot(t *testing.T) {
	got := dossier.Dir("/Users/someone/Code/widgets")
	want := "/Users/someone/Code/widgets/repos"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// The root is taken as the operator typed it, as in workdir.Path: `--root
// .` is the ordinary way to name an instance and every caller reads the
// answer from the directory the operator typed it in.
func TestDirKeepsARelativeRootRelative(t *testing.T) {
	if got := dossier.Dir("instance"); got != "instance/repos" {
		t.Errorf("got %q, want %q", got, "instance/repos")
	}
}

// Dir is where root is named, and the only place: everything else in this
// package answers for whatever directory it is handed, which is what lets
// stub_test.go and houserules_test.go run against a bare t.TempDir() with
// no instance around them.
func TestPathIsTheRepoDossierUnderWhateverDirectoryItIsGiven(t *testing.T) {
	if got := dossier.Path("/tmp/scratch", "service-a"); got != "/tmp/scratch/service-a.md" {
		t.Errorf("got %q, want %q", got, "/tmp/scratch/service-a.md")
	}
}
