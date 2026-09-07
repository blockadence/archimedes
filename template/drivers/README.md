# Drivers

A driver is whatever actually produces a repo's context map — an AI coding
agent, a wrapped third-party CLI, a script. `scripts/context-map-all.sh`
orchestrates *which* repos need mapping and in what order; it never knows how
any given driver does its job. That split is the point: swapping the
configured driver never requires touching orchestration.

## Selecting a driver

`repos.yaml`'s top-level `driver` field sets the instance-wide default,
used for any repo that doesn't set one of its own:

```yaml
driver: openspec
repos:
  - name: some-repo
    ...
```

A single repo can override that default for itself alone by setting its own
`driver` field — this takes precedence over the instance-wide default when
both are present:

```yaml
repos:
  - name: some-repo
    driver: openspec       # this repo always uses openspec, regardless of the default above
  - name: other-repo
    driver: null            # falls back to the instance-wide default (or interactive, if unset)
```

If neither is set, `ARCHIMEDES_DRIVER=<name>` (checked when `repos.yaml` has
no top-level `driver`) is a per-invocation way to set the same instance-wide
default without editing the file. Leaving every level unset falls back to an
interactive, human-in-the-loop session.

Naming a driver that doesn't exist under `drivers/` — at either level — is a
misconfiguration: `scripts/run-driver.sh` fails immediately with an "unknown
driver" error rather than silently falling back to the interactive session.

Each driver lives in its own directory here, named after itself:

```
drivers/
  <name>/
    driver.yaml   # manifest
    <command>     # the executable named in the manifest, run.sh by convention
```

## Manifest (`driver.yaml`)

```yaml
name: <name>              # must match the directory name
description: <string>     # human-readable, one line
output_mode: path-parameterized
command: run.sh            # path to the executable, relative to this directory
```

`output_mode` declares which invocation contract the driver honors.
`scripts/run-driver.sh` supports two modes:

- **path-parameterized** — the driver accepts an explicit output location and
  writes exactly there. `scripts/run-driver.sh <name> <repo-path>
  <output-path>` invokes it as:

  ```
  <driver-dir>/<command> <repo-path> <output-path>
  ```

  `<repo-path>` is an absolute path to the target repo. The driver must
  write its finished context map to exactly `<output-path>` (creating parent
  directories as needed) and exit non-zero — without leaving a file behind —
  on failure. `run-driver.sh` treats a zero exit with no file at
  `<output-path>` as an error.

- **fixed-location** — the driver can't be told where to write; it always
  writes into whatever repo it's run in, at a fixed path relative to that
  repo's root. The manifest must also declare `fixed_path` (e.g.
  `CONTEXT.md`). `scripts/run-driver.sh <name> <repo-path> <output-path>`
  invokes it as:

  ```
  <driver-dir>/<command> <repo-path>
  ```

  The driver must write to exactly `<repo-path>/<fixed_path>` and exit
  non-zero — without leaving a file behind — on failure. `run-driver.sh`
  then harvests that file itself: it moves (not copies)
  `<repo-path>/<fixed_path>` to `<output-path>`, so the canonical copy ends
  up wherever the caller asked and the target repo is left with no trace of
  it. A zero exit with no file at `<repo-path>/<fixed_path>` is treated as
  an error, same as path-parameterized.

  `fixed_path` may be nested (`.specify/memory/constitution.md`); after
  moving the file out, `run-driver.sh` removes the directories that move
  emptied, stopping at the first one that still holds something. `git
  status` wouldn't have caught those — git doesn't track directories — but
  they're a trace of the run all the same.

  "No trace" is a joint obligation, and the half `run-driver.sh` can't
  discharge belongs to the driver: it must leave the target repo exactly as
  it found it apart from `<fixed_path>`. A driver that only ever writes one
  file (`pocock`) gets this for free. One that has to scaffold a whole
  toolchain into the repo before it can produce anything (`spec-kit`) has to
  undo that scaffolding itself before exiting — see
  `drivers/spec-kit/repo-snapshot.sh` for the snapshot-then-restore approach
  that generalizes to any such tool.

## Trying one directly

Every driver can be exercised outside of `context-map-all.sh`:

```
scripts/run-driver.sh <name> <path-to-a-repo> <path-to-write-the-map-to>
```

## Available drivers

- `openspec` — wraps the [OpenSpec CLI](https://github.com/Fission-AI/OpenSpec)
  (`npm install -g @fission-ai/openspec`). Initializes OpenSpec in the target
  repo if needed, then captures `openspec context`'s working-context report
  as the context map.
- `pocock` — wraps [Matt Pocock's `domain-modeling`
  skill](https://github.com/mattpocock/skills) via a headless `claude -p`
  session. The skill always writes `CONTEXT.md` at the root of whatever repo
  it's run in, so this is a `fixed-location` driver (`fixed_path:
  CONTEXT.md`) — requires the `claude` CLI and the `domain-modeling` skill
  installed.
- `spec-kit` — wraps [GitHub's Spec Kit](https://github.com/github/spec-kit)
  (`uv tool install specify-cli --from
  git+https://github.com/github/spec-kit.git`), also via a headless `claude
  -p` session. Requires both the `specify` and `claude` CLIs, and bash 4+
  (checked before anything is written, so an old bash fails the run rather
  than stranding a half-unpacked toolchain in the target repo).

  Spec Kit is the awkward case the `fixed-location` mode exists for. It has
  no "point at a repo, write a report over here" mode at all: `specify init`
  unpacks templates, helper scripts and agent skills into the repo, and its
  one whole-repo artifact — the constitution — is always written to
  `.specify/memory/constitution.md`. So the driver snapshots the repo,
  scaffolds, has the `speckit-constitution` skill fill the constitution in
  from the codebase, then restores everything except the constitution
  itself, which `run-driver.sh` harvests. Rollback is the driver's exit
  trap, not something on its success path, so a failed `specify init`, a
  session that does nothing, and a Ctrl-C halfway through all leave the repo
  as it was found.

  The one thing that defeats it is a session that commits: everything the
  rollback reasons about is relative to the commit `HEAD` pointed at when
  the run started, so moving `HEAD` makes the scaffolding indistinguishable
  from the repo's own history — while leaving `git status` reading clean.
  The driver checks for that and fails loudly rather than reporting a
  context map for a repo it quietly left a toolchain in.

  What you get is a different shape of context map from the other two: a
  repo's *principles and constraints* — the conventions its existing code
  already follows, the boundaries between its modules, what its dependencies
  rule out — rather than `openspec`'s structural report or `pocock`'s domain
  vocabulary. Which of the three is the useful one is a per-repo judgment,
  which is why the driver is a per-repo setting.
