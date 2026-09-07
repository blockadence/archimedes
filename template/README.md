# (rename this to your instance)

An [Archimedes](https://github.com/blockadence/gh-archimedes) instance.

This directory holds the cross-repo planning layer for actual work: which
repos exist, how they depend on each other, and their branching/release
conventions. It is data only — the `archimedes` binary, installed once on
this machine, is what acts on it, and nothing here needs updating when that
tool does.

Everything in here is yours, including the parts it started life with. The
PR and issue templates in `scaffolding/`, the convention-pack examples in
`convention-packs/`, this README and `AGENTS.md` were seeded once, when the
instance was created, and are never refreshed or overwritten from anywhere:
they carry your org's wording and your repos' conventions, so a tool that
kept re-supplying them would be overwriting the work. Edit them freely.
Upgrading `archimedes` will not bring newer versions of them, and that is
the trade — starting points you own beat boilerplate you cannot keep.

The one exception is `drivers/`, which is not inert data but programs that
run inside your repositories, and where a fix reaching you matters more than
a starting point staying put. See `drivers/README.md`.

See the Archimedes README for how the pattern itself works, and
`archimedes --help` for the full command list. Everything below this line is
specific to this instance.

## Setup

1. `archimedes bootstrap <github-org>` — discover and clone repos, scaffold
   `repos.yaml` and `repos/*.md` dossiers.
2. Fill in each `repos/*.md` dossier's branching/release sections by hand,
   including `## House rules` — mandated decisions for that repo. Editing it
   here is the only place a house rule needs to change: `archimedes spawn`
   injects an ephemeral copy into every worktree automatically, and
   `archimedes sync-house-rules <repo>` pushes a durable, committed copy
   into the repo itself.
3. `archimedes context-map --dry-run` to see the mapping order and which
   repos are stale, then without `--dry-run` to work through it, repo by
   repo. Safe to re-run any time — repos already current for their base
   branch's latest commit are skipped.
4. `archimedes render-map` after any `repos.yaml` change.
5. `archimedes sync-templates --dry-run` to preview, then without
   `--dry-run` to open a PR in each tracked repo introducing/updating the
   canonical PR/issue templates from `scaffolding/`.
