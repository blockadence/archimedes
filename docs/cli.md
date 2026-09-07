# archimedes (CLI)

The tool, installed once per machine and independent of any instance. An
instance is pure data (`repos.yaml`, `repos/*.md`, `work/`, `drivers/`,
`scaffolding/`) — this binary is the only thing that acts on it, so every
instance on a machine is served by one install and none of them carry a
copy of anything. Not one file of an instance is a program: the drivers
ship in here too (see "Two embedded trees" below).

## Build / install

```
go build ./cmd/archimedes            # local binary at ./archimedes
go install ./cmd/archimedes          # installs `archimedes` to $GOBIN
```

From anywhere, with no checkout:

```
go install github.com/blockadence/gh-archimedes/cmd/archimedes@latest
gh extension install blockadence/gh-archimedes
```

Requires `git` and `gh` (authenticated); `sync-templates` additionally
requires `multi-gitter`. Each subcommand checks for what it needs before it
starts.

## The two installs

The tool is installed either as a standalone `archimedes` on `$PATH` or as
a gh extension invoked `gh archimedes`. The second is additive and came
later; what makes it cheap is that it is not a second build, a second entry
point, or a second code path. gh installs an extension by downloading one
binary out of a GitHub release and running it with the subcommand and flags
forwarded verbatim, so the artifact a release carries is the same artifact
`go install` would have produced, and by the time `main` runs the two are
indistinguishable.

Three things had to be true for that, and each is guarded by
`tests/gh_extension_packaging.sh`, which drives the real release build
rather than a copy of its recipe.

**Every subcommand works with no checkout beside it.** An extension install
is a lone binary in `~/.local/share/gh/extensions/`; there is no clone of
this repository anywhere near it. That is not a packaging concern so much
as the thing that had to be settled first — it is why the instance template
and the drivers are carried inside the binary (see "Two embedded trees"
above) rather than copied out of a checkout.

**The name in the help is the name that was typed.** The one thing gh
cannot forward is what it was called: an operator who typed `gh archimedes`
and is answered with `archimedes spawn <slug> <repo>` has been handed a
command they have not got. gh announces itself by setting `GH_EXTENSION=1`
(documented in `gh help environment` for exactly this); `internal/invocation`
is the one place that reads it, and `Name()` is what every operator-facing
string is built from.

`newRootCmd` feeds it to cobra as the display-name annotation, which
rewrites the whole tree's usage lines at once — as an annotation rather
than as `Use`, because the first word of `Use` is the command's *name*, and
a two-word `Use` would make every subcommand a child of something called
`gh`. The prose around those usage lines gets no such sweep, and neither
does runtime output: `init`'s "run this next" line, the dashboard's empty
states, the command a notification says to run about a condition. Those are
ordinary strings and are formatted with `invocation.Name()` one at a time,
which is the part that rots quietly — so a test walks the command tree
under `GH_EXTENSION=1` looking for any `archimedes <subcommand>` that
should have been `gh archimedes <subcommand>`.

Two kinds of mention deliberately stay fixed, and the test is scoped to
`archimedes <subcommand>` so that they can:

- Prose naming the program as a thing rather than as something to type —
  "the drivers archimedes ships", "inside the archimedes binary" — which is
  the same program under either install.
- Anything written into a *file*: the `HOUSE_RULES.md` `sync-house-rules`
  commits into a target repo, the PR templates `sync-templates` pushes, the
  dossier stubs `bootstrap` scaffolds. Keying a committed file's content to
  how the operator who generated it happened to install would put a
  spurious diff in every such repo the first time somebody with the other
  install ran the sync. The MCP tool descriptions stay fixed for the same
  reason: they document which subcommand a tool is equivalent to, to an
  agent that is not going to type either.

**The binary knows which build it is.** See below.

### Version

`--version` has to answer "which build is this?", and a downloaded release
artifact is the case that makes it matter: there is no working tree beside
it to look at instead, so whatever the binary says is the only evidence
there is. `internal/version` assembles that answer from whichever of the
three build routes ran, most authoritative first:

1. **A release build links the tag in**, with a `-ldflags` `-X` against
   `internal/version.stamped`. `.github/release-build.sh` is the only thing
   that passes it. This is the route the criterion is about, and the only
   one available to a downloaded artifact.
2. **Otherwise the module system answers for itself**, and since Go 1.24 it
   answers for more than it used to: `go install <module>@v1.2.3` reports
   that version, and a plain `go build` from a checkout reports a
   pseudo-version synthesized from the commit —
   `v0.0.0-20260907224930-f96985b1261c+dirty`, carrying the sha and a
   `+dirty` marker for a modified tree. It is passed through exactly as
   given: it is built from the same VCS stamps anything here would use, and
   is already the better answer than one reassembled by hand.
3. **Failing both, `dev`.** `(devel)` is the module system saying it has
   nothing to give, and passing *that* through would be reporting a
   placeholder as a version, which is the thing this exists to stop.

(3) is narrower than it looks, and worth knowing exactly because this
project is worked on in worktrees: it means a build whose toolchain
recorded nothing at all — `-buildvcs=false`, or a build made inside a git
worktree, which Go does not recognise as a checkout because it looks for a
`.git` *directory* and a worktree's is a file. In an ordinary checkout you
get (2). Nothing works around the worktree case; neither route that
installs the tool comes through there.

The version is not only for `--version`. `serve-mcp` reports it to the
agent tool on the other end of the protocol, which has even less ability to
go and look.

### Cutting a release

Tag and push; `.github/workflows/release.yml` does the rest.

```
git tag v0.1.0 && git push origin v0.1.0
```

The workflow runs the test suites first (`needs: test`, calling the same
job a push runs) and publishes only if they pass, so a tag on a red tree
produces no release, no assets, and no half-uploaded matrix.

`cli/gh-extension-precompile` builds the platform matrix, creates the
release, and attaches the binaries. A tag containing a `-` (`v0.2.0-rc.1`)
publishes as a prerelease, which `gh extension install` will not hand to
anyone — the safe way to exercise the workflow end to end.

The build itself is ours (`build_script_override`) rather than the action's,
for two reasons the action cannot accommodate: the main package is
`./cmd/archimedes` and not the repository root (the root is the library
package carrying the embedded trees, so the action's default build has
nothing to compile), and the version has to be linked in — the action's
`go_build_options` reaches `go build` as a single word, which cannot carry
both a `-ldflags` value and a package path. Everything else in
`.github/release-build.sh` mirrors the action's own build deliberately:
same platform list, same `go tool dist list` guard, same `-trimpath` and
`-s -w`. Divergence there would be an accident rather than a decision.

Asset names end with `<os>-<arch>`, because matching the tail of an asset
name against the platform it is installing onto is how gh picks the file to
download. A rename that appends anything after that leaves a release that
looks fine on GitHub and installs on nothing.

### Why the repository is named `gh-archimedes`

gh will only install an extension from a repository whose name starts with
`gh-`, and it refuses before it goes anywhere near the network. That is a
namespace rule about the repository, not a statement about the tool: the
binary is `archimedes`, standalone is the primary install, and the
extension is the additive one.

The module path was realigned with it, which is the one thing the rename
breaks: `go install github.com/blockadence/archimedes/cmd/archimedes@latest`
stops resolving. GitHub's rename redirect gets Go to the right repository
but not past the check that follows — the `go.mod` it finds there declares
`gh-archimedes`, and Go refuses a module whose declared path is not the one
requested. Leaving the module path alone would have dodged that at the cost
of a repository whose name and import path disagree forever, and of an
install line that depends on a redirect. Nothing had been tagged or
published under the old path, so the cost was one line in the README.

The alternative was a second, release-only repository, which was weighed
and rejected. It needed a standing cross-repository token (a workflow's
`GITHUB_TOKEN` cannot create a release elsewhere) or a second tag lineage
that could drift from this one — and a version that can drift is the exact
failure the section above exists to prevent. It also meant publishing, to
people gh explicitly tells to review an extension's source before trusting
it, a repository containing no source.

## Two embedded trees

The module root is the repository root, and `template/` and `drivers/` sit
beside it at the top level. That is deliberate: `go:embed` cannot reach
outside the package directory it appears in, so carrying either in the
binary meant either burying hand-maintained prose and bash
(`template/AGENTS.md`, `template/drivers/README.md`, the convention-pack
examples, three drivers' worth of shell) inside the Go package tree, or
moving the module up to meet them. The module moved.

`template.go` at the root is the whole of the mechanism: two `//go:embed
all:` directives, exposed as `archimedes.Template()` and
`archimedes.Drivers()`. Both trees stay ordinary files, edited by editing
them — nothing generates them, and no Go source holds a second copy. The
`all:` prefix is load-bearing; without it the template's dotfiles
(`.gitignore`, and the `.gitkeep` markers that give an instance its
`repos/` and `work/` directories) are silently dropped.

The two are carried for opposite reasons, and the difference is the whole of
issue 29's answer:

- **`template/` is seed data.** It is written into an instance once, by
  `init`, and is that instance's from then on. Nothing refreshes it.
- **`drivers/` is never written into an instance at all.** A driver is read
  out of the binary at the moment it runs. That is what makes a fix to one
  reach an instance that *already exists*: upgrading the tool is the whole
  of the delivery, and there is no step that copies tooling into an
  instance — the thing 15 retired and that this must not reintroduce under
  another name.

So `template/drivers/` holds a README and no drivers. A driver seeded into
an instance would be a driver beyond the reach of any fix, which is exactly
the state that had to be resolved.

The rest of the template — `scaffolding/`'s PR and issue templates, the
convention-pack examples, `AGENTS.md` — stays seed data, deliberately, and
the docs say so where an operator will read it (`template/README.md`).
Those carry an org's own wording and an instance's own conventions, so
re-supplying them would overwrite the work rather than deliver a fix. The
line is not "shipped versus not" but *inert content an operator tailors*
versus *programs that run inside their repositories*, and only the second
kind is worth a delivery route.

The layering is `internal/driver`'s `Set`: the instance's own `drivers/`
over the built-ins, most specific first. An operator's driver — written
from scratch, or taken over with `drivers adopt` — always wins, so owning a
driver means owning it, and the `drivers` listing marks such a name
`shadows built-in` because fixes to the shipped one stop arriving there.
`Set` answers with a *directory on disk* rather than with which layer won,
unpacking a built-in to a temp dir for the duration of the run; past that
point a built-in is an ordinary driver directory and there is one code path,
not two. Unpacking per run rather than caching is deliberate: a cache would
need invalidating on upgrade, and delivering upgrades is the point.

One thing an embedded filesystem cannot carry is file modes, and a driver
depends on exactly one: its command has to be runnable. `internal/driver`
restores it from what each `driver.yaml` declares its `command` to be,
rather than from what a filename looks like — a driver's *sourced* helper
(`spec-kit/repo-snapshot.sh`) is not a program and must stay inert. It is
the only mode anything restores; `internal/instance` needs none, because
nothing in the template is a program.

## Adding a subcommand

Each subcommand lives in its own `internal/cmd/<name>.go`, exposing a
`new<Name>Cmd() *cobra.Command` constructor registered in
`internal/cmd/root.go`'s `newRootCmd`. Business logic belongs in its own
`internal/<package>`, kept independent of cobra/CLI concerns, so it can be
unit-tested directly (see `internal/workspacemap` for the pattern the
`render-map` subcommand follows).

A subcommand that is about to *work inside* one named repo — spawn a
worktree in it, edit its build file, push a commit — asks
`internal/manifest` for it rather than repeating the lookup:
`LoadInstance` reads the instance's `repos.yaml`, and `Manifest.Checkout`
turns a repo name into that repo's entry, its checkout path resolved
against the instance root, and whether anything is cloned there yet
(`CheckoutOf` answers the same for an entry already in hand, for a caller
walking every repo). The two shortfalls stay apart — `Listed` for a name
`repos.yaml` doesn't carry, `Cloned` for one it carries but nobody has
cloned — because commands react differently on purpose: a mapping pass
skips an uncloned repo and carries on where `spawn` refuses, and each
command that refuses words its own message. `Ready()` is for the ones that
treat both the same.

`Manifest.Resolve` is the same lookup without the disk: it answers where a
repo *would* be and nothing about whether it is there. That is what a
reader wants — `status` resolving the repo named on a `status.md` row, the
PR-state lookup behind `prune` and `notify` — since none of them are about
to work in the checkout, and a repo that isn't cloned is a row reporting
"no PR" rather than a command that has to stop.

Anything that shells out to `git` goes through `internal/gitutil` rather
than calling `exec.Command("git", ...)` directly, so "run git and interpret
the result" lives in one place. Helpers that more than one subcommand needs
— deriving a repo's `owner/name` slug from its origin remote, removing a
worktree or branch — belong there too. Tests are exempt: a test that builds
a git fixture drives git directly, so a bug in `gitutil` can't hide itself
by also breaking the fixture.

## Tests

Two suites, two commands, from a checkout:

```
go test ./...            # the Go suite
./tests/run-all.sh       # the bash suite (drivers, packaging, scaffolding)
```

Neither needs the network or a credential: the Go tests build their git
fixtures on disk, and the bash tests build the CLI once and drive it against
fixtures that are their own origins.

`.github/workflows/test.yml` runs exactly these two commands on every push,
and `release.yml` calls that same job as a gate — a tag whose tests fail
publishes nothing. The workflow is deliberately not a third recipe: if the
commands above change, that file changes with them.

A test file that cannot run says so and exits 77, and `run-all.sh` counts it
as skipped and names it in the summary rather than folding it into "0
failed" — the distinction between a suite that passed and a suite that
mostly didn't run. Two files are opt-in that way, because they make a real,
billed `claude -p` call: `tests/pocock-driver-e2e.sh` and
`tests/spec-kit-driver-e2e.sh`, both behind `ARCHIMEDES_TEST_LIVE_DRIVERS=1`
and both run by hand rather than by CI.

```
ARCHIMEDES_TEST_LIVE_DRIVERS=1 ./tests/run-all.sh   # includes the live e2e files
```

The third file that guards itself, `tests/openspec-driver-e2e.sh`, needs
only the `openspec` CLI (`npm install -g @fission-ai/openspec`), which the
workflow installs so that it runs there too — pinned to a version there,
since that install is the one part of a release gate that reaches the
network, and an upstream reword should not be able to hold up a tag.

## Test fixtures

A test that needs a real repository to work against builds one with
`internal/testrepo`: a bare "origin" plus a clone of it carrying one commit,
already pushed to the base branch, which is what makes the fetch, `rev-parse
origin/<base>`, worktree, and push paths exercisable against real git with
no network.

```go
repo := testrepo.New(t, testrepo.Spec{Dir: tmp, Name: "app"})
// repo.Clone, repo.Origin, repo.Branch
```

`Spec` defaults to `<Dir>/<Name>` for the clone, `<Dir>/<Name>.git` for the
origin, `main` for the branch, and a single seeded `README.md`; override any
of them for the cases that need it (a different base branch, a seed clone
that must not sit where the code under test is about to clone, a repo whose
tracked `.gitignore` is the point of the test). It is test-only scaffolding —
a test in the package fails if the shipped binary ever ends up depending on
it, and it drives git directly rather than through `internal/gitutil` under
the same exemption every git fixture has: a bug in `gitutil` must not be
able to hide itself by also breaking the fixture.

A test that drives git a step further than `New` does — committing on top,
pushing a second branch, reading back a ref — uses the same package's runner
rather than wrapping `exec.Command("git", ...)` itself:

```go
testrepo.Git(t, repo.Clone, "checkout", "-q", "-b", "auth-api")   // for effect
testrepo.GitOut(t, repo.Clone, "rev-parse", "origin/main")        // for output
```

Both fail the test with git's own output if the command doesn't succeed.
`GitOut` returns stdout with surrounding whitespace trimmed, since git
terminates nearly everything it prints with a newline no caller wants.

Every fixture the package builds carries a fixed commit identity, so no test
has to spell one out to commit. A test whose subject *builds the repository
itself* has no fixture checkout to carry it, and calls
`testrepo.IsolateGit(t)` instead: git gets a global config of that test's
own holding an identity and nothing else, so the test doesn't pass or fail
on whether the machine running the suite happens to have a global
`user.name` — and, on one that does, doesn't silently record it. Prefer the
fixtures wherever the test owns the repository; reach for this only when it
doesn't.

The bash suite under `tests/` exercises the shipped drivers end to end
against this binary, and builds the same repo shape from
`tests/gitfixture.sh`; keep the two in step.
`tests/driver_ownership.sh` is where the instance/tool split is proved the
only way that means anything: a copy of the binary somewhere else on disk,
with no checkout of this repo in reach.
`tests/gh_extension_packaging.sh` does the same for the release artifact:
it runs `.github/release-build.sh` for the one platform it can execute,
then drives the result both as `archimedes` and as `gh-archimedes` under
`GH_EXTENSION=1` and compares what each scaffolds. Running the real build
script is the point — a test that restated the ldflags recipe would keep
passing after the recipe it copied had changed.

## Subcommands

- `init` — scaffold a new instance from the embedded template
- `bootstrap` — discover an org's repos, clone and scaffold them
- `render-map` — regenerate `WORKSPACE-MAP.md`'s repo list
- `context-map` — sequence a context-mapping pass across every repo
- `run-driver` — invoke one context-mapping driver directly
- `drivers` — list the drivers an instance can run, and where each is from
- `spawn` — create the branch and worktree for one unit of work
- `status` — live PR/branch state across every spawned worktree
- `prune` — remove worktrees whose PR has merged or closed
- `sync-templates` — push the canonical PR/issue templates out
- `sync-house-rules` — push one repo's house rules out
- `apply-convention-pack` — scaffold a repo onto its declared convention
- `notify` — the staleness and prune-eligibility checks above, run on a
  schedule instead of by hand
- `dashboard` — a live view of the above
- `serve-mcp` — the same instance served over the Model Context Protocol

`init` is the one subcommand that runs before an instance exists, so it is
also the only one with no `--root`: it takes the name to create and the
parent directory to create it in. It writes the embedded template there and
commits the result as the instance's first commit — a fresh history, so instance-specific (possibly
sensitive) content never shares one with this repo. A destination that
already exists is refused rather than merged into, and a run that fails
part-way removes what it wrote, so the retry fails for the real reason
instead of "already exists".

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
rather than a stale local checkout, and resolves its start point by
precedence (`--stack-on` over `--base` over the repo's own base branch). It
then materializes the unit of work's reference material plus the target
repo's house rules into the worktree's `.archimedes/`, under the no-commit
guarantee (see `internal/spawn/materialize.go`).

`status` reads every `work/<slug>/status.md`, looks up each row's live PR
state via `gh pr list`, and prints a fixed-width table (or `--json` for a
machine-readable report). A row's PR lookup degrading to "no PR" — a missing
`gh` auth, no network, an unset repo — never fails the rest of the report:
one unanswerable row shouldn't cost you the other nine.

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
commit (`--dry-run` reports that plan without acting on it). It splits two
ways: `internal/contextmap` decides *which* repos need mapping and in what
order, `internal/driver` knows *how* to invoke one driver — so swapping the
configured driver never touches orchestration, and neither half hardcodes
any particular driver. Which driver runs is resolved
most-specific-first: a repo's own `driver` field, then `repos.yaml`'s
top-level one, then `ARCHIMEDES_DRIVER`; with none set, each repo becomes an
interactive session the operator confirms. Recording a repo as mapped goes
through `manifest.SetRepoField`, sharing the node-tree editing described
above so a hand-maintained `repos.yaml` survives the rewrite.

`run-driver` is that second half on its own: one driver, one repo, one
output path, with no pass around it and nothing read from or recorded in
`repos.yaml`. It exists because a driver is the part of an instance most
likely to be written or debugged locally, and stepping through a whole
mapping pass to exercise one is a poor way to do that. It keeps the driver's
own output on stderr so stdout carries only where the map landed.

`run-driver`, a mapping pass, and `drivers` all build their `driver.Set`
through the one `driver.SetFor` — `--root` or `ARCHIMEDES_DRIVERS_DIR` for
the instance layer, `archimedes.Drivers()` underneath — so none of them can
disagree about what a name means. Anything that resolves a driver goes
through there; a second answer to that question would mean a driver that
works in a pass and is missing outside it, or a listing that promises what
a run won't deliver. `ARCHIMEDES_DRIVERS_DIR` moves the *instance* layer,
not the whole lookup: the built-ins stay underneath whatever it names, so
pointing it at a scratch directory to test one driver does not make the
other three vanish.

One manifest reader (`driver.readManifest`) serves both layers, over an
`fs.FS` so the embedded copy and a real directory go through the same
parse. A manifest that loaded in a listing and failed in a run, or the
reverse, would be a disagreement about what a driver is. Listing keeps a
per-driver failure on that driver's entry rather than returning it, because
`drivers` is the command an operator reaches for when something is already
wrong, and one broken manifest must not cost them the report on the rest.

`drivers` is that resolution reported rather than acted on: every name the
three routes above could resolve, and which layer answers it. `drivers
adopt <name>` copies a shipped driver into the instance's own `drivers/`,
which is how a shipped driver gets edited. It is one-way and one-time —
nothing re-syncs an adopted driver, and adopting over one already there is
refused rather than resolved, since that copy may be the edit that was the
reason for adopting.

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
`contextmap.Survey` and `prune.Scan`, the same reads the dashboard and the
MCP server make — so a notification can't reach a different conclusion than
the `context-map` or `prune` run made in response to it. It surveys with
`FetchedSHA` rather than the dashboard's `LocalSHA`: a watch is the one
reader with no human waiting on it, and reading whatever the checkout last
fetched would leave a repo nobody has fetched in weeks looking current —
exactly the silence this exists to break. A merged unit of work something
else is still stacked on isn't reported, because `prune` would refuse to
remove it; it becomes news once the dependent is rebased, which is when
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
one place a *status row's* repo name becomes a checkout path plus a base
branch (over `manifest.Resolve`, the disk-free lookup above), and
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
