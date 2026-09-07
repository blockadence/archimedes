package contextmap_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blockadence/archimedes/internal/contextmap"
	"github.com/blockadence/archimedes/internal/manifest"
	"github.com/blockadence/archimedes/internal/testrepo"
)

// fixedSHA answers every repo with the same commit, for the tests that care
// about the verdict rather than about git.
func fixedSHA(sha string) contextmap.SHALookup {
	return func(string, string) (string, error) { return sha, nil }
}

func TestStateReportsARepoAsCurrentWhenItsMapMatchesItsBaseBranch(t *testing.T) {
	inst := newInstance(t, "repos:\n  - name: alpha\n    path: ../alpha\n    base_branch: main\n", "alpha")
	mustWriteFile(t, inst.contextFile("alpha"), "# alpha\n")

	state := contextmap.State(inst.root, manifest.Repo{
		Name: "alpha", Path: "../alpha", BaseBranch: "main", ContextModeledSHA: "abc123",
	}, "CONTEXT.md", fixedSHA("abc123"))

	if !state.Cloned || state.Err != nil {
		t.Fatalf("expected an assessable repo, got cloned=%v err=%v", state.Cloned, state.Err)
	}
	if state.Stale {
		t.Errorf("expected current, got stale: %q", state.Reason)
	}
	if state.CurrentSHA != "abc123" || state.MappedSHA != "abc123" {
		t.Errorf("SHAs = current %q, mapped %q", state.CurrentSHA, state.MappedSHA)
	}
	if want := filepath.Join(inst.repoPaths["alpha"], "CONTEXT.md"); state.ContextPath != want {
		t.Errorf("ContextPath = %q, want %q", state.ContextPath, want)
	}
}

func TestStateReportsARepoAsStaleWhenItsBaseBranchHasMovedOn(t *testing.T) {
	inst := newInstance(t, "repos:\n  - name: alpha\n    path: ../alpha\n    base_branch: main\n", "alpha")
	mustWriteFile(t, inst.contextFile("alpha"), "# alpha\n")

	state := contextmap.State(inst.root, manifest.Repo{
		Name: "alpha", Path: "../alpha", BaseBranch: "main", ContextModeledSHA: "0000000000",
	}, "CONTEXT.md", fixedSHA("1111111111"))

	if !state.Stale {
		t.Fatal("expected stale after the base branch moved")
	}
	if !strings.Contains(state.Reason, "00000000") || !strings.Contains(state.Reason, "11111111") {
		t.Errorf("Reason = %q, want it to name both commits", state.Reason)
	}
}

func TestStateReportsARepoThatIsNotClonedYet(t *testing.T) {
	inst := newInstance(t, "repos: []\n")

	state := contextmap.State(inst.root, manifest.Repo{
		Name: "ghost", Path: "../ghost", BaseBranch: "main",
	}, "CONTEXT.md", fixedSHA("abc123"))

	if state.Cloned {
		t.Fatal("expected an uncloned repo")
	}
	// Nothing was asked of git, so nothing is claimed about the map: a
	// zero Assessment here means unanswered, not current.
	if state.Stale || state.CurrentSHA != "" || state.Err != nil {
		t.Errorf("expected an unassessed repo, got stale=%v sha=%q err=%v", state.Stale, state.CurrentSHA, state.Err)
	}
}

func TestStateCarriesAnUnreadableBaseBranchRatherThanGuessing(t *testing.T) {
	inst := newInstance(t, "repos:\n  - name: alpha\n    path: ../alpha\n    base_branch: main\n", "alpha")
	boom := errors.New("no such ref")

	state := contextmap.State(inst.root, manifest.Repo{
		Name: "alpha", Path: "../alpha", BaseBranch: "main",
	}, "CONTEXT.md", func(string, string) (string, error) { return "", boom })

	if !errors.Is(state.Err, boom) {
		t.Fatalf("Err = %v, want %v", state.Err, boom)
	}
	if state.Stale {
		t.Error("an unreadable base branch must not be reported as staleness")
	}
}

func TestLocalSHAReadsTheCheckoutWithoutFetching(t *testing.T) {
	inst := newInstance(t, "repos:\n  - name: alpha\n    path: ../alpha\n    base_branch: main\n", "alpha")

	// A commit pushed behind the clone's back: LocalSHA must report the
	// origin/main the checkout already has, not the one on the remote.
	before := inst.sha(t, "alpha")
	other := filepath.Join(inst.tmp, "other")
	testrepo.Git(t, inst.tmp, "clone", "-q", filepath.Join(inst.tmp, "alpha.git"), other)
	mustWriteFile(t, filepath.Join(other, "NEW.md"), "new\n")
	testrepo.Git(t, other, "add", "-A")
	testrepo.Git(t, other, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "-m", "second")
	testrepo.Git(t, other, "push", "-q", "origin", "main")

	got, err := contextmap.LocalSHA(inst.repoPaths["alpha"], "main")
	if err != nil {
		t.Fatal(err)
	}
	if got != before {
		t.Errorf("LocalSHA = %q, want the unfetched %q", got, before)
	}
}

func TestSurveyAssessesEveryRepoInDependencyOrder(t *testing.T) {
	reposYAML := "repos:\n" +
		"  - name: app\n    path: ../app\n    base_branch: main\n    depends_on: [lib]\n" +
		"  - name: lib\n    path: ../lib\n    base_branch: main\n"
	inst := newInstance(t, reposYAML, "app", "lib")
	mustWriteFile(t, inst.contextFile("lib"), "# lib\n")
	setSHA(t, inst, "lib", "abc123")

	m, err := manifest.Load(filepath.Join(inst.root, "repos.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	states, warning := contextmap.Survey(inst.root, m, "CONTEXT.md", fixedSHA("abc123"))
	if warning != "" {
		t.Errorf("unexpected ordering warning: %s", warning)
	}
	if len(states) != 2 {
		t.Fatalf("got %d states, want 2", len(states))
	}
	if states[0].Name != "lib" || states[1].Name != "app" {
		t.Errorf("order = %s, %s; want the dependency first", states[0].Name, states[1].Name)
	}
	if states[0].Stale {
		t.Errorf("lib should be current: %q", states[0].Reason)
	}
	if states[1].Reason != "never mapped" {
		t.Errorf("app Reason = %q, want \"never mapped\"", states[1].Reason)
	}
}

func TestSurveyReportsAnOrderingWarningAlongsideItsStates(t *testing.T) {
	reposYAML := "repos:\n" +
		"  - name: app\n    path: ../app\n    base_branch: main\n    depends_on: [nowhere]\n"
	inst := newInstance(t, reposYAML, "app")

	m, err := manifest.Load(filepath.Join(inst.root, "repos.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	states, warning := contextmap.Survey(inst.root, m, "CONTEXT.md", fixedSHA("abc123"))
	if !strings.Contains(warning, "app") {
		t.Errorf("warning = %q, want it to name the repo left unordered", warning)
	}
	if len(states) != 1 {
		t.Fatalf("a bad depends_on must still assess every repo, got %d states", len(states))
	}
}
