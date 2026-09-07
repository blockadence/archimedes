// Package instance scaffolds a new Archimedes instance: the starting data
// on its own fresh git history, so instance-specific (possibly sensitive)
// content never lives in this repo's history.
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

	"github.com/blockadence/archimedes/internal/gitutil"
)

// Create scaffolds an instance named name under destParent from the
// template tree src, returning the path it created.
func Create(src fs.FS, name, destParent string) (string, error) {
	dest := filepath.Join(destParent, name)
	if _, err := os.Stat(dest); err == nil {
		return "", fmt.Errorf("%s already exists", dest)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("checking %s: %w", dest, err)
	}

	if err := scaffold(src, name, dest); err != nil {
		// Nothing was there a moment ago, so everything under dest is
		// ours to take back — and leaving half an instance behind would
		// make the retry fail with "already exists" rather than with
		// whatever actually went wrong.
		_ = os.RemoveAll(dest)
		return "", err
	}
	return dest, nil
}

func scaffold(src fs.FS, name, dest string) error {
	if err := materialize(src, dest); err != nil {
		return err
	}
	for _, args := range [][]string{
		{"init", "-q"},
		{"add", "-A"},
		{"commit", "-q", "-m", "Scaffold " + name + " from Archimedes template"},
	} {
		if _, err := gitutil.Run(dest, args...); err != nil {
			return err
		}
	}
	return nil
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
