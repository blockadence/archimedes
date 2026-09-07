package cmd

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/blockadence/archimedes/cli/internal/dashboard"
)

func TestRunDashboardWithoutATerminalPointsAtStatus(t *testing.T) {
	err := runDashboard(&bytes.Buffer{}, strings.NewReader(""), dashboard.Options{Root: "."}, time.Second)
	if err == nil {
		t.Fatal("expected a dashboard drawn into a buffer to be refused")
	}
	if !strings.Contains(err.Error(), "archimedes status") {
		t.Errorf("error = %v, want it to name the subcommand that works in a pipe", err)
	}
}

func TestDashboardOptionsHonorTheSameEnvironmentAsTheSubcommandsItMirrors(t *testing.T) {
	env := map[string]string{
		contextFileEnvVar: "ARCHITECTURE.md",
		maxStreamsEnvVar:  "7",
	}

	opts := dashboardOptions("/instance", func(k string) string { return env[k] })

	if opts.Root != "/instance" {
		t.Errorf("Root = %q, want /instance", opts.Root)
	}
	if opts.ContextFile != "ARCHITECTURE.md" {
		t.Errorf("ContextFile = %q, want the ARCHIMEDES_CONTEXT_FILE override", opts.ContextFile)
	}
	if opts.GuardrailMax != 7 {
		t.Errorf("GuardrailMax = %d, want the ARCHIMEDES_MAX_STREAMS override", opts.GuardrailMax)
	}
	if opts.SHA == nil || opts.Sources.PR == nil || opts.Sources.Merged == nil || opts.Sources.Refs == nil {
		t.Error("every source a reading needs should be wired up")
	}
}

func TestDashboardOptionsFallBackToTheSameDefaultsAsStatus(t *testing.T) {
	opts := dashboardOptions(".", func(string) string { return "" })

	if opts.ContextFile != "" {
		t.Errorf("ContextFile = %q, want it left empty for internal/contextmap to default", opts.ContextFile)
	}
	if opts.GuardrailMax != 3 {
		t.Errorf("GuardrailMax = %d, want status's default of 3", opts.GuardrailMax)
	}
}

// The dashboard is additive: adding it must not have moved anything the
// plain CLI already offered.
func TestRootStillCarriesEveryPortedSubcommand(t *testing.T) {
	want := []string{
		"bootstrap", "render-map", "context-map", "spawn", "status", "prune",
		"sync-templates", "sync-house-rules", "dashboard",
	}

	have := map[string]bool{}
	for _, c := range newRootCmd().Commands() {
		have[c.Name()] = true
	}
	for _, name := range want {
		if !have[name] {
			t.Errorf("subcommand %q is missing from the command tree", name)
		}
	}
}
