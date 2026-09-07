package archimedes_test

import (
	"io/fs"
	"testing"

	"github.com/blockadence/archimedes"
)

// The template is prose and configuration maintained by hand at the top of
// this repo; the binary only carries a copy of it. These tests are about
// the carrying, not the content: the parts of the tree that an embed
// directive is most likely to drop silently — dotfiles, the empty markers
// that give an instance its directories, a driver's command — are the ones
// asserted here.

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

func TestTemplateCarriesEveryDriverItShips(t *testing.T) {
	tmpl := archimedes.Template()

	entries, err := fs.ReadDir(tmpl, "drivers")
	if err != nil {
		t.Fatal(err)
	}

	var drivers int
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		drivers++
		if _, err := fs.Stat(tmpl, "drivers/"+e.Name()+"/driver.yaml"); err != nil {
			t.Errorf("driver %s has no manifest: %v", e.Name(), err)
		}
	}
	if drivers == 0 {
		t.Error("the template ships no drivers at all")
	}
}
