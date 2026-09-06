package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blockadence/archimedes/cli/internal/status"
)

func writeInstanceFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	reposYAML := "repos:\n" +
		"  - name: service-a\n" +
		"    path: ../service-a\n" +
		"    base_branch: main\n" +
		"  - name: service-b\n" +
		"    path: ../service-b\n" +
		"    base_branch: main\n"
	if err := os.WriteFile(filepath.Join(dir, "repos.yaml"), []byte(reposYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	slugDir := filepath.Join(dir, "work", "my-slug")
	if err := os.MkdirAll(slugDir, 0o755); err != nil {
		t.Fatal(err)
	}
	statusMD := "# my-slug\n\n| repo | branch | worktree | note | pr |\n|---|---|---|---|---|\n" +
		"| service-a | my-slug | /wt/service-a | based on main | - |\n" +
		"| service-b | my-slug | /wt/service-b | stacked on service-a:my-slug | - |\n"
	if err := os.WriteFile(filepath.Join(slugDir, "status.md"), []byte(statusMD), 0o644); err != nil {
		t.Fatal(err)
	}

	return dir
}

func fixtureLookup(repoPath, headBranch string) (status.PR, error) {
	if filepath.Base(repoPath) == "service-a" {
		return status.PR{Number: "42", State: "OPEN"}, nil
	}
	return status.PR{Number: "-", State: "no PR"}, nil
}

func TestRunStatusHumanTable(t *testing.T) {
	dir := writeInstanceFixture(t)

	var buf bytes.Buffer
	if err := runStatus(&buf, dir, "", false, fixtureLookup); err != nil {
		t.Fatalf("runStatus returned error: %v", err)
	}

	got := buf.String()
	for _, want := range []string{
		"SLUG", "REPO", "PR#", "STATE", "NOTE",
		"my-slug              service-a      42       OPEN       based on main",
		"my-slug              service-b      -        no PR      stacked on service-a:my-slug",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected output to contain %q, got:\n%s", want, got)
		}
	}
}

func TestRunStatusFiltersBySlug(t *testing.T) {
	dir := writeInstanceFixture(t)

	slugDir := filepath.Join(dir, "work", "other-slug")
	if err := os.MkdirAll(slugDir, 0o755); err != nil {
		t.Fatal(err)
	}
	statusMD := "# other-slug\n\n| repo | branch | worktree | note | pr |\n|---|---|---|---|---|\n" +
		"| service-a | other-slug | /wt/service-a-2 | based on main | - |\n"
	if err := os.WriteFile(filepath.Join(slugDir, "status.md"), []byte(statusMD), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := runStatus(&buf, dir, "my-slug", false, fixtureLookup); err != nil {
		t.Fatalf("runStatus returned error: %v", err)
	}

	got := buf.String()
	if strings.Contains(got, "other-slug") {
		t.Errorf("expected other-slug to be filtered out, got:\n%s", got)
	}
	if !strings.Contains(got, "my-slug") {
		t.Errorf("expected my-slug in output, got:\n%s", got)
	}
}

func TestRunStatusJSON(t *testing.T) {
	dir := writeInstanceFixture(t)

	var buf bytes.Buffer
	if err := runStatus(&buf, dir, "", true, fixtureLookup); err != nil {
		t.Fatalf("runStatus returned error: %v", err)
	}

	var report status.Report
	if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
		t.Fatalf("output isn't valid JSON: %v\n%s", err, buf.String())
	}

	if report.Count != 2 {
		t.Errorf("expected count 2, got %d", report.Count)
	}
	want := status.Row{Slug: "my-slug", Repo: "service-a", PRNumber: "42", PRState: "OPEN", Note: "based on main"}
	if report.Rows[0] != want {
		t.Errorf("row mismatch\n got: %#v\nwant: %#v", report.Rows[0], want)
	}
}

func TestRunStatusMissingManifestErrors(t *testing.T) {
	dir := t.TempDir()

	var buf bytes.Buffer
	if err := runStatus(&buf, dir, "", false, fixtureLookup); err == nil {
		t.Fatal("expected error when repos.yaml is missing, got nil")
	}
}

func TestRunStatusGuardrailWarning(t *testing.T) {
	dir := writeInstanceFixture(t)
	t.Setenv("ARCHIMEDES_MAX_STREAMS", "1")

	var buf bytes.Buffer
	if err := runStatus(&buf, dir, "", false, fixtureLookup); err != nil {
		t.Fatalf("runStatus returned error: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "Warning: 2 active worktree streams open, guardrail is 1. Consider closing some out.") {
		t.Errorf("expected guardrail warning, got:\n%s", got)
	}
}
