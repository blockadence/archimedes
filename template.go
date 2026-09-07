// Package archimedes carries the instance template — the starting data a
// new instance is scaffolded from — inside the binary, so creating one
// needs nothing but the installed tool.
//
// The template lives at template/ in this repository and stays ordinary
// files: prose (AGENTS.md, drivers/README.md), example drivers and
// convention packs, maintained by editing them. Nothing here generates it
// and nothing here holds a second copy of it. This package exists at the
// module root for one reason: go:embed cannot reach outside the package
// directory it appears in, and the template belongs at the top of the repo
// where its maintainers can find it rather than buried in the Go tree.
package archimedes

import (
	"embed"
	"io/fs"
)

// all: is load-bearing. Without it embed drops every path beginning with a
// dot, taking the instance's .gitignore and the .gitkeep markers that give
// it its repos/ and work/ directories with it.
//
//go:embed all:template
var embedded embed.FS

// Template returns the instance template rooted at its own top level, so
// callers see repos.yaml and drivers/ rather than template/repos.yaml.
func Template() fs.FS {
	sub, err := fs.Sub(embedded, "template")
	if err != nil {
		// Unreachable: the embed directive above is what puts the
		// subtree there, and it is checked at compile time.
		panic(err)
	}
	return sub
}
