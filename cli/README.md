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

Walking skeleton: one real subcommand (`render-map`, the port of
`template/scripts/render-map.sh`) proves the structure end to end — build,
install, help text, `--version`, and shell completion (via
`spf13/cobra` + `charmbracelet/fang`). The rest of `template/scripts/*.sh`
get ported the same way, one subcommand at a time.
