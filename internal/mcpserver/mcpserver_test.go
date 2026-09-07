package mcpserver_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blockadence/gh-archimedes/internal/mcpserver"
	"github.com/blockadence/gh-archimedes/internal/spawn"
	"github.com/blockadence/gh-archimedes/internal/status"
)

// quietSources answers every live lookup without touching gh or the
// network, so the status tool's own behavior is what's under test.
func quietSources() status.Sources {
	return status.Sources{
		PR:     func(string, string) (status.PR, error) { return status.PR{Number: "-", State: "no PR"}, nil },
		Refs:   noRefs{},
		Merged: func(string, string) bool { return false },
	}
}

type noRefs struct{}

func (noRefs) HasRef(string, string) bool             { return false }
func (noRefs) IsAncestor(string, string, string) bool { return false }

func TestServesTheFourWaysIn(t *testing.T) {
	inst := newInstance(t)
	cs := connect(t, mcpserver.Options{Root: inst.root})

	tools, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("listing tools: %v", err)
	}

	readOnly := map[string]bool{}
	for _, tool := range tools.Tools {
		if tool.Description == "" {
			t.Errorf("tool %s has no description for a model to read", tool.Name)
		}
		readOnly[tool.Name] = tool.Annotations != nil && tool.Annotations.ReadOnlyHint
	}

	for _, want := range []string{"list_repos", "repo_status", "context_map_status"} {
		got, ok := readOnly[want]
		if !ok {
			t.Errorf("missing tool %s, got %v", want, readOnly)
			continue
		}
		if !got {
			t.Errorf("%s only reads state, so it should be annotated read-only", want)
		}
	}
	if got, ok := readOnly["spawn_worktree"]; !ok {
		t.Errorf("missing tool spawn_worktree, got %v", readOnly)
	} else if got {
		t.Error("spawn_worktree creates a branch and worktree, so it must not claim to be read-only")
	}
}

func TestListReposReportsTheManifest(t *testing.T) {
	inst := newInstance(t)
	cs := connect(t, mcpserver.Options{Root: inst.root})

	var got mcpserver.Repos
	call(t, cs, "list_repos", map[string]any{}, &got)

	if len(got.Repos) != 2 {
		t.Fatalf("expected both tracked repos, got %#v", got.Repos)
	}
	app := got.Repos[0]
	want := mcpserver.Repo{
		Name:           "app",
		Path:           "../app",
		Checkout:       filepath.Join(inst.root, "../app"),
		BaseBranch:     "main",
		DependsOn:      []string{"shared"},
		ConventionPack: "go",
	}
	if app.Name != want.Name || app.Path != want.Path || app.BaseBranch != want.BaseBranch ||
		app.ConventionPack != want.ConventionPack || len(app.DependsOn) != 1 || app.DependsOn[0] != "shared" {
		t.Errorf("app entry mismatch\n got: %#v\nwant: %#v", app, want)
	}
	// The recorded path is relative to the instance; an agent needs one it
	// can actually open.
	if _, err := os.Stat(app.Checkout); err != nil {
		t.Errorf("checkout %q is not a usable path: %v", app.Checkout, err)
	}
}

func TestRepoStatusReportsEveryRow(t *testing.T) {
	inst := newInstance(t)
	cs := connect(t, mcpserver.Options{Root: inst.root, Status: quietSources()})

	var got status.Report
	call(t, cs, "repo_status", map[string]any{}, &got)

	if got.Count != 2 {
		t.Fatalf("expected both rows, got %#v", got.Rows)
	}
	if got.Rows[0].Slug != "widget-fix" || got.Rows[0].Repo != "app" {
		t.Errorf("first row mismatch: %#v", got.Rows[0])
	}
	if got.GuardrailMax != status.DefaultGuardrailMax {
		t.Errorf("guardrail max = %d, want the default %d", got.GuardrailMax, status.DefaultGuardrailMax)
	}
}

func TestRepoStatusNarrowsToOneSlug(t *testing.T) {
	inst := newInstance(t)
	mustWriteFile(t, filepath.Join(inst.root, "work", "other-slug", "status.md"),
		"# other-slug\n\n| repo | branch | worktree | note | pr |\n|---|---|---|---|---|\n"+
			"| app | other-slug | /wt/app-2 | based on main | - |\n")
	cs := connect(t, mcpserver.Options{Root: inst.root, Status: quietSources()})

	var got status.Report
	call(t, cs, "repo_status", map[string]any{"slug": "widget-fix"}, &got)

	if got.Count != 2 {
		t.Fatalf("expected only widget-fix's two rows, got %#v", got.Rows)
	}
	for _, row := range got.Rows {
		if row.Slug != "widget-fix" {
			t.Errorf("slug filter leaked %q into the report", row.Slug)
		}
	}
}

func TestContextMapStatusReportsStalenessInDependencyOrder(t *testing.T) {
	inst := newInstance(t)
	cs := connect(t, mcpserver.Options{Root: inst.root})

	var got mcpserver.Plan
	call(t, cs, "context_map_status", map[string]any{}, &got)

	if len(got.Order) != 2 || got.Order[0] != "shared" || got.Order[1] != "app" {
		t.Fatalf("expected dependency-first order [shared app], got %v", got.Order)
	}
	for _, r := range got.Repos {
		if !r.Stale || r.Reason != "never mapped" {
			t.Errorf("%s: expected an unmapped repo reported stale, got %#v", r.Name, r)
		}
	}

	// Once a map exists and is recorded, the same question answers the
	// other way — the tool reads live state, not a snapshot.
	mustWriteFile(t, filepath.Join(inst.repoPaths["shared"], "CONTEXT.md"), "# shared\n")
	setSHA(t, inst, "shared")

	var after mcpserver.Plan
	call(t, cs, "context_map_status", map[string]any{}, &after)
	if after.Repos[0].Name != "shared" || after.Repos[0].Stale {
		t.Errorf("expected shared reported current after being mapped, got %#v", after.Repos[0])
	}
	if !after.Stale() {
		t.Error("expected the plan to still report app outstanding")
	}
}

func TestSpawnWorktreeCreatesTheBranchAndWorktree(t *testing.T) {
	inst := newInstance(t)
	cs := connect(t, mcpserver.Options{Root: inst.root})

	var got spawn.Result
	call(t, cs, "spawn_worktree", map[string]any{"slug": "new-thing", "repo": "app"}, &got)

	if got.Branch != "new-thing" || got.Repo != "app" || got.StartRef != "origin/main" {
		t.Errorf("result mismatch: %#v", got)
	}
	if info, err := os.Stat(got.Worktree); err != nil || !info.IsDir() {
		t.Fatalf("worktree %q was not created: %v", got.Worktree, err)
	}
	// The unit of work is recorded, so the status tool sees it next time.
	row := readFile(t, filepath.Join(inst.root, "work", "new-thing", "status.md"))
	if !strings.Contains(row, "new-thing") || !strings.Contains(row, "app") {
		t.Errorf("status file missing the spawned row:\n%s", row)
	}
}

func TestSpawnWorktreeStacksOnAnotherSlug(t *testing.T) {
	inst := newInstance(t)
	cs := connect(t, mcpserver.Options{Root: inst.root})

	var base spawn.Result
	call(t, cs, "spawn_worktree", map[string]any{"slug": "auth-api", "repo": "app"}, &base)

	var got spawn.Result
	call(t, cs, "spawn_worktree", map[string]any{"slug": "auth-ui", "repo": "app", "stack_on": "app:auth-api"}, &got)

	if got.StartRef != "auth-api" {
		t.Errorf("start ref = %q, want the base slug's branch", got.StartRef)
	}
	if got.Note != "stacked on app:auth-api" {
		t.Errorf("note = %q, want it to record the stack", got.Note)
	}
}

// A bad argument is the model's mistake to correct, so it comes back as a
// tool error it can read — not as a protocol failure that kills the call.
func TestSpawnWorktreeReportsAnUnknownRepoAsAToolError(t *testing.T) {
	inst := newInstance(t)
	cs := connect(t, mcpserver.Options{Root: inst.root})

	res := callRaw(t, cs, "spawn_worktree", map[string]any{"slug": "new-thing", "repo": "nope"})

	if !res.IsError {
		t.Fatalf("expected a tool error for an unknown repo, got %#v", res.StructuredContent)
	}
	if !strings.Contains(resultText(res), "unknown repo") {
		t.Errorf("error text %q doesn't say what went wrong", resultText(res))
	}
}

// Everything the tools shell out to writes to the progress stream, never to
// stdout — over stdio, stdout is the protocol itself.
func TestGitProgressGoesToTheConfiguredStream(t *testing.T) {
	inst := newInstance(t)
	var progress strings.Builder
	cs := connect(t, mcpserver.Options{Root: inst.root, Progress: &progress})

	call(t, cs, "spawn_worktree", map[string]any{"slug": "new-thing", "repo": "app"}, nil)

	if !strings.Contains(progress.String(), "worktree") {
		t.Errorf("expected git's own chatter on the progress stream, got:\n%s", progress.String())
	}
}

// The guardrail setting is carried unparsed and read by the same parser the
// status command uses, so "0" is a threshold of zero rather than an unset
// field falling back to the default.
func TestRepoStatusHonoursAZeroGuardrail(t *testing.T) {
	inst := newInstance(t)
	cs := connect(t, mcpserver.Options{Root: inst.root, Status: quietSources(), MaxStreams: "0"})

	var got status.Report
	call(t, cs, "repo_status", map[string]any{}, &got)

	if got.GuardrailMax != 0 || !got.GuardrailHit {
		t.Errorf("expected two open worktrees to trip a guardrail of 0, got %#v", got)
	}
}
