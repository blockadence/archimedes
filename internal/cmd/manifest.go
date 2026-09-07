package cmd

import (
	"github.com/blockadence/gh-archimedes/internal/manifest"
)

// loadManifest loads root's repos.yaml, the manifest every subcommand
// operating on an instance needs.
//
// The absolute root manifest.LoadInstance resolves is dropped on purpose:
// these subcommands go on resolving repo paths against the root the
// operator typed, so that a relative --root keeps producing the relative
// paths their output and their git invocations have always used.
func loadManifest(root string) (*manifest.Manifest, error) {
	_, m, err := manifest.LoadInstance(root)
	return m, err
}
