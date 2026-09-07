package contextmap_test

import (
	"io"
	"testing"

	"github.com/blockadence/archimedes/cli/internal/contextmap"
)

func survey(t *testing.T, opts contextmap.Options) contextmap.Plan {
	t.Helper()
	s, err := contextmap.Survey(opts, io.Discard)
	if err != nil {
		t.Fatalf("expected a survey of the instance, got error: %v", err)
	}
	return s
}

func TestSurveyOrdersDependenciesFirstAndReportsStaleness(t *testing.T) {
	inst := newInstance(t, twoRepoYAML, "app", "shared")

	got := survey(t, contextmap.Options{Root: inst.root})

	if len(got.Repos) != 2 {
		t.Fatalf("expected both repos surveyed, got %#v", got.Repos)
	}
	if got.Warning != "" {
		t.Errorf("expected no ordering warning, got %q", got.Warning)
	}
	if got.Repos[0].Name != "shared" || got.Repos[1].Name != "app" {
		t.Errorf("expected dependency-first order [shared app], got %q %q", got.Repos[0].Name, got.Repos[1].Name)
	}
	for _, r := range got.Repos {
		if !r.Stale || r.Reason != "never mapped" {
			t.Errorf("%s: expected an unmapped repo to be stale, got %#v", r.Name, r)
		}
		if r.CurrentSHA != inst.sha(t, r.Name) {
			t.Errorf("%s: current sha = %q, want the base branch tip %q", r.Name, r.CurrentSHA, inst.sha(t, r.Name))
		}
		if !r.Cloned {
			t.Errorf("%s: expected a cloned repo, got %#v", r.Name, r)
		}
	}
}

func TestSurveyReportsAnUpToDateRepoAsCurrent(t *testing.T) {
	inst := newInstance(t, twoRepoYAML, "app", "shared")
	mustWriteFile(t, inst.contextFile("shared"), "# shared\n")
	setSHA(t, inst, "shared", inst.sha(t, "shared"))
	setSHA(t, inst, "app", "0123456789abcdef0123456789abcdef01234567")
	mustWriteFile(t, inst.contextFile("app"), "# app\n")

	got := survey(t, contextmap.Options{Root: inst.root})

	shared, app := got.Repos[0], got.Repos[1]
	if shared.Stale {
		t.Errorf("expected shared current, got %#v", shared)
	}
	if !app.Stale || app.Reason != "stale, 01234567 -> "+contextmap.Short(inst.sha(t, "app")) {
		t.Errorf("expected app reported stale against what's recorded, got %#v", app)
	}
	if !got.Stale() {
		t.Error("expected the survey to report work outstanding while app is stale")
	}
}

// A repo listed in repos.yaml but never cloned can't be assessed at all;
// it's reported as such rather than dropped, so the operator learns they
// need to bootstrap rather than wondering where the repo went.
func TestSurveyReportsAnUnclonedRepo(t *testing.T) {
	inst := newInstance(t, twoRepoYAML, "shared")

	got := survey(t, contextmap.Options{Root: inst.root})

	app := got.Repos[1]
	if app.Name != "app" {
		t.Fatalf("expected app second, got %q", app.Name)
	}
	if app.Cloned || app.Stale {
		t.Errorf("expected an uncloned repo reported as not cloned and not assessed, got %#v", app)
	}
	if app.Reason != "not cloned yet (run bootstrap first)" {
		t.Errorf("reason = %q, want it to say the repo isn't cloned", app.Reason)
	}
}

func TestSurveyReportsWhichDriverWouldMapEachRepo(t *testing.T) {
	inst := newInstance(t, "driver: instance-wide\n"+twoRepoYAML, "app", "shared")

	got := survey(t, contextmap.Options{Root: inst.root, Driver: "environment"})

	for _, r := range got.Repos {
		if r.Driver != "instance-wide" {
			t.Errorf("%s: driver = %q, want repos.yaml's own to win over the environment's", r.Name, r.Driver)
		}
	}
}
