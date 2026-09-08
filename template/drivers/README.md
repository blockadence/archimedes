# Drivers

A driver is whatever actually produces a repo's context map — an AI coding
agent, a wrapped third-party CLI, a script. `context-map` orchestrates
*which* repos need mapping and in what order; it never knows how any given
driver does its job. That split is the point: swapping the configured driver
never requires touching orchestration.

## Which drivers exist, and who owns them

Two places, and the difference is who maintains what:

- **This directory is yours.** Anything you put here is the instance's.
  Nothing ever overwrites, refreshes or removes it, and no upgrade to
  `archimedes` will touch it. It starts empty.
- **Archimedes ships a few drivers inside the binary** — `openspec`,
  `pocock`, `spec-kit` — and reads them out of it at the moment one runs.
  They are never copied into an instance. That is deliberate: it is what
  lets a bug fixed in one of them reach *your* instance, which already
  exists, the next time you upgrade the tool — however you installed it. A
  driver that scaffolds a third-party toolchain into someone else's
  repository and unwinds it afterwards is not a thing you want a stale copy
  of.

The `drivers` listing shows what this instance can run, and where each one
comes from.

A name in this directory **wins** over a shipped one. So an instance that
writes its own `spec-kit/` gets its own, always, and the listing marks it
`shadows built-in` — a reminder that fixes to the shipped `spec-kit` no
longer reach you, which is the deal you took when you took it over.

### Editing a driver Archimedes ships

Take it over first:

`drivers adopt spec-kit` copies the whole driver — command and manifest —
into `drivers/spec-kit/`, runnable, for you to edit, along with the shared
helpers it sources into `drivers/lib/` (see below). From then on it is yours
by the rule above. To hand the name back to the version Archimedes
maintains, delete `drivers/spec-kit/`.

A `drivers/lib/` you already have is left exactly as it is — it is yours by
the same rule, and adopting a second driver that sources it must not
overwrite an edit you made there. The corollary is that handing a driver
back leaves `drivers/lib/` behind: delete that too, unless another driver of
yours sources it. A stale one left there is a copy no adopt will refresh and
no upgrade will reach.

It is a one-time act, not a subscription: nothing re-syncs an adopted
driver, in either direction, and adopting over one you already have is
refused rather than resolved.

**If your instance predates this arrangement** it may still hold copies of
`openspec/`, `pocock/` and `spec-kit/` that were scaffolded into it. Those
copies still run, and no fix made to the shipped drivers will ever reach
them — the `drivers` listing flags each one as `shadows built-in`. Delete
the ones you never edited; keep (and own) the ones you did.

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

`ARCHIMEDES_DRIVERS_DIR=<path>` moves *this* directory somewhere else for
one invocation — useful for trying a driver you are writing without putting
it in the instance yet. It moves the instance layer only: the drivers
Archimedes ships stay underneath whatever it names, so pointing it at a
scratch directory holding one driver still leaves the other three
resolvable.

Naming a driver neither this directory nor the binary supplies — at either
level — is a misconfiguration: the run fails immediately with an "unknown
driver" error naming both places it looked, rather than silently falling
back to the interactive session.

## Writing one of your own

A driver lives in its own directory here, named after itself:

```
drivers/
  lib/            # helpers more than one driver sources — not a driver itself
  <name>/
    driver.yaml   # manifest
    <command>     # the executable named in the manifest, run.sh by convention
```

The command has to be executable (`chmod +x`) — Archimedes runs it, it does
not source it. Anything the command *sources* rather than runs should stay
non-executable; `lib/repo-snapshot.sh` is the worked example.

`lib/` is where a helper goes once more than one driver needs it. A driver
reaches it at `../lib/` relative to its own directory, which resolves the
same way whether the driver is yours or was read out of the binary. It
declares no manifest, so it is not a driver: it never appears in the
`drivers` listing, and naming it is an unknown driver like any other name
nothing supplies. A helper only one driver uses can just sit beside that
driver's command instead.

A driver is handed its own directory in the sense that files beside its
command are there to be sourced or read — but not as durable storage. A
shipped driver is unpacked somewhere fresh for each run and thrown away
afterwards, so anything written beside the command is gone by the next one.
State that has to survive belongs in the repo being mapped, or in the output
path the driver was given.

That unpacking uses the system temp directory. On a machine that mounts it
`noexec`, the shipped drivers cannot run from it — point `TMPDIR` somewhere
executable. Drivers in this directory are unaffected, so the symptom is that
only the shipped ones fail.

## Manifest (`driver.yaml`)

```yaml
name: <name>              # must match the directory name
description: <string>     # human-readable, one line
output_mode: path-parameterized
command: run.sh            # path to the executable, relative to this directory
```

`output_mode` declares which invocation contract the driver honors. Two
modes are supported:

- **path-parameterized** — the driver accepts an explicit output location and
  writes exactly there. `run-driver <name> <repo-path> <output-path>`
  invokes it as:

  ```
  <driver-dir>/<command> <repo-path> <output-path>
  ```

  `<repo-path>` is an absolute path to the target repo. The driver must
  write its finished context map to exactly `<output-path>` (creating parent
  directories as needed) and exit non-zero — without leaving a file behind —
  on failure. A zero exit with no file at `<output-path>` is treated as an
  error.

- **fixed-location** — the driver can't be told where to write; it always
  writes into whatever repo it's run in, at a fixed path relative to that
  repo's root. The manifest must also declare `fixed_path` (e.g.
  `CONTEXT.md`). `run-driver <name> <repo-path> <output-path>` invokes it
  as:

  ```
  <driver-dir>/<command> <repo-path>
  ```

  The driver must write to exactly `<repo-path>/<fixed_path>` and exit
  non-zero — without leaving a file behind — on failure. Archimedes then
  harvests that file itself: it moves (not copies)
  `<repo-path>/<fixed_path>` to `<output-path>`, so the canonical copy ends
  up wherever the caller asked and the target repo is left with no trace of
  it. A zero exit with no file at `<repo-path>/<fixed_path>` is treated as
  an error, same as path-parameterized.

  `fixed_path` may be nested (`.specify/memory/constitution.md`); after
  moving the file out, the directories that move emptied are removed too,
  stopping at the first one that still holds something. `git
  status` wouldn't have caught those — git doesn't track directories — but
  they're a trace of the run all the same.

  "No trace" is a joint obligation, and the half Archimedes can't
  discharge belongs to the driver: it must leave the target repo exactly as
  it found it apart from `<fixed_path>`. Neither shipped driver in this mode
  gets that for free. `spec-kit` has to scaffold a whole toolchain into the
  repo before it can produce anything; `pocock` writes only one file itself,
  but what it *runs* is an agent session with write access to the repo, and
  a sentence in a prompt is not a guarantee. Both take the same route:
  snapshot the repo, then put back whatever the run added or changed, on
  every exit path including the interrupt. `lib/repo-snapshot.sh` is that
  route written down, and it generalizes to any driver in this position
  (`drivers adopt spec-kit` puts a copy here to read).

  For the drivers Archimedes ships, that obligation is checked rather than
  taken on trust: `tests/fixed_location_conformance.sh` in the Archimedes
  repository finds every driver declaring this mode by reading the manifests
  — no list to add one to — points each at a throwaway repo with a stub
  standing in for the CLI it runs, has that stub write past the declared
  `fixed_path` the way a real scaffolder or a real agent session does, and
  asks the one question the contract turns on: did the repo come back as it
  was found? It runs on every push and costs nothing, and a driver added
  there needs no test of its own to be held to it.

  A driver *you* write here is outside that suite's reach — it runs in the
  Archimedes repository, over the drivers that ship from there — so this
  half of the promise is yours to keep. `lib/repo-snapshot.sh` is the route,
  and `drivers adopt spec-kit` puts a worked example beside it.

  What a driver does about the leftovers it finds is its own call, and the
  two shipped ones answer differently on purpose. `spec-kit`'s scaffolding
  was always going to be there, so putting it back is routine and the run
  succeeds. `pocock`'s session was asked for one file, so anything else is
  the prompt having lost an argument with a non-deterministic agent: it puts
  the files back, names them, and fails the run rather than harvesting a map
  from a session that would not keep to what it was told.

## Trying one directly

Every driver can be exercised outside of a mapping pass, with
`run-driver <name> <path-to-a-repo> <path-to-write-the-map-to>`.

## The drivers Archimedes ships

These come from the binary, not from this directory (see the ownership rule
above). The `drivers` listing shows whichever ones your install carries.

- `openspec` — wraps the [OpenSpec CLI](https://github.com/Fission-AI/OpenSpec)
  (`npm install -g @fission-ai/openspec`). Initializes OpenSpec in the target
  repo if needed, then captures `openspec context`'s working-context report
  as the context map.
- `pocock` — wraps [Matt Pocock's `domain-modeling`
  skill](https://github.com/mattpocock/skills) via a headless `claude -p`
  session. The skill always writes `CONTEXT.md` at the root of whatever repo
  it's run in, so this is a `fixed-location` driver (`fixed_path:
  CONTEXT.md`) — requires the `claude` CLI and the `domain-modeling` skill
  installed, plus bash 4+ and a target that is a git repo (both checked
  before the session starts, so a machine that cannot support the rollback
  fails the run rather than paying for one it cannot clean up after).

  Its prompt asks the session to write that one file and nothing else, at
  length, because the skill's own criteria call for ADRs — so the prompt is
  arguing with the thing it invokes, on every run, and will sometimes lose.
  The driver assumes it will: it snapshots the repo first, puts back
  everything the session wrote beyond `CONTEXT.md`, and fails the run naming
  those files. That throws away a billed session's usable map, deliberately.
  A run that quietly deleted an agent's work in your repository and reported
  success would leave you with no way to know the prompt had stopped
  working.
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
  itself, which Archimedes harvests. Rollback is the driver's exit
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
