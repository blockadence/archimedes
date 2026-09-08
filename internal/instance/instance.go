// Package instance scaffolds a new Archimedes instance: the starting data
// on its own fresh git history, so instance-specific (possibly sensitive)
// content never lives in this repo's history.
//
// "On its own history" is as far as it goes on a machine where git has no
// identity to commit under, which is every fresh laptop, container and CI
// runner: the instance is written and its repository initialized, and the
// first commit waits for the operator. Create says which of the two
// happened so its caller can pass that on. It will not invent an identity
// to close the gap — an instance is the operator's own repository and its
// first commit is in that history forever, so a fabricated author there is
// worse than an instance that is merely uncommitted.
//
// What lands in an instance is data and nothing else — a manifest, dossier
// and work directories, scaffolding it owns from here on, and an empty
// drivers/ for whatever drivers it comes to own. Not one file of it is a
// program: the tooling that acts on an instance is the archimedes binary,
// and so are the drivers it ships (see internal/driver's Set). So there is
// nothing in an instance to keep in step with this repo, nothing to
// re-vendor into it, and no file mode to restore on the way in.
//
// The template is a filesystem the caller supplies rather than a path this
// package goes looking for: cmd hands it the copy embedded in the binary
// (see the archimedes root package), which is what lets a machine with no
// clone of this repo create an instance at all.
package instance

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/blockadence/gh-archimedes/internal/gitutil"
)

// Result is the instance Create made: where it is, and whether it starts on
// a first commit.
type Result struct {
	// Path is the instance directory that was created.
	Path string

	// Committed reports whether the scaffolding is the instance's first
	// commit.
	Committed bool
}

// CommitSubject is the subject Create gives an instance's first commit. It
// is exported for the caller that has to tell an operator how to make that
// commit by hand, so what they are told to type is the line itself rather
// than a paraphrase of it that drifts from it.
func CommitSubject(name string) string {
	return "Scaffold " + name + " from Archimedes template"
}

// Create scaffolds an instance named name under destParent from the
// template tree src.
func Create(src fs.FS, name, destParent string) (Result, error) {
	dest := filepath.Join(destParent, name)
	if _, err := os.Stat(dest); err == nil {
		return Result{}, fmt.Errorf("%s already exists", dest)
	} else if !os.IsNotExist(err) {
		return Result{}, fmt.Errorf("checking %s: %w", dest, err)
	}

	committed, err := scaffold(src, name, dest)
	if err != nil {
		// Nothing was there a moment ago, so everything under dest is
		// ours to take back — and leaving half an instance behind would
		// make the retry fail with "already exists" rather than with
		// whatever actually went wrong.
		_ = os.RemoveAll(dest)
		return Result{}, err
	}
	return Result{Path: dest, Committed: committed}, nil
}

func scaffold(src fs.FS, name, dest string) (committed bool, err error) {
	if err := materialize(src, dest); err != nil {
		return false, err
	}
	if _, err := gitutil.Run(dest, "init", "-q"); err != nil {
		return false, err
	}

	// Asked after `git init` rather than before, so an identity set on this
	// repository alone counts the same as a global one — and asked at all
	// because the alternative is handing an operator git's "Please tell me
	// who you are" as the first thing this tool ever says to them.
	//
	// Nothing is staged on the way out. What such an operator is told to run
	// is `git add -A && git commit`, and an index left half-filled here would
	// make that line quietly wrong about what it commits.
	if !gitutil.HasCommitIdentity(dest) {
		return false, nil
	}

	for _, args := range [][]string{
		{"add", "-A"},
		{"commit", "-q", "-m", CommitSubject(name)},
	} {
		if _, err := gitutil.Run(dest, args...); err != nil {
			return false, err
		}
	}
	return true, nil
}

// materialize writes every file in src under dest.
func materialize(src fs.FS, dest string) error {
	return fs.WalkDir(src, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dest, filepath.FromSlash(path))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := fs.ReadFile(src, path)
		if err != nil {
			return fmt.Errorf("reading template %s: %w", path, err)
		}
		return os.WriteFile(target, data, 0o644)
	})
}
