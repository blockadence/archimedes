package dossier_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/blockadence/archimedes/cli/internal/dossier"
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

var stubHeredocRE = regexp.MustCompile(`(?s)cat > "\$dossier" <<EOF\n(.*?)\nEOF\n`)

// Both implementations scaffold dossiers, and sync-house-rules.sh and
// spawn's context materialization both parse what they scaffold. If lib.sh's
// stub is reshaped without updating this package, the two would start
// producing structurally different dossiers depending on which entry point
// created the repo. Fail loudly instead.
func TestStubMatchesLibSh(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "template", "scripts", "lib.sh"))
	if err != nil {
		t.Fatal(err)
	}
	m := stubHeredocRE.FindSubmatch(data)
	if m == nil {
		t.Fatal("could not find write_dossier_stub's heredoc in lib.sh")
	}

	want := strings.NewReplacer(
		"$HOUSE_RULES_HEADING", dossier.HouseRulesHeading,
		"$HOUSE_RULES_STUB_BODY", stubBodyFromLibSh(t),
		"$name", "r",
		"$path", "../r",
		"$base", "trunk",
	).Replace(string(m[1])) + "\n"

	dir := t.TempDir()
	if _, err := dossier.WriteStub(dir, dossier.Stub{Name: "r", Path: "../r", BaseBranch: "trunk"}); err != nil {
		t.Fatalf("WriteStub: %v", err)
	}

	if got := readDossier(t, dir, "r"); got != want {
		t.Errorf("lib.sh's write_dossier_stub has drifted from this package's copy;\n"+
			"update stubTemplate in stub.go to match.\n got: %q\nwant: %q", got, want)
	}
}

func readDossier(t *testing.T, dir, repo string) string {
	t.Helper()
	data, err := os.ReadFile(dossier.Path(dir, repo))
	if err != nil {
		t.Fatalf("reading dossier: %v", err)
	}
	return string(data)
}
