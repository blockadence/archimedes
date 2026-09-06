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

## Status

Two subcommands ported so far: `render-map` (`template/scripts/render-map.sh`)
and `status` (`template/scripts/status.sh`). The rest of
`template/scripts/*.sh` get ported the same way, one subcommand at a time.

`status` reads every `work/<slug>/status.md`, looks up each row's live PR
state via `gh pr list`, and prints the same fixed-width table the shell
script did (or `--json` for a machine-readable report). A row's PR lookup
degrading to "no PR" — a missing `gh` auth, no network, an unset repo — never
fails the rest of the report, matching the original script's `|| echo '{}'`
fallback.
