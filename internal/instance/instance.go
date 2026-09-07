// Package instance scaffolds a new Archimedes instance: the starting data
// on its own fresh git history, so instance-specific (possibly sensitive)
// content never lives in this repo's history.
//
// What lands in an instance is data and nothing else — a manifest, dossier
// and work directories, drivers and scaffolding it owns from here on. The
// tooling that acts on it is the archimedes binary itself, so there is
// nothing in an instance to keep in step with this repo and nothing to
// re-vendor into it.
//
// The template is a filesystem the caller supplies rather than a path this
// package goes looking for: cmd hands it the copy embedded in the binary
// (see the archimedes root package), which is what lets a machine with no
// clone of this repo create an instance at all.
package instance

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/blockadence/archimedes/internal/driver"
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
	if err := markDriverCommandsExecutable(dest); err != nil {
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

// markDriverCommandsExecutable restores the one file mode an instance
// depends on. A template filesystem carries no modes (the embedded one
// cannot), and a driver whose command isn't runnable is a driver that
// doesn't work — so the mode is derived from what each driver.yaml declares
// its command to be, rather than guessed from a filename. A driver's
// sourced helper is not a program and stays inert; so does everything
// outside drivers/.
func markDriverCommandsExecutable(dest string) error {
	driversDir := filepath.Join(dest, driver.DirName)
	entries, err := os.ReadDir(driversDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading %s: %w", driversDir, err)
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		m, err := driver.LoadManifest(driversDir, e.Name())
		// A directory under drivers/ that declares nothing is not a
		// driver, and this is not the place to complain about it. A
		// manifest that exists and won't load is a different problem.
		if errors.Is(err, driver.ErrNoManifest) {
			continue
		}
		if err != nil {
			return err
		}
		if m.Command == "" {
			return fmt.Errorf("driver %s: its manifest names no command", e.Name())
		}
		if err := os.Chmod(driver.CommandPath(driversDir, e.Name(), m), 0o755); err != nil {
			return fmt.Errorf("making driver %s runnable: %w", e.Name(), err)
		}
	}
	return nil
}
