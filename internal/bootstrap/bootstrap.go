// Package bootstrap brings an instance up to date with the org it tracks:
// discover the org's repos, clone the ones missing locally, and scaffold a
// repos.yaml entry and dossier stub for each. Kept independent of
// cobra/CLI concerns so it can be unit-tested directly.
//
// Every step is idempotent, because re-running against an
// already-bootstrapped instance is the normal case rather than the
// exception: it adds only what's missing and never touches what an operator
// has written since.
package bootstrap

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/blockadence/gh-archimedes/internal/dossier"
	"github.com/blockadence/gh-archimedes/internal/gitutil"
	"github.com/blockadence/gh-archimedes/internal/manifest"
	"github.com/blockadence/gh-archimedes/internal/workspacemap"
)

// emptyManifest seeds an instance that has no repos.yaml yet.
const emptyManifest = "repos: []\n"

// Options is one bootstrap pass: which instance, against which org.
type Options struct {
	// Root is the instance directory holding repos.yaml and repos/.
	Root string
	// Org is the GitHub organization to discover repos in.
	Org string
	// IncludeArchived keeps the org's archived repos, which are dropped
	// by default.
	IncludeArchived bool
	// List discovers the org's repos, and is required: production
	// callers pass ListOrgRepos, tests a fake.
	List RepoLister
}

// Run discovers Org's repos and brings Root into line with them: an entry
// in repos.yaml, a local clone, and a dossier stub for each, then a
// regenerated WORKSPACE-MAP.md.
//
// progress receives git's own clone output; out receives the result lines
// meant for the caller.
func Run(opts Options, out, progress io.Writer) error {
	// Absolutized up front: repo paths are relative to the instance
	// ("../<name>"), so resolving them against anything else would clone
	// into the wrong place.
	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return fmt.Errorf("resolving instance root %s: %w", opts.Root, err)
	}

	// A mistyped --root must fail here rather than scaffold a second,
	// empty instance somewhere the operator never meant to put one.
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return fmt.Errorf("no instance directory at %s", opts.Root)
	}

	discovered, err := opts.List(opts.Org)
	if err != nil {
		return err
	}

	manifestPath := manifest.Path(root)
	if err := ensureManifest(manifestPath); err != nil {
		return err
	}

	added := 0
	for _, r := range filter(discovered, opts.IncludeArchived) {
		entry := manifest.Repo{
			Name: r.Name,
			// Relative to the instance root, so an instance stays
			// movable and its manifest stays readable on someone
			// else's machine.
			Path:       "../" + r.Name,
			BaseBranch: r.BaseBranch(),
		}

		isNew, err := manifest.AppendRepo(manifestPath, entry)
		if err != nil {
			return fmt.Errorf("adding %s to %s: %w", entry.Name, manifestPath, err)
		}
		if isNew {
			added++
		}

		checkout := manifest.CheckoutOf(root, entry)
		if !checkout.Cloned {
			fmt.Fprintf(out, "cloning %s\n", entry.Name)
			if err := gitutil.Clone(r.SSHURL, checkout.Path, progress); err != nil {
				return err
			}
		}

		if _, err := dossier.WriteStub(dossier.Dir(root), dossier.Stub{
			Name:       entry.Name,
			Path:       entry.Path,
			BaseBranch: entry.BaseBranch,
		}); err != nil {
			return fmt.Errorf("scaffolding %s's dossier: %w", entry.Name, err)
		}
	}

	fmt.Fprintf(out, "New repos added: %d\n", added)

	// Re-read rather than tracking the list in memory: the map should
	// reflect what the manifest now says, including repos this pass
	// didn't touch.
	m, err := manifest.Load(manifestPath)
	if err != nil {
		return fmt.Errorf("loading %s: %w", manifestPath, err)
	}

	return workspacemap.Update(root, m.Repos)
}

// ensureManifest seeds an empty repos.yaml for an instance that has none
// yet, leaving an existing one — comments and all — alone.
func ensureManifest(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	if err := os.WriteFile(path, []byte(emptyManifest), 0o644); err != nil {
		return fmt.Errorf("creating %s: %w", path, err)
	}
	return nil
}
