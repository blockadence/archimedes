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
	return "# " + slug + "\n\n| repo | branch | worktree | note |\n|---|---|---|---|\n"
}

// legacyPreambleFor is the header instances spawned before the pr column
// was dropped carry, spelled out for the same reason. Those files are in
// somebody's git history, so this is not a shape the package is done with.
func legacyPreambleFor(slug string) string {
	return "# " + slug + "\n\n| repo | branch | worktree | note | pr |\n|---|---|---|---|---|\n"
}

func legacyFileBody(slug string, rows ...string) string {
	return legacyPreambleFor(slug) + strings.Join(rows, "\n") + "\n"
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
	// The same four lines, counted three ways: what a read skips, what the
	// title takes, and what naming the columns takes. Held together here
	// rather than left to agree by hand.
	if titleLines+len(columns()) != headerLines {
		t.Errorf("%d title lines plus %d column lines is not the %d a read skips",
			titleLines, len(columns()), headerLines)
	}
	if got := len(columnNames(columns()[0])); got != cells {
		t.Errorf("the header names %d columns, a row is read for %d", got, cells)
	}
}

func TestParseReadsEveryCellOfEveryRow(t *testing.T) {
	got := Parse([]byte(fileBody("widget-fix",
		"| service-a | widget-fix | ../service-a-worktrees/widget-fix | based on main |",
		"| service-b | widget-fix | ../service-b-worktrees/widget-fix | stacked on service-a:widget-fix |",
	)), "widget-fix")

	want := []Row{
		{Slug: "widget-fix", Repo: "service-a", Branch: "widget-fix", Worktree: "../service-a-worktrees/widget-fix", Note: "based on main"},
		{Slug: "widget-fix", Repo: "service-b", Branch: "widget-fix", Worktree: "../service-b-worktrees/widget-fix", Note: "stacked on service-a:widget-fix"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestParseSkipsTheHeaderBlanksAndAnythingThatIsNotARow(t *testing.T) {
	// The header's own line would read as a row about a repo called "repo"
	// if the preamble weren't skipped.
	content := preambleFor("solo") + "\n" +
		"| service-a | solo | /wt | a note |\n" +
		"not a table row\n" +
		"|  | solo | /wt | no repo |\n"

	got := Parse([]byte(content), "solo")
	want := []Row{
		{Slug: "solo", Repo: "service-a", Branch: "solo", Worktree: "/wt", Note: "a note"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

// A row a hand has broken short of its cells is not a row. It used to be
// the pr cell that could go missing and still leave a row standing, which
// cost nothing because nobody read it; the same tolerance over four columns
// would let the note cell go missing instead, and the note is what says a
// branch is somebody's stacked base. A row that quietly lost its note reads
// as a row with nothing stacked on it, which is how prune removes a branch
// out from under the work stacked on it — silently, on the destroying side.
//
// So a short row is invisible, which is loud: the operator sees their unit
// of work gone from `status` rather than reported with a note it does not
// have.
func TestARowShortOfItsCellsIsNotARow(t *testing.T) {
	got := Parse([]byte(fileBody("widget-fix",
		"| service-a | widget-fix | /wt/a |",
		"| service-b | widget-fix | /wt/b | stacked on service-a:widget-fix |",
	)), "widget-fix")

	want := []Row{
		{Slug: "widget-fix", Repo: "service-b", Branch: "widget-fix", Worktree: "/wt/b", Note: "stacked on service-a:widget-fix"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

// A cell with nothing in it is still a cell: spawn writes every one of
// them, and a note it had nothing to put in is not a row short of a note.
func TestARowWhoseLastCellIsEmptyIsStillARow(t *testing.T) {
	got := Parse([]byte(fileBody("widget-fix",
		"| service-a | widget-fix | /wt/a |  |",
	)), "widget-fix")

	want := []Row{{Slug: "widget-fix", Repo: "service-a", Branch: "widget-fix", Worktree: "/wt/a"}}
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
		"| service-a | widget-fix | /wt | stacked on service-b:shim-fix |",
	)), "widget-fix")
	aligned := Parse([]byte(fileBody("widget-fix",
		"|  service-a  |  widget-fix  |  /wt  |  stacked   on    service-b:shim-fix  |",
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

	// Byte for byte.
	want := fileBody("widget-fix",
		"| service-a | widget-fix | ../a | based on main |",
		"| service-b | widget-fix | ../b | stacked on service-a:widget-fix |",
	)
	data, err := os.ReadFile(Path(root, "widget-fix"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Errorf("file after two appends\n got: %q\nwant: %q", data, want)
	}
}

// The rows an instance already carries were written with a fifth cell in
// them. They are in somebody's git history, so they have to go on reading
// as the rows they are — the same four columns, and no fifth thing for a
// caller to reach for.
func TestARowWrittenUnderTheOldFiveColumnShapeStillParses(t *testing.T) {
	got := Parse([]byte(legacyFileBody("widget-fix",
		"| service-a | widget-fix | ../service-a-worktrees/widget-fix | based on main | - |",
		"| service-b | widget-fix | ../service-b-worktrees/widget-fix | stacked on service-a:widget-fix | 42 |",
	)), "widget-fix")

	want := []Row{
		{Slug: "widget-fix", Repo: "service-a", Branch: "widget-fix", Worktree: "../service-a-worktrees/widget-fix", Note: "based on main"},
		{Slug: "widget-fix", Repo: "service-b", Branch: "widget-fix", Worktree: "../service-b-worktrees/widget-fix", Note: "stacked on service-a:widget-fix"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

// An operator reading the file is the reader this is for: a header naming
// a column is a promise about the cells under it, so a file this package
// writes to comes away naming the columns it is read for. The rows are
// left exactly as they were — a fifth cell past a four-column header is
// not rendered, and re-rendering them would take an operator's alignment
// with it.
func TestWritingToAFileOfTheOldShapeBringsItsHeaderUpToDate(t *testing.T) {
	root := t.TempDir()
	legacyRow := "| service-a | widget-fix | ../a | based on main | - |"
	write(t, root, "widget-fix", legacyFileBody("widget-fix", legacyRow))

	if err := Append(root, Row{Slug: "widget-fix", Repo: "service-b", Branch: "widget-fix", Worktree: "../b", Note: "based on main"}); err != nil {
		t.Fatal(err)
	}

	want := preambleFor("widget-fix") + legacyRow + "\n" +
		"| service-b | widget-fix | ../b | based on main |\n"
	data, err := os.ReadFile(Path(root, "widget-fix"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Errorf("file after appending to an old-shape file\n got: %q\nwant: %q", data, want)
	}
}

// The other writer, for the same reason: prune rewrites the file, and a
// file it has rewritten must not be left claiming a column nobody reads.
func TestRemoveRowsBringsAnOldShapeHeaderUpToDateToo(t *testing.T) {
	root := t.TempDir()
	path := write(t, root, "widget-fix", legacyFileBody("widget-fix",
		"| service-a | widget-fix | /wt/a | based on main | - |",
		"| service-b | widget-fix | /wt/b | based on main | 42 |",
	))

	if err := RemoveRows(path, func(r Row) bool { return r.Repo == "service-a" }); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), preambleFor("widget-fix")) {
		t.Errorf("header was left in the old shape:\n%s", data)
	}
	if rows := Parse(data, "widget-fix"); len(rows) != 1 || rows[0].Repo != "service-b" {
		t.Errorf("got %#v, want only service-b left", rows)
	}
}

// Only the two lines that say what the columns are. What an operator has
// written above the table is theirs.
func TestBringingAHeaderUpToDateLeavesTheTitleAlone(t *testing.T) {
	root := t.TempDir()
	titled := "# widget-fix — the auth rewrite\n\n| repo | branch | worktree | note | pr |\n|---|---|---|---|---|\n"
	write(t, root, "widget-fix", titled)

	if err := Append(root, Row{Slug: "widget-fix", Repo: "service-a", Branch: "widget-fix", Worktree: "../a", Note: "based on main"}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(Path(root, "widget-fix"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "# widget-fix — the auth rewrite\n\n| repo | branch | worktree | note |\n|---|---|---|---|\n") {
		t.Errorf("title did not survive:\n%s", data)
	}
}

// A row this version writes is read by the version before it too: an
// operator's other checkout, a binary they haven't upgraded. The rule that
// version applied is spelled out here rather than imported, because it is
// gone from the code and a fixture that agrees with what produces it
// cannot catch anything.
func TestARowWrittenNowStillParsesUnderTheReadingThatCameBefore(t *testing.T) {
	line := strings.TrimSuffix(formatRow(Row{Repo: "service-a", Branch: "widget-fix", Worktree: "../a", Note: "based on main"}), "\n")

	// Five cells, so six fields at least, and the cells are fields 1..5.
	fields := strings.Split(line, "|")
	if len(fields) < 6 {
		t.Fatalf("%q splits into %d fields; the reading before this one wanted at least 6", line, len(fields))
	}
	for i, want := range []string{"service-a", "widget-fix", "../a", "based on main"} {
		if got := cell(fields[i+1]); got != want {
			t.Errorf("field %d = %q, want %q", i+1, got, want)
		}
	}
}

// A file whose column lines are not where this package puts them is not
// this package's to rewrite. An operator with a paragraph under the title
// keeps their paragraph — the header is brought forward only when the file
// is one an older Archimedes wrote.
func TestWhatIsNotAnOldHeaderIsLeftAlone(t *testing.T) {
	root := t.TempDir()
	prose := "# widget-fix\n\nWhy this exists: the auth rewrite, see the ticket.\n\n" +
		"| repo | branch | worktree | note | pr |\n|---|---|---|---|---|\n"
	write(t, root, "widget-fix", prose)

	if err := Append(root, Row{Slug: "widget-fix", Repo: "service-a", Branch: "widget-fix", Worktree: "../a", Note: "based on main"}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(Path(root, "widget-fix"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), prose) {
		t.Errorf("the operator's own lines did not survive:\n%s", data)
	}
}

// The header an editor's table formatter has aligned is the header it was
// before it: the same columns, read the way a row's cells are read.
func TestAnOldHeaderTheOperatorAlignedIsStillBroughtForward(t *testing.T) {
	root := t.TempDir()
	write(t, root, "widget-fix", "# widget-fix\n\n"+
		"| repo      | branch     | worktree | note          | pr |\n"+
		"|-----------|------------|----------|---------------|----|\n")

	if err := Append(root, Row{Slug: "widget-fix", Repo: "service-a", Branch: "widget-fix", Worktree: "../a", Note: "based on main"}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(Path(root, "widget-fix"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), preambleFor("widget-fix")) {
		t.Errorf("an aligned old header was not brought forward:\n%s", data)
	}
}

// A file whose last line has no newline on it takes the next row as its
// own line, not onto the end of the last one. The row that would otherwise
// be lost is the one already recorded.
func TestAppendToAFileWithNoFinalNewlineDoesNotEatTheLastRow(t *testing.T) {
	root := t.TempDir()
	write(t, root, "widget-fix", strings.TrimSuffix(fileBody("widget-fix",
		"| service-a | widget-fix | ../a | based on main |",
	), "\n"))

	if err := Append(root, Row{Slug: "widget-fix", Repo: "service-b", Branch: "widget-fix", Worktree: "../b", Note: "based on main"}); err != nil {
		t.Fatal(err)
	}

	rows := Parse(readFile(t, Path(root, "widget-fix")), "widget-fix")
	if len(rows) != 2 || rows[0].Repo != "service-a" || rows[1].Repo != "service-b" {
		t.Errorf("got %#v, want both rows", rows)
	}
}

// A file truncated to nothing gets the header a new one gets, rather than
// a row nothing can read because the read starts four lines in.
func TestAppendToAnEmptyFileWritesTheHeaderFirst(t *testing.T) {
	root := t.TempDir()
	write(t, root, "widget-fix", "")

	if err := Append(root, Row{Slug: "widget-fix", Repo: "service-a", Branch: "widget-fix", Worktree: "../a", Note: "based on main"}); err != nil {
		t.Fatal(err)
	}

	want := fileBody("widget-fix", "| service-a | widget-fix | ../a | based on main |")
	if got := string(readFile(t, Path(root, "widget-fix"))); got != want {
		t.Errorf("file after appending to an empty one\n got: %q\nwant: %q", got, want)
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

func TestReadFileOfAMissingFileErrors(t *testing.T) {
	if _, err := ReadFile(filepath.Join(t.TempDir(), "missing.md"), "slug"); err == nil {
		t.Fatal("expected an error for a missing file, got nil")
	}
}

func TestDiscoverReadsEveryUnitOfWorkInPathOrder(t *testing.T) {
	root := t.TempDir()
	write(t, root, "beta", fileBody("beta", "| service-a | beta | /wt/beta | based on main |"))
	write(t, root, "alpha", fileBody("alpha", "| service-a | alpha | /wt/alpha | based on main |"))

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
		"| service-a | widget-fix | /wt/a | based on main |",
		"| service-b | widget-fix | /wt/b | stacked on service-a:widget-fix |",
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
		"| repo | widget-fix | /wt/a | based on main |",
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
