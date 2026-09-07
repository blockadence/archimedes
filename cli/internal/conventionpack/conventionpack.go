// Package conventionpack wires a repo up to the shared build/lint
// convention it declares — the port of
// template/scripts/apply-convention-pack.sh.
//
// A convention pack is a named, language-agnostic definition living in the
// instance's own convention-packs/<name>.yaml: which build tool it targets
// and which published artifact carries the config. A repo declares one by
// name in repos.yaml's convention_pack field. Both halves are instance
// data; nothing here reads a separate config of its own.
//
// This is one-time scaffolding, not ongoing sync. Once the reference is in
// the target repo's build file, that repo owns it like any other
// dependency — nothing pushes updates back into it later — so applying a
// pack twice is a no-op rather than a rewrite.
//
// Apply dispatches on the pack's build_tool, so teaching Archimedes a
// second language/build tool is one entry in scaffolders plus its
// function (see gradle.go as the worked example). Nothing above that
// dispatch knows Java, Gradle, or any one build tool.
package conventionpack

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// DirName is the instance directory holding pack definitions, one YAML
// file per pack.
const DirName = "convention-packs"

// Artifact is the published artifact carrying a pack's shared config,
// named in whatever terms the build tool's ecosystem uses (Maven/Gradle
// group, npm scope, PyPI owner, ...).
type Artifact struct {
	Group   string `yaml:"group"`
	ID      string `yaml:"id"`
	Version string `yaml:"version"`
}

// Pack is one convention-packs/<name>.yaml: the generic core every pack
// has, plus whatever its build tool needs on top, left undecoded in Tool.
type Pack struct {
	Name        string   `yaml:"name"`
	Language    string   `yaml:"language"`
	BuildTool   string   `yaml:"build_tool"`
	Description string   `yaml:"description"`
	Artifact    Artifact `yaml:"artifact"`
	// Tool holds every build-tool-named block the file carries, still as
	// YAML. Each scaffolder decodes its own through DecodeTool, so the
	// shape of one build tool's extra detail is known only to the code
	// that scaffolds that build tool — and a new one costs a scaffolder,
	// not a field here.
	Tool map[string]yaml.Node `yaml:",inline"`
}

// DecodeTool decodes the block named after the pack's own build_tool into
// v, whose type is the scaffolder's business. A pack missing that block
// leaves v alone: the scaffolder's own check for the fields it needs
// reports that by name, rather than a second error saying the same thing
// less usefully.
func (p Pack) DecodeTool(v any) error {
	node, ok := p.Tool[p.BuildTool]
	if !ok {
		return nil
	}
	if err := node.Decode(v); err != nil {
		return fmt.Errorf("parsing convention pack %s's %s block: %w", p.Name, p.BuildTool, err)
	}
	return nil
}

// Target is the repo being wired up: its name in repos.yaml (for messages
// the operator reads) and its checkout on disk (what gets edited).
type Target struct {
	Name string
	Path string
}

// Load reads the pack named name from packsDir. A pack nobody has defined
// is a misconfiguration that names the file it looked for, so the fix is
// obvious; a pack file that exists but won't parse blames its contents
// instead of the name.
func Load(packsDir, name string) (Pack, error) {
	path := filepath.Join(packsDir, name+".yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Pack{}, fmt.Errorf("unknown convention pack: %s (no %s)", name, path)
		}
		return Pack{}, fmt.Errorf("reading %s: %w", path, err)
	}

	var p Pack
	if err := yaml.Unmarshal(data, &p); err != nil {
		return Pack{}, fmt.Errorf("parsing %s: %w", path, err)
	}
	return p, nil
}

// scaffolder adds one build tool's reference to pack into target's build
// file, returning the line to report to the operator. It is called only
// for a pack whose BuildTool it was registered under.
type scaffolder func(Pack, Target) (string, error)

// scaffolders maps a pack's build_tool to the code that knows how that
// build tool pulls a dependency in. Supporting a new one is an entry here,
// not a change to Apply.
var scaffolders = map[string]scaffolder{
	"gradle": applyGradle,
}

// Apply wires target up to pack, and returns the line describing what it
// did. It is idempotent: a repo already on the pack is left exactly as it
// is. A build file that already carries a conflicting block of its own is
// refused with a *ManualEditError rather than rewritten.
func Apply(pack Pack, target Target) (string, error) {
	scaffold, ok := scaffolders[pack.BuildTool]
	if !ok {
		return "", fmt.Errorf(
			"no scaffold logic yet for build_tool %q (pack: %s); register one in internal/conventionpack — see convention-packs/README.md",
			pack.BuildTool, pack.Name)
	}
	return scaffold(pack, target)
}

// ManualEditError refuses a build file whose existing content the scaffold
// can't safely extend, and carries the lines a human should add instead.
// Typed so a caller can tell "this needs a person" apart from "this broke".
//
// The message is a single line, since that is all an error is; the lines
// themselves come back from Instructions, for the caller to print where
// they stay readable rather than folded into an error string.
type ManualEditError struct {
	// BuildFile is the file's base name, as the operator refers to it.
	BuildFile string
	// Reason says what about the file blocked the scaffold, as a verb
	// phrase: "already has a buildscript {} block".
	Reason string
	// Fix is the imperative sentence introducing Lines: what to do with
	// them, and whereabouts in the file they go.
	Fix string
	// Lines are the additions, in that build file's own syntax.
	Lines []string
}

func (e *ManualEditError) Error() string {
	return fmt.Sprintf("refusing to edit %s: it %s, so the lines it needs must be added by hand",
		e.BuildFile, e.Reason)
}

// Instructions is the operator-facing block naming what to add and where.
func (e *ManualEditError) Instructions() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s.\n%s:", e.BuildFile, e.Reason, e.Fix)
	for _, line := range e.Lines {
		fmt.Fprintf(&b, "\n  %s", line)
	}
	return b.String()
}
