package manifest

import "path/filepath"

// Path is where the instance at root keeps its manifest.
//
// One file, and the only one this package answers for: `repos.yaml`, the
// list of repos an instance tracks and how they relate. What is *in* it
// records paths of its own — a checkout as `../<name>` — and those are not
// resolved through here; where a checkout is comes from the entry that
// records it (CheckoutOf), against the root that entry was read under.
//
// root is answered as it was given, as in workdir.Path and dossier.Dir:
// `--root .` is the ordinary way to name an instance, and the answer is a
// file this process opens, from the working directory the operator typed
// the root in. What a caller may then do with a path it resolves against
// that same relative root is a separate question with a decided answer, and
// cmd.loadManifest is where it is written down (issue 59): resolving
// against the typed root is right for a path a tool is going to run *in*,
// and wrong for one handed to a tool as an argument, because that tool has
// a working directory of its own. LoadInstance absolutizes for that reason
// and says so; it absolutizes the root, then asks here.
//
// LoadInstance being a caller is half of what this is for. Eight
// production sites built this join by hand and one of them was inside this
// package, which knew the answer, used it, and handed back everything
// except the path it had just built (issue 75). The alternative — widening
// LoadInstance to return that path — would have served the two callers who
// load an instance and then need the file's name anyway (contextmap,
// reposync's template sync), and done nothing for the five that never call
// LoadInstance at all: bootstrap, which names the file to seed an empty one
// before there is anything to load, and status, dashboard, mcpserver and
// applyconventionpack, which need the name in order to Load it.
//
// The file stays a parameter to the rest of this package — Load,
// AppendRepo, SetRepoField all take a path rather than a root. That
// parameter is what lets a manifest be read and rewritten against a bare
// t.TempDir() with no instance around it, which setfield_test.go and
// append_test.go do exactly; it is worth more than the join it would save,
// and it is the same answer dossier.Dir reached for the same reason.
func Path(root string) string {
	return filepath.Join(root, "repos.yaml")
}
