package contextmap

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/blockadence/archimedes/cli/internal/driver"
	"github.com/blockadence/archimedes/cli/internal/manifest"
)

// Defaults for the parts of a mapping pass an operator can reconfigure
// without touching orchestration.
const (
	// DefaultContextFile is where a repo's context map lives, relative to
	// that repo's root.
	DefaultContextFile = "CONTEXT.md"
	// DefaultAgentCmd is the agent CLI an interactive session suggests
	// when none is configured.
	DefaultAgentCmd = "claude"
	// DriversDirName is where an instance keeps its drivers.
	DriversDirName = "drivers"
)

// Options is one context-mapping pass.
type Options struct {
	// Root is the instance directory holding repos.yaml and drivers/.
	Root string
	// DryRun reports the planned order and each repo's staleness without
	// invoking any driver, launching any session, or touching repos.yaml.
	DryRun bool
	// ContextFile overrides where each repo's map is written, relative to
	// that repo's root. Empty means DefaultContextFile.
	ContextFile string
	// DriversDir overrides where drivers are looked up. Empty means
	// <Root>/drivers.
	DriversDir string
	// Driver is the environment-level default driver, the least specific
	// of the three levels SelectDriver resolves.
	Driver string
	// AgentCmd is the agent CLI an interactive session suggests. Empty
	// means DefaultAgentCmd.
	AgentCmd string
	// ContextPrompt replaces the first message an interactive session is
	// told to start from. Empty means the generated one.
	ContextPrompt string
}

// DriversDir is where drivers are looked up for an instance rooted at root:
// override when the operator named one (ARCHIMEDES_DRIVERS_DIR), otherwise
// the instance's own drivers/.
//
// Exported because more than a mapping pass asks the question — `run-driver`
// exercises one driver on its own — and the two answering it differently
// would mean a driver that works in a pass and is missing outside it, or the
// reverse.
func DriversDir(root, override string) string {
	if override != "" {
		return override
	}
	return filepath.Join(root, DriversDirName)
}

// Run sequences a context-mapping pass across every repo in the instance at
// opts.Root, dependency/base repos first, skipping any repo whose map is
// already current for its base branch's latest commit. Each repo it does
// map is recorded in repos.yaml as mapped at that commit, so the next pass
// picks up only what has moved since.
//
// out receives the operator-facing account of the pass; progress receives
// git's and the driver's own output, keeping that stream separate from the
// record of what happened. in is where an interactive session's "this one
// is done" confirmation is read from.
func Run(opts Options, out, progress io.Writer, in io.Reader) error {
	root, m, err := manifest.LoadInstance(opts.Root)
	if err != nil {
		return err
	}
	manifestPath := filepath.Join(root, "repos.yaml")

	contextFile := opts.ContextFile
	if contextFile == "" {
		contextFile = DefaultContextFile
	}
	driversDir := DriversDir(root, opts.DriversDir)
	confirm := bufio.NewReader(in)

	order, warning := Order(m.Repos)
	if warning != "" {
		fmt.Fprintln(progress, warning)
	}
	fmt.Fprintf(out, "Planned order: %s\n", strings.Join(order, " "))

	// Staleness is measured against the remote's base branch, not the
	// local checkout, so a stale local clone can't make a repo look
	// current — a pass is about to spend a driver run on the answer.
	sha := FetchedSHA(progress)

	for _, name := range order {
		repo, ok := m.Find(name)
		if !ok {
			continue
		}

		state := State(root, repo, contextFile, sha)
		if !state.Cloned {
			fmt.Fprintf(out, "Skipping %s, not cloned yet (run bootstrap first).\n", name)
			continue
		}
		if state.Err != nil {
			return state.Err
		}

		turn := repoTurn{
			repo:       repo,
			path:       state.Path,
			outputPath: state.ContextPath,
			currentSHA: state.CurrentSHA,
		}

		if !state.Stale {
			fmt.Fprintf(out, "Up to date: %s (@ %s)\n", name, Short(state.CurrentSHA))
			continue
		}

		fmt.Fprintf(out, "\n=== %s (%s) ===\n", name, state.Reason)
		if opts.DryRun {
			continue
		}

		if driverName := SelectDriver(repo.Driver, m.Driver, opts.Driver); driverName != "" {
			fmt.Fprintf(out, "Running driver %q against %s...\n", driverName, turn.path)
			if err := driver.Run(driversDir, driverName, turn.path, turn.outputPath, progress); err != nil {
				return err
			}
			fmt.Fprintf(out, "Wrote %s\n", turn.outputPath)
		} else if err := interactiveSession(opts, m, root, turn, contextFile, out, confirm); err != nil {
			return err
		}

		if err := manifest.SetRepoField(manifestPath, name, manifest.FieldContextModeledSHA, turn.currentSHA); err != nil {
			return fmt.Errorf("recording %s as mapped: %w", name, err)
		}
	}

	fmt.Fprintln(out)
	if opts.DryRun {
		fmt.Fprintln(out, "Dry run: no sessions launched, no repos.yaml changes made.")
	} else {
		fmt.Fprintln(out, "Context-mapping pass complete.")
	}
	return nil
}

// repoTurn is one repo's turn in the pass: everything resolved about it by
// the time it's clear the repo needs mapping.
type repoTurn struct {
	repo manifest.Repo
	// path is the repo's local checkout.
	path string
	// outputPath is where this repo's context map goes.
	outputPath string
	// currentSHA is its base branch's current commit — what the repo is
	// recorded as mapped at once the map is built.
	currentSHA string
}

// interactiveSession is the no-driver fallback: rather than build the map
// itself, print everything the operator needs to run the session by hand —
// where the repo is, which already-mapped dependencies to prime it with,
// and what to open it with — then wait for them to say it's done.
//
// The wait is what makes the pass safe to record: only a human confirming
// the session happened marks the repo as mapped. Nothing to read from means
// nobody confirmed anything, so it stops rather than marking every repo
// mapped on the strength of a session that never ran.
func interactiveSession(opts Options, m *manifest.Manifest, root string, turn repoTurn, contextFile string, out io.Writer, confirm *bufio.Reader) error {
	fmt.Fprintf(out, "Path: %s\n", turn.path)

	if len(turn.repo.DependsOn) > 0 {
		fmt.Fprintln(out, "Depends on (already mapped, prime the session with these):")
		for _, dep := range turn.repo.DependsOn {
			depPath := dep
			// A dependency that isn't in repos.yaml already surfaced
			// as an ordering warning; name it anyway rather than
			// dropping it silently.
			if depRepo, err := m.Resolve(root, dep); err == nil {
				depPath = depRepo.Path
			}
			fmt.Fprintf(out, "  - %s: %s\n", dep, filepath.Join(depPath, contextFile))
		}
	}

	agentCmd := opts.AgentCmd
	if agentCmd == "" {
		agentCmd = DefaultAgentCmd
	}
	prompt := opts.ContextPrompt
	if prompt == "" {
		prompt = SessionPrompt(contextFile, turn.repo.DependsOn)
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "Run:")
	fmt.Fprintf(out, "  cd %s && %s\n", turn.path, agentCmd)
	fmt.Fprintln(out, "First message:")
	fmt.Fprintf(out, "  %s\n", prompt)
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Press enter once that session is done, to record %s as mapped at %s... ", turn.repo.Name, Short(turn.currentSHA))

	if _, err := confirm.ReadString('\n'); err != nil {
		return fmt.Errorf("no confirmation available that %s was mapped: run this interactively, or configure a driver to map unattended", turn.repo.Name)
	}
	return nil
}

// SessionPrompt is the first message an interactive mapping session starts
// from, naming the dependencies already mapped ahead of this repo so the
// session can be primed with them.
func SessionPrompt(contextFile string, deps []string) string {
	prompt := fmt.Sprintf("Build or refresh this repo's %s: describe its purpose, structure, and relationship to its dependencies.", contextFile)
	if len(deps) > 0 {
		prompt += fmt.Sprintf(" Dependencies: %s.", strings.Join(deps, " "))
	}
	return prompt
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
