package contextmap_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/blockadence/archimedes/cli/internal/contextmap"
)

// runPass drives one mapping pass and returns what the operator sees on the
// result stream. stdin is empty unless the caller supplies confirmations.
func runPass(t *testing.T, opts contextmap.Options, confirmations string) (out string, err error) {
	t.Helper()
	var buf bytes.Buffer
	err = contextmap.Run(opts, &buf, io.Discard, strings.NewReader(confirmations))
	return buf.String(), err
}

const twoRepoYAML = `repos:
  - name: app
    path: ../app
    base_branch: main
    depends_on: [shared]
    context_modeled_sha: null
  - name: shared
    path: ../shared
    base_branch: main
    depends_on: []
    context_modeled_sha: null
`

func TestRunMapsEveryStaleRepoWithAPathParameterizedDriver(t *testing.T) {
	inst := newInstance(t, "driver: stub-ok\n"+twoRepoYAML, "app", "shared")
	inst.installPathDriver(t, "stub-ok")

	out, err := runPass(t, contextmap.Options{Root: inst.root}, "")
	if err != nil {
		t.Fatalf("Run returned error: %v\n%s", err, out)
	}

	if want := "Planned order: shared app"; !strings.Contains(out, want) {
		t.Errorf("output missing %q:\n%s", want, out)
	}
	for _, name := range []string{"app", "shared"} {
		if got := readFile(t, inst.contextFile(name)); !strings.Contains(got, "stub-ok mapped") {
			t.Errorf("%s CONTEXT.md = %q, want it written by the configured driver", name, got)
		}
		if got, want := inst.recordedSHA(t, name), inst.sha(t, name); got != want {
			t.Errorf("%s recorded sha = %q, want the current base-branch commit %q", name, got, want)
		}
	}
	if want := "Context-mapping pass complete."; !strings.Contains(out, want) {
		t.Errorf("output missing %q:\n%s", want, out)
	}
}

func TestRunMapsWithAFixedLocationDriver(t *testing.T) {
	inst := newInstance(t, "driver: stub-fixed\n"+twoRepoYAML, "app", "shared")
	// Writes only where it likes (OUT.md), never told an output path.
	inst.installDriver(t, "stub-fixed",
		"name: stub-fixed\noutput_mode: fixed-location\nfixed_path: OUT.md\ncommand: run.sh\n",
		"#!/usr/bin/env bash\nset -euo pipefail\necho \"stub-fixed mapped $1\" > \"$1/OUT.md\"\n")

	out, err := runPass(t, contextmap.Options{Root: inst.root}, "")
	if err != nil {
		t.Fatalf("Run returned error: %v\n%s", err, out)
	}

	for _, name := range []string{"app", "shared"} {
		if got := readFile(t, inst.contextFile(name)); !strings.Contains(got, "stub-fixed mapped") {
			t.Errorf("%s CONTEXT.md = %q, want the driver's output harvested into it", name, got)
		}
		assertNoFile(t, inst.repoPath(name)+"/OUT.md", "harvesting leaves no trace, so the target repo")
		if got, want := inst.recordedSHA(t, name), inst.sha(t, name); got != want {
			t.Errorf("%s recorded sha = %q, want %q", name, got, want)
		}
	}
}

func TestRunHonorsInstanceDefaultAndPerRepoOverride(t *testing.T) {
	yaml := `driver: instance-default
repos:
  - name: app
    path: ../app
    base_branch: main
    depends_on: []
    context_modeled_sha: null
  - name: shared
    path: ../shared
    base_branch: main
    depends_on: []
    context_modeled_sha: null
    driver: repo-override
`
	inst := newInstance(t, yaml, "app", "shared")
	inst.installPathDriver(t, "instance-default")
	inst.installPathDriver(t, "repo-override")

	if out, err := runPass(t, contextmap.Options{Root: inst.root}, ""); err != nil {
		t.Fatalf("Run returned error: %v\n%s", err, out)
	}

	if got := readFile(t, inst.contextFile("app")); !strings.Contains(got, "instance-default mapped") {
		t.Errorf("app used %q, want the instance-wide default driver", got)
	}
	if got := readFile(t, inst.contextFile("shared")); !strings.Contains(got, "repo-override mapped") {
		t.Errorf("shared used %q, want its own driver override", got)
	}
}

func TestRunUsesTheEnvironmentDefaultWhenReposYAMLNamesNoDriver(t *testing.T) {
	inst := newInstance(t, twoRepoYAML, "app", "shared")
	inst.installPathDriver(t, "from-environment")

	if out, err := runPass(t, contextmap.Options{Root: inst.root, Driver: "from-environment"}, ""); err != nil {
		t.Fatalf("Run returned error: %v\n%s", err, out)
	}

	if got := readFile(t, inst.contextFile("app")); !strings.Contains(got, "from-environment mapped") {
		t.Errorf("app CONTEXT.md = %q, want the environment-configured driver", got)
	}
}

func TestRunFailsClearlyOnAnUnknownDriver(t *testing.T) {
	inst := newInstance(t, "driver: does-not-exist\n"+twoRepoYAML, "app", "shared")

	out, err := runPass(t, contextmap.Options{Root: inst.root}, "")
	if err == nil {
		t.Fatalf("expected an error for an unknown driver, got nil\n%s", out)
	}
	if !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("error %q does not name the unknown driver", err)
	}
	assertNoFile(t, inst.contextFile("shared"), "an unknown driver maps nothing, so the repo")
	if got := inst.recordedSHA(t, "shared"); got != "" {
		t.Errorf("recorded sha = %q, want nothing recorded for a repo that was never mapped", got)
	}
}

func TestRunFailsClearlyOnAnUnknownPerRepoOverride(t *testing.T) {
	yaml := `driver: stub-ok
repos:
  - name: app
    path: ../app
    base_branch: main
    depends_on: []
    context_modeled_sha: null
    driver: also-does-not-exist
`
	inst := newInstance(t, yaml, "app")
	inst.installPathDriver(t, "stub-ok")

	out, err := runPass(t, contextmap.Options{Root: inst.root}, "")
	if err == nil {
		t.Fatalf("expected an error for an unknown per-repo override, got nil\n%s", out)
	}
	if !strings.Contains(err.Error(), "also-does-not-exist") {
		t.Errorf("error %q does not name the unknown driver — a bad override must not fall back to the instance default", err)
	}
	assertNoFile(t, inst.contextFile("app"), "a bad override maps nothing, so the repo")
}

func TestRunDryRunShowsOrderAndStalenessWithoutInvokingAnyDriver(t *testing.T) {
	inst := newInstance(t, "driver: stub-ok\n"+twoRepoYAML, "app", "shared")
	inst.installPathDriver(t, "stub-ok")

	out, err := runPass(t, contextmap.Options{Root: inst.root, DryRun: true}, "")
	if err != nil {
		t.Fatalf("Run returned error: %v\n%s", err, out)
	}

	for _, want := range []string{
		"Planned order: shared app",
		"=== shared (never mapped) ===",
		"=== app (never mapped) ===",
		"Dry run: no sessions launched, no repos.yaml changes made.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output missing %q:\n%s", want, out)
		}
	}
	for _, name := range []string{"app", "shared"} {
		assertNoFile(t, inst.contextFile(name), "a dry run invokes no driver, so the repo")
		if got := inst.recordedSHA(t, name); got != "" {
			t.Errorf("%s recorded sha = %q, want a dry run to leave repos.yaml alone", name, got)
		}
	}
}

func TestRunDryRunReportsStalenessAgainstWhatIsRecorded(t *testing.T) {
	inst := newInstance(t, twoRepoYAML, "app", "shared")
	// shared is current; app was mapped at a commit that isn't the tip.
	mustWriteFile(t, inst.contextFile("shared"), "# shared\n")
	mustWriteFile(t, inst.contextFile("app"), "# app\n")
	setSHA(t, inst, "shared", inst.sha(t, "shared"))
	setSHA(t, inst, "app", "0123456789abcdef0123456789abcdef01234567")

	out, err := runPass(t, contextmap.Options{Root: inst.root, DryRun: true}, "")
	if err != nil {
		t.Fatalf("Run returned error: %v\n%s", err, out)
	}

	if want := "Up to date: shared (@ " + contextmap.Short(inst.sha(t, "shared")) + ")"; !strings.Contains(out, want) {
		t.Errorf("output missing %q:\n%s", want, out)
	}
	if want := "=== app (stale, 01234567 -> " + contextmap.Short(inst.sha(t, "app")); !strings.Contains(out, want) {
		t.Errorf("output missing %q:\n%s", want, out)
	}
}

func TestRunRemapsWhenTheContextFileWasDeleted(t *testing.T) {
	inst := newInstance(t, "driver: stub-ok\n"+twoRepoYAML, "app", "shared")
	inst.installPathDriver(t, "stub-ok")
	setSHA(t, inst, "shared", inst.sha(t, "shared"))
	setSHA(t, inst, "app", inst.sha(t, "app"))
	mustWriteFile(t, inst.contextFile("app"), "# app\n")

	out, err := runPass(t, contextmap.Options{Root: inst.root}, "")
	if err != nil {
		t.Fatalf("Run returned error: %v\n%s", err, out)
	}

	if want := "=== shared (CONTEXT.md missing) ==="; !strings.Contains(out, want) {
		t.Errorf("output missing %q:\n%s", want, out)
	}
	if want := "Up to date: app"; !strings.Contains(out, want) {
		t.Errorf("output missing %q:\n%s", want, out)
	}
	if got := readFile(t, inst.contextFile("shared")); !strings.Contains(got, "stub-ok mapped") {
		t.Errorf("shared CONTEXT.md = %q, want it rebuilt", got)
	}
	if got := readFile(t, inst.contextFile("app")); got != "# app\n" {
		t.Errorf("app CONTEXT.md = %q, want an up-to-date repo left alone", got)
	}
}

func TestRunSkipsAReposYAMLEntryThatIsNotClonedYet(t *testing.T) {
	yaml := `driver: stub-ok
repos:
  - name: app
    path: ../app
    base_branch: main
  - name: not-cloned
    path: ../not-cloned
    base_branch: main
`
	inst := newInstance(t, yaml, "app")
	inst.installPathDriver(t, "stub-ok")

	out, err := runPass(t, contextmap.Options{Root: inst.root}, "")
	if err != nil {
		t.Fatalf("Run returned error: %v\n%s", err, out)
	}

	if !strings.Contains(out, "Skipping not-cloned, not cloned yet") {
		t.Errorf("output missing the not-cloned-yet skip:\n%s", out)
	}
	if got := readFile(t, inst.contextFile("app")); !strings.Contains(got, "stub-ok mapped") {
		t.Errorf("app CONTEXT.md = %q, want the cloned repo mapped anyway", got)
	}
}

func TestRunHonorsAnAlternateContextFileName(t *testing.T) {
	inst := newInstance(t, "driver: stub-ok\n"+twoRepoYAML, "app", "shared")
	inst.installPathDriver(t, "stub-ok")

	out, err := runPass(t, contextmap.Options{Root: inst.root, ContextFile: "docs/DOMAIN.md"}, "")
	if err != nil {
		t.Fatalf("Run returned error: %v\n%s", err, out)
	}

	if got := readFile(t, inst.repoPath("app")+"/docs/DOMAIN.md"); !strings.Contains(got, "stub-ok mapped") {
		t.Errorf("map = %q, want it at the configured context file path", got)
	}
	assertNoFile(t, inst.contextFile("app"), "with a configured context file name, the default location")
}

func TestRunInteractiveSessionPrimesDependenciesAndRecordsOnConfirmation(t *testing.T) {
	inst := newInstance(t, twoRepoYAML, "app", "shared")

	out, err := runPass(t, contextmap.Options{Root: inst.root, AgentCmd: "my-agent"}, "\n\n")
	if err != nil {
		t.Fatalf("Run returned error: %v\n%s", err, out)
	}

	for _, want := range []string{
		"cd " + inst.repoPath("app") + " && my-agent",
		"Depends on (already mapped, prime the session with these):",
		"- shared: " + inst.repoPath("shared") + "/CONTEXT.md",
		"Dependencies: shared.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("interactive output missing %q:\n%s", want, out)
		}
	}
	for _, name := range []string{"app", "shared"} {
		if got, want := inst.recordedSHA(t, name), inst.sha(t, name); got != want {
			t.Errorf("%s recorded sha = %q, want %q after confirming the session", name, got, want)
		}
	}
}

func TestRunInteractiveSessionUsesAConfiguredPrompt(t *testing.T) {
	inst := newInstance(t, twoRepoYAML, "app", "shared")

	out, err := runPass(t, contextmap.Options{Root: inst.root, ContextPrompt: "do the thing"}, "\n\n")
	if err != nil {
		t.Fatalf("Run returned error: %v\n%s", err, out)
	}

	if !strings.Contains(out, "do the thing") {
		t.Errorf("output missing the configured prompt:\n%s", out)
	}
	if strings.Contains(out, "Build or refresh") {
		t.Errorf("output still carries the default prompt:\n%s", out)
	}
}

// Without a driver the pass needs a human to confirm each session actually
// happened. Nothing to read from means nobody confirmed anything, so it
// must stop rather than record every repo as mapped.
func TestRunInteractiveSessionWithNothingToConfirmFails(t *testing.T) {
	inst := newInstance(t, twoRepoYAML, "app", "shared")

	out, err := runPass(t, contextmap.Options{Root: inst.root}, "")
	if err == nil {
		t.Fatalf("expected an error when no confirmation is available, got nil\n%s", out)
	}
	if got := inst.recordedSHA(t, "shared"); got != "" {
		t.Errorf("recorded sha = %q, want nothing recorded for an unconfirmed session", got)
	}
}

func TestRunWarnsAboutADependencyCycleAndStillMapsEverything(t *testing.T) {
	yaml := `driver: stub-ok
repos:
  - name: app
    path: ../app
    base_branch: main
    depends_on: [shared]
  - name: shared
    path: ../shared
    base_branch: main
    depends_on: [app]
`
	inst := newInstance(t, yaml, "app", "shared")
	inst.installPathDriver(t, "stub-ok")

	var out, progress bytes.Buffer
	err := contextmap.Run(contextmap.Options{Root: inst.root}, &out, &progress, strings.NewReader(""))
	if err != nil {
		t.Fatalf("Run returned error: %v\n%s", err, out.String())
	}

	if !strings.Contains(progress.String(), "Cycle or unresolved dependency among") {
		t.Errorf("cycle warning missing from the diagnostic stream:\n%s", progress.String())
	}
	if want := "Planned order: app shared"; !strings.Contains(out.String(), want) {
		t.Errorf("output missing %q:\n%s", want, out.String())
	}
	for _, name := range []string{"app", "shared"} {
		if got := readFile(t, inst.contextFile(name)); !strings.Contains(got, "stub-ok mapped") {
			t.Errorf("%s was not mapped despite the cycle: %q", name, got)
		}
	}
}

// The result stream is the operator's record of what happened; a chatty
// driver must not end up interleaved into it.
func TestRunKeepsDriverChatterOffTheResultStream(t *testing.T) {
	inst := newInstance(t, "driver: chatty\n"+twoRepoYAML, "app", "shared")
	inst.installDriver(t, "chatty",
		"name: chatty\noutput_mode: path-parameterized\ncommand: run.sh\n",
		"#!/usr/bin/env bash\nset -euo pipefail\necho 'chatty driver thinking out loud'\necho 'chatty mapped' > \"$2\"\n")

	var out, progress bytes.Buffer
	if err := contextmap.Run(contextmap.Options{Root: inst.root}, &out, &progress, strings.NewReader("")); err != nil {
		t.Fatalf("Run returned error: %v\n%s", err, out.String())
	}

	if strings.Contains(out.String(), "thinking out loud") {
		t.Errorf("driver output leaked onto the result stream:\n%s", out.String())
	}
	if !strings.Contains(progress.String(), "thinking out loud") {
		t.Errorf("driver output missing from the diagnostic stream:\n%s", progress.String())
	}
}

func TestRunOnAnInstanceWithNoRepos(t *testing.T) {
	inst := newInstance(t, "repos: []\n")

	out, err := runPass(t, contextmap.Options{Root: inst.root}, "")
	if err != nil {
		t.Fatalf("Run returned error: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Context-mapping pass complete.") {
		t.Errorf("output missing the completion line:\n%s", out)
	}
}

func TestRunOnAMissingInstanceFails(t *testing.T) {
	if _, err := runPass(t, contextmap.Options{Root: t.TempDir()}, ""); err == nil {
		t.Fatal("expected an error for a directory with no repos.yaml, got nil")
	}
}
