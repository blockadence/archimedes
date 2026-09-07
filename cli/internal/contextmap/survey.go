package contextmap

import (
	"io"
)

// PlannedRepo is one repo's place in a planned mapping pass: where it sits
// in the order, whether its map is still good for its base branch's current
// commit, and which driver would build it if it isn't.
type PlannedRepo struct {
	Name string `json:"name"`
	// Path is the repo's local checkout, resolved against the instance root.
	Path string `json:"path"`
	// ContextFile is where this repo's map lives, relative to Path.
	ContextFile string `json:"context_file"`
	// Cloned is false for a repo listed in repos.yaml whose checkout isn't
	// on disk yet. Nothing below it has been read in that case, so its
	// staleness is unknown rather than false.
	Cloned bool `json:"cloned"`
	// Stale is true when the repo needs mapping; Reason says why, or, for
	// a repo that isn't cloned, why it couldn't be assessed at all.
	Stale  bool   `json:"stale"`
	Reason string `json:"reason,omitempty"`
	// StoredSHA is the commit repos.yaml records the map as built against;
	// CurrentSHA is the commit its base branch is actually on now.
	StoredSHA  string `json:"stored_sha,omitempty"`
	CurrentSHA string `json:"current_sha,omitempty"`
	// Driver is the driver that would map this repo, resolved
	// most-specific-first. Empty means the pass would fall back to an
	// interactive session.
	Driver string `json:"driver,omitempty"`
}

// Plan is a mapping pass as it would run right now: every repo in
// dependency order, each with its staleness, without mapping anything.
type Plan struct {
	// Order is the repo names in the order a pass would visit them.
	Order []string `json:"order"`
	// Warning describes a cycle or undeclared dependency that forced the
	// ordering to fall back to declared order. Empty when the order is
	// clean.
	Warning string        `json:"warning,omitempty"`
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

// Survey reports what a mapping pass would do without doing any of it: the
// dependency order, and each repo's staleness against its base branch's
// current commit. It is the structured form of what `context-map --dry-run`
// prints, computed by the same code the pass itself uses, so the two can't
// disagree about whether a repo needs mapping.
//
// Like a dry run it does fetch — staleness is measured against the remote's
// base branch, so an unfetched clone can't make a repo look current — but it
// invokes no driver, launches no session, and never touches repos.yaml.
//
// progress receives git's own output.
func Survey(opts Options, progress io.Writer) (Plan, error) {
	p, err := resolve(opts)
	if err != nil {
		return Plan{}, err
	}

	order, warning := Order(p.manifest.Repos)
	plan := Plan{Order: order, Warning: warning, Repos: make([]PlannedRepo, 0, len(order))}

	for _, name := range order {
		repo, ok := p.manifest.Find(name)
		if !ok {
			continue
		}
		turn, err := p.assess(repo, progress)
		if err != nil {
			return Plan{}, err
		}

		planned := PlannedRepo{
			Name:        name,
			Path:        turn.path,
			ContextFile: p.contextFile,
			Cloned:      turn.cloned,
			StoredSHA:   repo.ContextModeledSHA,
			CurrentSHA:  turn.currentSHA,
			Driver:      SelectDriver(repo.Driver, p.manifest.Driver, opts.Driver),
		}
		if turn.cloned {
			planned.Stale, planned.Reason = turn.assessment.Stale, turn.assessment.Reason
		} else {
			// Mirrors the line a pass prints when it skips a repo, so the
			// two ways of asking say the same thing.
			planned.Reason = "not cloned yet (run bootstrap first)"
		}
		plan.Repos = append(plan.Repos, planned)
	}

	return plan, nil
}
