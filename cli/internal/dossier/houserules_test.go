package dossier_test

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/blockadence/archimedes/cli/internal/dossier"
)

func writeDossier(t *testing.T, dir, repo, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, repo+".md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestHouseRules(t *testing.T) {
	cases := []struct {
		name    string
		dossier string
		want    string
	}{
		{
			name: "reads the section body, trimming surrounding blank lines",
			dossier: "# r\n\n## House rules\n\n" +
				"Never rebase a shared branch.\nAll schema changes go through the migration tool.\n\n" +
				"## Known gotchas\nn/a\n",
			want: "Never rebase a shared branch.\nAll schema changes go through the migration tool.",
		},
		{
			name:    "empty section means no rules",
			dossier: "# r\n\n## House rules\n\n## Known gotchas\nn/a\n",
			want:    "",
		},
		{
			name:    "missing section means no rules",
			dossier: "# r\n\n## Known gotchas\nn/a\n",
			want:    "",
		},
		{
			name:    "section running to end of file needs no following heading",
			dossier: "# r\n\n## House rules\n\nNo force pushes.\n",
			want:    "No force pushes.",
		},
		{
			name:    "blank lines inside the body are preserved",
			dossier: "# r\n\n## House rules\n\nFirst rule.\n\nSecond rule.\n\n## Known gotchas\nn/a\n",
			want:    "First rule.\n\nSecond rule.",
		},
		{
			name:    "a deeper heading does not end the section",
			dossier: "# r\n\n## House rules\n\nRule.\n### Detail\nMore.\n\n## Known gotchas\nn/a\n",
			want:    "Rule.\n### Detail\nMore.",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeDossier(t, dir, "r", tc.dossier)

			got, err := dossier.HouseRules(dir, "r")
			if err != nil {
				t.Fatalf("HouseRules: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHouseRulesMissingDossier(t *testing.T) {
	got, err := dossier.HouseRules(t.TempDir(), "nonexistent")
	if err != nil {
		t.Fatalf("a missing dossier should not error: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

// A freshly bootstrapped, never-edited dossier carries instructional
// boilerplate. Delivering that as though it were a real mandated rule would
// be worse than delivering nothing.
func TestHouseRulesTreatsUneditedStubAsNoRules(t *testing.T) {
	dir := t.TempDir()
	writeDossier(t, dir, "r", "# r\n\n"+dossier.HouseRulesHeading+"\n"+stubBodyFromLibSh(t)+"\n\n## Known gotchas\nTBD\n")

	got, err := dossier.HouseRules(dir, "r")
	if err != nil {
		t.Fatalf("HouseRules: %v", err)
	}
	if got != "" {
		t.Errorf("unedited stub was treated as a real house rule: %q", got)
	}
}

var stubBodyRE = regexp.MustCompile(`(?s)HOUSE_RULES_STUB_BODY='(.*?)'`)

// stubBodyFromLibSh reads the canonical stub text out of the shell
// implementation, so this package's copy is checked against the real thing
// rather than against itself.
func stubBodyFromLibSh(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "..", "..", "template", "scripts", "lib.sh")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	m := stubBodyRE.FindSubmatch(data)
	if m == nil {
		t.Fatalf("could not find HOUSE_RULES_STUB_BODY in %s", path)
	}
	return string(m[1])
}

// The Go and shell implementations both decide what counts as an unedited
// stub. If lib.sh's wording is reworded without updating this package, a
// freshly bootstrapped dossier would start delivering boilerplate as a real
// rule — silently, and only via the Go CLI. Fail loudly instead.
func TestStubBodyMatchesLibSh(t *testing.T) {
	dir := t.TempDir()
	writeDossier(t, dir, "r", "# r\n\n"+dossier.HouseRulesHeading+"\n"+stubBodyFromLibSh(t)+"\n")

	got, err := dossier.HouseRules(dir, "r")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("lib.sh's HOUSE_RULES_STUB_BODY has drifted from this package's copy;\n"+
			"update houseRulesStubBody in houserules.go to match.\nlib.sh has:\n%s", got)
	}
}

// The heading the Go code looks for must be the one lib.sh writes.
func TestHeadingMatchesLibSh(t *testing.T) {
	path := filepath.Join("..", "..", "..", "template", "scripts", "lib.sh")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}

	want := "HOUSE_RULES_HEADING='" + dossier.HouseRulesHeading + "'"
	if !regexp.MustCompile(regexp.QuoteMeta(want)).Match(data) {
		t.Errorf("lib.sh does not define %s; heading has drifted", want)
	}
}
