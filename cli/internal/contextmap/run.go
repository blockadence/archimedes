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
	driversDir := opts.DriversDir
	if driversDir == "" {
		driversDir = filepath.Join(root, DriversDirName)
	}
	confirm := bufio.NewReader(in)

	order, warning := Order(m.Repos)
	if warning != "" {
		fmt.Fprintln(progress, warning)
	}
	fmt.Fprintf(out, "Planned order: %s\n", strings.Join(order, " "))

	for _, name := range order {
		repo, ok := m.Find(name)
		if !ok {
			continue
		}
		// A pass acts on each repo as it reaches it, so it inspects them
		// one at a time rather than surveying up front: a repo mapped
		// early is a dependency the next one's session should be primed
		// with, and its own state is read after that happened.
		turn := Inspect(root, contextFile, repo, progress)
		// Unlike a survey, a pass can't work around either gap: there is
		// nothing to hand a driver, and recording a repo mapped against a
		// commit git wouldn't name is worse than stopping.
		if turn.Err != nil {
			return turn.Err
		}
		if !turn.Cloned() {
			fmt.Fprintf(out, "Skipping %s, not cloned yet (run bootstrap first).\n", name)
			continue
		}

		if !turn.Stale {
			fmt.Fprintf(out, "Up to date: %s (@ %s)\n", name, Short(turn.CurrentSHA))
			continue
		}

		fmt.Fprintf(out, "\n=== %s (%s) ===\n", name, turn.Reason)
		if opts.DryRun {
			continue
		}

		if driverName := SelectDriver(repo.Driver, m.Driver, opts.Driver); driverName != "" {
			fmt.Fprintf(out, "Running driver %q against %s...\n", driverName, turn.Path)
			if err := driver.Run(driversDir, driverName, turn.Path, turn.OutputPath, progress); err != nil {
				return err
			}
			fmt.Fprintf(out, "Wrote %s\n", turn.OutputPath)
		} else if err := interactiveSession(opts, m, root, turn, contextFile, out, confirm); err != nil {
			return err
		}

		if err := manifest.SetRepoField(manifestPath, name, manifest.FieldContextModeledSHA, turn.CurrentSHA); err != nil {
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

// interactiveSession is the no-driver fallback: rather than build the map
// itself, print everything the operator needs to run the session by hand —
// where the repo is, which already-mapped dependencies to prime it with,
// and what to open it with — then wait for them to say it's done.
//
// The wait is what makes the pass safe to record: only a human confirming
// the session happened marks the repo as mapped. Nothing to read from means
// nobody confirmed anything, so it stops rather than marking every repo
// mapped on the strength of a session that never ran.
func interactiveSession(opts Options, m *manifest.Manifest, root string, turn RepoState, contextFile string, out io.Writer, confirm *bufio.Reader) error {
	fmt.Fprintf(out, "Path: %s\n", turn.Path)

	if len(turn.Repo.DependsOn) > 0 {
		fmt.Fprintln(out, "Depends on (already mapped, prime the session with these):")
		for _, dep := range turn.Repo.DependsOn {
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
		prompt = SessionPrompt(contextFile, turn.Repo.DependsOn)
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "Run:")
	fmt.Fprintf(out, "  cd %s && %s\n", turn.Path, agentCmd)
	fmt.Fprintln(out, "First message:")
	fmt.Fprintf(out, "  %s\n", prompt)
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Press enter once that session is done, to record %s as mapped at %s... ", turn.Repo.Name, Short(turn.CurrentSHA))

	if _, err := confirm.ReadString('\n'); err != nil {
		return fmt.Errorf("no confirmation available that %s was mapped: run this interactively, or configure a driver to map unattended", turn.Repo.Name)
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
