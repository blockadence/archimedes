// Package reposync pushes canonical control-repo content into the tracked
// target repos as pull requests: the shared PR/issue templates
// (template/scripts/sync-templates.sh) and a repo's own house rules
// (template/scripts/sync-house-rules.sh).
//
// The two differ in shape, not intent. Templates are identical across every
// repo, so that sync is a thin wrapper around multi-gitter's fan-out — no
// hand-rolled multi-repo PR engine here. House rules are specific to one
// repo, so that sync works directly on that repo's existing local clone
// with plain git plus gh.
//
// Both are kept independent of cobra/CLI concerns so they can be
// unit-tested directly; everything they shell out to that isn't git goes
// through ExecFunc, the seam tests replace.
package reposync

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/blockadence/archimedes/cli/internal/manifest"
)

// loadInstance resolves an instance root to an absolute path and reads its
// manifest — the prologue both syncs share. The path is absolutized up
// front because git runs with its working directory set to a target repo,
// so a relative root would resolve against the checkout instead of the
// instance.
func loadInstance(rootOption string) (root string, m *manifest.Manifest, err error) {
	root, err = filepath.Abs(rootOption)
	if err != nil {
		return "", nil, fmt.Errorf("resolving instance root %s: %w", rootOption, err)
	}

	manifestPath := filepath.Join(root, "repos.yaml")
	m, err = manifest.Load(manifestPath)
	if err != nil {
		return "", nil, fmt.Errorf("loading %s: %w", manifestPath, err)
	}

	return root, m, nil
}

// ExecFunc runs an external command with its output going to stdout and
// stderr. It's the seam tests replace to keep multi-gitter and gh out of
// the unit tests; production callers pass RunCommand. (git is not routed
// through here — it goes through internal/gitutil, which every subcommand
// already shares.)
type ExecFunc func(name string, args []string, stdout, stderr io.Writer) error

// RunCommand is the production ExecFunc: it runs name with args, streaming
// both output streams to the given writers.
func RunCommand(name string, args []string, stdout, stderr io.Writer) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

// ghToken reads the GitHub token from the operator's existing gh session,
// the way both scripts do (`gh auth token`) — so neither sync needs a
// credential of its own.
func ghToken(run ExecFunc, progress io.Writer) (string, error) {
	var out bytes.Buffer
	if err := run("gh", []string{"auth", "token"}, &out, progress); err != nil {
		return "", fmt.Errorf("reading GitHub token from gh: %w", err)
	}

	token := strings.TrimSpace(out.String())
	if token == "" {
		return "", fmt.Errorf("gh auth token returned nothing; run `gh auth login` first")
	}
	return token, nil
}
