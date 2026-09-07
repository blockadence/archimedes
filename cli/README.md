# archimedes (CLI)

Compiled replacement for `template/scripts/*.sh`, installable once, globally,
independent of any instance. An instance becomes pure data (`repos.yaml`,
`repos/*.md`, `work/`) — this binary is the only thing that acts on it.

## Build / install

```
go build ./cmd/archimedes            # local binary at ./archimedes
go install ./cmd/archimedes          # installs `archimedes` to $GOBIN
```

## Adding a subcommand

Each subcommand lives in its own `internal/cmd/<name>.go`, exposing a
`new<Name>Cmd() *cobra.Command` constructor registered in
`internal/cmd/root.go`'s `newRootCmd`. Business logic belongs in its own
`internal/<package>`, kept independent of cobra/CLI concerns, so it can be
unit-tested directly (see `internal/workspacemap` for the pattern the
`render-map` subcommand follows).

Anything that shells out to `git` goes through `internal/gitutil` rather
than calling `exec.Command("git", ...)` directly, so "run git and interpret
the result" lives in one place. Helpers that more than one subcommand needs
— deriving a repo's `owner/name` slug from its origin remote, removing a
worktree or branch — belong there too. Tests are exempt: a test that builds
a git fixture drives git directly, so a bug in `gitutil` can't hide itself
by also breaking the fixture.

## Status

Walking skeleton, growing one subcommand at a time from `template/scripts/*.sh`:

- `bootstrap` — port of `template/scripts/bootstrap.sh`
- `render-map` — port of `template/scripts/render-map.sh`
- `context-map` — port of `template/scripts/context-map-all.sh` (plus
  `run-driver.sh`, as `internal/driver`)
- `spawn` — port of `template/scripts/spawn.sh`
- `status` — port of `template/scripts/status.sh`
- `prune` — port of `template/scripts/prune.sh`
- `sync-templates` — port of `template/scripts/sync-templates.sh`
- `sync-house-rules` — port of `template/scripts/sync-house-rules.sh`
- `notify` — no script counterpart: the staleness and prune-eligibility
  checks above, run on a schedule instead of by hand

`bootstrap` discovers a GitHub org's repos, clones the ones not already
checked out beside the instance, and scaffolds each one's `repos.yaml` entry
and dossier stub before regenerating `WORKSPACE-MAP.md`. Every step is
idempotent, since re-running as the org grows is the normal case: an entry
already listed, a checkout already present, and a dossier already written are
each left exactly as they are, so a run that discovers nothing new leaves the
instance byte-for-byte unchanged.

Scaffolded entries spell out every per-repo field, including the ones nothing
sets yet (`convention_pack`, `driver`, `depends_on`, `context_modeled_sha`) —
declaring a convention pack is filling in a key that's already there rather
than remembering its name. `repos.yaml` is edited as a YAML node tree rather
than re-marshalled, so its comments and any fields the CLI doesn't model
survive the rewrite.

`spawn` creates the branch and worktree for one unit of work in one target
repo. It always fetches first, so a branch starts from current remote state
rather than a stale local checkout, and resolves its start point with the
same precedence the script used (`--stack-on` over `--base` over the repo's
own base branch). It then materializes the unit of work's reference material
plus the target repo's house rules into the worktree's `.archimedes/`, under
the no-commit guarantee (see `internal/spawn/materialize.go`).

`status` reads every `work/<slug>/status.md`, looks up each row's live PR
state via `gh pr list`, and prints the same fixed-width table the shell
script did (or `--json` for a machine-readable report). A row's PR lookup
degrading to "no PR" — a missing `gh` auth, no network, an unset repo — never
fails the rest of the report, matching the original script's `|| echo '{}'`
fallback.

It also flags stacked branches left behind by a squash- or rebase-merged
base — the case where a dependent branch would open a pull request
re-proposing work that has already landed. A row whose note reads `stacked
on <repo>:<slug>` is flagged when three things hold: the base's commits are
still in the branch's history, those commits are *not* on the branch's
`origin/<base branch>`, and the base's pull request has actually merged
(`gh pr list --state merged`, since a rewriting merge leaves nothing git can
recognize).

Each condition earns its place. Testing against the base's own tip rather
than against upstream is what makes the flag stable: it clears when the
branch is rebased and stays clear as `origin/<base branch>` moves on, where
an "is the branch behind upstream?" test would re-fire on every unrelated
merge. The second condition is why a merge-commit or fast-forward base isn't
flagged — its commits are on upstream as themselves, so the dependent's pull
request already shows only the dependent's own work and no rebase is owed.
And without the third, every healthy stack would match.

Both git questions are asked of the *dependent's* checkout, since that's
where `spawn` resolved the start point (`--stack-on` names the base's repo
for bookkeeping, but branches from the base's slug in the repo being spawned
into); only the merged lookup goes to the base's own repo, where its pull
request lives. Anything unanswerable — no `gh`, no network, an upstream ref
nobody has fetched — reports no flag rather than guessing.

`prune` removes worktrees, branches, and status rows for units of work whose
PR has merged or closed. It's a dry run unless `--force` is passed, and it
refuses to remove a branch still acting as another unit of work's stacked
base.

`context-map` sequences a mapping pass across every repo, dependency/base
repos first, skipping any repo already current for its base branch's latest
commit (`--dry-run` reports that plan without acting on it). It splits the
same two ways the scripts did: `internal/contextmap` decides *which* repos
need mapping and in what order, `internal/driver` knows *how* to invoke one
driver — so swapping the configured driver never touches orchestration, and
neither half hardcodes any particular driver. Which driver runs is resolved
most-specific-first: a repo's own `driver` field, then `repos.yaml`'s
top-level one, then `ARCHIMEDES_DRIVER`; with none set, each repo becomes an
interactive session the operator confirms. Recording a repo as mapped goes
through `manifest.SetRepoField`, sharing the node-tree editing described
above so a hand-maintained `repos.yaml` survives the rewrite.

`sync-templates` and `sync-house-rules` (both in `internal/reposync`) push
canonical control-repo content into the target repos as pull requests. The
templates are identical everywhere, so that sync stays a thin wrapper around
`multi-gitter`'s fan-out — the repo list comes straight from `repos.yaml`,
and the per-repo change is a generated mod script `multi-gitter` runs inside
each clone. House rules are specific to one repo, so that sync works
directly on that repo's existing local clone with plain git plus `gh`,
reading the content from the same dossier section `spawn` delivers into a
worktree (`internal/dossier`), so a house rule is still only ever edited in
one place. Both take `--dry-run`.

Neither shells out to anything but git through `internal/gitutil`; every
other external command (`multi-gitter`, `gh`) goes through
`reposync.ExecFunc`, the seam tests replace.

`notify` asks the same two questions on a schedule instead of by hand: has
a repo's context map gone stale, and has a worktree become prune-eligible.
Each condition is reported once, when it becomes true, and again only if it
clears and comes back — a map that goes staler while still unaddressed is
not news twice.

Nothing stays resident to make that work. A pass compares what holds now
against a small state file beside `repos.yaml` and exits, so the thing that
keeps running is an ordinary scheduler:

```
*/15 * * * * cd /path/to/instance && archimedes notify
```

That file is the memory a daemon would otherwise hold in RAM, and it's what
lets a machine that was asleep for a week report each condition once rather
than not at all. A pass with no news prints nothing, so a scheduler that
mails a job's output mails only what's worth reading; `--seed` records
what's true now without reporting any of it, for adopting the notifier on
an instance whose backlog you already know about.

Reasoning from absence is what makes that work, and also what it has to be
careful about: a condition that stops being reported has either cleared or
gone unasked-about, and only the first should let it notify again. So a
pass names the repos whose remote wouldn't answer and the units of work
`gh` wouldn't report on, and carries their recorded conditions forward
untouched. Without that, one expired `gh` session or one flaky network
would erase the record and re-announce the whole backlog on the next pass
that worked — which is how a notifier gets muted. It is also why
`prune.LookupPRState` reports *why* it came back with no pull request:
prune only needs the safe answer ("no PR, don't touch it"), but a watch
needs to know whether anyone actually asked.

Both conditions are read through the packages that own them —
`contextmap.Survey` and `prune.Scan` — so a notification can't reach a
different conclusion than the `context-map` or `prune` run made in response
to it. That's what the `Survey`/`Inspect` split in `internal/contextmap`
buys: `context-map` inspects each repo as it reaches it, since a repo
mapped early is a dependency the next one's session gets primed with, while
a watch surveys them all and acts on none. A merged unit of work something
else is still stacked on isn't reported, because `prune` would refuse to
remove it — it becomes news once the dependent is rebased, which is when
there is something to do about it.

Where a notification goes is the operator's business. With
`ARCHIMEDES_NOTIFY_CMD` (or `--command`) set, each one is handed to
whatever they already run — `terminal-notifier`, `notify-send`, `ntfy`, a
webhook — invoked via `sh` with the event in its environment
(`ARCHIMEDES_EVENT_TITLE`, `_MESSAGE`, `_KIND`, `_SUBJECT`, `_DETAIL`,
`_REMEDY`) and its text on stdin; with none set it is printed. Event data
never reaches the hook as part of the command string, so a repo or branch
name can't become shell on the machine watching it. A hook that fails
leaves its condition out of the state file and fails the pass: the
scheduler learns the notifier is broken, and the condition is still owed
rather than filed away as news broken to someone who never heard it.

### Terminal workspace integration (opt-in)

`spawn` can also hand the finished worktree to a terminal workspace
manager, so a new unit of work arrives in a pane already rooted at its own
checkout instead of needing a manual `cd`. It is off unless you turn it on:

```
export ARCHIMEDES_WORKSPACE=herdr   # instance-wide, in your shell profile
archimedes spawn widget-fix target --workspace herdr   # or just this once
archimedes spawn widget-fix target --workspace off     # ...or not this once
archimedes spawn widget-fix target --focus             # and switch to it
```

[herdr](https://herdr.dev) is the one integration implemented today
(`internal/workspace`). It's invoked as `herdr worktree open`, adopting the
checkout git already made rather than creating a second one, and labelled
`<repo>:<slug>` — the same shape `--stack-on` parses — because one slug can
be spawned into several repos. herdr also opens a workspace for the parent
repo if it doesn't already have one; that's its own worktree model, not
something spawn asks for.

Without `--focus` the new workspace opens in the background, so a spawn
never yanks you out of what you were doing — including a sweep that spawns
one slug across several repos in a row.

The integration is best-effort by construction. By the time it runs, the
branch, the worktree, its materialized context, and its status row all
exist, so nothing that happens here can fail a spawn — the operator would
only be left cleaning up state that was already complete. A tool that isn't
installed is reported as a `note:` on stderr, since that's the expected
state on most machines and says nothing is wrong; a tool that *is*
installed and still refused the call (its server isn't running, say) gets a
`warning:` carrying whatever it said for itself. The one thing that is a
hard error is naming an integration that doesn't exist — a typo fails
loudly, before any git work, rather than silently withholding the pane you
asked for.

Adding another workspace manager means adding a case to
`workspace.Select` and an `Opener` beside `openHerdr`. Everything above
`internal/workspace` — `spawn`, the flags, the warning path — is written
against the `Integration` type, not against herdr.
