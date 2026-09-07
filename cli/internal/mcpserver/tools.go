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

// PlannedRepo is one repo's place in a planned mapping pass: where it sits
// in the order, whether its map is still good for its base branch's current
// commit, and which driver would build it if it isn't.
type PlannedRepo struct {
	Name string `json:"name" jsonschema:"the repo's name, as every other tool refers to it"`
	Path string `json:"path" jsonschema:"the repo's local checkout, as an absolute path"`
	// ContextFile is where this repo's map lives, relative to Path.
	ContextFile string `json:"context_file" jsonschema:"where this repo's context map lives, relative to its checkout"`
	BaseBranch  string `json:"base_branch" jsonschema:"the branch staleness is measured against"`
	// Cloned is false for a repo listed in repos.yaml whose checkout isn't
	// on disk yet. Nothing below it has been read in that case, so its
	// staleness is unknown rather than false.
	Cloned bool `json:"cloned" jsonschema:"false for a repo repos.yaml lists but bootstrap hasn't cloned yet"`
	// Stale is true when the repo needs mapping; Reason says why, or, for
	// a repo that isn't cloned, why it couldn't be assessed at all.
	Stale  bool   `json:"stale" jsonschema:"true when this repo's context map needs rebuilding"`
	Reason string `json:"reason,omitempty" jsonschema:"why the map is stale, or why the repo couldn't be assessed"`
	// StoredSHA is the commit repos.yaml records the map as built against;
	// CurrentSHA is the commit its base branch is actually on now.
	StoredSHA  string `json:"stored_sha,omitempty" jsonschema:"the commit repos.yaml records the map as built against; empty means never mapped"`
	CurrentSHA string `json:"current_sha,omitempty" jsonschema:"the commit the base branch is on now"`
	// Driver is the driver that would map this repo, resolved
	// most-specific-first. Empty means the pass would fall back to an
	// interactive session.
	Driver string `json:"driver,omitempty" jsonschema:"the driver that would build this repo's map; empty means an interactive session"`
	// Error is why this repo couldn't be read at all. Reported per repo
	// rather than failing the call, so one unreachable remote still leaves
	// a usable answer about every other repo.
	Error string `json:"error,omitempty" jsonschema:"why this repo's state couldn't be read; the rest of the plan is still valid"`
}

// Plan is a mapping pass as it would run right now: every repo in
// dependency order, each with its staleness, without mapping anything.
type Plan struct {
	// Order is the repo names in the order a pass would visit them.
	Order []string `json:"order" jsonschema:"the repo names in the order a mapping pass would visit them"`
	// Warning describes a cycle or undeclared dependency that forced the
	// ordering to fall back to declared order. Empty when the order is
	// clean.
	Warning string        `json:"warning,omitempty" jsonschema:"set when a dependency cycle forced the order to fall back to the declared one"`
	Repos   []PlannedRepo `json:"repos"`
}

// Stale reports whether any repo in the plan needs mapping.
func (p Plan) Stale() bool {
	for _, r := range p.Repos {
		if r.Stale {
			return true
		}
	}
	return false
}

func (s server) contextMapStatus(context.Context, *mcp.CallToolRequest, noArgs) (*mcp.CallToolResult, Plan, error) {
	m, err := manifest.Load(s.manifestPath())
	if err != nil {
		return nil, Plan{}, fmt.Errorf("loading %s: %w", s.manifestPath(), err)
	}
	contextFile := s.contextMap.ContextFile
	if contextFile == "" {
		contextFile = contextmap.DefaultContextFile
	}

	// FetchedSHA, not LocalSHA: this tool answers the question `context-map
	// --dry-run` answers, and that one measures staleness against the
	// remote so a clone nobody has fetched can't make a repo look current.
	// The dashboard is the caller that wants the offline reading.
	states, warning := contextmap.Survey(s.root, m, contextFile, contextmap.FetchedSHA(s.progress))

	plan := Plan{
		Warning: warning,
		Order:   make([]string, 0, len(states)),
		Repos:   make([]PlannedRepo, 0, len(states)),
	}
	for _, state := range states {
		repo, _ := m.Find(state.Name)
		planned := PlannedRepo{
			Name:        state.Name,
			Path:        state.Path,
			ContextFile: contextFile,
			BaseBranch:  state.BaseBranch,
			Cloned:      state.Cloned,
			StoredSHA:   state.MappedSHA,
			CurrentSHA:  state.CurrentSHA,
			Driver:      contextmap.SelectDriver(repo.Driver, m.Driver, s.contextMap.Driver),
		}
		// RepoState's fields are conditional on the ones before them, so
		// they're read in that order: an uncloned repo has nothing to
		// assess, and an unreadable one wasn't assessed either.
		switch {
		case !state.Cloned:
			// Mirrors the line a pass prints when it skips a repo, so the
			// two ways of asking say the same thing.
			planned.Reason = "not cloned yet (run bootstrap first)"
		case state.Err != nil:
			planned.Error = state.Err.Error()
		default:
			planned.Stale, planned.Reason = state.Stale, state.Reason
		}
		plan.Order = append(plan.Order, state.Name)
		plan.Repos = append(plan.Repos, planned)
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
