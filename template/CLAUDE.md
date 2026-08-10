# Operating rules for this Archimedes instance

When a session is rooted here, your job is cross-repo planning and worktree
lifecycle orchestration, not implementation.

- Root planning sessions (impact-mapping, "which repos does this touch")
  under `work/<slug>/`, using `WORKSPACE-MAP.md` and `repos/*.md` for
  context.
- Hand off actual code changes to a spawned worktree in the target repo
  (`scripts/spawn.sh <slug> <repo>`), so that session isn't cluttered with
  every other repo's context.
- Never duplicate a target repo's own `CONTEXT.md`/`CONTEXT-MAP.md` content
  here, link to it instead. This instance only owns the cross-repo
  relationship layer that has no single-repo home.
- Keep a cap on concurrent worktree streams matched to actual review
  bandwidth. `scripts/status.sh` warns past a configurable threshold
  (`ARCHIMEDES_MAX_STREAMS`, default 3).
- Destructive operations (`scripts/prune.sh`) default to a dry run; only
  `--force` deletes anything.
