// Package driver invokes a context-mapping driver by name against a target
// repo, writing its output to an exact path. It is the port of
// template/scripts/run-driver.sh, and the only thing orchestration (see
// internal/contextmap) needs to know about drivers: no specific driver's
// invocation is hardcoded anywhere else, so swapping which driver is
// configured never touches orchestration.
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
// rather than silently falling back to some other way of mapping.
func LoadManifest(driversDir, name string) (Manifest, error) {
	path := filepath.Join(driversDir, name, ManifestName)
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("unknown driver: %s (no manifest at %s)", name, path)
	}

	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("parsing %s: %w", path, err)
	}
	return m, nil
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

	switch m.OutputMode {
	case ModePathParameterized, ModeFixedLocation:
	default:
		return fmt.Errorf("driver %q declares output_mode %q, which archimedes doesn't support (only %s, %s)",
			name, m.OutputMode, ModePathParameterized, ModeFixedLocation)
	}
	if m.OutputMode == ModeFixedLocation && m.FixedPath == "" {
		return fmt.Errorf("driver %q declares output_mode %q but has no fixed_path in its manifest", name, ModeFixedLocation)
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

	if m.OutputMode == ModePathParameterized {
		if err := invoke(bin, progress, repoPath, outputPath); err != nil {
			return fmt.Errorf("driver %q: %w", name, err)
		}
		if !isFile(outputPath) {
			return fmt.Errorf("driver %q exited 0 but did not write %s", name, outputPath)
		}
		return nil
	}

	// fixed-location: the driver can't be told where to write, so we
	// invoke it with just the repo path and harvest its manifest-declared
	// fixed_path ourselves — moving rather than copying, so the target
	// repo ends up with no trace of the artifact.
	writtenAt := filepath.Join(repoPath, m.FixedPath)
	if err := invoke(bin, progress, repoPath); err != nil {
		return fmt.Errorf("driver %q: %w", name, err)
	}
	if !isFile(writtenAt) {
		return fmt.Errorf("driver %q exited 0 but did not write %s", name, writtenAt)
	}
	return move(writtenAt, outputPath)
}

// invoke runs the driver executable, streaming both its streams to
// progress so whatever it reports reaches the operator.
func invoke(bin string, progress io.Writer, args ...string) error {
	cmd := exec.Command(bin, args...)
	cmd.Stdout = progress
	cmd.Stderr = progress
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", bin, err)
	}
	return nil
}

// executable reports whether path is a regular file with an execute bit
// set — the port of run-driver.sh's `[ -x "$DRIVER_BIN" ]` check, which
// catches a manifest naming a command that was never shipped or never made
// executable before it produces a confusing exec failure.
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
// share one, and os.Rename can't cross that boundary the way mv does).
func move(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
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
