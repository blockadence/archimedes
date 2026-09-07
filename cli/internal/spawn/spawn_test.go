package spawn_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/blockadence/archimedes/cli/internal/dossier"
	"github.com/blockadence/archimedes/cli/internal/spawn"
)

func TestParseStackRef(t *testing.T) {
	cases := []struct {
		in   string
		want spawn.StackRef
	}{
		{"", spawn.StackRef{}},
		{"target:widget-fix", spawn.StackRef{Repo: "target", Slug: "widget-fix"}},
		{"target", spawn.StackRef{Repo: "target"}},
		{"target:widget:fix", spawn.StackRef{Repo: "target", Slug: "widget:fix"}},
	}
	for _, tc := range cases {
		if got := spawn.ParseStackRef(tc.in); got != tc.want {
			t.Errorf("ParseStackRef(%q) = %+v, want %+v", tc.in, got, tc.want)
		}
	}
}

func TestResolveStartPoint(t *testing.T) {
	cases := []struct {
		name                     string
		baseBranch, baseOverride string
		stack                    spawn.StackRef
		wantRef, wantNote        string
	}{
		{
			name:       "default uses origin/<base branch>",
			baseBranch: "main",
			wantRef:    "origin/main",
			wantNote:   "based on main",
		},
		{
			name:         "base override wins over default",
			baseBranch:   "main",
			baseOverride: "release/1.2",
			wantRef:      "release/1.2",
			wantNote:     "based on release/1.2",
		},
		{
			name:         "stack ref wins over base override",
			baseBranch:   "main",
			baseOverride: "release/1.2",
			stack:        spawn.StackRef{Repo: "target", Slug: "widget-fix"},
			wantRef:      "widget-fix",
			wantNote:     "stacked on target:widget-fix",
		},
		{
			name:       "stack presence is keyed on Repo, not Slug",
			baseBranch: "main",
			stack:      spawn.StackRef{Repo: "target"},
			wantRef:    "",
			wantNote:   "stacked on target:",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := spawn.ResolveStartPoint(tc.baseBranch, tc.baseOverride, tc.stack)
			if got.Ref != tc.wantRef || got.Note != tc.wantNote {
				t.Errorf("got {Ref: %q, Note: %q}, want {Ref: %q, Note: %q}", got.Ref, got.Note, tc.wantRef, tc.wantNote)
			}
		})
	}
}

func TestWorktreePath(t *testing.T) {
	got := spawn.WorktreePath("/instance/target-repo", "widget-fix")
	want := "/instance/target-repo-worktrees/widget-fix"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNextStepHint(t *testing.T) {
	if got, want := spawn.NextStepHint("/wt", ""), "cd /wt && claude"; got != want {
		t.Errorf("default agent: got %q, want %q", got, want)
	}
	if got, want := spawn.NextStepHint("/wt", "codex"), "cd /wt && codex"; got != want {
		t.Errorf("configured agent: got %q, want %q", got, want)
	}
}

// run invokes spawn.Run with the given options, failing the test on error
// and returning what it wrote to its result stream.
func run(t *testing.T, opts spawn.Options) string {
	t.Helper()
	var out, progress bytes.Buffer
	if err := spawn.Run(opts, &out, &progress); err != nil {
		t.Fatalf("spawn.Run: %v\nprogress:\n%s", err, progress.String())
	}
	return out.String()
}

func TestRunFetchesFirstAndDefaultsToBaseBranch(t *testing.T) {
	inst := newInstance(t)
	tmp := filepath.Dir(inst.targetRepo)

	// Simulate the local checkout being stale relative to origin: push a
	// new commit straight to origin without updating the clone.
	otherClone := filepath.Join(tmp, "other-clone")
	gitOK(t, tmp, "clone", "-q", filepath.Join(tmp, "target-repo.git"), otherClone)
	mustWriteFile(t, filepath.Join(otherClone, "new-file.txt"), "new")
	gitOK(t, otherClone, "add", "-A")
	gitCommit(t, otherClone, "remote-advances")
	gitOK(t, otherClone, "push", "-q", "origin", "main")

	slug := "widget-fix"
	inst.workSlug(t, slug)

	out := run(t, spawn.Options{Root: inst.root, Slug: slug, Repo: "target"})

	wt := spawn.WorktreePath(inst.targetRepo, slug)
	if _, err := os.Stat(filepath.Join(wt, "new-file.txt")); err != nil {
		t.Errorf("worktree did not start from freshly-fetched origin/main: %v", err)
	}
	if got := gitOut(t, wt, "rev-parse", "--abbrev-ref", "HEAD"); got != slug {
		t.Errorf("branch name = %q, want %q", got, slug)
	}
	if !bytes.Contains([]byte(out), []byte("based on main")) {
		t.Errorf("output missing default resolution note: %s", out)
	}
}

func TestRunBaseOverride(t *testing.T) {
	inst := newInstance(t)

	gitOK(t, inst.targetRepo, "checkout", "-q", "-b", "release/1.0")
	mustWriteFile(t, filepath.Join(inst.targetRepo, "release-marker.txt"), "r1")
	gitOK(t, inst.targetRepo, "add", "-A")
	gitCommit(t, inst.targetRepo, "release branch")
	gitOK(t, inst.targetRepo, "push", "-q", "origin", "release/1.0")
	gitOK(t, inst.targetRepo, "checkout", "-q", "main")

	slug := "hotfix"
	inst.workSlug(t, slug)

	out := run(t, spawn.Options{Root: inst.root, Slug: slug, Repo: "target", Base: "release/1.0"})

	wt := spawn.WorktreePath(inst.targetRepo, slug)
	if _, err := os.Stat(filepath.Join(wt, "release-marker.txt")); err != nil {
		t.Errorf("worktree did not start from the base override: %v", err)
	}
	if !bytes.Contains([]byte(out), []byte("based on release/1.0")) {
		t.Errorf("output missing base-override resolution note: %s", out)
	}
}

func TestRunStackedOnAnotherSlug(t *testing.T) {
	inst := newInstance(t)

	base := "widget-fix"
	inst.workSlug(t, base)
	run(t, spawn.Options{Root: inst.root, Slug: base, Repo: "target"})

	baseWT := spawn.WorktreePath(inst.targetRepo, base)
	mustWriteFile(t, filepath.Join(baseWT, "base-work.txt"), "base")
	gitOK(t, baseWT, "add", "-A")
	gitCommit(t, baseWT, "base slug work")

	stacked := "widget-fix-followup"
	inst.workSlug(t, stacked)
	out := run(t, spawn.Options{
		Root: inst.root, Slug: stacked, Repo: "target",
		Stack: spawn.StackRef{Repo: "target", Slug: base},
	})

	stackedWT := spawn.WorktreePath(inst.targetRepo, stacked)
	if _, err := os.Stat(filepath.Join(stackedWT, "base-work.txt")); err != nil {
		t.Errorf("stacked worktree did not start from the base slug's branch: %v", err)
	}
	if !bytes.Contains([]byte(out), []byte("stacked on target:widget-fix")) {
		t.Errorf("output missing stacked-branch resolution note: %s", out)
	}
}

func TestRunMaterializesContextAndTracksStatus(t *testing.T) {
	inst := newInstance(t)

	slug := "widget-fix"
	workSlugDir := inst.workSlug(t, slug)
	mustWriteFile(t, filepath.Join(workSlugDir, "ticket.md"), "# Ticket\n")

	out := run(t, spawn.Options{Root: inst.root, Slug: slug, Repo: "target"})

	wt := spawn.WorktreePath(inst.targetRepo, slug)
	if _, err := os.Stat(filepath.Join(wt, spawn.ContextDirName, "ticket.md")); err != nil {
		t.Errorf("ticket.md was not materialized: %v", err)
	}
	if status := gitOut(t, wt, "status", "--porcelain"); status != "" {
		t.Errorf("git status surfaced the materialized context: %q", status)
	}
	if !bytes.Contains([]byte(out), []byte("Worktree ready: "+wt)) {
		t.Errorf("missing 'Worktree ready' line: %s", out)
	}

	statusContent, err := os.ReadFile(filepath.Join(workSlugDir, spawn.StatusFileName))
	if err != nil {
		t.Fatalf("status.md was not written: %v", err)
	}
	want := "# widget-fix\n\n| repo | branch | worktree | note | pr |\n|---|---|---|---|---|\n" +
		"| target | widget-fix | " + wt + " | based on main | - |\n"
	if string(statusContent) != want {
		t.Errorf("status.md mismatch\n got: %q\nwant: %q", statusContent, want)
	}

	// A second repo for the same slug appends a row, and must not receive
	// the first spawn's bookkeeping as if it were reference material.
	out2 := run(t, spawn.Options{Root: inst.root, Slug: slug, Repo: "target2", AgentCmd: "codex"})
	if !bytes.Contains([]byte(out2), []byte("codex")) {
		t.Errorf("next-step hint did not respect the configured agent: %s", out2)
	}

	wt2 := spawn.WorktreePath(inst.targetRepo2, slug)
	if _, err := os.Stat(filepath.Join(wt2, spawn.ContextDirName, spawn.StatusFileName)); !os.IsNotExist(err) {
		t.Error("status.md (bookkeeping) leaked into the second worktree's materialized context")
	}
	if _, err := os.Stat(filepath.Join(wt2, spawn.ContextDirName, "ticket.md")); err != nil {
		t.Errorf("ticket.md was not materialized into the second worktree: %v", err)
	}

	statusContent, err = os.ReadFile(filepath.Join(workSlugDir, spawn.StatusFileName))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(statusContent, []byte("| target2 |")) {
		t.Errorf("second repo's row was not appended: %s", statusContent)
	}
}

// A relative --root must resolve against the caller's working directory,
// not against the target repo git runs in. Getting this wrong nests the
// worktree inside the checkout it should sit beside, and materializes the
// context into a different directory entirely.
func TestRunRelativeRootDoesNotNestWorktreeInsideRepo(t *testing.T) {
	inst := newInstance(t)

	slug := "widget-fix"
	workSlugDir := inst.workSlug(t, slug)
	mustWriteFile(t, filepath.Join(workSlugDir, "ticket.md"), "# Ticket\n")

	t.Chdir(inst.root)
	out := run(t, spawn.Options{Root: ".", Slug: slug, Repo: "target"})

	wt := spawn.WorktreePath(inst.targetRepo, slug)
	if _, err := os.Stat(filepath.Join(wt, spawn.ContextDirName, "ticket.md")); err != nil {
		t.Errorf("ticket.md was not materialized into the real worktree: %v", err)
	}
	if status := gitOut(t, inst.targetRepo, "status", "--porcelain"); status != "" {
		t.Errorf("worktree was nested inside the target repo, polluting its git status: %q", status)
	}

	// The recorded path must stay usable from anywhere, since prune and
	// the printed hint both consume it from outside this directory.
	statusContent, err := os.ReadFile(filepath.Join(workSlugDir, spawn.StatusFileName))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(statusContent, []byte(wt)) {
		t.Errorf("status.md recorded a non-absolute worktree path: %s", statusContent)
	}
	if !bytes.Contains([]byte(out), []byte(wt)) {
		t.Errorf("next-step hint recorded a non-absolute worktree path: %s", out)
	}
}

// repos.yaml's path is not required to be the "../<name>" sibling layout
// bootstrap.sh happens to produce. A repo checked out *below* the instance
// root is the case where a mis-resolved relative path is worst: the
// worktree lands inside the target checkout and shows up in its git status.
func TestRunResolvesRepoPathsBelowInstanceRoot(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "instance")
	mustMkdirAll(t, filepath.Join(root, "repos"))

	targetRepo := makeTargetRepo(t, tmp, "target-repo")
	nested := filepath.Join(root, "repos", "target")
	if err := os.Rename(targetRepo, nested); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(root, "repos.yaml"),
		"repos:\n  - name: target\n    path: repos/target\n    base_branch: main\n")

	slug := "widget-fix"
	workSlugDir := filepath.Join(root, "work", slug)
	mustMkdirAll(t, workSlugDir)
	mustWriteFile(t, filepath.Join(workSlugDir, "ticket.md"), "# Ticket\n")

	t.Chdir(root)
	run(t, spawn.Options{Root: ".", Slug: slug, Repo: "target"})

	if status := gitOut(t, nested, "status", "--porcelain"); status != "" {
		t.Errorf("worktree was nested inside the target repo, polluting its git status: %q", status)
	}
	wt := spawn.WorktreePath(nested, slug)
	if _, err := os.Stat(filepath.Join(wt, spawn.ContextDirName, "ticket.md")); err != nil {
		t.Errorf("ticket.md was not materialized into the real worktree: %v", err)
	}
}

// The end-to-end equivalent of tests/house_rules_spawn.sh: spawning
// delivers the target repo's house rules even when the slug has no
// reference material of its own.
func TestRunDeliversHouseRules(t *testing.T) {
	inst := newInstance(t)

	rules := "Never rebase a shared branch.\nAll schema changes go through the migration tool, no exceptions."
	mustMkdirAll(t, filepath.Join(inst.root, "repos"))
	mustWriteFile(t, filepath.Join(inst.root, "repos", "target.md"),
		"# target\n\n## House rules\n\n"+rules+"\n\n## Known gotchas\nn/a\n")

	slug := "quiet-fix"
	inst.workSlug(t, slug)

	run(t, spawn.Options{Root: inst.root, Slug: slug, Repo: "target"})

	wt := spawn.WorktreePath(inst.targetRepo, slug)
	got, err := os.ReadFile(filepath.Join(wt, spawn.ContextDirName, dossier.HouseRulesFileName))
	if err != nil {
		t.Fatalf("%s was not delivered: %v", dossier.HouseRulesFileName, err)
	}
	if string(got) != rules+"\n" {
		t.Errorf("house rules diverged from the dossier\n got: %q\nwant: %q", got, rules+"\n")
	}
	if status := gitOut(t, wt, "status", "--porcelain"); status != "" {
		t.Errorf("the ephemeral house-rules copy surfaced in git status: %q", status)
	}

	// A repo with no dossier at all gets no house rules, and with no
	// reference material either, no context directory at all.
	other := "another-fix"
	inst.workSlug(t, other)
	run(t, spawn.Options{Root: inst.root, Slug: other, Repo: "target2"})

	wt2 := spawn.WorktreePath(inst.targetRepo2, other)
	if _, err := os.Stat(filepath.Join(wt2, spawn.ContextDirName)); !os.IsNotExist(err) {
		t.Error("an empty context dir was created for a repo with no house rules and no reference material")
	}
}

func TestRunUnknownRepoErrors(t *testing.T) {
	inst := newInstance(t)
	inst.workSlug(t, "widget-fix")

	var out, progress bytes.Buffer
	err := spawn.Run(spawn.Options{Root: inst.root, Slug: "widget-fix", Repo: "does-not-exist"}, &out, &progress)
	if err == nil {
		t.Fatal("expected an error for an unknown repo, got nil")
	}
}

func TestRunGitProgressStaysOffResultStream(t *testing.T) {
	inst := newInstance(t)
	slug := "widget-fix"
	inst.workSlug(t, slug)

	var out, progress bytes.Buffer
	if err := spawn.Run(spawn.Options{Root: inst.root, Slug: slug, Repo: "target"}, &out, &progress); err != nil {
		t.Fatalf("spawn.Run: %v", err)
	}

	// The result stream carries exactly the two caller-facing lines; git's
	// own chatter belongs on the progress stream.
	if lines := bytes.Count(out.Bytes(), []byte("\n")); lines != 2 {
		t.Errorf("expected 2 result lines, got %d:\n%s", lines, out.String())
	}
	if bytes.Contains(out.Bytes(), []byte("Preparing worktree")) {
		t.Errorf("git progress leaked onto the result stream:\n%s", out.String())
	}
}
