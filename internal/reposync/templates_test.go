package reposync_test

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blockadence/archimedes/internal/manifest"
	"github.com/blockadence/archimedes/internal/reposync"
)

func TestSelectReposFilters(t *testing.T) {
	m := &manifest.Manifest{Repos: []manifest.Repo{
		{Name: "alpha"}, {Name: "beta"}, {Name: "gamma"},
	}}

	cases := []struct {
		name, filter string
		want         []string
	}{
		{"no filter selects every repo", "", []string{"alpha", "beta", "gamma"}},
		{"filter selects just that repo", "beta", []string{"beta"}},
		{"filter matching nothing selects nothing", "delta", nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, r := range reposync.SelectRepos(m, tc.filter) {
				got = append(got, r.Name)
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("SelectRepos(%q) = %v, want %v", tc.filter, got, tc.want)
			}
		})
	}
}

func TestMultiGitterArgs(t *testing.T) {
	args := reposync.MultiGitterArgs("/tmp/mod.sh", []string{"acme/alpha", "acme/beta"}, "tok", false)

	if args[0] != "run" || args[1] != "/tmp/mod.sh" {
		t.Errorf("expected `run <script>` first, got %v", args[:2])
	}

	joined := strings.Join(args, " ")
	for _, want := range []string{
		"-R acme/alpha", "-R acme/beta",
		"--branch " + reposync.TemplatesBranch,
		"--token tok",
		"--commit-message", "--pr-title", "--pr-body",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("args missing %q: %v", want, args)
		}
	}
	if strings.Contains(joined, "--dry-run") {
		t.Errorf("non-dry-run args should not carry --dry-run: %v", args)
	}

	dry := reposync.MultiGitterArgs("/tmp/mod.sh", []string{"acme/alpha"}, "tok", true)
	if !contains(dry, "--dry-run") {
		t.Errorf("dry run args missing --dry-run: %v", dry)
	}
}

func contains(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

// writeScaffolding builds the canonical template set an instance carries in
// scaffolding/, the source the mod script copies from.
func writeScaffolding(t *testing.T, dir string) string {
	t.Helper()
	scaffold := filepath.Join(dir, "scaffolding")
	mustMkdirAll(t, filepath.Join(scaffold, "ISSUE_TEMPLATE"))
	mustWriteFile(t, filepath.Join(scaffold, "PULL_REQUEST_TEMPLATE.md"), "## What changed\n")
	mustWriteFile(t, filepath.Join(scaffold, "ISSUE_TEMPLATE", "bug.yml"), "name: Bug\n")
	mustWriteFile(t, filepath.Join(scaffold, "ISSUE_TEMPLATE", "enhancement.yml"), "name: Enhancement\n")
	mustWriteFile(t, filepath.Join(scaffold, "ISSUE_TEMPLATE", "config.yml"), "blank_issues_enabled: false\n")
	return scaffold
}

// TestModScriptAppliesTemplates runs the generated mod script the way
// multi-gitter does — as an executable with the working directory set to a
// cloned repo — and checks what it leaves behind.
func TestModScriptAppliesTemplates(t *testing.T) {
	tmp := t.TempDir()
	scaffold := writeScaffolding(t, tmp)

	clone := filepath.Join(tmp, "clone")
	mustMkdirAll(t, filepath.Join(clone, ".github"))
	// A legacy single-file issue template the canonical set supersedes.
	mustWriteFile(t, filepath.Join(clone, ".github", "ISSUE_TEMPLATE.md"), "old\n")

	script := filepath.Join(tmp, "mod.sh")
	mustWriteFile(t, script, reposync.ModScript(scaffold))
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(script)
	cmd.Dir = clone
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("mod script failed: %v\n%s", err, out)
	}

	wantFiles := map[string]string{
		filepath.Join(".github", "PULL_REQUEST_TEMPLATE.md"):          "## What changed\n",
		filepath.Join(".github", "ISSUE_TEMPLATE", "bug.yml"):         "name: Bug\n",
		filepath.Join(".github", "ISSUE_TEMPLATE", "enhancement.yml"): "name: Enhancement\n",
		filepath.Join(".github", "ISSUE_TEMPLATE", "config.yml"):      "blank_issues_enabled: false\n",
	}
	for rel, want := range wantFiles {
		got, err := os.ReadFile(filepath.Join(clone, rel))
		if err != nil {
			t.Fatalf("reading %s: %v", rel, err)
		}
		if string(got) != want {
			t.Errorf("%s = %q, want %q", rel, got, want)
		}
	}

	if _, err := os.Stat(filepath.Join(clone, ".github", "ISSUE_TEMPLATE.md")); !os.IsNotExist(err) {
		t.Error("legacy .github/ISSUE_TEMPLATE.md should have been removed")
	}
}

// TestModScriptHandlesAwkwardScaffoldPath guards the quoting of the
// scaffolding path baked into the script.
func TestModScriptHandlesAwkwardScaffoldPath(t *testing.T) {
	tmp := t.TempDir()
	awkward := filepath.Join(tmp, "a dir's name")
	mustMkdirAll(t, awkward)
	scaffold := writeScaffolding(t, awkward)

	clone := filepath.Join(tmp, "clone")
	mustMkdirAll(t, clone)

	script := filepath.Join(tmp, "mod.sh")
	mustWriteFile(t, script, reposync.ModScript(scaffold))
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(script)
	cmd.Dir = clone
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("mod script failed for path with a quote: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(clone, ".github", "PULL_REQUEST_TEMPLATE.md")); err != nil {
		t.Errorf("template not applied: %v", err)
	}
}

// syncTemplates runs SyncTemplates against inst with a fake exec seam,
// returning that seam plus what the command reported.
func syncTemplates(t *testing.T, opts reposync.TemplatesOptions, fake *fakeExec) (string, error) {
	t.Helper()
	var out, progress bytes.Buffer
	err := reposync.SyncTemplates(opts, &out, &progress, fake.run)
	return out.String() + progress.String(), err
}

func templatesInstance(t *testing.T) instance {
	t.Helper()
	inst := newInstance(t, "alpha", "beta")
	inst.setGitHubRemote(t, "alpha", "acme/alpha")
	inst.setGitHubRemote(t, "beta", "acme/beta")
	writeScaffolding(t, inst.root)
	return inst
}

func TestSyncTemplatesFansOutOverEveryRepo(t *testing.T) {
	inst := templatesInstance(t)
	fake := &fakeExec{}

	if _, err := syncTemplates(t, reposync.TemplatesOptions{Root: inst.root}, fake); err != nil {
		t.Fatalf("SyncTemplates: %v", err)
	}

	mg, ok := fake.find("multi-gitter")
	if !ok {
		t.Fatalf("multi-gitter was never invoked; calls: %v", fake.calls)
	}
	for _, want := range []string{"-R acme/alpha", "-R acme/beta", "--token fake-token"} {
		if !strings.Contains(mg.joined(), want) {
			t.Errorf("multi-gitter args missing %q: %v", want, mg.args)
		}
	}
	if strings.Contains(mg.joined(), "--dry-run") {
		t.Errorf("a real run should not pass --dry-run: %v", mg.args)
	}
}

func TestSyncTemplatesRepoFilterNarrowsTheFanOut(t *testing.T) {
	inst := templatesInstance(t)
	fake := &fakeExec{}

	if _, err := syncTemplates(t, reposync.TemplatesOptions{Root: inst.root, Repo: "beta"}, fake); err != nil {
		t.Fatalf("SyncTemplates: %v", err)
	}

	mg, _ := fake.find("multi-gitter")
	if !strings.Contains(mg.joined(), "-R acme/beta") {
		t.Errorf("filtered run should target beta: %v", mg.args)
	}
	if strings.Contains(mg.joined(), "acme/alpha") {
		t.Errorf("filtered run should not target alpha: %v", mg.args)
	}
}

func TestSyncTemplatesDryRunPassesTheFlagThrough(t *testing.T) {
	inst := templatesInstance(t)
	fake := &fakeExec{}

	if _, err := syncTemplates(t, reposync.TemplatesOptions{Root: inst.root, DryRun: true}, fake); err != nil {
		t.Fatalf("SyncTemplates: %v", err)
	}

	mg, _ := fake.find("multi-gitter")
	if !contains(mg.args, "--dry-run") {
		t.Errorf("dry run should pass --dry-run to multi-gitter: %v", mg.args)
	}
}

// TestSyncTemplatesHandsMultiGitterARunnableScript checks the one thing the
// fan-out can't verify for itself: that the mod script path multi-gitter is
// given exists and is executable at the moment it's handed over.
func TestSyncTemplatesHandsMultiGitterARunnableScript(t *testing.T) {
	inst := templatesInstance(t)

	var scriptPath string
	fake := &fakeExec{}
	inspect := func(name string, args []string, stdout, stderr io.Writer) error {
		if name == "multi-gitter" {
			scriptPath = args[1]
			info, err := os.Stat(scriptPath)
			if err != nil {
				t.Errorf("mod script %s missing when multi-gitter runs: %v", scriptPath, err)
			} else if info.Mode().Perm()&0o100 == 0 {
				t.Errorf("mod script %s is not executable (mode %v)", scriptPath, info.Mode())
			}
		}
		return fake.run(name, args, stdout, stderr)
	}

	var out, progress bytes.Buffer
	if err := reposync.SyncTemplates(reposync.TemplatesOptions{Root: inst.root}, &out, &progress, inspect); err != nil {
		t.Fatalf("SyncTemplates: %v", err)
	}

	if scriptPath == "" {
		t.Fatal("multi-gitter was never given a script")
	}
	if _, err := os.Stat(scriptPath); !os.IsNotExist(err) {
		t.Errorf("mod script %s should be cleaned up after the run", scriptPath)
	}
}

func TestSyncTemplatesUnmatchedFilterIsAnError(t *testing.T) {
	inst := templatesInstance(t)
	fake := &fakeExec{}

	_, err := syncTemplates(t, reposync.TemplatesOptions{Root: inst.root, Repo: "nope"}, fake)
	if err == nil {
		t.Fatal("expected an error when no repo matches the filter")
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Errorf("error should name the unmatched filter, got: %v", err)
	}
	if _, ran := fake.find("multi-gitter"); ran {
		t.Error("multi-gitter should not run when no repos matched")
	}
}

func TestSyncTemplatesMissingScaffoldingIsAnError(t *testing.T) {
	inst := newInstance(t, "alpha")
	inst.setGitHubRemote(t, "alpha", "acme/alpha")
	fake := &fakeExec{}

	_, err := syncTemplates(t, reposync.TemplatesOptions{Root: inst.root}, fake)
	if err == nil {
		t.Fatal("expected an error when the instance has no scaffolding/ directory")
	}
	if !strings.Contains(err.Error(), "scaffolding") {
		t.Errorf("error should name the missing scaffolding directory, got: %v", err)
	}
}
