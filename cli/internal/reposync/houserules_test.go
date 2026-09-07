package reposync_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blockadence/archimedes/cli/internal/dossier"
	"github.com/blockadence/archimedes/cli/internal/reposync"
	"github.com/blockadence/archimedes/cli/internal/testrepo"
)

const testRules = "Never force-push to `main`.\nEvery migration needs a paired rollback script."

func TestRenderHouseRules(t *testing.T) {
	got := reposync.RenderHouseRules("Never force-push.")
	want := "# House rules\n\nNever force-push.\n"
	if got != want {
		t.Errorf("RenderHouseRules = %q, want %q", got, want)
	}
}

// houseRulesInstance is an instance with one target repo whose dossier
// records real house rules.
func houseRulesInstance(t *testing.T) instance {
	t.Helper()
	inst := newInstance(t, "target")
	inst.writeDossier(t, "target", testRules)
	return inst
}

func syncHouseRules(t *testing.T, opts reposync.HouseRulesOptions, fake *fakeExec) (string, error) {
	t.Helper()
	var out, progress bytes.Buffer
	err := reposync.SyncHouseRules(opts, &out, &progress, fake.run)
	return out.String() + progress.String(), err
}

func TestSyncHouseRulesDryRunShowsPendingContentAndChangesNothing(t *testing.T) {
	inst := houseRulesInstance(t)
	fake := &fakeExec{}

	out, err := syncHouseRules(t, reposync.HouseRulesOptions{Root: inst.root, Repo: "target", DryRun: true}, fake)
	if err != nil {
		t.Fatalf("SyncHouseRules: %v\n%s", err, out)
	}

	// A repo with no HOUSE_RULES.md yet should diff as a clean addition:
	// every line added, no phantom context line for a file that isn't there.
	for _, want := range []string{"+# House rules", "+Never force-push to `main`.", "@@ -0,0 +1,4 @@"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry run diff missing %q:\n%s", want, out)
		}
	}

	clone := inst.repoPath("target")
	if _, err := os.Stat(filepath.Join(clone, dossier.HouseRulesFileName)); !os.IsNotExist(err) {
		t.Error("dry run wrote HOUSE_RULES.md into the local clone")
	}
	if got := testrepo.GitOut(t, clone, "rev-parse", "--abbrev-ref", "HEAD"); got != "main" {
		t.Errorf("dry run left the clone on %q, want main", got)
	}
	if branches := testrepo.GitOut(t, clone, "branch", "--list", reposync.HouseRulesBranch); branches != "" {
		t.Errorf("dry run created the sync branch: %q", branches)
	}
	if fake.count("gh") != 0 {
		t.Errorf("dry run called gh: %v", fake.calls)
	}
}

func TestSyncHouseRulesPushesBranchAndOpensPR(t *testing.T) {
	inst := houseRulesInstance(t)
	fake := &fakeExec{}

	out, err := syncHouseRules(t, reposync.HouseRulesOptions{Root: inst.root, Repo: "target"}, fake)
	if err != nil {
		t.Fatalf("SyncHouseRules: %v\n%s", err, out)
	}

	clone := inst.repoPath("target")
	pushed := testrepo.GitOut(t, clone, "show", "origin/"+reposync.HouseRulesBranch+":"+dossier.HouseRulesFileName)
	for _, want := range []string{"# House rules", "Never force-push to `main`.", "Every migration needs a paired rollback script."} {
		if !strings.Contains(pushed, want) {
			t.Errorf("pushed HOUSE_RULES.md missing %q:\n%s", want, pushed)
		}
	}

	if got := testrepo.GitOut(t, clone, "rev-parse", "--abbrev-ref", "HEAD"); got != "main" {
		t.Errorf("sync left the clone on %q, want main", got)
	}

	pr, ok := fake.find("gh")
	if !ok {
		t.Fatalf("gh was never invoked: %v", fake.calls)
	}
	// The first gh call is the token read; the PR create is the one that matters.
	var prCall call
	for _, c := range fake.calls {
		if c.name == "gh" && len(c.args) >= 2 && c.args[0] == "pr" && c.args[1] == "create" {
			prCall = c
		}
	}
	if prCall.name == "" {
		t.Fatalf("gh pr create was never invoked (first gh call: %v)", pr.args)
	}
	joined := prCall.joined()
	for _, want := range []string{"--base main", "--head " + reposync.HouseRulesBranch, "--title"} {
		if !strings.Contains(joined, want) {
			t.Errorf("gh pr create missing %q: %v", want, prCall.args)
		}
	}
}

func TestSyncHouseRulesExistingPRIsReportedNotFatal(t *testing.T) {
	inst := houseRulesInstance(t)
	fake := &fakeExec{failOn: "pr create", failWith: errors.New("a pull request for that branch already exists")}

	out, err := syncHouseRules(t, reposync.HouseRulesOptions{Root: inst.root, Repo: "target"}, fake)
	if err != nil {
		t.Fatalf("an existing PR should not be fatal, got: %v\n%s", err, out)
	}
	if !strings.Contains(out, "already exists") && !strings.Contains(out, "check manually") {
		t.Errorf("the existing-PR situation wasn't surfaced: %s", out)
	}
	if got := testrepo.GitOut(t, inst.repoPath("target"), "rev-parse", "--abbrev-ref", "HEAD"); got != "main" {
		t.Errorf("a failed PR create left the clone on %q, want main", got)
	}
}

func TestSyncHouseRulesAlreadyCurrentIsANoOp(t *testing.T) {
	inst := houseRulesInstance(t)
	clone := inst.repoPath("target")

	// Simulate the sync PR having merged: the target repo's base branch
	// already carries exactly the content the dossier would produce.
	mustWriteFile(t, filepath.Join(clone, dossier.HouseRulesFileName), reposync.RenderHouseRules(testRules))
	testrepo.Git(t, clone, "add", "-A")
	testrepo.Git(t, clone, "commit", "-q", "-m", "house rules")
	testrepo.Git(t, clone, "push", "-q", "origin", "main")

	fake := &fakeExec{}
	out, err := syncHouseRules(t, reposync.HouseRulesOptions{Root: inst.root, Repo: "target"}, fake)
	if err != nil {
		t.Fatalf("SyncHouseRules: %v\n%s", err, out)
	}

	if !strings.Contains(out, "already current") {
		t.Errorf("an unchanged sync should report already-current: %s", out)
	}
	if fake.count("gh") != 0 {
		t.Errorf("an already-current sync still called gh: %v", fake.calls)
	}
	if branches := testrepo.GitOut(t, clone, "branch", "--list", reposync.HouseRulesBranch); branches != "" {
		t.Errorf("an already-current sync created the sync branch: %q", branches)
	}
}

func TestSyncHouseRulesUnknownRepoIsAnError(t *testing.T) {
	inst := houseRulesInstance(t)
	fake := &fakeExec{}

	_, err := syncHouseRules(t, reposync.HouseRulesOptions{Root: inst.root, Repo: "ghost"}, fake)
	if err == nil || !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("expected an unknown-repo error naming ghost, got: %v", err)
	}
}

func TestSyncHouseRulesWithoutRecordedRulesIsAnError(t *testing.T) {
	inst := newInstance(t, "target") // no dossier written
	fake := &fakeExec{}

	_, err := syncHouseRules(t, reposync.HouseRulesOptions{Root: inst.root, Repo: "target"}, fake)
	if err == nil || !strings.Contains(err.Error(), "no house rules") {
		t.Fatalf("expected a no-house-rules error, got: %v", err)
	}
	if fake.count("gh") != 0 {
		t.Errorf("nothing to sync should not call gh: %v", fake.calls)
	}
}
