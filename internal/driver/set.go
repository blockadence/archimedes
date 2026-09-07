package driver

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// Set is where a driver is looked up by name, in two layers: the drivers an
// instance keeps under its own drivers/, then the ones this binary ships.
// The instance layer wins, so a driver an operator wrote or took over is
// never displaced by one of ours, and a shipped driver nobody has touched
// is the tool's to fix — a bug in the 200 lines of bash that scaffold a
// third-party toolchain into somebody's repository is repaired by upgrading
// the tool, not by every instance separately.
//
// The built-in layer is a filesystem the caller supplies rather than a path
// this package goes looking for, the same arrangement internal/instance
// makes with the template: cmd hands over the copy embedded in the binary
// (see the archimedes root package), which is what lets an instance created
// on a machine with no clone of this repo run a shipped driver at all.
type Set struct {
	// Dir is the instance's own drivers/, searched first. It need not
	// exist: a fresh instance has none of its own, and everything it can
	// run comes from the tool.
	Dir string
	// Builtin holds the drivers this binary ships, rooted at their names
	// (so "spec-kit/driver.yaml", not "drivers/spec-kit/driver.yaml").
	// Nil means this Set has no built-in layer, which in production it
	// never is — see SetFor, the one place a Set is built.
	Builtin fs.FS
}

// SetFor is how a driver is looked up for an instance rooted at root: the
// instance's own drivers/ — or the one the operator named instead
// (ARCHIMEDES_DRIVERS_DIR) — over the drivers builtin ships.
//
// Everything that resolves a driver comes through here: a mapping pass,
// `run-driver`, and the `drivers` listing. Any two of them answering
// differently would mean a driver that works in a pass and is missing
// outside it, or a listing that promises what a run won't deliver.
func SetFor(root, override string, builtin fs.FS) Set {
	dir := override
	if dir == "" {
		dir = filepath.Join(root, dirName)
	}
	return Set{Dir: dir, Builtin: builtin}
}

// Run invokes the driver named name against repoPath and guarantees the
// finished context map ends up at exactly outputPath, whichever layer
// supplies the driver and whichever contract it declares.
func (s Set) Run(name, repoPath, outputPath string, progress io.Writer) error {
	dir, release, err := s.resolve(name)
	if err != nil {
		return err
	}
	defer release()

	return runIn(dir, name, repoPath, outputPath, progress)
}

// resolve returns a directory on disk holding the named driver, plus a
// function releasing whatever resolving it had to unpack. Answering with a
// directory rather than with the layer that won is what keeps the two
// layers from being two ways of running a driver: past this point a
// built-in is an ordinary driver directory like any other.
func (s Set) resolve(name string) (dir string, release func(), err error) {
	if s.owns(name) {
		return s.Dir, func() {}, nil
	}
	if s.ships(name) {
		return s.unpack(name)
	}
	return "", nil, fmt.Errorf("%w: %s (no driver by that name in %s, and none ships with archimedes)",
		errNoManifest, name, s.Dir)
}

// owns reports whether the instance layer declares a driver by this name.
func (s Set) owns(name string) bool {
	_, err := os.Stat(filepath.Join(s.Dir, name, manifestName))
	return err == nil
}

// ships reports whether the built-in layer declares a driver by this name.
func (s Set) ships(name string) bool {
	if s.Builtin == nil {
		return false
	}
	_, err := fs.Stat(s.Builtin, path.Join(name, manifestName))
	return err == nil
}

// unpack writes a built-in driver to a temporary directory so it can be
// executed, since nothing embedded in a binary can be. It is unpacked per
// run and thrown away afterwards rather than cached: a cache would have to
// be invalidated when the tool is upgraded, and the whole point of the
// built-in layer is that upgrading the tool is what delivers a fix. Driver
// runs cost minutes and network; unpacking a few files costs neither.
//
// It goes wherever os.MkdirTemp puts it, which a driver has to be able to
// execute from. On a machine whose temp directory is mounted noexec, point
// TMPDIR somewhere it isn't — the instance's own drivers still run either
// way, so the symptom is that only the shipped ones fail.
func (s Set) unpack(name string) (dir string, release func(), err error) {
	dir, err = os.MkdirTemp("", "archimedes-driver-")
	if err != nil {
		return "", nil, err
	}
	release = func() { _ = os.RemoveAll(dir) }

	if err := unpackInto(s.Builtin, name, filepath.Join(dir, name)); err != nil {
		release()
		return "", nil, err
	}
	if err := makeCommandExecutable(dir, name); err != nil {
		release()
		return "", nil, err
	}
	return dir, release, nil
}

// unpackInto writes the built-in driver named name out to dest.
func unpackInto(builtin fs.FS, name, dest string) error {
	return fs.WalkDir(builtin, name, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dest, filepath.FromSlash(strings.TrimPrefix(path, name)))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := fs.ReadFile(builtin, path)
		if err != nil {
			return fmt.Errorf("reading built-in driver %s: %w", path, err)
		}
		return os.WriteFile(target, data, 0o644)
	})
}

// makeCommandExecutable restores the one file mode a driver depends on. No
// filesystem the built-in layer can be built from carries modes, and a
// driver whose command isn't runnable is a driver that doesn't work — so
// the mode is derived from what the manifest declares its command to be,
// rather than guessed from a filename. A driver's sourced helper is not a
// program and stays inert.
func makeCommandExecutable(driversDir, name string) error {
	m, err := readManifest(os.DirFS(driversDir), driversDir, name)
	if err != nil {
		return err
	}
	if m.Command == "" {
		return fmt.Errorf("driver %s: its manifest names no command", name)
	}
	if err := os.Chmod(commandPath(driversDir, name, m), 0o755); err != nil {
		return fmt.Errorf("making driver %s runnable: %w", name, err)
	}
	return nil
}

// Origin says which layer of a Set supplied a driver, which is the same as
// saying who maintains it.
type Origin string

const (
	// FromInstance drivers are the instance's own, under its drivers/.
	FromInstance Origin = "instance"
	// FromBuiltin drivers are the ones this binary ships.
	FromBuiltin Origin = "built-in"
)

// Entry is one name a Set can answer, and where the answer comes from.
type Entry struct {
	Name        string
	Description string
	Origin      Origin
	// Shadows marks an instance driver that has taken over a name the
	// tool also ships. It is not an error — an operator is free to take a
	// shipped driver over — but it is worth saying out loud, because it
	// is exactly the state in which fixes made to the shipped driver stop
	// arriving.
	Shadows bool
	// Err is why this driver could not be described — an unparsable
	// manifest, usually. It is carried on the entry rather than failing
	// the listing, because listing is the command an operator reaches for
	// when something is wrong, and one broken driver must not cost them
	// the report on the other five.
	Err error
}

// List reports every driver this Set can run, name-ordered, resolved the
// same way Run resolves one. A directory that declares no driver is not one
// and is passed over in silence; a manifest that exists and won't parse is
// a real problem, and is reported on its own entry rather than taking the
// listing down with it.
func (s Set) List() ([]Entry, error) {
	byName := map[string]Entry{}

	if s.Builtin != nil {
		names, err := driverDirs(s.Builtin)
		if err != nil {
			return nil, fmt.Errorf("reading the drivers archimedes ships: %w", err)
		}
		for _, name := range names {
			byName[name] = s.describe(s.Builtin, "", name, FromBuiltin, false)
		}
	}

	own, err := driverDirs(os.DirFS(s.Dir))
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", s.Dir, err)
	}
	for _, name := range own {
		_, alsoShipped := byName[name]
		e := s.describe(os.DirFS(s.Dir), s.Dir, name, FromInstance, alsoShipped)
		// A directory under drivers/ that declares nothing is not a
		// driver, and this is not the place to complain about it. If the
		// tool ships one by that name, its entry stands.
		if errors.Is(e.Err, errNoManifest) {
			continue
		}
		byName[name] = e
	}

	entries := make([]Entry, 0, len(byName))
	for _, e := range byName {
		entries = append(entries, e)
	}
	slices.SortFunc(entries, func(a, b Entry) int { return strings.Compare(a.Name, b.Name) })
	return entries, nil
}

// describe reads one driver's manifest for the listing, keeping a failure
// on the entry instead of returning it.
func (s Set) describe(fsys fs.FS, where, name string, origin Origin, shadows bool) Entry {
	e := Entry{Name: name, Origin: origin, Shadows: shadows}
	m, err := readManifest(fsys, where, name)
	if err != nil {
		e.Err = err
		return e
	}
	e.Description = m.Description
	return e
}

// driverDirs lists the directories under fsys, each a candidate driver. A
// drivers directory that isn't there at all is not an error — a fresh
// instance has none of its own — so it reads as no drivers rather than as a
// broken instance.
func driverDirs(fsys fs.FS) ([]string, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// Adopt copies the shipped driver named name into the instance's own
// drivers/, returning where it landed. From then on it is the instance's:
// it wins over the shipped one, and fixes made to the shipped one stop
// arriving — which is the deal, and the reason this is something an
// operator asks for rather than something that happens to them.
//
// It is not a sync step in either direction. Nothing is copied out of a
// checkout, nothing is refreshed later, and running it twice is refused
// rather than resolved: the copy already there may be the edit that was the
// whole point of adopting.
func (s Set) Adopt(name string) (string, error) {
	if !s.ships(name) {
		return "", fmt.Errorf("%w: %s does not ship with archimedes (run `archimedes drivers` to see what does)", errNoManifest, name)
	}
	dest := filepath.Join(s.Dir, name)
	if _, err := os.Stat(dest); err == nil {
		return "", fmt.Errorf("%s already exists — this instance already has its own %s", dest, name)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("checking %s: %w", dest, err)
	}

	if err := unpackInto(s.Builtin, name, dest); err != nil {
		// Nothing was there a moment ago, so a half-written driver is
		// ours to take back rather than leave for the retry to trip on.
		_ = os.RemoveAll(dest)
		return "", err
	}
	if err := makeCommandExecutable(s.Dir, name); err != nil {
		_ = os.RemoveAll(dest)
		return "", err
	}
	return dest, nil
}
