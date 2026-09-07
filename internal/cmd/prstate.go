package cmd

import (
	"fmt"

	"github.com/blockadence/archimedes/internal/gitutil"
	"github.com/blockadence/archimedes/internal/manifest"
	"github.com/blockadence/archimedes/internal/prune"
)

// repoPRState adapts a "owner/repo" pull request lookup to the repo names
// status.md rows actually carry, resolving each one through the manifest
// to its checkout and that checkout's origin remote. Shared by every
// subcommand that reads PR state off a status.md row (prune, notify), so
// they can't disagree about which repository a row refers to.
//
// Either way the state is "NONE" — no PR to act on — because that is the
// safe direction: a lookup failure must never be
// mistaken for permission to prune something. The two are told apart by
// the error instead. A repo the manifest doesn't know is a settled answer
// (there is nothing here to prune); an origin that can't be read is a
// question that went unanswered, and a caller that draws conclusions from
// silence needs to know which it got.
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
				return "NONE", fmt.Errorf("reading %s's origin remote: %w", repo, err)
			}
			slugs[repo] = slug
		}

		return lookup(slug, headBranch)
	}
}
