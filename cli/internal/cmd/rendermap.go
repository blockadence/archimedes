package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/blockadence/archimedes/cli/internal/manifest"
	"github.com/blockadence/archimedes/cli/internal/workspacemap"
)

func newRenderMapCmd() *cobra.Command {
	var root string

	cmd := &cobra.Command{
		Use:   "render-map",
		Short: "Regenerate WORKSPACE-MAP.md's repo list from repos.yaml",
		Long: `Regenerates the "## Repos" block of WORKSPACE-MAP.md from repos.yaml.
The "## Relationships" section below it is hand-written and left untouched.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runRenderMap(root)
		},
	}

	cmd.Flags().StringVar(&root, "root", ".", "instance root containing repos.yaml and WORKSPACE-MAP.md")

	return cmd
}

func runRenderMap(root string) error {
	manifestPath := filepath.Join(root, "repos.yaml")
	m, err := manifest.Load(manifestPath)
	if err != nil {
		return fmt.Errorf("loading %s: %w", manifestPath, err)
	}

	mapPath := filepath.Join(root, "WORKSPACE-MAP.md")
	existing, err := os.ReadFile(mapPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("reading %s: %w", mapPath, err)
		}
		existing = []byte(workspacemap.DefaultContent)
	}

	rendered := workspacemap.Render(string(existing), m.Repos)

	if err := os.WriteFile(mapPath, []byte(rendered), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", mapPath, err)
	}

	return nil
}
