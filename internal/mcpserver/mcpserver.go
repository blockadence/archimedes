// Package mcpserver exposes one Archimedes instance over the Model Context
// Protocol, so an MCP-capable agent tool can ask about repos, worktrees and
// context-map staleness — and spawn a new unit of work — as structured tool
// calls instead of shelling out to the CLI and parsing its tables.
//
// It is a second way in, not a second implementation: every tool delegates
// to the same internal package the equivalent subcommand does, so the two
// can't drift on what the instance currently looks like. What lives here is
// only the mapping between a tool call and that package's arguments.
package mcpserver

import (
	"context"
	"io"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/blockadence/gh-archimedes/internal/contextmap"
	"github.com/blockadence/gh-archimedes/internal/status"
)

// ServerName is how the server identifies itself to clients.
const ServerName = "archimedes"

// Options is one server's configuration, bound to one instance for its
// lifetime — the same instance a CLI invocation would name with --root.
type Options struct {
	// Root is the instance directory holding repos.yaml, work/ and drivers/.
	// It is resolved to an absolute path when the server is built, so every
	// tool reports paths a client can actually open, whatever directory the
	// server was started from.
	Root string
	// Version is reported to clients as the server's version.
	Version string
	// Status is what a status report reads the world through: gh, and the
	// repo checkouts on disk. The zero value means the real sources, so a
	// test can substitute the parts it cares about without the server
	// knowing it happened.
	Status status.Sources
	// ContextMap carries the environment-level context-map settings
	// (drivers directory, default driver, context file), so the staleness
	// tool assesses repos exactly as `context-map --dry-run` would.
	ContextMap contextmap.Options
	// MaxStreams is the raw open-worktree guardrail setting, in the form
	// the ARCHIMEDES_MAX_STREAMS environment variable carries it. It is
	// held unparsed and handed to status.ParseGuardrailMax, so the same
	// setting can't mean one threshold here and another on the command
	// line — "0" is a threshold of zero either way, and only an empty
	// value falls back to the default.
	MaxStreams string
	// Progress is where git's and any subprocess's own output goes. Nil
	// discards it — and it must never be stdout: over the stdio transport,
	// stdout carries the protocol itself, so a stray line of git chatter
	// would corrupt the session.
	Progress io.Writer
}

// server is one running server's resolved state: every default Options
// leaves open, decided once at construction rather than on each call, so
// two tools in the same session can't answer from different settings.
type server struct {
	// root is Options.Root made absolute.
	root         string
	status       status.Sources
	contextMap   contextmap.Options
	guardrailMax int
	progress     io.Writer
}

func newServer(opts Options) server {
	s := server{
		root:         opts.Root,
		status:       opts.Status,
		contextMap:   opts.ContextMap,
		guardrailMax: status.ParseGuardrailMax(opts.MaxStreams),
		progress:     opts.Progress,
	}

	// Paths a tool reports are paths a client will try to open, and it
	// won't share this process's working directory. A root that can't be
	// absolutized is left as given rather than failing the server: the
	// tools report their own errors, where a client can read them.
	if abs, err := filepath.Abs(s.root); err == nil {
		s.root = abs
	}
	s.contextMap.Root = s.root

	if s.progress == nil {
		s.progress = io.Discard
	}
	// The live lookups a caller didn't supply, so wanting the real thing
	// means leaving Status alone and a test can replace one lookup without
	// having to supply the rest.
	if s.status.PR == nil {
		s.status.PR = status.GHLookup
	}
	if s.status.Refs == nil {
		s.status.Refs = status.LocalRefs{}
	}
	if s.status.Merged == nil {
		s.status.Merged = status.GHMerged
	}

	return s
}

// repo resolves a path recorded relative to the instance root, so every
// tool reports the same absolute location for the same repo.
func (s server) repo(path string) string { return filepath.Join(s.root, path) }

func (s server) manifestPath() string { return filepath.Join(s.root, "repos.yaml") }

// readOnly annotates a tool that only reads instance state, so a client can
// tell at a glance which calls are safe to make unprompted.
func readOnly(title string) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{Title: title, ReadOnlyHint: true}
}

// New builds the server and registers every tool it serves. The caller
// connects it to whichever transport it wants; Serve does that over stdio.
func New(opts Options) *mcp.Server {
	version := opts.Version
	if version == "" {
		version = "dev"
	}
	s := newServer(opts)

	server := mcp.NewServer(&mcp.Implementation{Name: ServerName, Version: version}, &mcp.ServerOptions{
		Instructions: instructions,
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_repos",
		Description: "List every repo this Archimedes instance tracks, as repos.yaml records it: where each one is checked out, which branch its work is based on, which repos it depends on, and whether its context map has ever been built.",
		Annotations: readOnly("List tracked repos"),
	}, s.listRepos)

	mcp.AddTool(server, &mcp.Tool{
		Name: "repo_status",
		Description: "Report every worktree this instance has spawned and each one's live pull-request state, " +
			"the same report `archimedes status` prints. Flags any stacked unit of work whose base branch has " +
			"since merged and so needs a rebase, and reports whether more worktree streams are open than the " +
			"instance's guardrail allows.",
		Annotations: readOnly("Worktree and PR status"),
	}, s.repoStatus)

	mcp.AddTool(server, &mcp.Tool{
		Name: "context_map_status",
		Description: "Report which repos' context maps are stale and in what order a mapping pass would rebuild " +
			"them — dependency and base repos first — the same plan `archimedes context-map --dry-run` prints. " +
			"Fetches each repo's base branch to measure staleness against the remote, but builds nothing and " +
			"changes nothing.",
		Annotations: readOnly("Context-map staleness"),
	}, s.contextMapStatus)

	mcp.AddTool(server, &mcp.Tool{
		Name: "spawn_worktree",
		Description: "Create the branch and worktree for one unit of work in one tracked repo, the same thing " +
			"`archimedes spawn` does: fetches first so the branch starts from current remote state, materializes " +
			"the unit of work's reference material into the new worktree, and records the row in its status file. " +
			"Writes to disk — ask before calling it.",
		Annotations: &mcp.ToolAnnotations{Title: "Spawn a worktree"},
	}, s.spawnWorktree)

	return server
}

// instructions tell a client what this server is for and, just as
// importantly, which of its tools writes to disk.
const instructions = `Archimedes orchestrates planning and git-worktree lifecycle across a family of
related repos. This server is bound to one instance; every tool acts on that
instance, and none of them takes a path to another one.

list_repos, repo_status and context_map_status only read. spawn_worktree
creates a branch and a worktree on disk — ask before calling it.`

// Serve runs the server over stdio until the client disconnects.
//
// opts.Progress must not be os.Stdout: stdout carries the protocol. A
// caller that wants git's output kept should point Progress at stderr,
// which is where a client collects a server's logs; nil discards it.
func Serve(ctx context.Context, opts Options) error {
	return New(opts).Run(ctx, &mcp.StdioTransport{})
}
