package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/blockadence/gh-archimedes/internal/invocation"
	"github.com/blockadence/gh-archimedes/internal/mcpserver"
	"github.com/blockadence/gh-archimedes/internal/version"
)

func newServeMCPCmd() *cobra.Command {
	var root string

	cmd := &cobra.Command{
		Use:   "serve-mcp",
		Short: "Serve this instance to MCP-capable agent tools over stdio",
		Long: fmt.Sprintf(`Serves one Archimedes instance over the Model Context Protocol, so an
MCP-capable agent tool can list tracked repos, read worktree and pull-request
status, check context-map staleness, and spawn a unit of work as structured
tool calls — rather than shelling out to this CLI and parsing its tables.

It is a second way in, not a replacement: each tool delegates to the same
code the equivalent subcommand does, so the two report the same thing about
the same instance. The instance is fixed for the server's lifetime by --root;
no tool takes a path to another one.

The server speaks over stdin/stdout and runs until the client disconnects,
so it is started by the agent tool rather than by hand. Everything else —
git's own output, and any warning — goes to stderr, since stdout carries the
protocol itself. Register it with a client roughly as:

  %[1]s serve-mcp --root /path/to/instance`, invocation.Name()),
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			// git only, not gh: every gh lookup a tool makes goes
			// through status's, which degrades to "no PR" rather than
			// failing. Demanding gh here would refuse to start a server on
			// a machine where "archimedes status" itself works fine.
			if err := requireBins("git"); err != nil {
				return err
			}
			return mcpserver.Serve(c.Context(), serveMCPOptions(root, os.Getenv, c.ErrOrStderr()))
		},
	}

	cmd.Flags().StringVar(&root, "root", ".", "instance root containing repos.yaml, work/ and drivers/")

	return cmd
}

// serveMCPOptions assembles the server from the flag plus the same
// environment overrides the equivalent subcommands read, so a tool call
// lands on the configuration `archimedes status` or `archimedes
// context-map` would have used.
func serveMCPOptions(root string, env func(string) string, progress io.Writer) mcpserver.Options {
	return mcpserver.Options{
		Root:    root,
		Version: version.Current(),
		// Handed over unparsed: mcpserver runs it through the same
		// status.ParseGuardrailMax the status command does, so one setting
		// can't mean two thresholds.
		MaxStreams: env(maxStreamsEnvVar),
		ContextMap: contextMapOptions(root, false, env),
		Progress:   progress,
	}
}
