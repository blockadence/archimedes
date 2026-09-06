package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/blockadence/archimedes/cli/internal/manifest"
)

// loadManifest loads root's repos.yaml, the manifest every subcommand
// operating on an instance needs.
func loadManifest(root string) (*manifest.Manifest, error) {
	manifestPath := filepath.Join(root, "repos.yaml")
	m, err := manifest.Load(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("loading %s: %w", manifestPath, err)
	}
	return m, nil
}
