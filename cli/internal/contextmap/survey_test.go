package contextmap_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/blockadence/archimedes/cli/internal/contextmap"
)

// find returns the surveyed state for one repo by name.
func find(t *testing.T, states []contextmap.RepoState, name string) contextmap.RepoState {
	t.Helper()
	for _, s := range states {
		if s.Repo.Name == name {
			return s
		}
	}
	t.Fatalf("survey has no entry for %s (got %d entries)", name, len(states))
	return contextmap.RepoState{}
}

func TestSurveyReportsEachReposStalenessInDependencyOrder(t *testing.T) {
	inst := newInstance(t, twoRepoYAML, "app", "shared")
	// shared was mapped at its current commit; app never was.
	setSHA(t, inst, "shared", inst.sha(t, "shared"))
	mustWriteFile(t, inst.contextFile("shared"), "# shared\n")

	states, err := contextmap.Survey(inst.root, "", io.Discard)
	if err != nil {
		t.Fatalf("Survey: %v", err)
	}

	if got := []string{states[0].Repo.Name, states[1].Repo.Name}; got[0] != "shared" || got[1] != "app" {
		t.Errorf("survey order = %v, want dependencies first (shared app)", got)
	}
	if s := find(t, states, "shared"); s.Stale {
		t.Errorf("shared mapped at its current commit is stale: %+v", s.Assessment)
	}
	if s := find(t, states, "app"); !s.Stale || s.Reason != "never mapped" {
		t.Errorf("app = %+v, want stale/never mapped", s.Assessment)
	}
	if s := find(t, states, "app"); s.CurrentSHA != inst.sha(t, "app") {
		t.Errorf("app CurrentSHA = %q, want its base branch tip %q", s.CurrentSHA, inst.sha(t, "app"))
	}
}

func TestSurveyMapsNothingAndRecordsNothing(t *testing.T) {
	inst := newInstance(t, "driver: stub-ok\n"+twoRepoYAML, "app", "shared")
	inst.installPathDriver(t, "stub-ok")
	before := readFile(t, filepath.Join(inst.root, "repos.yaml"))

	if _, err := contextmap.Survey(inst.root, "", io.Discard); err != nil {
		t.Fatalf("Survey: %v", err)
	}

	for _, name := range []string{"app", "shared"} {
		assertNoFile(t, inst.contextFile(name), "a survey invokes no driver, so")
	}
	if after := readFile(t, filepath.Join(inst.root, "repos.yaml")); after != before {
		t.Error("a survey rewrote repos.yaml")
	}
}

func TestSurveyReportsAnUnclonedRepoAsNotCloned(t *testing.T) {
	inst := newInstance(t, twoRepoYAML, "shared")

	states, err := contextmap.Survey(inst.root, "", io.Discard)
	if err != nil {
		t.Fatalf("Survey: %v", err)
	}

	app := find(t, states, "app")
	if app.Cloned() || app.Stale {
		t.Errorf("app = %+v, want neither cloned nor assessed until bootstrap has cloned it", app)
	}
	if !find(t, states, "shared").Cloned() {
		t.Error("shared is cloned but the survey says otherwise")
	}
}

func TestSurveyKeepsGoingWhenOneReposGitFails(t *testing.T) {
	inst := newInstance(t, twoRepoYAML, "app", "shared")
	// A directory that is not a git repo at all: present, but unreadable
	// by fetch/rev-parse.
	if err := os.RemoveAll(filepath.Join(inst.repoPaths["app"], ".git")); err != nil {
		t.Fatal(err)
	}

	states, err := contextmap.Survey(inst.root, "", io.Discard)
	if err != nil {
		t.Fatalf("Survey: %v", err)
	}

	if app := find(t, states, "app"); app.Err == nil {
		t.Error("app's broken checkout should be reported as this repo's error")
	}
	if shared := find(t, states, "shared"); shared.Err != nil || !shared.Stale {
		t.Errorf("shared = %+v, want it assessed regardless of app's failure", shared)
	}
}

func TestSurveyHonorsAContextFileOverride(t *testing.T) {
	inst := newInstance(t, twoRepoYAML, "app", "shared")
	setSHA(t, inst, "shared", inst.sha(t, "shared"))
	// Mapped at the current commit, but the map it produced lives
	// somewhere else, so CONTEXT.md's absence is not what's checked.
	mustWriteFile(t, filepath.Join(inst.repoPaths["shared"], "docs", "DOMAIN.md"), "# shared\n")

	states, err := contextmap.Survey(inst.root, "docs/DOMAIN.md", io.Discard)
	if err != nil {
		t.Fatalf("Survey: %v", err)
	}

	if s := find(t, states, "shared"); s.Stale {
		t.Errorf("shared = %+v, want it current for the overridden context file", s.Assessment)
	}
}
