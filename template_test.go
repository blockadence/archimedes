package archimedes_test

import (
	"io/fs"
	"slices"
	"testing"

	"github.com/blockadence/archimedes"
)

// The template and the shipped drivers are prose, YAML and bash maintained
// by hand at the top of this repo; the binary only carries copies of them.
// These tests are about the carrying, not the content: the parts of each
// tree that an embed directive is most likely to drop silently — dotfiles,
// the empty markers that give an instance its directories, a helper a
// driver sources rather than runs — are the ones asserted here. So is the
// line between the two trees, which is the whole of the ownership answer:
// what the template seeds, an instance owns; what the binary carries stays
// the tool's to fix.

func TestTemplateCarriesTheTreeAHumanMaintains(t *testing.T) {
	tmpl := archimedes.Template()

	for _, path := range []string{
		"repos.yaml",
		"WORKSPACE-MAP.md",
		"AGENTS.md",
		"README.md",
		"drivers/README.md",
		"convention-packs/README.md",
		"scaffolding/PULL_REQUEST_TEMPLATE.md",
		"scaffolding/ISSUE_TEMPLATE/bug.yml",
	} {
		if _, err := fs.Stat(tmpl, path); err != nil {
			t.Errorf("template is missing %s: %v", path, err)
		}
	}
}

func TestTemplateCarriesDotfilesAndEmptyDirectoryMarkers(t *testing.T) {
	tmpl := archimedes.Template()

	// Without embed's all: prefix these vanish, and an instance loses its
	// .gitignore and its repos/ and work/ directories with them.
	for _, path := range []string{".gitignore", "repos/.gitkeep", "work/.gitkeep"} {
		if _, err := fs.Stat(tmpl, path); err != nil {
			t.Errorf("template is missing %s: %v", path, err)
		}
	}
}

// The template seeds an instance and the instance owns what it seeds, so a
// driver in there would be a driver no fix could ever reach again. An
// instance's drivers/ starts as the operator's empty shelf.
func TestTheTemplateSeedsNoDrivers(t *testing.T) {
	tmpl := archimedes.Template()

	entries, err := fs.ReadDir(tmpl, "drivers")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() {
			t.Errorf("the template seeds a driver (%s): a seeded driver is one no fix can reach", e.Name())
		}
	}
}

// The drivers ride in the binary instead, so that fixing one here fixes it
// for instances that already exist. Same failure mode as the template's:
// what an embed directive drops, it drops silently.
func TestTheBinaryCarriesEveryDriverItShips(t *testing.T) {
	drivers := archimedes.Drivers()

	entries, err := fs.ReadDir(drivers, ".")
	if err != nil {
		t.Fatal(err)
	}

	var found []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		found = append(found, e.Name())
		if _, err := fs.Stat(drivers, e.Name()+"/driver.yaml"); err != nil {
			t.Errorf("driver %s has no manifest: %v", e.Name(), err)
		}
	}

	for _, want := range []string{"openspec", "pocock", "spec-kit"} {
		if !slices.Contains(found, want) {
			t.Errorf("the binary does not carry the %s driver (carries %v)", want, found)
		}
	}
}

// A driver is a directory, not a script: spec-kit's command sources a
// helper beside it, and a copy carried without that helper would fail only
// once it was already running inside somebody's repository.
func TestTheBinaryCarriesWhatADriversCommandSourcesBesideIt(t *testing.T) {
	if _, err := fs.Stat(archimedes.Drivers(), "spec-kit/repo-snapshot.sh"); err != nil {
		t.Errorf("spec-kit's sourced helper is missing from the carried copy: %v", err)
	}
}
