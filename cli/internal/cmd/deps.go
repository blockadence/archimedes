package cmd

import (
	"fmt"
	"os/exec"
)

// requireBinaries fails with the same "missing dependency" message the shell
// scripts' require() used, for each external tool a subcommand shells out to.
func requireBinaries(names ...string) error {
	for _, name := range names {
		if _, err := exec.LookPath(name); err != nil {
			return fmt.Errorf("missing dependency: %s", name)
		}
	}
	return nil
}
