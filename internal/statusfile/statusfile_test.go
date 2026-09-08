package statusfile_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/blockadence/gh-archimedes/internal/statusfile"
)

// write puts one unit of work's status.md where an instance keeps it.
func write(t *testing.T, root, slug, body string) string {
	t.Helper()
	path := statusfile.Path(root, slug)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func body(slug string, rows ...string) string {
	return statusfile.Header(slug) + strings.Join(rows, "\n") + "\n"
}

func TestParseReadsEveryCellOfEveryRow(t *testing.T) {
	got := statusfile.Parse([]byte(body("widget-fix",
		"| service-a | widget-fix | ../service-a-worktrees/widget-fix | based on main | - |",
		"| service-b | widget-fix | ../service-b-worktrees/widget-fix | stacked on service-a:widget-fix | 42 |",
	)), "widget-fix")

	want := []statusfile.Row{
		{Slug: "widget-fix", Repo: "service-a", Branch: "widget-fix", Worktree: "../service-a-worktrees/widget-fix", Note: "based on main", PR: "-"},
		{Slug: "widget-fix", Repo: "service-b", Branch: "widget-fix", Worktree: "../service-b-worktrees/widget-fix", Note: "stacked on service-a:widget-fix", PR: "42"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestParseSkipsTheHeaderBlanksAndAnythingThatIsNotARow(t *testing.T) {
	// The header's own line would read as a row named "repo" if the
	// preamble weren't skipped.
	content := statusfile.Header("solo") + "\n" +
		"| service-a | solo | /wt | a note | - |\n" +
		"not a table row\n" +
		"|  | solo | /wt | no repo | - |\n"

	got := statusfile.Parse([]byte(content), "solo")
	want := []statusfile.Row{
		{Slug: "solo", Repo: "service-a", Branch: "solo", Worktree: "/wt", Note: "a note", PR: "-"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestParseOfAFileWithNoRowsYet(t *testing.T) {
	if got := statusfile.Parse([]byte(statusfile.Header("solo")), "solo"); len(got) != 0 {
		t.Errorf("got %#v, want no rows", got)
	}
}

// The whole point of one reader: a table someone has aligned by hand — or
// let an editor's formatter align — says exactly what the terse one spawn
// wrote says. Nothing downstream gets to see the spacing.
func TestAHandAlignedTableParsesAsTheTerseOneDoes(t *testing.T) {
	terse := statusfile.Parse([]byte(body("widget-fix",
		"| service-a | widget-fix | /wt | stacked on service-b:shim-fix | - |",
	)), "widget-fix")
	aligned := statusfile.Parse([]byte(body("widget-fix",
		"|  service-a  |  widget-fix  |  /wt  |  stacked   on    service-b:shim-fix  |  -  |",
	)), "widget-fix")

	if !reflect.DeepEqual(terse, aligned) {
		t.Errorf("aligned table read differently\nterse:   %#v\naligned: %#v", terse, aligned)
	}
}

// The preamble the parser skips is the one the writer writes. Held by a
// test rather than by two people counting the same four lines.
func TestTheHeaderIsExactlyThePreambleTheParserSkips(t *testing.T) {
	h := statusfile.Header("widget-fix")
	if !strings.HasSuffix(h, "\n") {
		t.Fatalf("header %q does not end its last line", h)
	}
	// One row appended to a bare header has to come back, which it only
	// does if the parser skips exactly as many lines as the header has.
	rows := statusfile.Parse([]byte(h+statusfile.FormatRow(statusfile.Row{Repo: "service-a", Branch: "widget-fix"})), "widget-fix")
	if len(rows) != 1 || rows[0].Repo != "service-a" {
		t.Fatalf("got %#v, want the one row that follows the header", rows)
	}
}

func TestFormatRowWritesAPlaceholderForAnEmptyPRCell(t *testing.T) {
	line := statusfile.FormatRow(statusfile.Row{Repo: "service-a", Branch: "widget-fix", Worktree: "/wt", Note: "based on main"})
	if line != "| service-a | widget-fix | /wt | based on main | - |\n" {
		t.Errorf("FormatRow = %q", line)
	}
}

func TestAppendCreatesTheFileWithItsHeaderThenAddsToIt(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(statusfile.Path(root, "widget-fix")), 0o755); err != nil {
		t.Fatal(err)
	}

	first := statusfile.Row{Slug: "widget-fix", Repo: "service-a", Branch: "widget-fix", Worktree: "../a", Note: "based on main"}
	second := statusfile.Row{Slug: "widget-fix", Repo: "service-b", Branch: "widget-fix", Worktree: "../b", Note: "stacked on service-a:widget-fix"}
	for _, r := range []statusfile.Row{first, second} {
		if err := statusfile.Append(root, r); err != nil {
			t.Fatal(err)
		}
	}

	file, err := statusfile.ReadFile(statusfile.Path(root, "widget-fix"), "widget-fix")
	if err != nil {
		t.Fatal(err)
	}
	want := []statusfile.Row{
		{Slug: "widget-fix", Repo: "service-a", Branch: "widget-fix", Worktree: "../a", Note: "based on main", PR: "-"},
		{Slug: "widget-fix", Repo: "service-b", Branch: "widget-fix", Worktree: "../b", Note: "stacked on service-a:widget-fix", PR: "-"},
	}
	if !reflect.DeepEqual(file.Rows, want) {
		t.Errorf("rows after two appends\n got: %#v\nwant: %#v", file.Rows, want)
	}
}

func TestReadFileOfAMissingFileErrors(t *testing.T) {
	if _, err := statusfile.ReadFile(filepath.Join(t.TempDir(), "missing.md"), "slug"); err == nil {
		t.Fatal("expected an error for a missing file, got nil")
	}
}

func TestDiscoverReadsEveryUnitOfWorkInPathOrder(t *testing.T) {
	root := t.TempDir()
	write(t, root, "beta", body("beta", "| service-a | beta | /wt/beta | based on main | - |"))
	write(t, root, "alpha", body("alpha", "| service-a | alpha | /wt/alpha | based on main | - |"))

	files, err := statusfile.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("got %d files, want 2: %#v", len(files), files)
	}
	if files[0].Slug != "alpha" || files[1].Slug != "beta" {
		t.Errorf("got slugs %q, %q, want them in path order", files[0].Slug, files[1].Slug)
	}
	if files[0].Path != statusfile.Path(root, "alpha") {
		t.Errorf("path = %q, want %q", files[0].Path, statusfile.Path(root, "alpha"))
	}
	if len(files[0].Rows) != 1 || files[0].Rows[0].Slug != "alpha" {
		t.Errorf("rows = %#v, want one tagged with its slug", files[0].Rows)
	}
}

func TestDiscoverOnAnInstanceWithNothingSpawnedYet(t *testing.T) {
	files, err := statusfile.Discover(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("got %#v, want no files", files)
	}
}

func TestRemoveRowsDropsWhatItIsAskedForAndLeavesTheHeader(t *testing.T) {
	root := t.TempDir()
	path := write(t, root, "widget-fix", body("widget-fix",
		"| service-a | widget-fix | /wt/a | based on main | - |",
		"| service-b | widget-fix | /wt/b | stacked on service-a:widget-fix | - |",
	))

	if err := statusfile.RemoveRows(path, func(r statusfile.Row) bool { return r.Repo == "service-a" }); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), statusfile.Header("widget-fix")) {
		t.Errorf("header did not survive:\n%s", data)
	}
	rows := statusfile.Parse(data, "widget-fix")
	if len(rows) != 1 || rows[0].Repo != "service-b" {
		t.Errorf("got %#v, want only service-b left", rows)
	}
}

// The header's cells are not data, whatever a repo happens to be called.
func TestRemoveRowsWillNotEatTheHeaderRow(t *testing.T) {
	root := t.TempDir()
	path := write(t, root, "widget-fix", body("widget-fix",
		"| repo | widget-fix | /wt/a | based on main | - |",
	))

	if err := statusfile.RemoveRows(path, func(r statusfile.Row) bool { return r.Repo == "repo" }); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), statusfile.Header("widget-fix")) {
		t.Errorf("header did not survive:\n%s", data)
	}
	if rows := statusfile.Parse(data, "widget-fix"); len(rows) != 0 {
		t.Errorf("got %#v, want the data row gone", rows)
	}
}

func TestBranchNameFallsBackToTheSlug(t *testing.T) {
	if got := (statusfile.Row{Slug: "widget-fix", Branch: "other"}).BranchName(); got != "other" {
		t.Errorf("BranchName = %q, want the row's own branch column", got)
	}
	if got := (statusfile.Row{Slug: "widget-fix"}).BranchName(); got != "widget-fix" {
		t.Errorf("BranchName = %q, want the slug for a row written without a branch", got)
	}
}
