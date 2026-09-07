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
- `apply-convention-pack` — port of
  `template/scripts/apply-convention-pack.sh`
- `dashboard` — a live view of the above; no script behind it
- `serve-mcp` — the same instance served over the Model Context Protocol;
  no script behind it either

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

`apply-convention-pack` wires one repo up to the shared build/lint
convention it declares, by adding whatever reference that repo's build tool
needs to start pulling in the pack's published config artifact. Both halves
are instance data — the pack name from the repo's `repos.yaml` entry, the
definition from the instance's `convention-packs/<name>.yaml` — so there is
no config of its own to keep in step with either.

It is one-time scaffolding rather than sync: afterwards the repo owns that
reference like any other dependency, and nothing pushes updates back into
it later. Re-running is a no-op, and the edit is left uncommitted for a
human to review.

`internal/conventionpack` dispatches on the pack's `build_tool`, so adding
a second language/build tool is one entry in its `scaffolders` map plus the
function it names — `gradle.go` is the worked example. Nothing above that
dispatch knows Java or Gradle, down to the pack's build-tool-named block,
which stays undecoded until a scaffolder asks for it in its own shape; so
a new build tool costs a file, not a field on the shared `Pack` type. A build file that already carries a
`buildscript {}` block of its own is refused: the two lines it needs are
printed for a human to place by hand, since where they belong inside an
existing block is a judgment call, not a rewrite worth guessing at.

### Dashboard (optional)

`archimedes dashboard` opens a live view of the whole instance: the worktree
table `status` prints — PR state, stack notes, the rebase-needed list, the
concurrent-stream guardrail — next to the per-repo context-map staleness
`context-map --dry-run` reports. It retakes the reading every 30 seconds
(`--refresh`, or `0` for on-demand only), on `r`, and quits on `q`.

It is a presentation layer and nothing else. `internal/dashboard` collects a
`Snapshot` by calling the same `status.BuildReport` and `contextmap.Assess`
the two subcommands call, renders it, and loops; nothing about what a row
*means* is decided there. So the dashboard can't drift from the CLI, and
every command works exactly as it did before — the dashboard is additive and
nothing depends on it.

That parity is what the shared seams are for. `status.ManifestRepos` is the
one place a repo name becomes a checkout path plus a base branch, and
`contextmap.Survey` is the one place "assess every repo, dependency order
first" lives — `context-map` walks its pass through the same
`contextmap.State` the dashboard reads through.

The one deliberate difference is the network. A mapping pass fetches before
assessing, because it's about to spend a driver run on the answer; a
dashboard refresh reads `origin/<base branch>` as the checkout last saw it,
because a screen that repaints every 30 seconds must not drag the network in
with it. That's the `contextmap.SHALookup` seam — `FetchedSHA` for a pass,
`LocalSHA` for a reading — and it means a repo nobody has fetched lately can
under-report, which is the safe direction: the dashboard stays quiet about a
pass that's due rather than inventing one.

Everything narrower than an unreadable `repos.yaml` is carried in the
snapshot rather than raised: a row whose `gh` lookup failed reads "no PR", a
repo whose base branch couldn't be resolved says so in its own row, and a
refresh that fails outright leaves the last good reading on screen under a
visible error. A dashboard that blanks itself over one unreachable repo
would be worse than one showing that repo as unknown.

`Collect` and `Render` are ordinary functions over data — no terminal, no
clock — and `Model` takes its clock by injection, so the whole thing is
tested by driving messages through `Update` and asserting on frames. Only
`internal/cmd/dashboard.go` touches a terminal; without one (a pipe, a CI
log) it refuses and points at `archimedes status`, which answers the same
question in a form a pipe can hold.
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

### MCP server

`serve-mcp` serves one instance over the Model Context Protocol, so an
MCP-capable agent tool can query and act on repo/worktree state as
structured tool calls instead of shelling out to this CLI and parsing its
tables:

```
archimedes serve-mcp --root /path/to/instance
```

It speaks over stdin/stdout and runs until the client disconnects, so it is
started by the agent tool rather than by hand. Everything else — git's own
output, any warning — goes to stderr, because stdout carries the protocol
itself.

Four tools, matching the subcommands an agent would otherwise have had to
run:

| Tool | Reports | Equivalent |
| --- | --- | --- |
| `list_repos` | every tracked repo, its checkout, base branch, dependencies and context-map bookkeeping | `repos.yaml` itself |
| `repo_status` | every spawned worktree, its live PR state, the rebase flag and the guardrail verdict | `status --json` |
| `context_map_status` | which repos' maps are stale, and the dependency order a pass would rebuild them in | `context-map --dry-run` |
| `spawn_worktree` | the branch and worktree it created for one unit of work | `spawn` |

The first three are annotated read-only; `spawn_worktree` is the one that
writes, and both its description and the server's instructions say so.

It is a second way in, not a second implementation. Each tool is a thin
mapping from a tool call onto the same `internal/` package the subcommand
calls — `status.Collect`, `contextmap.Survey`, `spawn.Run` — so the two
paths can't drift on what the instance currently looks like. That's what
`internal/cmd/servemcp_test.go` pins: it asks the same instance the same
question both ways and compares the answers, so a change that only moves
one of them fails there.

The staleness tool shares `contextmap.Survey` with the dashboard, and the
`SHALookup` seam is what lets one primitive serve both: it passes
`FetchedSHA`, because it answers the question `context-map --dry-run`
answers and that one measures staleness against the remote, where the
dashboard passes `LocalSHA` rather than drag the network into a screen
refresh. `mcpserver.Plan` is the wire projection of the `[]RepoState` that
comes back — JSON tags and schema descriptions belong to the protocol
boundary, not to `contextmap`.

Four things the CLI does that a tool call deliberately doesn't. No
interactive mapping session is offered, which is why the context-map tool
surveys staleness rather than running a pass. No terminal workspace is
opened: a pane appearing on the operator's machine is something they ask
for at their own prompt, not a side effect of an agent's tool call. A
spawn's `cd <worktree> && <agent>` next-step hint is dropped, since its
whole content is already in the result and it is addressed to a person who
isn't there; git's own output still reaches the log.

And a repo whose state can't be read — an unreachable remote, a base branch
that isn't there — is reported as an `error` on that repo rather than
failing the call, where a pass stops at the first one. That is the one
place a tool answers differently from its command, and deliberately: a pass
is about to spend a driver run and can't proceed on an unknown, while a
reader asking "what needs mapping?" is still better off with the answer for
every other repo than with nothing. It is `RepoState.Err`'s documented
contract, and the dashboard reads it the same way.

The server is bound to one instance by `--root` for its lifetime, resolved to
an absolute path when the server is built — a client won't share the working
directory the server was started from, so every path a tool reports is one it
can open. No tool takes a path to another instance.

It reads the same environment as the subcommands (`ARCHIMEDES_MAX_STREAMS`,
`ARCHIMEDES_DRIVER`, `ARCHIMEDES_DRIVERS_DIR`, `ARCHIMEDES_CONTEXT_FILE`), and
carries each setting in the form the environment holds it so the *same*
parser decides what it means — `ARCHIMEDES_MAX_STREAMS=0` is a guardrail of
zero to a tool call exactly as it is to `archimedes status`, not an unset
field falling back to the default. It requires `git` but not `gh`: a PR
lookup degrades to "no PR" rather than failing, so demanding `gh` would
refuse to start a server on a machine where `archimedes status` itself works.

It is built on the official
[Go MCP SDK](https://github.com/modelcontextprotocol/go-sdk); tool schemas
are inferred from the Go argument and result types in
`internal/mcpserver/tools.go`, so a field gains a schema entry by being
declared, not by being described twice.
