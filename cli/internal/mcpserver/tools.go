package mcpserver

import (
	"context"
	"fmt"
	"io"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/blockadence/archimedes/cli/internal/contextmap"
	"github.com/blockadence/archimedes/cli/internal/manifest"
	"github.com/blockadence/archimedes/cli/internal/spawn"
	"github.com/blockadence/archimedes/cli/internal/stackref"
	"github.com/blockadence/archimedes/cli/internal/status"
)

// noArgs is the input type for a tool that asks the instance a question
// with no parameters. The MCP spec requires an object input schema, so it
// is a struct rather than nothing at all.
type noArgs struct{}

// Repo is one tracked repo as repos.yaml records it, plus the checkout path
// that record resolves to.
type Repo struct {
	Name string `json:"name" jsonschema:"the repo's name, as every other tool refers to it"`
	// Path is what repos.yaml records, relative to the instance root;
	// Checkout is that path resolved, which is what a caller can open.
	Path              string   `json:"path" jsonschema:"the repo's location as repos.yaml records it, relative to the instance root"`
	Checkout          string   `json:"checkout" jsonschema:"the repo's local checkout, as an absolute path"`
	BaseBranch        string   `json:"base_branch" jsonschema:"the branch a unit of work in this repo is cut from"`
	DependsOn         []string `json:"depends_on,omitempty" jsonschema:"the other tracked repos a unit of work here tends to reach into"`
	ContextModeledSHA string   `json:"context_modeled_sha,omitempty" jsonschema:"the base-branch commit this repo's context map was last built against; empty means never mapped"`
	ConventionPack    string   `json:"convention_pack,omitempty" jsonschema:"the shared build/lint convention this repo follows"`
	Driver            string   `json:"driver,omitempty" jsonschema:"the context-mapping driver this repo overrides the instance default with"`
}

// Repos is every repo the instance tracks.
type Repos struct {
	Root string `json:"root" jsonschema:"the instance root this server is bound to, as an absolute path"`
	// Driver is the instance-wide default context-mapping driver, used for
	// any repo that doesn't name one of its own.
	Driver string `json:"driver,omitempty" jsonschema:"the instance-wide default context-mapping driver"`
	Repos  []Repo `json:"repos"`
}

func (s server) listRepos(context.Context, *mcp.CallToolRequest, noArgs) (*mcp.CallToolResult, Repos, error) {
	m, err := manifest.Load(s.manifestPath())
	if err != nil {
		return nil, Repos{}, fmt.Errorf("loading %s: %w", s.manifestPath(), err)
	}

	out := Repos{Root: s.root, Driver: m.Driver, Repos: make([]Repo, 0, len(m.Repos))}
	for _, r := range m.Repos {
		out.Repos = append(out.Repos, Repo{
			Name:              r.Name,
			Path:              r.Path,
			Checkout:          s.repo(r.Path),
			BaseBranch:        r.BaseBranch,
			DependsOn:         r.DependsOn,
			ContextModeledSHA: r.ContextModeledSHA,
			ConventionPack:    r.ConventionPack,
			Driver:            r.Driver,
		})
	}

	return nil, out, nil
}

// statusArgs narrows a status report to one unit of work.
type statusArgs struct {
	Slug string `json:"slug,omitempty" jsonschema:"limit the report to this unit of work; omit to report every one"`
}

func (s server) repoStatus(_ context.Context, _ *mcp.CallToolRequest, args statusArgs) (*mcp.CallToolResult, status.Report, error) {
	report, err := status.Collect(s.root, args.Slug, s.status, s.guardrailMax)
	if err != nil {
		return nil, status.Report{}, err
	}
	return nil, report, nil
}

func (s server) contextMapStatus(context.Context, *mcp.CallToolRequest, noArgs) (*mcp.CallToolResult, contextmap.Plan, error) {
	plan, err := contextmap.Survey(s.contextMap, s.progress)
	if err != nil {
		return nil, contextmap.Plan{}, err
	}
	return nil, plan, nil
}

// spawnArgs is one spawn request, mirroring the subcommand's arguments and
// flags.
type spawnArgs struct {
	Slug string `json:"slug" jsonschema:"names the unit of work; it becomes the branch name"`
	Repo string `json:"repo" jsonschema:"the target repo's name, as list_repos reports it"`
	Base string `json:"base,omitempty" jsonschema:"start the new branch from this ref instead of the repo's own base branch"`
	// StackOn is spelled the way --stack-on takes it, so an agent that has
	// read a status row's "stacked on <repo>:<slug>" note can pass it
	// straight back.
	StackOn string `json:"stack_on,omitempty" jsonschema:"start the new branch on top of another unit of work's branch, as <repo>:<slug>"`
}

func (s server) spawnWorktree(_ context.Context, _ *mcp.CallToolRequest, args spawnArgs) (*mcp.CallToolResult, spawn.Result, error) {
	// Two things the subcommand does that a tool call deliberately doesn't.
	//
	// The result stream is discarded: its two lines are how a terminal
	// reports a spawn to a person — including a "cd <worktree> && <agent>"
	// hint whose whole content is already in the Result, addressed to
	// someone who isn't here. Git's own output still reaches the progress
	// stream, so the log of what happened is intact.
	//
	// And no Workspace is passed: a pane opening on the operator's machine
	// is something they ask for at their own prompt, not a side effect of
	// an agent's tool call. Everything else is the spawn the subcommand
	// would have run.
	result, err := spawn.Run(spawn.Options{
		Root:  s.root,
		Slug:  args.Slug,
		Repo:  args.Repo,
		Base:  args.Base,
		Stack: stackref.ParseFlag(args.StackOn),
	}, io.Discard, s.progress)
	if err != nil {
		return nil, spawn.Result{}, err
	}
	return nil, result, nil
}
