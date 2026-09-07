package cmd

import (
	"testing"
)

func TestResolveWorkspaceIsOffByDefault(t *testing.T) {
	t.Setenv(workspaceEnvVar, "")

	got, err := resolveWorkspace("")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("got %+v, want nil: the integration must be opt-in", got)
	}
}

func TestResolveWorkspaceReadsTheEnvironment(t *testing.T) {
	t.Setenv(workspaceEnvVar, "herdr")

	got, err := resolveWorkspace("")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Name != "herdr" {
		t.Errorf("got %+v, want the herdr integration", got)
	}
}

// The flag is the per-invocation override, in both directions: it turns
// the integration on for one spawn, and — the case that matters when it's
// on instance-wide — off again for one spawn.
func TestResolveWorkspaceFlagOverridesTheEnvironment(t *testing.T) {
	t.Setenv(workspaceEnvVar, "")
	got, err := resolveWorkspace("herdr")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Error("--workspace herdr did not enable the integration")
	}

	t.Setenv(workspaceEnvVar, "herdr")
	got, err = resolveWorkspace("off")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("--workspace off did not override %s=herdr: %+v", workspaceEnvVar, got)
	}
}

func TestResolveWorkspaceRejectsAnUnknownName(t *testing.T) {
	t.Setenv(workspaceEnvVar, "")

	if _, err := resolveWorkspace("tmux"); err == nil {
		t.Error("expected an error for an unknown integration, got nil")
	}
}
