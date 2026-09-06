package status

import (
	"os/exec"
	"testing"
)

func TestGhSlugParsesRemoteURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"ssh", "git@github.com:blockadence/archimedes.git", "blockadence/archimedes"},
		{"https", "https://github.com/blockadence/archimedes.git", "blockadence/archimedes"},
		{"non-github unchanged", "git@gitlab.com:blockadence/archimedes.git", "git@gitlab.com:blockadence/archimedes.git"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			run(t, dir, "git", "init", "-q")
			run(t, dir, "git", "remote", "add", "origin", tt.url)

			got, err := ghSlug(dir)
			if err != nil {
				t.Fatalf("ghSlug returned error: %v", err)
			}
			if got != tt.want {
				t.Errorf("ghSlug(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestGhSlugNoRemoteErrors(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "git", "init", "-q")

	if _, err := ghSlug(dir); err == nil {
		t.Fatal("expected error when origin remote is missing, got nil")
	}
}

func run(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}

func TestParsePRList(t *testing.T) {
	tests := []struct {
		name string
		json string
		want PR
	}{
		{"empty array", `[]`, noPR},
		{"one pr", `[{"number":42,"state":"OPEN"}]`, PR{Number: "42", State: "OPEN"}},
		{"malformed json", `not json`, noPR},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parsePRList([]byte(tt.json))
			if got != tt.want {
				t.Errorf("parsePRList(%q) = %#v, want %#v", tt.json, got, tt.want)
			}
		})
	}
}
