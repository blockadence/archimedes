// Package archimedes carries inside the binary the two trees an instance
// needs from this repository, so neither one requires a clone of it: the
// instance template — the starting data a new instance is scaffolded from —
// and the drivers the tool ships.
//
// They are carried for opposite reasons. The template is seed data: it is
// written into an instance once, at creation, and is that instance's from
// then on. The shipped drivers are never written into an instance at all;
// they are read out of here at the moment one runs, so that fixing a driver
// here fixes it for every instance on the next upgrade rather than only for
// instances created afterwards. An instance that wants a driver of its own —
// including its own version of one of these — puts it in its own drivers/,
// where it wins (see internal/driver's Set).
//
// Both live at the top of this repository and stay ordinary files: prose,
// YAML, and bash, maintained by editing them. Nothing here generates either
// and nothing here holds a second copy of them. This package exists at the
// module root for one reason: go:embed cannot reach outside the package
// directory it appears in, and both trees belong where their maintainers
// can find them rather than buried in the Go tree.
package archimedes

import (
	"embed"
	"io/fs"
	"sync"
)

// all: is load-bearing. Without it embed drops every path beginning with a
// dot, taking the instance's .gitignore and the .gitkeep markers that give
// it its repos/ and work/ directories with it.
//
//go:embed all:template
var embedded embed.FS

// all: again, for the same reason: a driver is free to keep a dotfile
// beside its command, and one silently dropped from the copy that ships
// would be missing only in the built-in layer.
//
//go:embed all:drivers
var embeddedDrivers embed.FS

// Template returns the instance template rooted at its own top level, so
// callers see repos.yaml and drivers/ rather than template/repos.yaml.
var Template = sync.OnceValue(func() fs.FS { return rooted(embedded, "template") })

// Drivers returns the drivers this binary ships, rooted at their names, so
// callers see spec-kit/driver.yaml rather than drivers/spec-kit/driver.yaml
// — the shape internal/driver's Set wants for its built-in layer.
//
// One value, handed out to every caller: these end up inside configuration
// structs that are compared for equality, and a fresh wrapper per call
// would make two identically configured passes look different.
var Drivers = sync.OnceValue(func() fs.FS { return rooted(embeddedDrivers, "drivers") })

func rooted(embedded embed.FS, dir string) fs.FS {
	sub, err := fs.Sub(embedded, dir)
	if err != nil {
		// Unreachable: the embed directives above are what put these
		// subtrees there, and they are checked at compile time.
		panic(err)
	}
	return sub
}
