package prune_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blockadence/gh-archimedes/internal/prune"
)

func writeStatus(t *testing.T, dir, slug, body string) string {
	t.Helper()
	slugDir := filepath.Join(dir, slug)
	if err := os.MkdirAll(slugDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(slugDir, "status.md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func statusBody(slug string, rows ...string) string {
	body := "# " + slug + "\n\n| repo | branch | worktree | note | pr |\n|---|---|---|---|---|\n"
	for _, r := range rows {
		body += r + "\n"
	}
	return body
}

func TestParseStatusFile(t *testing.T) {
	body := statusBody("widget-fix",
		"| service-a | widget-fix | /wt/service-a | based on main | - |",
		"| service-b | widget-fix | /wt/service-b | stacked on service-a:widget-fix | 42 |",
	)

	rows := prune.ParseStatusFile([]byte(body))
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if rows[0].Repo != "service-a" || rows[0].Worktree != "/wt/service-a" || rows[0].Note != "based on main" {
		t.Errorf("row 0 = %+v", rows[0])
	}
	if rows[1].Repo != "service-b" || rows[1].Note != "stacked on service-a:widget-fix" || rows[1].PR != "42" {
		t.Errorf("row 1 = %+v", rows[1])
	}
}

func TestParseStatusFileSkipsHeaderAndBlankRows(t *testing.T) {
	body := statusBody("solo") // no data rows at all
	rows := prune.ParseStatusFile([]byte(body))
	if len(rows) != 0 {
		t.Fatalf("got %d rows, want 0", len(rows))
	}
}

func alwaysMerged(_, _ string) (string, error) { return "MERGED", nil }

func TestScanFindsMergedCandidate(t *testing.T) {
	dir := t.TempDir()
	writeStatus(t, dir, "widget-fix", statusBody("widget-fix",
		"| service-a | widget-fix | /wt/service-a | based on main | - |",
	))

	items, err := prune.Scan(dir, "", alwaysMerged)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	it := items[0]
	if it.Slug != "widget-fix" || it.Repo != "service-a" || it.Worktree != "/wt/service-a" {
		t.Errorf("item = %+v", it)
	}
	if !it.Prunable() {
		t.Errorf("expected item to be prunable, got blockers: %v", it.Blockers)
	}
}

func TestScanIgnoresOpenAndUnknownPRs(t *testing.T) {
	dir := t.TempDir()
	writeStatus(t, dir, "widget-fix", statusBody("widget-fix",
		"| service-a | widget-fix | /wt/service-a | based on main | - |",
	))

	open := func(_, _ string) (string, error) { return "OPEN", nil }
	items, err := prune.Scan(dir, "", open)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("got %d items, want 0 for an open PR", len(items))
	}

	none := func(_, _ string) (string, error) { return "NONE", nil }
	items, err = prune.Scan(dir, "", none)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("got %d items, want 0 when there's no PR", len(items))
	}
}

func TestScanTreatsLookupErrorAsNone(t *testing.T) {
	dir := t.TempDir()
	writeStatus(t, dir, "widget-fix", statusBody("widget-fix",
		"| service-a | widget-fix | /wt/service-a | based on main | - |",
	))

	failing := func(_, _ string) (string, error) { return "", errBoom }
	items, err := prune.Scan(dir, "", failing)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("got %d items, want 0 when the PR lookup fails", len(items))
	}
}

func TestScanFiltersBySlug(t *testing.T) {
	dir := t.TempDir()
	writeStatus(t, dir, "widget-fix", statusBody("widget-fix",
		"| service-a | widget-fix | /wt/a | based on main | - |",
	))
	writeStatus(t, dir, "other-fix", statusBody("other-fix",
		"| service-a | other-fix | /wt/b | based on main | - |",
	))

	items, err := prune.Scan(dir, "widget-fix", alwaysMerged)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Slug != "widget-fix" {
		t.Fatalf("got %+v, want just widget-fix", items)
	}
}

func TestScanRefusesToPruneAStackedBase(t *testing.T) {
	dir := t.TempDir()
	// widget-fix/service-a has merged, but shim-fix stacks on it.
	writeStatus(t, dir, "widget-fix", statusBody("widget-fix",
		"| service-a | widget-fix | /wt/widget-fix | based on main | - |",
	))
	writeStatus(t, dir, "shim-fix", statusBody("shim-fix",
		"| service-a | shim-fix | /wt/shim-fix | stacked on service-a:widget-fix | - |",
	))

	items, err := prune.Scan(dir, "", alwaysMerged)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2 (both merged rows)", len(items))
	}

	byRepo := map[string]prune.Item{}
	for _, it := range items {
		byRepo[it.Slug] = it
	}

	base := byRepo["widget-fix"]
	if base.Prunable() {
		t.Errorf("expected widget-fix:service-a to be blocked, got prunable")
	}
	if len(base.Blockers) != 1 || base.Blockers[0] != filepath.Join(dir, "shim-fix", "status.md") {
		t.Errorf("unexpected blockers: %v", base.Blockers)
	}

	// shim-fix itself has nothing stacked on it, so it stays prunable.
	if !byRepo["shim-fix"].Prunable() {
		t.Errorf("expected shim-fix to remain prunable")
	}
}

func TestRemoveStatusRowDropsOnlyMatchingRepo(t *testing.T) {
	dir := t.TempDir()
	path := writeStatus(t, dir, "widget-fix", statusBody("widget-fix",
		"| service-a | widget-fix | /wt/a | based on main | - |",
		"| service-b | widget-fix | /wt/b | stacked on service-a:widget-fix | - |",
	))

	if err := prune.RemoveStatusRow(path, "service-a"); err != nil {
		t.Fatal(err)
	}

	rows := prune.ParseStatusFile(readFile(t, path))
	if len(rows) != 1 || rows[0].Repo != "service-b" {
		t.Fatalf("got rows %+v, want only service-b left", rows)
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

type boomErr struct{}

func (boomErr) Error() string { return "boom" }

var errBoom = boomErr{}
