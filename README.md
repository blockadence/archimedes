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
│   ├── repos/*.md           # per-repo dossier: branching, release procedure,
│   │                        # house rules, known gotchas
│   ├── work/<slug>/          # one folder per active unit of work
│   ├── drivers/              # pluggable context-mapping drivers
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

### House rules

A dossier's `## House rules` section (distinct from `## Known gotchas`) is
for mandated decisions that must be respected even if unusual — the kind of
thing good judgment alone would get wrong. It's the one place they're
edited, and it's delivered two ways from there: `sync-house-rules.sh` pushes
a durable, committed `HOUSE_RULES.md` into the target repo (so humans
browsing it on GitHub see it too), and `spawn.sh` injects an ephemeral copy
into every worktree it spawns for that repo, automatically.

## Getting started

```
./scripts/init.sh <instance-name> <parent-dir-for-your-repos>
cd <parent-dir-for-your-repos>/<instance-name>
scripts/bootstrap.sh <github-org>       # discovers + clones repos via `gh`
scripts/context-map-all.sh --dry-run    # see the planned + stale/fresh order
```

Requires `git`, `gh` (authenticated), `yq` (v4), `jq`. `sync-templates.sh`
additionally requires [`multi-gitter`](https://github.com/lindell/multi-gitter).

## Compiled CLI

A `archimedes` binary is being built at [`cli/`](./cli) to replace the
vendored bash scripts below with one globally-installed tool (see
[`cli/README.md`](./cli/README.md)). It currently has one subcommand,
`render-map`, ported from `render-map.sh`; the rest follow the same
pattern one at a time.

## Scripts (in `template/scripts/`, vendored into each instance)

- `bootstrap.sh` — discover org repos via `gh repo list`, clone what's
  missing, scaffold `repos.yaml` and per-repo dossier stubs.
- `render-map.sh` — regenerate `WORKSPACE-MAP.md`'s repo list from
  `repos.yaml` (the `## Relationships` section stays hand-written).
- `context-map-all.sh` — sequence a context-mapping pass across every repo,
  dependency/base repos first, priming each session with already-mapped
  dependencies and skipping any repo whose map is already current for its
  base branch's latest commit (tracked via `context_modeled_sha` in
  `repos.yaml`), so re-runs after new repos or merges are incremental.
  Orchestration only — it doesn't assume any particular coding agent, skill,
  or driver. By default the mapping itself is an interactive, human-in-the-
  loop session per repo (`ARCHIMEDES_AGENT_CMD`/`ARCHIMEDES_CONTEXT_PROMPT`/
  `ARCHIMEDES_CONTEXT_FILE` override the defaults); set `ARCHIMEDES_DRIVER`
  to a name under `drivers/` to build the map unattended instead, via
  `run-driver.sh` (see `drivers/README.md`).
- `run-driver.sh <driver-name> <repo-path> <output-path>` — invoke one
  driver's context-mapping contract directly. A driver declares, in its
  `driver.yaml` manifest, whether it accepts an explicit output path
  (`output_mode: path-parameterized`) or always writes into whatever repo
  it's run in (`output_mode: fixed-location`, harvested afterward so the
  target repo ends up clean). An `openspec` driver wrapping the
  [OpenSpec CLI](https://github.com/Fission-AI/OpenSpec) and a `pocock`
  driver wrapping Matt Pocock's `domain-modeling` skill ship as working
  examples of each mode.
- `spawn.sh <slug> <repo> [--base <branch>|--stack-on <repo>:<slug>]` —
  fetch-first worktree creation for one unit of work in one repo. Also
  materializes `work/<slug>/`'s contents (a ticket, a spec, whatever
  reference material the planning session left behind), plus the target
  repo's house rules (see below), into the new worktree at `.archimedes/`,
  and makes sure that directory can never show up in `git status`/`git add
  -A` or get committed there — no `.gitignore` edit needed in the target
  repo, and removing the worktree removes the copy with it.
- `status.sh [<slug>]` — live PR/branch status across every spawned
  worktree, with a warning past a configurable concurrent-stream cap.
- `prune.sh [<slug>] [--force]` — list (or, with `--force`, remove)
  worktrees/branches whose PR has merged or closed. Refuses to remove a
  branch still acting as another worktree's stack base.
- `sync-templates.sh [--dry-run] [<repo-name>]` — push the canonical PR/issue
  templates (`scaffolding/`) into every tracked repo's `.github/` as a pull
  request, via `multi-gitter`. `--dry-run` shows which repos would receive
  changes without pushing or opening anything; an optional repo name limits
  the run to one repo. Requires `multi-gitter` and `gh auth login`.
- `sync-house-rules.sh <repo> [--dry-run]` — push one repo's house rules
  (the `## House rules` section of its dossier, `repos/<repo>.md`) into that
  repo as a durably committed `HOUSE_RULES.md`, via a pull request. Content
  is per-repo rather than identical across every tracked repo, so — unlike
  `sync-templates.sh` — this isn't a `multi-gitter` fan-out; it opens the PR
  itself via `gh`. `--dry-run` shows the pending diff without committing,
  pushing, or opening anything. A no-op once the target repo's copy already
  matches the dossier.
- `apply-convention-pack.sh <repo>` — one-time scaffold: add whatever
  dependency/plugin reference a repo's declared `convention_pack` (see
  `repos.yaml` and `convention-packs/`) needs to start pulling in its shared
  build/lint/static-analysis config. Idempotent; not an ongoing sync.
- `update-from-archimedes.sh <path-to-this-repo>` — re-vendor `scripts/`,
  `drivers/`, and `scaffolding/` into an existing instance.

## Language-tooling convention packs

A repo can declare, via `convention_pack` in `repos.yaml`, which shared
build-tooling convention it's meant to follow — e.g. a Java repo pointing at
a shared Gradle convention plugin for Checkstyle/Spotless/JaCoCo. The pack
itself (language, build tool, and the shared artifact + version that carries
the config) is defined once in `convention-packs/<pack-name>.yaml` and
referenced by name from any repo that follows it. This is documentation
plus a one-time scaffold, not ongoing config-file distribution — see
`template/convention-packs/README.md` for the shape and how to add a pack
for another language/build tool.

## Status

Personal tool. Unfinished edges are called out as TODOs rather than papered
over — notably, stacked-branch rebase detection in `status.sh` isn't
implemented yet. Use at your own judgment.

## License

MIT, see [LICENSE](./LICENSE).
