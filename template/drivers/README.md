# Drivers

A driver is whatever actually produces a repo's context map — an AI coding
agent, a wrapped third-party CLI, a script. `scripts/context-map-all.sh`
orchestrates *which* repos need mapping and in what order; it never knows how
any given driver does its job. That split is the point: swapping the
configured driver (`ARCHIMEDES_DRIVER=<name>`) never requires touching
orchestration.

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
