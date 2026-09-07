// Package driver invokes a context-mapping driver by name against a target
// repo, writing its output to an exact path. It is the only thing
// orchestration (see internal/contextmap) needs to know about drivers: no
// specific driver's invocation is hardcoded anywhere else, so swapping
// which driver is configured never touches orchestration.
//
// A driver is a directory under the instance's drivers/ holding a
// driver.yaml manifest and the executable it names. The manifest's
// output_mode picks which of two invocation contracts it honors — see
// template/drivers/README.md for the contract as drivers see it.
package driver

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Output modes a driver can declare. Anything else is a misconfiguration.
const (
	// ModePathParameterized drivers accept an explicit output location and
	// write exactly there.
	ModePathParameterized = "path-parameterized"
	// ModeFixedLocation drivers can't be told where to write; they always
	// write to FixedPath relative to the repo they're run in, and we
	// harvest that file afterward.
	ModeFixedLocation = "fixed-location"
)

// ManifestName is the file every driver directory must contain.
const ManifestName = "driver.yaml"

// Manifest is a driver's driver.yaml.
type Manifest struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	OutputMode  string `yaml:"output_mode"`
	// FixedPath is where a fixed-location driver always writes, relative
	// to the repo it's run in. Unused by path-parameterized drivers.
	FixedPath string `yaml:"fixed_path"`
	// Command is the executable to run, relative to the driver directory.
	Command string `yaml:"command"`
}

// LoadManifest reads the manifest for the driver named name under
// driversDir. A driver with no manifest there is an unknown driver: naming
// one that doesn't exist is a misconfiguration that fails immediately,
// rather than silently falling back to some other way of mapping. A
// manifest that exists but can't be read is a different problem, and says
// so rather than blaming the name.
func LoadManifest(driversDir, name string) (Manifest, error) {
	path := filepath.Join(driversDir, name, ManifestName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Manifest{}, fmt.Errorf("unknown driver: %s (no manifest at %s)", name, path)
		}
		return Manifest{}, fmt.Errorf("reading %s: %w", path, err)
	}

	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("parsing %s: %w", path, err)
	}
	return m, nil
}

// invocation is how one output_mode gets a driver's output to outputPath.
// Picking one up front (see Manifest.plan) is what keeps the output_mode
// decision in a single place rather than re-tested at every step.
type invocation func(bin, repoPath, outputPath string, progress io.Writer) error

// plan validates a manifest and returns how to invoke it. Everything a
// mode requires beyond the mode itself — fixed-location's fixed_path — is
// checked here, so a misconfigured manifest is rejected before any driver
// runs rather than partway through one.
func (m Manifest) plan(name string) (invocation, error) {
	switch m.OutputMode {
	case ModePathParameterized:
		return writeWhereTold, nil
	case ModeFixedLocation:
		if m.FixedPath == "" {
			return nil, fmt.Errorf("driver %q declares output_mode %q but has no fixed_path in its manifest", name, ModeFixedLocation)
		}
		return harvestFrom(m.FixedPath), nil
	default:
		return nil, fmt.Errorf("driver %q declares output_mode %q, which archimedes doesn't support (only %s, %s)",
			name, m.OutputMode, ModePathParameterized, ModeFixedLocation)
	}
}

// Run invokes the driver named name against repoPath and guarantees the
// finished context map ends up at exactly outputPath, whichever contract
// the driver declares. The driver's own stdout/stderr go to progress.
//
// Every failure mode leaves no output file behind: an unknown driver, an
// output_mode we don't support, a command that isn't executable, a non-zero
// exit, and — the one a driver can't self-report — a zero exit that never
// produced the file it promised.
func Run(driversDir, name, repoPath, outputPath string, progress io.Writer) error {
	m, err := LoadManifest(driversDir, name)
	if err != nil {
		return err
	}
	invoke, err := m.plan(name)
	if err != nil {
		return err
	}

	bin := filepath.Join(driversDir, name, m.Command)
	if err := executable(bin); err != nil {
		return fmt.Errorf("driver %q command not executable: %s (%w)", name, bin, err)
	}

	// Drivers are run with their working directory left alone and handed
	// the repo as an argument, so it has to be absolute — a relative one
	// would resolve against whatever directory the driver chooses to work
	// in rather than against ours.
	repoPath, err = filepath.Abs(repoPath)
	if err != nil {
		return fmt.Errorf("resolving repo path %s: %w", repoPath, err)
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}

	if err := invoke(bin, repoPath, outputPath, progress); err != nil {
		return fmt.Errorf("driver %q: %w", name, err)
	}
	return nil
}

// writeWhereTold is the path-parameterized contract: the driver is handed
// the output location and writes exactly there.
func writeWhereTold(bin, repoPath, outputPath string, progress io.Writer) error {
	if err := run(bin, progress, repoPath, outputPath); err != nil {
		return err
	}
	if !isFile(outputPath) {
		return fmt.Errorf("exited 0 but did not write %s", outputPath)
	}
	return nil
}

// harvestFrom is the fixed-location contract: the driver can't be told
// where to write, so it's invoked with just the repo path and we harvest
// its manifest-declared fixedPath ourselves — moving rather than copying,
// so the target repo ends up with no trace of the artifact.
func harvestFrom(fixedPath string) invocation {
	return func(bin, repoPath, outputPath string, progress io.Writer) error {
		writtenAt := filepath.Join(repoPath, fixedPath)
		if err := run(bin, progress, repoPath); err != nil {
			return err
		}
		if !isFile(writtenAt) {
			return fmt.Errorf("exited 0 but did not write %s", writtenAt)
		}
		if err := move(writtenAt, outputPath); err != nil {
			return err
		}
		pruneEmptied(repoPath, fixedPath)
		return nil
	}
}

// pruneEmptied removes the directories that moving fixedPath out of
// repoPath left holding nothing, walking up toward — but never reaching —
// repoPath itself.
//
// A fixed_path can be nested, because a driver wrapping a tool that
// scaffolds itself into the repo has no say in where that tool writes (the
// spec-kit driver's is .specify/memory/constitution.md). `git status`
// wouldn't have caught what's left, since git doesn't track directories,
// but it's a trace of the run all the same.
//
// os.Remove refuses a directory that still holds anything, which is exactly
// right for one that predates the run or holds something else; the first
// refusal ends the walk. Failures are otherwise ignored: the artifact is
// already safely harvested by this point, and an undeletable directory
// isn't worth failing a run over.
func pruneEmptied(repoPath, fixedPath string) {
	for dir := filepath.Dir(fixedPath); dir != "." && dir != string(filepath.Separator); dir = filepath.Dir(dir) {
		if err := os.Remove(filepath.Join(repoPath, dir)); err != nil {
			return
		}
	}
}

// run executes the driver, streaming both its streams to progress so
// whatever it reports reaches the operator.
func run(bin string, progress io.Writer, args ...string) error {
	cmd := exec.Command(bin, args...)
	cmd.Stdout = progress
	cmd.Stderr = progress
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", bin, err)
	}
	return nil
}

// executable reports whether path is a regular file with an execute bit
// set, catching a manifest naming a command that was never shipped or never
// made executable before it produces a confusing exec failure.
func executable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() || info.Mode().Perm()&0o111 == 0 {
		return errors.New("not an executable file")
	}
	return nil
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// move relocates src to dst, falling back to copy-then-remove when the two
// are on different filesystems (the control repo and a target repo need not
// share one, and os.Rename can't cross that boundary the way mv does). The
// copy carries src's mode, so the harvested map is the file the driver
// wrote either way.
func move(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Remove(src)
}
