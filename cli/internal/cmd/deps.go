package cmd

import (
	"fmt"
	"os/exec"
)

// requireBins fails unless every named executable is on PATH: a subcommand
// that can't work without git or gh should say so up front, by name, rather
// than letting the gap surface later as a confusing failure from whichever
// subprocess happened to need it first.
func requireBins(bins ...string) error {
	for _, bin := range bins {
		if _, err := exec.LookPath(bin); err != nil {
			return fmt.Errorf("missing dependency: %s", bin)
		}
	}
	return nil
}
