// Package workdir owns work/<slug>: the directory an instance keeps one
// unit of work in.
//
// What is in it is that unit of work's reference material — the ticket, the
// spec, the mockup, the notes — which spawn materializes into every
// worktree the unit of work spans (see spawn.Materialize). That is why the
// directory is not per-machine state and cannot be: it is committed to the
// instance and read by everyone who has it, on machines that never ran the
// spawn and whose directory layout is their own. `template/.gitignore`
// ignores one file, `.archimedes-notify.json`, because it is local and
// disposable; work/ is not ignored, and sharing what is under it is the
// whole reason the directory exists.
//
// The one thing in it that is not reference material is Archimedes' own
// bookkeeping, work/<slug>/status.md — which internal/statusfile owns, and
// which spawn's materialize excludes from the copy for exactly that reason.
// So the directory is both, and each half is the premise of a decision
// already made elsewhere: the worktree column is recorded relative to the
// instance root because the file travels (internal/worktree), and the file
// has one parser because three packages read what one writes
// (internal/statusfile). Each of those packages was restating the premise
// to justify its own decision. It is stated here, and they cite it.
//
// spawn, spawn's materialize and statusfile each used to join the location
// by hand. One function, because the location is one fact — and a package
// rather than three joins so that what the directory is has somewhere to be
// said once, which is the part the three sites were each restating.
//
// Not internal/instance, which scaffolds what a new instance starts with
// and would otherwise be the obvious home: Create walks whatever template
// tree it is handed and never names work/ at all, so what knows this layout
// at creation time is template/, not the package that copies it.
package workdir

import "path/filepath"

// Path is where the instance at root keeps slug's directory.
//
// root is answered as it was given: `--root .` is the ordinary way to name
// an instance, and every caller joins onto this answer or reads it from the
// working directory the operator typed it in — unlike worktree.Resolve,
// whose answer goes to git running somewhere else entirely.
//
// A slug of "*" is how statusfile.Discover walks an instance's units of
// work: the same join, globbed rather than opened.
func Path(root, slug string) string {
	return filepath.Join(root, "work", slug)
}
