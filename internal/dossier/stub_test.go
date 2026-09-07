package dossier_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blockadence/gh-archimedes/internal/dossier"
)

func TestWriteStubScaffoldsDistinctOrderedSections(t *testing.T) {
	dir := t.TempDir()

	written, err := dossier.WriteStub(dir, dossier.Stub{Name: "widget-service", Path: "../widget-service", BaseBranch: "main"})
	if err != nil {
		t.Fatalf("WriteStub: %v", err)
	}
	if !written {
		t.Error("WriteStub reported no write for a repo with no dossier yet")
	}

	got := readDossier(t, dir, "widget-service")

	for _, want := range []string{"# widget-service", "**Path:** ../widget-service", "**Base branch:** main"} {
		if !strings.Contains(got, want) {
			t.Errorf("stub missing %q:\n%s", want, got)
		}
	}

	house := strings.Index(got, dossier.HouseRulesHeading)
	gotchas := strings.Index(got, "## Known gotchas")
	switch {
	case house < 0:
		t.Error("stub has no House rules heading")
	case gotchas < 0:
		t.Error("stub has no Known gotchas heading")
	case house > gotchas:
		t.Error("House rules must precede Known gotchas, as two separate sections")
	}
}

// A never-edited stub must read as "no rules recorded yet", so bootstrapping
// a repo and immediately syncing or spawning can't ship the stub's own
// instructional boilerplate as though it were a real mandated rule.
func TestWriteStubHouseRulesReadBackAsNoRules(t *testing.T) {
	dir := t.TempDir()

	if _, err := dossier.WriteStub(dir, dossier.Stub{Name: "r", Path: "../r", BaseBranch: "main"}); err != nil {
		t.Fatalf("WriteStub: %v", err)
	}

	rules, err := dossier.HouseRules(dir, "r")
	if err != nil {
		t.Fatalf("HouseRules: %v", err)
	}
	if rules != "" {
		t.Errorf("a freshly scaffolded stub read back as a real house rule: %q", rules)
	}
}

// Bootstrap rewrites every discovered repo's dossier path on every run, so
// the one thing a stub must never do is clobber the hand-written prose the
// dossier exists to hold.
func TestWriteStubLeavesAnExistingDossierAlone(t *testing.T) {
	dir := t.TempDir()
	writeDossier(t, dir, "r", "# r\n\nEdited by hand.\n")

	written, err := dossier.WriteStub(dir, dossier.Stub{Name: "r", Path: "../r", BaseBranch: "main"})
	if err != nil {
		t.Fatalf("WriteStub: %v", err)
	}
	if written {
		t.Error("WriteStub reported a write over an existing dossier")
	}

	if got := readDossier(t, dir, "r"); got != "# r\n\nEdited by hand.\n" {
		t.Errorf("existing dossier was modified:\n%s", got)
	}
}

func TestWriteStubCreatesMissingDossierDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "repos")

	if _, err := dossier.WriteStub(dir, dossier.Stub{Name: "r", Path: "../r", BaseBranch: "main"}); err != nil {
		t.Fatalf("WriteStub: %v", err)
	}
	readDossier(t, dir, "r")
}

func readDossier(t *testing.T, dir, repo string) string {
	t.Helper()
	data, err := os.ReadFile(dossier.Path(dir, repo))
	if err != nil {
		t.Fatalf("reading dossier: %v", err)
	}
	return string(data)
}
