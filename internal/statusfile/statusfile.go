// Package statusfile owns work/<slug>/status.md: the bookkeeping file an
// instance keeps for one unit of work, recording which repos it spans,
// where each one's worktree is, what each branch was cut from, and the
// pull request it is riding on.
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
// the file's name and where an instance keeps it, the header spawn writes,
// how a row is rendered and read back, and the walk over an instance's
// units of work. What is not here is what any of it means — whether a note
// names a stacked base is internal/stackref's, whether a worktree column
// resolves to a usable path is internal/worktree's, and whether a row is
// prunable is prune's.
package statusfile

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Name is the file itself. Archimedes' own bookkeeping rather than
// reference material, which is why spawn's materialize excludes it from
// what gets copied into a worktree.
const Name = "status.md"

// Path is where the instance at root keeps slug's file: stated once, so a
// writer, a reader and the walk cannot come to different conclusions about
// where the file is. The work/<slug> directory around it is not this
// package's — it is a unit of work's reference material, which spawn
// materializes and this file is deliberately excluded from.
func Path(root, slug string) string {
	return filepath.Join(root, "work", slug, Name)
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
	PR       string
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

// header is the preamble every status.md opens with: its title, a
// blank, and the table's header and separator. A slug's first spawn
// writes it; every read skips it.
func header(slug string) []string {
	return []string{
		"# " + slug,
		"",
		"| repo | branch | worktree | note | pr |",
		"|---|---|---|---|---|",
	}
}

// headerLines is how many lines header writes, and so how many lines a
// read skips before the data starts. A test holds the two together rather
// than two packages counting the same four lines.
const headerLines = 4

// cells is how many columns a data row has: the five the header names.
const cells = 5

// noCell is what a column with nothing in it yet is written as, so the
// table still renders as a table.
const noCell = "-"

// preamble is the header as it is written to a new file.
func preamble(slug string) string {
	return strings.Join(header(slug), "\n") + "\n"
}

// formatRow renders one data row. An empty PR cell is written as the
// placeholder, since spawn records a row before there is a pull request to
// name in it.
func formatRow(r Row) string {
	pr := r.PR
	if pr == "" {
		pr = noCell
	}
	return fmt.Sprintf("| %s | %s | %s | %s | %s |\n", r.Repo, r.Branch, r.Worktree, r.Note, pr)
}

// Append records one row in the instance at root, creating the file with
// its header first if this is the slug's first spawn. The work/<slug>
// directory is the caller's to have made — it is where the unit of work's
// reference material goes, so whoever is spawning has already created it.
func Append(root string, r Row) error {
	path := Path(root, r.Slug)

	if _, err := os.Stat(path); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if err := os.WriteFile(path, []byte(preamble(r.Slug)), 0o644); err != nil {
			return err
		}
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(formatRow(r))
	return err
}

// parseRow reads one "| repo | branch | worktree | note | pr |" line.
// Splitting on "|" puts an empty field before the opening pipe and another
// after the closing one, so the cells are fields 1 through cells: a line
// yielding fewer than cells+1 fields has no last cell to read, and is not
// a data row. Neither is one whose repo cell is empty — the blank line
// under the table and any prose added below it both land there.
//
// A row a hand has broken past that is invisible, and invisible to every
// reader alike: it is missing from the report as well as from prune's
// blocker scan, so the operator sees their unit of work gone from `status`
// rather than each reader answering differently about it. One answer is
// the whole point — see the package comment.
func parseRow(line string) (Row, bool) {
	fields := strings.Split(line, "|")
	if len(fields) < cells+1 {
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
		PR:       cell(fields[5]),
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
// true for. Everything that isn't a data row — the header above all, whose
// own cells would otherwise read as a row about a repo called "repo" — is
// left exactly as it was.
func RemoveRows(path string, drop func(Row) bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
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
