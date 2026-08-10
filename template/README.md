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
2. Fill in each `repos/*.md` dossier's branching/release sections by hand.
3. `scripts/domain-model-all.sh --dry-run` to see the modeling order, then
   without `--dry-run` to work through it, repo by repo.
4. `scripts/render-map.sh` after any `repos.yaml` change.
