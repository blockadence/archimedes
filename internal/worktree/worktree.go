// Package worktree owns one unit of work's worktree: where it lives beside
// its repo's checkout, how an instance writes that down, and how a reader
// turns what is written down back into a path it can use.
//
// The three belong together because the second is a decision, not a
// formatting detail. work/<slug>/status.md is the instance's own content —
// committed to its history, read by everyone who has the instance, on
// machines that never ran the spawn. `template/.gitignore` ignores one
// file, `.archimedes-notify.json`, because it is per-machine and
// disposable; work/ is not ignored, and cannot be, since work/<slug>/ is
// also where a unit of work's reference material lives and sharing that is
// the point.
//
// So the worktree column is held to the rule every other path in an
// instance already follows: repos.yaml records `../<name>`, WORKSPACE-MAP.md
// links relatively, a dossier stub names `../<repo>`. It is recorded
// relative to the instance root. What the row states is not "a directory
// that exists on the machine that spawned this" but where this unit of
// work's worktree belongs — which is as true on a teammate's clone as
// repos.yaml's `../service-a` is true before they have cloned anything.
//
// Resolve reads the old absolute shape unchanged, so an instance that
// already carries such rows keeps working where it always did: on the
// machine that wrote them. Nothing rewrites them — a row is rewritten by
// the spawn that replaces it or removed by the prune that retires it.
package worktree

import "path/filepath"

// Path is where a repo's <slug> worktree lives: a sibling of the repo
// checkout itself, so it stays easy to find next to it without ever
// nesting inside it.
func Path(repoPath, slug string) string {
	return repoPath + "-worktrees/" + slug
}

// Record is how an instance at root writes wt down: relative to root, in
// slash form, so the row means the same thing on every machine that has
// the instance and reads the same way in the file.
//
// Where no relative path exists — a root and a worktree on different
// Windows volumes — wt is recorded as it came in. That is the old shape,
// which Resolve still understands, so the fallback costs the row its
// portability and nothing else.
func Record(root, wt string) string {
	rel, err := filepath.Rel(root, wt)
	if err != nil {
		return wt
	}
	return filepath.ToSlash(rel)
}

// Resolve turns a recorded worktree column into a usable path, against
// root (the instance directory the status file sits under).
//
// An absolute recorded value is returned as it stands. That is a row from
// before this column was relative, and the machine it was written on is
// the one it is true on; joining it to root would turn a path that still
// works there into one that works nowhere.
func Resolve(root, recorded string) string {
	if recorded == "" || filepath.IsAbs(recorded) {
		return recorded
	}
	return filepath.Join(root, filepath.FromSlash(recorded))
}
