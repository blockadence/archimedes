package status

import (
	"errors"
	"strings"
	"testing"
)

func stubRepoPath(known map[string]string) RepoPath {
	return func(name string) (string, error) {
		path, ok := known[name]
		if !ok {
			return "", errors.New("unknown repo: " + name)
		}
		return path, nil
	}
}

func stubLookup(prs map[string]PR) PRLookup {
	return func(repoPath, headBranch string) (PR, error) {
		if pr, ok := prs[repoPath+"@"+headBranch]; ok {
			return pr, nil
		}
		return PR{}, errors.New("no stub for " + repoPath + "@" + headBranch)
	}
}

func TestBuildReportLooksUpEachRow(t *testing.T) {
	entries := []Entry{
		{Slug: "my-slug", Repo: "service-a", Note: "based on main"},
		{Slug: "my-slug", Repo: "service-b", Note: "stacked on service-a:my-slug"},
	}

	repoPath := stubRepoPath(map[string]string{
		"service-a": "/repos/service-a",
		"service-b": "/repos/service-b",
	})
	lookup := stubLookup(map[string]PR{
		"/repos/service-a@my-slug": {Number: "42", State: "OPEN"},
		"/repos/service-b@my-slug": {Number: "-", State: "no PR"},
	})

	got := BuildReport(entries, repoPath, lookup, 3)

	want := []Row{
		{Slug: "my-slug", Repo: "service-a", PRNumber: "42", PRState: "OPEN", Note: "based on main"},
		{Slug: "my-slug", Repo: "service-b", PRNumber: "-", PRState: "no PR", Note: "stacked on service-a:my-slug"},
	}
	if len(got.Rows) != len(want) {
		t.Fatalf("expected %d rows, got %d: %#v", len(want), len(got.Rows), got.Rows)
	}
	for i := range want {
		if got.Rows[i] != want[i] {
			t.Errorf("row %d mismatch\n got: %#v\nwant: %#v", i, got.Rows[i], want[i])
		}
	}
	if got.Count != 2 {
		t.Errorf("expected count 2, got %d", got.Count)
	}
	if got.GuardrailHit {
		t.Errorf("expected guardrail not hit at count=2, max=3")
	}
}

func TestBuildReportDegradesFailedLookupsToNoPR(t *testing.T) {
	entries := []Entry{
		{Slug: "my-slug", Repo: "unknown-repo", Note: "note"},
	}
	repoPath := stubRepoPath(map[string]string{})
	lookup := stubLookup(map[string]PR{})

	got := BuildReport(entries, repoPath, lookup, 3)

	want := Row{Slug: "my-slug", Repo: "unknown-repo", PRNumber: "-", PRState: "no PR", Note: "note"}
	if got.Rows[0] != want {
		t.Errorf("row mismatch\n got: %#v\nwant: %#v", got.Rows[0], want)
	}
}

func TestBuildReportGuardrail(t *testing.T) {
	entries := make([]Entry, 4)
	for i := range entries {
		entries[i] = Entry{Slug: "slug", Repo: "repo"}
	}
	repoPath := stubRepoPath(map[string]string{"repo": "/repos/repo"})
	lookup := stubLookup(map[string]PR{"/repos/repo@slug": {Number: "-", State: "no PR"}})

	got := BuildReport(entries, repoPath, lookup, 3)

	if got.Count != 4 {
		t.Errorf("expected count 4, got %d", got.Count)
	}
	if !got.GuardrailHit {
		t.Errorf("expected guardrail hit at count=4, max=3")
	}
}

func TestFormatHuman(t *testing.T) {
	report := Report{
		Rows: []Row{
			{Slug: "my-slug", Repo: "service-a", PRNumber: "42", PRState: "OPEN", Note: "based on main"},
		},
		Count:        1,
		GuardrailMax: 3,
		GuardrailHit: false,
	}

	got := FormatHuman(report)
	want := "SLUG                 REPO           PR#      STATE      NOTE                          \n" +
		"my-slug              service-a      42       OPEN       based on main                 \n" +
		"\n" +
		"\n" +
		todoTrailer + "\n"

	if got != want {
		t.Errorf("FormatHuman mismatch\n got: %q\nwant: %q", got, want)
	}
}

func TestParseGuardrailMax(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want int
	}{
		{"unset defaults to 3", "", 3},
		{"valid override", "5", 5},
		{"non-numeric falls back to default", "not-a-number", 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseGuardrailMax(tt.raw); got != tt.want {
				t.Errorf("ParseGuardrailMax(%q) = %d, want %d", tt.raw, got, tt.want)
			}
		})
	}
}

func TestFormatHumanIncludesGuardrailWarning(t *testing.T) {
	report := Report{
		Rows:         nil,
		Count:        4,
		GuardrailMax: 3,
		GuardrailHit: true,
	}

	got := FormatHuman(report)
	if !strings.Contains(got, "Warning: 4 active worktree streams open, guardrail is 3. Consider closing some out.") {
		t.Errorf("expected guardrail warning in output, got: %q", got)
	}
}
