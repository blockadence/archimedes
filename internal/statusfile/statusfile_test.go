package statusfile

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// preambleFor spells the header out rather than building it with the code
// under test: a fixture that agrees with whatever produces it cannot catch
// that producer changing, which is the drift this package exists to stop.
func preambleFor(slug string) string {
	return "# " + slug + "\n\n| repo | branch | worktree | note | pr |\n|---|---|---|---|---|\n"
}

func fileBody(slug string, rows ...string) string {
	return preambleFor(slug) + strings.Join(rows, "\n") + "\n"
}

// write puts one unit of work's status.md where an instance keeps it.
func write(t *testing.T, root, slug, body string) string {
	t.Helper()
	path := Path(root, slug)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// The preamble written and the preamble skipped are the same four lines.
// Held here rather than by two packages counting them.
func TestTheHeaderWrittenIsThePreambleSkipped(t *testing.T) {
	if got := preamble("widget-fix"); got != preambleFor("widget-fix") {
		t.Errorf("preamble = %q, want %q", got, preambleFor("widget-fix"))
	}
	if len(header("widget-fix")) != headerLines {
		t.Errorf("header writes %d lines, parser skips %d", len(header("widget-fix")), headerLines)
	}
}

func TestParseReadsEveryCellOfEveryRow(t *testing.T) {
	got := Parse([]byte(fileBody("widget-fix",
		"| service-a | widget-fix | ../service-a-worktrees/widget-fix | based on main | - |",
		"| service-b | widget-fix | ../service-b-worktrees/widget-fix | stacked on service-a:widget-fix | 42 |",
	)), "widget-fix")

	want := []Row{
		{Slug: "widget-fix", Repo: "service-a", Branch: "widget-fix", Worktree: "../service-a-worktrees/widget-fix", Note: "based on main", PR: "-"},
		{Slug: "widget-fix", Repo: "service-b", Branch: "widget-fix", Worktree: "../service-b-worktrees/widget-fix", Note: "stacked on service-a:widget-fix", PR: "42"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestParseSkipsTheHeaderBlanksAndAnythingThatIsNotARow(t *testing.T) {
	// The header's own line would read as a row about a repo called "repo"
	// if the preamble weren't skipped.
	content := preambleFor("solo") + "\n" +
		"| service-a | solo | /wt | a note | - |\n" +
		"not a table row\n" +
		"|  | solo | /wt | no repo | - |\n"

	got := Parse([]byte(content), "solo")
	want := []Row{
		{Slug: "solo", Repo: "service-a", Branch: "solo", Worktree: "/wt", Note: "a note", PR: "-"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestParseOfAFileWithNoRowsYet(t *testing.T) {
	if got := Parse([]byte(preambleFor("solo")), "solo"); len(got) != 0 {
		t.Errorf("got %#v, want no rows", got)
	}
}

// The whole point of one reader: a table someone has aligned by hand — or
// let an editor's formatter align — says exactly what the terse one spawn
// wrote says. Nothing downstream gets to see the spacing.
func TestAHandAlignedTableParsesAsTheTerseOneDoes(t *testing.T) {
	terse := Parse([]byte(fileBody("widget-fix",
		"| service-a | widget-fix | /wt | stacked on service-b:shim-fix | - |",
	)), "widget-fix")
	aligned := Parse([]byte(fileBody("widget-fix",
		"|  service-a  |  widget-fix  |  /wt  |  stacked   on    service-b:shim-fix  |  -  |",
	)), "widget-fix")

	if !reflect.DeepEqual(terse, aligned) {
		t.Errorf("aligned table read differently\nterse:   %#v\naligned: %#v", terse, aligned)
	}
}

func TestAppendCreatesTheFileWithItsHeaderThenAddsToIt(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(Path(root, "widget-fix")), 0o755); err != nil {
		t.Fatal(err)
	}

	rows := []Row{
		{Slug: "widget-fix", Repo: "service-a", Branch: "widget-fix", Worktree: "../a", Note: "based on main"},
		{Slug: "widget-fix", Repo: "service-b", Branch: "widget-fix", Worktree: "../b", Note: "stacked on service-a:widget-fix"},
	}
	for _, r := range rows {
		if err := Append(root, r); err != nil {
			t.Fatal(err)
		}
	}

	// Byte for byte, including the placeholder in the pr column a row is
	// recorded with before there is a pull request to name.
	want := fileBody("widget-fix",
		"| service-a | widget-fix | ../a | based on main | - |",
		"| service-b | widget-fix | ../b | stacked on service-a:widget-fix | - |",
	)
	data, err := os.ReadFile(Path(root, "widget-fix"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Errorf("file after two appends\n got: %q\nwant: %q", data, want)
	}
}

func TestReadFileOfAMissingFileErrors(t *testing.T) {
	if _, err := ReadFile(filepath.Join(t.TempDir(), "missing.md"), "slug"); err == nil {
		t.Fatal("expected an error for a missing file, got nil")
	}
}

func TestDiscoverReadsEveryUnitOfWorkInPathOrder(t *testing.T) {
	root := t.TempDir()
	write(t, root, "beta", fileBody("beta", "| service-a | beta | /wt/beta | based on main | - |"))
	write(t, root, "alpha", fileBody("alpha", "| service-a | alpha | /wt/alpha | based on main | - |"))

	files, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("got %d files, want 2: %#v", len(files), files)
	}
	if files[0].Slug != "alpha" || files[1].Slug != "beta" {
		t.Errorf("got slugs %q, %q, want them in path order", files[0].Slug, files[1].Slug)
	}
	if files[0].Path != Path(root, "alpha") {
		t.Errorf("path = %q, want %q", files[0].Path, Path(root, "alpha"))
	}
	if len(files[0].Rows) != 1 || files[0].Rows[0].Slug != "alpha" {
		t.Errorf("rows = %#v, want one tagged with its slug", files[0].Rows)
	}
}

func TestDiscoverOnAnInstanceWithNothingSpawnedYet(t *testing.T) {
	files, err := Discover(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("got %#v, want no files", files)
	}
}

func TestRemoveRowsDropsWhatItIsAskedForAndLeavesTheHeader(t *testing.T) {
	root := t.TempDir()
	path := write(t, root, "widget-fix", fileBody("widget-fix",
		"| service-a | widget-fix | /wt/a | based on main | - |",
		"| service-b | widget-fix | /wt/b | stacked on service-a:widget-fix | - |",
	))

	if err := RemoveRows(path, func(r Row) bool { return r.Repo == "service-a" }); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), preambleFor("widget-fix")) {
		t.Errorf("header did not survive:\n%s", data)
	}
	rows := Parse(data, "widget-fix")
	if len(rows) != 1 || rows[0].Repo != "service-b" {
		t.Errorf("got %#v, want only service-b left", rows)
	}
}

// The header's cells are not data, whatever a repo happens to be called.
func TestRemoveRowsWillNotEatTheHeaderRow(t *testing.T) {
	root := t.TempDir()
	path := write(t, root, "widget-fix", fileBody("widget-fix",
		"| repo | widget-fix | /wt/a | based on main | - |",
	))

	if err := RemoveRows(path, func(r Row) bool { return r.Repo == "repo" }); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), preambleFor("widget-fix")) {
		t.Errorf("header did not survive:\n%s", data)
	}
	if rows := Parse(data, "widget-fix"); len(rows) != 0 {
		t.Errorf("got %#v, want the data row gone", rows)
	}
}

func TestBranchNameFallsBackToTheSlug(t *testing.T) {
	if got := (Row{Slug: "widget-fix", Branch: "other"}).BranchName(); got != "other" {
		t.Errorf("BranchName = %q, want the row's own branch column", got)
	}
	if got := (Row{Slug: "widget-fix"}).BranchName(); got != "widget-fix" {
		t.Errorf("BranchName = %q, want the slug for a row written without a branch", got)
	}
}

// The file's place, stated here rather than assembled from workdir.Path:
// the directory around it has an owner now (internal/workdir), and what
// holds this package and that one together is a test that says what the
// two of them add up to instead of agreeing with whichever changes.
func TestPathIsTheStatusFileInTheUnitOfWorksDirectory(t *testing.T) {
	got := Path("/Users/someone/Code/widgets", "widget-fix")
	want := "/Users/someone/Code/widgets/work/widget-fix/status.md"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
