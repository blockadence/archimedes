// Package workspace hands a freshly created worktree to whatever terminal
// workspace manager the operator runs, so a spawned unit of work lands in
// a pane already rooted at its own checkout instead of needing a manual
// `cd`. It is opt-in: with nothing configured, nothing here runs, and a
// configured tool that turns out to be missing or unhappy is a warning,
// never a failure — the worktree is already on disk by the time any of
// this is reached.
package workspace

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// HerdrName is both the configuration value selecting the herdr
// integration and the executable it looks for on PATH.
const HerdrName = "herdr"

// ErrUnavailable reports that a configured integration's tool isn't usable
// on this machine — the ordinary state of affairs anywhere the operator
// hasn't installed it. Callers match it with errors.Is to distinguish "not
// set up here" from "set up, but the call failed".
var ErrUnavailable = errors.New("workspace integration unavailable")

// Request is one "put a terminal here" ask: the worktree a workspace
// should be rooted at, plus the labelling that lets a human pick it out
// from the other units of work already open.
type Request struct {
	// Repo is the main checkout Path is a linked worktree of. A workspace
	// manager identifies a worktree relative to the repo that owns it, not
	// by path alone.
	Repo string
	// Path is the worktree directory the workspace is rooted at.
	Path string
	// Label names the workspace in the integration's own UI.
	Label string
	// Focus asks to switch to the new workspace rather than opening it in
	// the background.
	Focus bool
}

// Opener hands one Request to a terminal workspace manager.
type Opener func(req Request) error

// Integration is one such manager, paired with the name that selects it in
// configuration.
type Integration struct {
	Name string
	Open Opener
}

// Select resolves a configured integration name to the integration that
// implements it. An empty or explicitly-off setting selects nothing (a nil
// Integration, no error), because this is opt-in — an operator who
// configures nothing gets exactly the behavior they had before. An
// unrecognized name is an error rather than a silent no-op, so a typo
// doesn't quietly withhold the pane they asked for.
func Select(name string) (*Integration, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "off", "none":
		return nil, nil
	case HerdrName:
		return &Integration{Name: HerdrName, Open: openHerdr}, nil
	default:
		return nil, fmt.Errorf("unknown workspace integration: %s (known: %s; %q or %q to disable)",
			name, HerdrName, "off", "none")
	}
}

// herdrArgs builds the `herdr worktree open` invocation for req. It's
// `open` rather than `create` because git has already made the checkout by
// this point — herdr adopts the worktree that exists instead of creating a
// second one of its own. --cwd names the repo that owns the worktree:
// herdr resolves --path against that repo's worktree list, so without it
// the lookup runs against whatever directory the CLI happens to be in and
// comes back "worktree path not found".
func herdrArgs(req Request) []string {
	args := []string{"worktree", "open"}
	if req.Repo != "" {
		args = append(args, "--cwd", req.Repo)
	}
	args = append(args, "--path", req.Path)
	if req.Label != "" {
		args = append(args, "--label", req.Label)
	}
	if req.Focus {
		return append(args, "--focus")
	}
	return append(args, "--no-focus")
}

// openHerdr opens req's worktree as a herdr workspace. herdr answers on
// stdout with JSON meant for programmatic callers and reports problems as
// JSON on stderr with a non-zero status; stdout is dropped (nothing here
// consumes it, and it would only clutter spawn's output) while stderr is
// carried into the error, since "it didn't work" alone leaves an operator
// with nowhere to go.
func openHerdr(req Request) error {
	bin, err := exec.LookPath(HerdrName)
	if err != nil {
		return fmt.Errorf("%w: %s is not on PATH", ErrUnavailable, HerdrName)
	}

	args := herdrArgs(req)
	cmd := exec.Command(bin, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w: %s", HerdrName, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}

	return nil
}
