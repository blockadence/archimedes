package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	repo := filepath.Join(tmp, "app")
	run(t, tmp, "git", "init", "-q", "--bare", "-b", "main", filepath.Join(tmp, "app.git"))
	run(t, tmp, "git", "clone", "-q", filepath.Join(tmp, "app.git"), repo)
	writeFile(t, filepath.Join(repo, "README.md"), "# app\n")
	run(t, repo, "git", "add", "-A")
	run(t, repo, "git", "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "-m", "init")
	run(t, repo, "git", "push", "-q", "origin", "main")

	root := filepath.Join(tmp, "instance")
	writeFile(t, filepath.Join(root, "repos.yaml"),
		"driver: stub\nrepos:\n  - name: app\n    path: ../app\n    base_branch: main\n")
	writeFile(t, filepath.Join(root, "drivers", "stub", "driver.yaml"),
		"name: stub\noutput_mode: path-parameterized\ncommand: run.sh\n")
	driverBin := filepath.Join(root, "drivers", "stub", "run.sh")
	writeFile(t, driverBin, "#!/usr/bin/env bash\nset -euo pipefail\necho 'stub mapped' > \"$2\"\n")
	if err := os.Chmod(driverBin, 0o755); err != nil {
		t.Fatal(err)
	}

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

// execute runs the whole command tree with args, returning stdout.
func execute(t *testing.T, args ...string) string {
	t.Helper()
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetIn(strings.NewReader(""))
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("archimedes %v: %v\n%s", args, err, out.String())
	}
	return out.String()
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
