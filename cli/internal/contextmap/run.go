package contextmap

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/blockadence/archimedes/cli/internal/driver"
	"github.com/blockadence/archimedes/cli/internal/gitutil"
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
	p, err := resolve(opts)
	if err != nil {
		return err
	}
	confirm := bufio.NewReader(in)

	order, warning := Order(p.manifest.Repos)
	if warning != "" {
		fmt.Fprintln(progress, warning)
	}
	fmt.Fprintf(out, "Planned order: %s\n", strings.Join(order, " "))

	for _, name := range order {
		repo, ok := p.manifest.Find(name)
		if !ok {
			continue
		}
		turn, err := p.assess(repo, progress)
		if err != nil {
			return err
		}
		if !turn.cloned {
			fmt.Fprintf(out, "Skipping %s, not cloned yet (run bootstrap first).\n", name)
			continue
		}
		if !turn.assessment.Stale {
			fmt.Fprintf(out, "Up to date: %s (@ %s)\n", name, Short(turn.currentSHA))
			continue
		}

		fmt.Fprintf(out, "\n=== %s (%s) ===\n", name, turn.assessment.Reason)
		if opts.DryRun {
			continue
		}

		if driverName := SelectDriver(repo.Driver, p.manifest.Driver, opts.Driver); driverName != "" {
			fmt.Fprintf(out, "Running driver %q against %s...\n", driverName, turn.path)
			if err := driver.Run(p.driversDir, driverName, turn.path, turn.outputPath, progress); err != nil {
				return err
			}
			fmt.Fprintf(out, "Wrote %s\n", turn.outputPath)
		} else if err := p.interactiveSession(opts, turn, out, confirm); err != nil {
			return err
		}

		if err := manifest.SetRepoField(p.manifestPath, name, manifest.FieldContextModeledSHA, turn.currentSHA); err != nil {
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
// the time it's clear whether the repo needs mapping.
type repoTurn struct {
	repo manifest.Repo
	// path is the repo's local checkout.
	path string
	// outputPath is where this repo's context map goes.
	outputPath string
	// cloned is false for a repo listed in repos.yaml whose checkout isn't
	// on disk yet; nothing below it has been read in that case.
	cloned bool
	// currentSHA is its base branch's current commit — what the repo is
	// recorded as mapped at once the map is built.
	currentSHA string
	// assessment is whether the repo's map is still good for currentSHA.
	assessment Assessment
}

// pass is one mapping pass's resolved settings — everything Options leaves
// optional, decided once — plus the manifest the pass reads repos from.
// Both a real pass and a survey of one are driven from it, so the two can't
// drift on where a repo lives, what counts as its map, or when that map is
// stale.
type pass struct {
	root         string
	manifestPath string
	manifest     *manifest.Manifest
	contextFile  string
	driversDir   string
}

func resolve(opts Options) (pass, error) {
	// Absolutized up front: repo paths are resolved against the instance
	// root, but git and drivers run with a working directory of their own,
	// so a relative root would resolve against the wrong thing.
	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return pass{}, fmt.Errorf("resolving instance root %s: %w", opts.Root, err)
	}
	manifestPath := filepath.Join(root, "repos.yaml")
	m, err := manifest.Load(manifestPath)
	if err != nil {
		return pass{}, fmt.Errorf("loading %s: %w", manifestPath, err)
	}

	p := pass{
		root:         root,
		manifestPath: manifestPath,
		manifest:     m,
		contextFile:  opts.ContextFile,
		driversDir:   opts.DriversDir,
	}
	if p.contextFile == "" {
		p.contextFile = DefaultContextFile
	}
	if p.driversDir == "" {
		p.driversDir = filepath.Join(root, DriversDirName)
	}
	return p, nil
}

// assess resolves one repo's turn: where its checkout and map live, its
// base branch's current commit, and whether that map is still good for it.
// A repo that isn't cloned yet comes back with cloned false and nothing
// else read, since there's nothing on disk to read it from.
func (p pass) assess(repo manifest.Repo, progress io.Writer) (repoTurn, error) {
	turn := repoTurn{repo: repo, path: filepath.Join(p.root, repo.Path)}
	turn.outputPath = filepath.Join(turn.path, p.contextFile)

	if info, err := os.Stat(turn.path); err != nil || !info.IsDir() {
		return turn, nil
	}
	turn.cloned = true

	// Staleness is measured against the remote's base branch, not the
	// local checkout, so a stale local clone can't make a repo look
	// current.
	if err := gitutil.RunOut(turn.path, progress, "fetch", "origin", repo.BaseBranch, "-q"); err != nil {
		return turn, err
	}
	currentSHA, err := gitutil.Run(turn.path, "rev-parse", "origin/"+repo.BaseBranch)
	if err != nil {
		return turn, err
	}
	turn.currentSHA = currentSHA
	turn.assessment = Assess(repo.ContextModeledSHA, currentSHA, p.contextFile, isFile(turn.outputPath))

	return turn, nil
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
func (p pass) interactiveSession(opts Options, turn repoTurn, out io.Writer, confirm *bufio.Reader) error {
	fmt.Fprintf(out, "Path: %s\n", turn.path)

	if len(turn.repo.DependsOn) > 0 {
		fmt.Fprintln(out, "Depends on (already mapped, prime the session with these):")
		for _, dep := range turn.repo.DependsOn {
			depPath := dep
			// A dependency that isn't in repos.yaml already surfaced
			// as an ordering warning; name it anyway rather than
			// dropping it silently.
			if depRepo, err := p.manifest.Resolve(p.root, dep); err == nil {
				depPath = depRepo.Path
			}
			fmt.Fprintf(out, "  - %s: %s\n", dep, filepath.Join(depPath, p.contextFile))
		}
	}

	agentCmd := opts.AgentCmd
	if agentCmd == "" {
		agentCmd = DefaultAgentCmd
	}
	prompt := opts.ContextPrompt
	if prompt == "" {
		prompt = SessionPrompt(p.contextFile, turn.repo.DependsOn)
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
