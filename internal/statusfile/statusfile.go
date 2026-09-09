// Package statusfile owns work/<slug>/status.md: the bookkeeping file an
// instance keeps for one unit of work, recording which repos it spans,
// where each one's worktree is, and what each branch was cut from.
//
// That is the whole of a row, and it is the whole of it on purpose: every
// column holds something spawn knows when it writes the row and that stays
// true for as long as the row exists. Pull request state is the thing that
// most looks like it belongs here and does not — it changes without anyone
// touching this file, so a number recorded in it would be a cache, and a
// reader holding one would have to decide whether to believe it. Nobody
// has to: status, prune and notify all ask gh for a row's live state, keyed
// by the repo and branch the row does carry (issue 65).
//
// One package because one file has three parties to it — spawn writes it,
// status reports what it says, prune acts on it — and each of them
// deciding for itself what a row is is how they came to disagree. They
// did: status collapsed a cell's internal whitespace and prune trimmed
// it, so a table an editor's formatter had re-spaced read one way in the
// report and another way in the command that removes worktrees. The
// disagreement was silent, and it was silent on the destroying side.
//
// So the file's shape has one owner, the way a worktree's does
// (internal/worktree) and the way the stack note's wording does
// (internal/stackref). What is here is everything that knows the format:
// the file's name, the header spawn writes, how a row is rendered and read
// back, and the walk over an instance's units of work. What is not here is
// what any of it means — whether a note names a stacked base is
// internal/stackref's, whether a worktree column resolves to a usable path
// is internal/worktree's, and whether a row is prunable is prune's — nor
// the work/<slug> directory the file sits in, which is internal/workdir's.
package statusfile

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/blockadence/gh-archimedes/internal/workdir"
)

// Name is the file itself. Archimedes' own bookkeeping rather than
// reference material, which is why spawn's materialize excludes it from
// what gets copied into a worktree.
const Name = "status.md"

// Path is where the instance at root keeps slug's file: stated once, so a
// writer, a reader and the walk cannot come to different conclusions about
// where the file is. What this package states is the name; the directory
// around it is internal/workdir's, which says what it is.
func Path(root, slug string) string {
	return filepath.Join(workdir.Path(root, slug), Name)
}

// Row is one data row: what the file says about one repo's part in one
// unit of work.
//
// Every cell comes back as the file states it, cleaned of nothing but its
// spacing (see cell). In particular Worktree is the recorded column —
// relative to the instance root, which is how it is written (see
// internal/worktree) — and not a path to hand git. Resolving it takes an
// instance root this layer is not given, so the layer that has one does it.
type Row struct {
	// Slug is the work/<slug> directory the row was found under, which the
	// file itself does not repeat on every line.
	Slug     string
	Repo     string
	Branch   string
	Worktree string
	Note     string
}

// BranchName is the git branch a row refers to. spawn names the branch
// after the slug and records both, so the two normally agree; the branch
// column is what the row itself claims, so prefer it and fall back to the
// slug only for a row written without one.
func (r Row) BranchName() string {
	if r.Branch != "" {
		return r.Branch
	}
	return r.Slug
}

// File is one unit of work's status.md, read.
type File struct {
	// Path is the file on disk, which is how a reader names it back to an
	// operator ("still a base for: ...").
	Path string
	// Slug is the work/<slug> directory it was found under.
	Slug string
	Rows []Row
}

// columns is the two lines that name the table's columns: the header row
// and the separator under it. Kept apart from the title above them
// because they are the two lines upgradeColumns replaces.
func columns() []string {
	return []string{
		"| repo | branch | worktree | note |",
		"|---|---|---|---|",
	}
}

// header is the preamble every status.md opens with: its title, a
// blank, and the table's header and separator. A slug's first spawn
// writes it; every read skips it.
func header(slug string) []string {
	return append([]string{"# " + slug, ""}, columns()...)
}

// titleLines is the part of the preamble that belongs to whoever is
// reading the file — the "# <slug>" heading and the blank under it — and
// so the offset the column lines sit at.
const titleLines = 2

// headerLines is how many lines header writes, and so how many lines a
// read skips before the data starts. A test holds the two together rather
// than two packages counting the same four lines.
const headerLines = 4

// supersededColumns is what this package's header used to name and no
// longer does: the pr column, dropped in issue 65 because nothing ever
// wrote to it and nobody read it. A file naming these is one an older
// Archimedes wrote, and upgradeColumns is what brings it forward.
func supersededColumns() []string {
	return []string{"repo", "branch", "worktree", "note", "pr"}
}

// columnNames reads the cells of a line that names columns, spacing
// collapsed the way a data row's cells are (see cell) — an operator's
// editor aligns the header along with the rows under it.
func columnNames(line string) []string {
	fields := strings.Split(line, "|")
	if len(fields) < 2 {
		return nil
	}
	names := make([]string, 0, len(fields)-2)
	for _, f := range fields[1 : len(fields)-1] {
		names = append(names, cell(f))
	}
	return names
}

// upgradeColumns brings a file's column lines up to the shape this package
// writes. A header names the columns the rows under it are read for, and
// this package is the only place that decides what those are; a file
// recorded before they last changed otherwise goes on naming a column over
// cells nothing reads, to an operator who has no way to tell that from a
// record.
//
// It fires only where the two lines it would replace name exactly the
// columns this package has stopped writing. Everything else is somebody
// else's: a paragraph an operator put under the title, a second table they
// keep below, a file that was never this package's at all — and the cost of
// guessing is their words gone, on a write they asked for something else.
//
// The rows are left byte for byte either way. A cell past the last column
// renders as nothing, so an old row under the new header already reads as
// what it is, and re-rendering the rows to be rid of it would take an
// operator's hand alignment with them.
func upgradeColumns(lines []string) {
	cols := columns()
	if len(lines) < titleLines+len(cols) {
		return
	}
	if !slices.Equal(columnNames(lines[titleLines]), supersededColumns()) {
		return
	}
	copy(lines[titleLines:], cols)
}

// cells is how many columns a data row has: the four the header names.
const cells = 4

// preamble is the header as it is written to a new file.
func preamble(slug string) string {
	return strings.Join(header(slug), "\n") + "\n"
}

// formatRow renders one data row.
func formatRow(r Row) string {
	return fmt.Sprintf("| %s | %s | %s | %s |\n", r.Repo, r.Branch, r.Worktree, r.Note)
}

// Append records one row in the instance at root, creating the file with
// its header first if this is the slug's first spawn. The work/<slug>
// directory (internal/workdir) is the caller's to have made — it is where
// the unit of work's reference material goes, so whoever is spawning has
// already created it.
func Append(root string, r Row) error {
	path := Path(root, r.Slug)

	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// Nothing there to append to — no file, or one truncated to nothing —
	// gets the header a first spawn writes. A row on its own would be a row
	// no read ever reaches, since a read starts where the header ends.
	if len(data) == 0 {
		return os.WriteFile(path, []byte(preamble(r.Slug)+formatRow(r)), 0o644)
	}

	// The file is rewritten rather than appended to because the header may
	// need bringing up to date (see upgradeColumns), and a row recorded
	// under a header that names other columns is the thing being fixed.
	lines := strings.Split(string(data), "\n")
	upgradeColumns(lines)

	// A file an operator left without its final newline would otherwise
	// take the new row onto the end of the last one, which loses the row
	// already recorded as well as the one being added.
	body := strings.Join(lines, "\n")
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	return os.WriteFile(path, []byte(body+formatRow(r)), 0o644)
}

// parseRow reads one "| repo | branch | worktree | note |" line.
// Splitting on "|" puts an empty field before the opening pipe and another
// after the closing one, so a full row yields cells+2 fields and the cells
// are fields 1 through cells. A line yielding fewer is short of a cell and
// is not a data row. Neither is one whose repo cell is empty — the blank
// line under the table and any prose added below it both land there.
//
// Short of a cell rather than short of a closing pipe, because the two are
// the same line and only one of them is safe to guess at. The reading this
// replaced asked for one field fewer, which let the last cell go missing
// and still leave a row standing. That cost nothing while the last cell was
// pr, since nobody read it. Over four columns the cell that goes missing is
// the note, and the note is what says a branch is somebody's stacked base
// (see internal/stackref): a row that quietly lost its note reads as a row
// with nothing stacked on it, which is how prune removes a branch out from
// under the work stacked on it.
//
// Cells past the fourth are read by nobody, which is what lets a file
// written before the pr column was dropped go on saying what it said: its
// rows carry a fifth cell, and the four this reads out of them are the
// four they always meant. See upgradeColumns, the other half of that.
//
// A row a hand has broken past that is invisible, and invisible to every
// reader alike: it is missing from the report as well as from prune's
// blocker scan, so the operator sees their unit of work gone from `status`
// rather than each reader answering differently about it. One answer is
// the whole point — see the package comment.
func parseRow(line string) (Row, bool) {
	fields := strings.Split(line, "|")
	if len(fields) < cells+2 {
		return Row{}, false
	}

	repo := cell(fields[1])
	if repo == "" {
		return Row{}, false
	}

	return Row{
		Repo:     repo,
		Branch:   cell(fields[2]),
		Worktree: cell(fields[3]),
		Note:     cell(fields[4]),
	}, true
}

// Parse reads one file's data rows, tagging each with the slug it was
// found under.
func Parse(data []byte, slug string) []Row {
	lines := strings.Split(string(data), "\n")
	if len(lines) <= headerLines {
		return nil
	}

	var rows []Row
	for _, line := range lines[headerLines:] {
		if row, ok := parseRow(line); ok {
			row.Slug = slug
			rows = append(rows, row)
		}
	}
	return rows
}

// ReadFile reads and parses the status.md at path, which was found under
// work/<slug>.
func ReadFile(path, slug string) (File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return File{}, err
	}
	return File{Path: path, Slug: slug, Rows: Parse(data, slug)}, nil
}

// Discover reads every work/<slug>/status.md the instance at root has, in
// path order so a run reports the same thing twice.
//
// It takes no slug filter on purpose. Its caller is the one that has to
// read every file whatever it was asked about — what another file's rows
// say is what tells prune the branch it is about to remove is somebody's
// stacked base. A reader that genuinely wants one unit of work reads that
// unit of work's file, through Path and ReadFile, which is what status
// does.
func Discover(root string) ([]File, error) {
	paths, err := filepath.Glob(Path(root, "*"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)

	files := make([]File, 0, len(paths))
	for _, p := range paths {
		file, err := ReadFile(p, filepath.Base(filepath.Dir(p)))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", p, err)
		}
		files = append(files, file)
	}
	return files, nil
}

// RemoveRows rewrites the file at path without the data rows drop reports
// true for. Everything that isn't a data row is left as it was — the header
// above all, whose own cells would otherwise read as a row about a repo
// called "repo" — with the one exception every write through this package
// makes: a header naming columns this package has stopped writing is
// brought forward (see upgradeColumns).
func RemoveRows(path string, drop func(Row) bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	upgradeColumns(lines)

	out := make([]string, 0, len(lines))
	for i, line := range lines {
		if i >= headerLines {
			if row, ok := parseRow(line); ok && drop(row) {
				continue
			}
		}
		out = append(out, line)
	}

	return os.WriteFile(path, []byte(strings.Join(out, "\n")), 0o644)
}

// cell reads one table cell: trimmed, with internal whitespace runs
// collapsed to a single space, so a table an operator (or their editor's
// table formatter) has aligned by hand says exactly what the terse one
// spawn wrote says. That is not cosmetic — the note cell is parsed, and a
// re-spaced "stacked  on   service-a:widget-fix" naming no base is how a
// branch gets removed out from under the work stacked on it.
//
// The cost is a recorded path that itself contains a run of spaces, which
// comes back with the run collapsed. A worktree lives beside its repo's
// checkout and is named after the slug, so that is a path nothing here
// produces; the row it would misread is one a hand already broke.
func cell(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
