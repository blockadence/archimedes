# Archimedes

A "control repo" / repo-of-repos pattern for planning and executing agentic
coding work that spans multiple independent git repositories.

## The problem

Once you're working across more than one or two related repos (a client SDK
and the CLI that depends on it, a core library and its several consumers),
two things get easy to lose track of:

- **Which repos does this specific unit of work actually touch?** A change
  that looks self-contained in one repo can have a silent dependent that
  needs a follow-up PR (a library release that a downstream CLI needs to
  pick up, for example).
- **Managing several of these in flight at once.** Git worktrees are the
  right primitive for parallel branches, but they're scoped to one repo at a
  time. Nothing tracks which worktrees exist across a whole family of repos,
  what their PR status is, or which ones are safe to prune.

## The pattern

A generated instance of this template becomes a sibling directory next to
your actual repo checkouts:

```
your-workspace/
├── your-instance/          # generated from this template
│   ├── WORKSPACE-MAP.md    # which repos exist, how they relate
│   ├── repos.yaml           # machine-readable source of truth
│   ├── repos/*.md           # per-repo dossier: branching, release procedure
│   ├── work/<slug>/          # one folder per active unit of work
│   └── scripts/
├── service-a/
├── service-b/
└── ...
```

The instance never contains the repos themselves (no submodules, no vendored
copies) — it references sibling checkouts by relative path. It also never
duplicates a repo's own domain-model/glossary docs if you keep those (e.g. a
`CONTEXT.md`), it only links to them. What it owns is the layer that has no
single-repo home: cross-repo relationships, and orchestration of worktrees
across repos for one logical change.

### Plan here, execute there

Root a planning/scoping session (a `wayfinder`/`grill-with-docs`-style pass,
or your own equivalent) inside `work/<slug>/` to answer "which repos does
this touch," using `WORKSPACE-MAP.md` and the repo dossiers for context.
Once that's answered, hand off actual implementation to a worktree spawned
in each target repo, so that session's context isn't cluttered with every
other repo in the workspace.

### Stacked work within one repo

When a unit of work needs two dependent PRs in the same repo (the second
can't land until the first merges), `spawn.sh` supports branching a new
worktree off another in-flight worktree's branch instead of off trunk — the
"stacked branches" pattern. The stack relationship is tracked explicitly, so
tooling can prompt a rebase once the base PR merges instead of assuming the
dependent branch is still current.

## Getting started

```
./scripts/init.sh <instance-name> <parent-dir-for-your-repos>
cd <parent-dir-for-your-repos>/<instance-name>
scripts/bootstrap.sh <github-org>       # discovers + clones repos via `gh`
scripts/domain-model-all.sh --dry-run   # see the planned modeling order
```

Requires `git`, `gh` (authenticated), `yq` (v4), `jq`.

## Scripts (in `template/scripts/`, vendored into each instance)

- `bootstrap.sh` — discover org repos via `gh repo list`, clone what's
  missing, scaffold `repos.yaml` and per-repo dossier stubs.
- `render-map.sh` — regenerate `WORKSPACE-MAP.md`'s repo list from
  `repos.yaml` (the `## Relationships` section stays hand-written).
- `domain-model-all.sh` — sequence a domain-modeling pass across every repo,
  dependency/base repos first, priming each session with already-modeled
  dependencies. Orchestration only — the modeling itself is still an
  interactive, human-in-the-loop session per repo.
- `spawn.sh <slug> <repo> [--base <branch>|--stack-on <repo>:<slug>]` —
  fetch-first worktree creation for one unit of work in one repo.
- `status.sh [<slug>]` — live PR/branch status across every spawned
  worktree, with a warning past a configurable concurrent-stream cap.
- `prune.sh [<slug>] [--force]` — list (or, with `--force`, remove)
  worktrees/branches whose PR has merged or closed. Refuses to remove a
  branch still acting as another worktree's stack base.
- `update-from-archimedes.sh <path-to-this-repo>` — re-vendor `scripts/`
  into an existing instance.

## Status

Personal tool. Unfinished edges are called out as TODOs rather than papered
over — notably, stacked-branch rebase detection in `status.sh` isn't
implemented yet. Use at your own judgment.

## License

MIT, see [LICENSE](./LICENSE).
