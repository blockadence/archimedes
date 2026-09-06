// Package workspacemap regenerates WORKSPACE-MAP.md's generated repo list
// from an instance's repos.yaml, leaving everything else in the file (most
// importantly the hand-written "## Relationships" section) untouched.
//
// This is a line-for-line port of template/scripts/render-map.sh's awk
// pass: replace every line between the "## Repos" heading and the next
// "## Relationships" heading, except blank lines, which are left in place.
// That quirk (a holdover from the original script) is preserved so output
// matches byte-for-byte.
package workspacemap

import (
	"fmt"
	"strings"

	"github.com/blockadence/archimedes/cli/internal/manifest"
)

// DefaultContent is written when WORKSPACE-MAP.md doesn't exist yet.
const DefaultContent = "# Workspace Map\n\n## Repos\n\n## Relationships\n"

const (
	reposHeading         = "## Repos"
	relationshipsHeading = "## Relationships"
)

// RepoLine renders one repo's line in the generated block.
func RepoLine(r manifest.Repo) string {
	return fmt.Sprintf("- [%s](%s) — base: `%s`. Dossier: [repos/%s.md](./repos/%s.md)",
		r.Name, r.Path, r.BaseBranch, r.Name, r.Name)
}

// Render returns existing with the block between "## Repos" and
// "## Relationships" replaced by one RepoLine per repo, in repos' order.
func Render(existing string, repos []manifest.Repo) string {
	lines := strings.Split(existing, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	blockLines := make([]string, len(repos))
	for i, r := range repos {
		blockLines[i] = RepoLine(r)
	}
	block := strings.Join(blockLines, "\n")

	out := make([]string, 0, len(lines)+2)
	skip := false
	for _, line := range lines {
		if strings.HasPrefix(line, reposHeading) {
			out = append(out, line, "", block)
			skip = true
			continue
		}
		if strings.HasPrefix(line, relationshipsHeading) {
			skip = false
		}
		if skip && line != "" {
			continue
		}
		out = append(out, line)
	}

	return strings.Join(out, "\n") + "\n"
}
