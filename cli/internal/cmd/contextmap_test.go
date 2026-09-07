package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blockadence/archimedes/cli/internal/testrepo"
)

func TestContextMapOptionsFromEnvironment(t *testing.T) {
	env := map[string]string{
		agentCmdEnvVar:      "my-agent",
		contextFileEnvVar:   "docs/DOMAIN.md",
		contextPromptEnvVar: "map it",
		driverEnvVar:        "openspec",
		driversDirEnvVar:    "/elsewhere/drivers",
	}

	opts := contextMapOptions("/instance", true, func(k string) string { return env[k] })

	if opts.Root != "/instance" || !opts.DryRun {
		t.Errorf("got Root=%q DryRun=%v, want the flag values through", opts.Root, opts.DryRun)
	}
	if opts.AgentCmd != "my-agent" || opts.ContextFile != "docs/DOMAIN.md" || opts.ContextPrompt != "map it" {
		t.Errorf("session overrides not picked up: %+v", opts)
	}
	if opts.Driver != "openspec" || opts.DriversDir != "/elsewhere/drivers" {
		t.Errorf("driver overrides not picked up: %+v", opts)
	}
}

func TestContextMapOptionsDefaultToEmptyWhenUnset(t *testing.T) {
	opts := contextMapOptions(".", false, func(string) string { return "" })

	if opts.AgentCmd != "" || opts.ContextFile != "" || opts.Driver != "" || opts.DriversDir != "" || opts.ContextPrompt != "" {
		t.Errorf("unset environment should leave the package's own defaults to apply: %+v", opts)
	}
}

// End-to-end through the command tree: a real instance, a real driver, and
// the flags an operator actually types.
func TestContextMapCommandDryRunThenMaps(t *testing.T) {
	tmp := t.TempDir()
	repo := testrepo.New(t, testrepo.Spec{Dir: tmp, Name: "app"}).Clone

	root := filepath.Join(tmp, "instance")
	writeFile(t, filepath.Join(root, "repos.yaml"),
		"driver: stub\nrepos:\n  - name: app\n    path: ../app\n    base_branch: main\n")
	writeFile(t, filepath.Join(root, "drivers", "stub", "driver.yaml"),
		"name: stub\noutput_mode: path-parameterized\ncommand: run.sh\n")
	writeExecutable(t, filepath.Join(root, "drivers", "stub", "run.sh"),
		"#!/usr/bin/env bash\nset -euo pipefail\necho 'stub mapped' > \"$2\"\n")

	out := execute(t, "context-map", "--root", root, "--dry-run")
	if !strings.Contains(out, "Planned order: app") || !strings.Contains(out, "Dry run") {
		t.Errorf("dry run output:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(repo, "CONTEXT.md")); err == nil {
		t.Error("dry run invoked the driver")
	}

	out = execute(t, "context-map", "--root", root)
	if !strings.Contains(out, "Context-mapping pass complete.") {
		t.Errorf("mapping pass output:\n%s", out)
	}
	if got, err := os.ReadFile(filepath.Join(repo, "CONTEXT.md")); err != nil || !strings.Contains(string(got), "stub mapped") {
		t.Errorf("CONTEXT.md = %q (err %v), want the driver's output", got, err)
	}
}
