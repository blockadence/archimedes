# (rename this to your instance)

An [Archimedes](https://github.com/blockadence/archimedes) instance.

This directory holds the cross-repo planning layer for actual work: which
repos exist, how they depend on each other, their branching/release
conventions, and scripts to spawn/track/prune worktrees across them for a
given unit of work.

See the Archimedes README for how the pattern itself works. Everything
below this line is specific to this instance.

## Setup

1. `scripts/bootstrap.sh <github-org>` — discover and clone repos, scaffold
   `repos.yaml` and `repos/*.md` dossiers.
2. Fill in each `repos/*.md` dossier's branching/release sections by hand,
   including `## House rules` — mandated decisions for that repo. Editing it
   here is the only place a house rule needs to change: `spawn.sh` injects
   an ephemeral copy into every worktree automatically, and
   `scripts/sync-house-rules.sh <repo>` pushes a durable, committed copy
   into the repo itself.
3. `scripts/context-map-all.sh --dry-run` to see the mapping order and which
   repos are stale, then without `--dry-run` to work through it, repo by
   repo. Safe to re-run any time — repos already current for their base
   branch's latest commit are skipped.
4. `scripts/render-map.sh` after any `repos.yaml` change.
5. `scripts/sync-templates.sh --dry-run` to preview, then without
   `--dry-run` to open a PR in each tracked repo introducing/updating the
   canonical PR/issue templates from `scaffolding/`.
