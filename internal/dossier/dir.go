package dossier

import "path/filepath"

// Dir is where the instance at root keeps its dossiers.
//
// Two different things can live under this name, and this is only one of
// them. `repos/<name>.md` is the prose Archimedes parses — the dossiers,
// which is what Dir answers for. `repos/<name>/` may be somebody's
// checkout of the repository that prose is about: `repos.yaml` records a
// checkout as `../<name>` and bootstrap clones beside the instance, but the
// manifest permits a path below the root too, and `repos/<name>` is exactly
// the layout internal/worktree's own test exercises for that case. So one
// instance can hold `repos/service-a.md` and `repos/service-a/` at once.
// Nothing resolves a checkout through Dir, and nothing should: where a
// checkout is comes from the manifest entry that records it
// (manifest.CheckoutOf), wherever that entry says.
//
// Dir rather than the DirName the packages owning an instance's other
// directories export (conventionpack.DirName, reposync.ScaffoldingDirName,
// driver.dirName): those hand out a name their callers join, and a name is
// what would put the join back at the three call sites this removes. Dir
// answers with the joined directory, the way workdir.Path does — Path being
// the name it cannot have here, since a dossier's own file already has it.
//
// root is answered as it was given, as in workdir.Path: `--root .` is the
// ordinary way to name an instance, and every caller reads this answer from
// the directory the operator typed it in.
//
// The directory stays a parameter to the rest of this package — WriteStub,
// HouseRules, Path — rather than each of them taking root and joining. That
// parameter is what lets a dossier be read and written against a bare
// t.TempDir() with no instance around it, which is worth more than the join
// it would save; this is the difference from internal/workdir, where the
// location was the whole of the new package. Callers ask Dir, so the three
// that used to write `filepath.Join(root, "repos")` out by hand — bootstrap,
// reposync's house-rules sync, and spawn's materialize — still agree, and
// nobody spells the layout out for a fourth time (issue 72).
func Dir(root string) string {
	return filepath.Join(root, "repos")
}
