package cmd

import (
	"github.com/spf13/cobra"

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
	m, err := loadManifest(root)
	if err != nil {
		return err
	}

	return workspacemap.Update(root, m.Repos)
}
