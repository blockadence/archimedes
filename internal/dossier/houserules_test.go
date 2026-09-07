package dossier_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blockadence/archimedes/internal/dossier"
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

// A dossier scaffolded before the scripts were retired carries the older
// wording of the stub. Its repo has no house rules recorded any more than a
// freshly scaffolded one does, and the cost of forgetting that is the worst
// this package has: instructional boilerplate committed into someone's repo
// as though it were a mandated rule.
func TestHouseRulesTreatsThePreRetirementStubAsNoRules(t *testing.T) {
	dir := t.TempDir()
	legacy := "TBD. Mandated decisions that must be respected even if unusual — the kind of\n" +
		"thing a new contributor (or agent) would otherwise get wrong by using good\n" +
		"judgment. Kept separate from \"Known gotchas\" below: gotchas are surprising\n" +
		"facts about the repo, house rules are standing directives. Edit this section\n" +
		"only here — `sync-house-rules.sh` pushes a durable copy into the repo\n" +
		"itself, and `spawn.sh` injects an ephemeral copy into every worktree\n" +
		"spawned for it, so this dossier is the one place changes need to be made."
	writeDossier(t, dir, "r", "# r\n\n"+dossier.HouseRulesHeading+"\n"+legacy+"\n\n## Known gotchas\nTBD\n")

	got, err := dossier.HouseRules(dir, "r")
	if err != nil {
		t.Fatalf("HouseRules: %v", err)
	}
	if got != "" {
		t.Errorf("an instance scaffolded before the retirement had its stub read as a real house rule:\n%s", got)
	}
}
