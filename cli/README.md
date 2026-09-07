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

- `render-map` — port of `template/scripts/render-map.sh`
- `spawn` — port of `template/scripts/spawn.sh`
- `status` — port of `template/scripts/status.sh`
- `prune` — port of `template/scripts/prune.sh`

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
