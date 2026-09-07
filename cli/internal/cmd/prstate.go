package cmd

import (
	"github.com/blockadence/archimedes/cli/internal/gitutil"
	"github.com/blockadence/archimedes/cli/internal/manifest"
	"github.com/blockadence/archimedes/cli/internal/prune"
)

// repoPRState adapts a "owner/repo" pull request lookup to the repo names
// status.md rows actually carry, resolving each one through the manifest
// to its checkout and that checkout's origin remote. Shared by every
// subcommand that reads PR state off a status.md row (prune, notify), so
// they can't disagree about which repository a row refers to.
//
// A repo the manifest doesn't know, or whose origin can't be read, comes
// back as "NONE": no PR to act on. That matches lib.sh, and it is the safe
// direction — a lookup failure must never be mistaken for permission to
// prune something.
//
// Slugs are cached because one repo typically owns several rows and the
// answer can't change within a run.
func repoPRState(root string, m *manifest.Manifest, lookup prune.PRStateFunc) prune.PRStateFunc {
	slugs := map[string]string{}

	return func(repo, headBranch string) (string, error) {
		entry, err := m.Resolve(root, repo)
		if err != nil {
			return "NONE", nil
		}

		slug, cached := slugs[repo]
		if !cached {
			if slug, err = gitutil.GHSlug(entry.Path); err != nil {
				return "NONE", nil
			}
			slugs[repo] = slug
		}

		return lookup(slug, headBranch)
	}
}
