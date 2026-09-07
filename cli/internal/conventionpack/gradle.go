package conventionpack

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// gradle is the build-tool block a Gradle pack carries alongside the
// generic core: what Gradle's own dependency mechanism needs beyond the
// artifact itself. Decoded from the pack by applyGradle, so no type above
// the build-tool dispatch has to know this shape exists.
type gradle struct {
	PluginID string `yaml:"plugin_id"`
}

// coordinate renders an artifact the way Gradle and Maven address one.
// That form is those two ecosystems', not every build tool's, so it lives
// here rather than on Artifact.
func coordinate(a Artifact) string {
	return a.Group + ":" + a.ID + ":" + a.Version
}

// buildscriptBlock matches a build file already opening a buildscript {}
// block of its own, at any indentation. Anchored to the start of a line so
// a mention of the word in a comment or a string isn't mistaken for one.
var buildscriptBlock = regexp.MustCompile(`(?m)^[ \t]*buildscript[ \t]*\{`)

// gradleBuildFiles are the build files a Gradle project can have, in the
// order they're looked for: a repo carrying both is a Kotlin project with
// a leftover, so the Kotlin one wins.
var gradleBuildFiles = []struct {
	name  string
	isKts bool
}{
	{"build.gradle.kts", true},
	{"build.gradle", false},
}

// applyGradle wires a Gradle repo up to pack via the buildscript-classpath
// idiom (`buildscript { dependencies { classpath "group:id:version" } }`
// plus `apply plugin: "id"`) rather than the plugins {} DSL. The plugins
// {} DSL only resolves a plugin already on the Gradle Plugin Portal, or
// one the target repo's settings.gradle already points pluginManagement
// at. Applying by binary coordinate works for a privately-published shared
// artifact without assuming either, and it's what actually consumes the
// pack's own artifact.{group,id,version} fields.
func applyGradle(pack Pack, target Target) (string, error) {
	var cfg gradle
	if err := pack.DecodeTool(&cfg); err != nil {
		return "", err
	}
	pluginID := cfg.PluginID

	if gaps := missing(
		field{"artifact.group", pack.Artifact.Group},
		field{"artifact.id", pack.Artifact.ID},
		field{"artifact.version", pack.Artifact.Version},
		field{"gradle.plugin_id", pluginID},
	); len(gaps) > 0 {
		return "", fmt.Errorf("convention pack %s is missing %s", pack.Name, strings.Join(gaps, ", "))
	}

	buildFile, isKts, err := findGradleBuildFile(target.Path)
	if err != nil {
		return "", err
	}
	base := filepath.Base(buildFile)

	contents, err := os.ReadFile(buildFile)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(buildFile)
	if err != nil {
		return "", err
	}

	// Checked before the buildscript {} block below, so a repo that pulled
	// the plugin in by hand — buildscript block and all — reports as
	// already on the pack instead of as a conflict.
	if strings.Contains(string(contents), pluginID) {
		return fmt.Sprintf("%s is already on %s (%s found in %s).", target.Name, pack.Name, pluginID, base), nil
	}

	classpathLine, applyLine := gradleLines(coordinate(pack.Artifact), pluginID, isKts)

	if buildscriptBlock.Match(contents) {
		return "", &ManualEditError{
			BuildFile: base,
			Reason:    "already has a buildscript {} block",
			Fix:       "Add this dependency inside its dependencies {} block by hand, plus the apply line near the top",
			Lines:     []string{classpathLine, applyLine},
		}
	}

	block := fmt.Sprintf("buildscript {\n    dependencies {\n        %s\n    }\n}\n%s\n\n", classpathLine, applyLine)
	if err := rewriteInPlace(buildFile, append([]byte(block), contents...), info.Mode().Perm()); err != nil {
		return "", err
	}

	return fmt.Sprintf("Added %s (plugin %s) to %s. Review the diff and commit it in %s.",
		coordinate(pack.Artifact), pluginID, base, target.Name), nil
}

// findGradleBuildFile picks the build file to edit in repoPath. A repo
// with neither isn't a Gradle project yet, which is the operator's problem
// to fix rather than something to scaffold around.
func findGradleBuildFile(repoPath string) (path string, isKts bool, err error) {
	for _, candidate := range gradleBuildFiles {
		path := filepath.Join(repoPath, candidate.name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, candidate.isKts, nil
		}
	}
	return "", false, fmt.Errorf("no build.gradle(.kts) found at %s, not a Gradle project yet", repoPath)
}

// gradleLines renders the two additions in the build file's own DSL.
func gradleLines(coordinate, pluginID string, isKts bool) (classpath, apply string) {
	if isKts {
		return fmt.Sprintf("classpath(%q)", coordinate), fmt.Sprintf("apply(plugin = %q)", pluginID)
	}
	return fmt.Sprintf("classpath '%s'", coordinate), fmt.Sprintf("apply plugin: '%s'", pluginID)
}

// rewriteInPlace replaces path's contents with data, keeping the file's
// mode and swapping the new contents in with a single rename — so a
// failure part way through leaves the operator's build file as it was
// rather than half-written. The scratch file is cleaned up on either
// failure, since it lands inside the repo being edited, where a leftover
// would show up in the diff the operator is about to review.
func rewriteInPlace(path string, data []byte, mode fs.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// field is one pack field the scaffolder needs filled in, named by its
// path in the pack file so a missing one points straight at the line to
// edit.
type field struct {
	name  string
	value string
}

// missing reports which of fields are empty, in the order given. A pack
// filled in halfway would otherwise scaffold a coordinate made of empty
// strings into someone's build file, which is worse than not running at
// all.
func missing(fields ...field) []string {
	var names []string
	for _, f := range fields {
		if f.value == "" {
			names = append(names, f.name)
		}
	}
	return names
}
