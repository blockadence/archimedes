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
Currently only `path-parameterized` is supported by
`scripts/run-driver.sh`:

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

  (A `fixed-location` mode, for drivers that can only ever write into the
  repo they're run in, is future work — see issue 06 in the v2 plan.)

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
