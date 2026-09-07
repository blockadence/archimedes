package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blockadence/gh-archimedes/internal/prune"
	"github.com/blockadence/gh-archimedes/internal/testrepo"
)

// setupInstance builds an instance root with one target repo (with a
// spawned worktree+branch for slug) plus a matching work/<slug>/status.md,
// the same shape spawn produces.
func setupInstance(t *testing.T, root, repoName, slug, note string) (repoPath, wt string) {
	t.Helper()

	repoPath = testrepo.New(t, testrepo.Spec{
		Dir:    root,
		Name:   repoName,
		Origin: repoName + "-origin.git",
	}).Clone

	wt = filepath.Join(root, repoName+"-worktrees", slug)
	if err := os.MkdirAll(filepath.Dir(wt), 0o755); err != nil {
		t.Fatal(err)
	}
	testrepo.Git(t, repoPath, "worktree", "add", wt, "-b", slug, "main")

	reposYAML := "repos:\n  - name: " + repoName + "\n    path: ./" + repoName + "\n    base_branch: main\n"
	if err := os.WriteFile(filepath.Join(root, "repos.yaml"), []byte(reposYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	statusDir := filepath.Join(root, "work", slug)
	if err := os.MkdirAll(statusDir, 0o755); err != nil {
		t.Fatal(err)
	}
	status := "# " + slug + "\n\n| repo | branch | worktree | note | pr |\n|---|---|---|---|---|\n" +
		"| " + repoName + " | " + slug + " | " + wt + " | " + note + " | - |\n"
	if err := os.WriteFile(filepath.Join(statusDir, "status.md"), []byte(status), 0o644); err != nil {
		t.Fatal(err)
	}

	return repoPath, wt
}

func TestRunPruneDryRunListsCandidateWithoutRemoving(t *testing.T) {
	root := t.TempDir()
	_, wt := setupInstance(t, root, "service-a", "widget-fix", "based on main")

	var buf bytes.Buffer
	merged := func(_, _ string) (string, error) { return "MERGED", nil }
	if err := runPrune(&buf, root, "", false, merged); err != nil {
		t.Fatalf("runPrune: %v", err)
	}

	if _, err := os.Stat(wt); err != nil {
		t.Errorf("dry run must not remove the worktree, stat err = %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "PRUNE CANDIDATE: service-a:widget-fix (MERGED) at "+wt) {
		t.Errorf("missing candidate line, got:\n%s", out)
	}
	if !strings.Contains(out, "Dry run. Re-run with --force") {
		t.Errorf("missing dry-run notice, got:\n%s", out)
	}
}

func TestRunPruneForceRemovesWorktreeBranchAndStatusRow(t *testing.T) {
	root := t.TempDir()
	repoPath, wt := setupInstance(t, root, "service-a", "widget-fix", "based on main")

	var buf bytes.Buffer
	merged := func(_, _ string) (string, error) { return "MERGED", nil }
	if err := runPrune(&buf, root, "", true, merged); err != nil {
		t.Fatalf("runPrune: %v", err)
	}

	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Errorf("expected worktree to be removed, stat err = %v", err)
	}

	if branches := testrepo.GitOut(t, repoPath, "branch", "--list", "widget-fix"); branches != "" {
		t.Errorf("expected branch widget-fix to be gone, got %q", branches)
	}

	statusPath := filepath.Join(root, "work", "widget-fix", "status.md")
	rows := prune.ParseStatusFile(readFile(t, statusPath))
	if len(rows) != 0 {
		t.Errorf("expected status.md row to be removed, got %+v", rows)
	}

	out := buf.String()
	if !strings.Contains(out, "removed.") {
		t.Errorf("missing removal confirmation, got:\n%s", out)
	}
	if strings.Contains(out, "Dry run") {
		t.Errorf("force run must not print the dry-run notice, got:\n%s", out)
	}
}

func TestRunPruneRefusesToRemoveAStackedBase(t *testing.T) {
	root := t.TempDir()
	repoPath, wt := setupInstance(t, root, "service-a", "widget-fix", "based on main")

	// A second unit of work, stacked on widget-fix, also merged.
	stackDir := filepath.Join(root, "work", "shim-fix")
	if err := os.MkdirAll(stackDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stackStatus := "# shim-fix\n\n| repo | branch | worktree | note | pr |\n|---|---|---|---|---|\n" +
		"| service-a | shim-fix | " + filepath.Join(root, "service-a-worktrees", "shim-fix") + " | stacked on service-a:widget-fix | - |\n"
	if err := os.WriteFile(filepath.Join(stackDir, "status.md"), []byte(stackStatus), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	// Only widget-fix's PR has merged; shim-fix's is still open, so this
	// test isolates the stacked-base refusal to widget-fix.
	onlyWidgetFixMerged := func(_, headBranch string) (string, error) {
		if headBranch == "widget-fix" {
			return "MERGED", nil
		}
		return "OPEN", nil
	}
	if err := runPrune(&buf, root, "", true, onlyWidgetFixMerged); err != nil {
		t.Fatalf("runPrune: %v", err)
	}

	if _, err := os.Stat(wt); err != nil {
		t.Errorf("expected stacked-on worktree to survive, stat err = %v", err)
	}
	if branches := testrepo.GitOut(t, repoPath, "branch", "--list", "widget-fix"); branches == "" {
		t.Errorf("expected branch widget-fix to survive since shim-fix stacks on it")
	}

	out := buf.String()
	if !strings.Contains(out, "SKIP service-a:widget-fix (MERGED), still a base for:") {
		t.Errorf("missing SKIP line, got:\n%s", out)
	}
	if !strings.Contains(out, "Rebase that one first.") {
		t.Errorf("missing rebase hint, got:\n%s", out)
	}
}

func TestRunPruneMissingDependencyErrors(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // a PATH with neither git nor gh on it

	var buf bytes.Buffer
	err := runPrune(&buf, t.TempDir(), "", false, func(_, _ string) (string, error) { return "NONE", nil })
	if err == nil {
		t.Fatal("expected an error when git/gh aren't on PATH, got nil")
	}
	if !strings.Contains(err.Error(), "missing dependency") {
		t.Errorf("expected a missing-dependency error, got: %v", err)
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
