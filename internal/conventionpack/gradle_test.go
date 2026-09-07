package conventionpack_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blockadence/archimedes/internal/conventionpack"
)

const existingBuildFile = "plugins {\n    id 'java'\n}\n\nrepositories {\n    mavenCentral()\n}\n"

// gradleRepo is a checkout whose only build file is the named one.
func gradleRepo(t *testing.T, buildFileName, contents string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, buildFileName), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func applyJavaGradle(t *testing.T, repo string) (string, error) {
	t.Helper()
	return conventionpack.Apply(loadJavaGradle(t), conventionpack.Target{Name: "widget", Path: repo})
}

func TestApplyGradleGroovyAddsBuildscriptAndApplyLine(t *testing.T) {
	repo := gradleRepo(t, "build.gradle", existingBuildFile)

	msg, err := applyJavaGradle(t, repo)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	got := readFile(t, filepath.Join(repo, "build.gradle"))
	want := "buildscript {\n" +
		"    dependencies {\n" +
		"        classpath 'com.example.buildtools:java-conventions-plugin:1.4.0'\n" +
		"    }\n" +
		"}\n" +
		"apply plugin: 'com.example.buildtools.java-conventions'\n\n" +
		existingBuildFile
	if got != want {
		t.Errorf("build.gradle =\n%s\nwant\n%s", got, want)
	}
	for _, part := range []string{"com.example.buildtools:java-conventions-plugin:1.4.0", "build.gradle", "widget"} {
		if !strings.Contains(msg, part) {
			t.Errorf("message %q does not mention %q", msg, part)
		}
	}
}

// The Kotlin DSL takes the same two lines in Kotlin syntax, chosen off the
// build file that's actually there rather than off anything in the pack.
func TestApplyGradleKotlinUsesKotlinSyntax(t *testing.T) {
	repo := gradleRepo(t, "build.gradle.kts", "plugins {\n    java\n}\n")

	if _, err := applyJavaGradle(t, repo); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	got := readFile(t, filepath.Join(repo, "build.gradle.kts"))
	want := "buildscript {\n" +
		"    dependencies {\n" +
		"        classpath(\"com.example.buildtools:java-conventions-plugin:1.4.0\")\n" +
		"    }\n" +
		"}\n" +
		"apply(plugin = \"com.example.buildtools.java-conventions\")\n\n" +
		"plugins {\n    java\n}\n"
	if got != want {
		t.Errorf("build.gradle.kts =\n%s\nwant\n%s", got, want)
	}
}

// A repo carrying both gets the Kotlin one, matching the script's ordering.
func TestApplyGradlePrefersKotlinBuildFile(t *testing.T) {
	repo := gradleRepo(t, "build.gradle.kts", "plugins {\n    java\n}\n")
	if err := os.WriteFile(filepath.Join(repo, "build.gradle"), []byte(existingBuildFile), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := applyJavaGradle(t, repo); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if strings.Contains(readFile(t, filepath.Join(repo, "build.gradle")), "buildscript") {
		t.Error("the Groovy build file was edited, want the Kotlin one preferred")
	}
	if !strings.Contains(readFile(t, filepath.Join(repo, "build.gradle.kts")), "buildscript") {
		t.Error("the Kotlin build file was left untouched")
	}
}

func TestApplyGradleIsANoOpOnARepoAlreadyOnThePack(t *testing.T) {
	repo := gradleRepo(t, "build.gradle", existingBuildFile)

	if _, err := applyJavaGradle(t, repo); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	after := readFile(t, filepath.Join(repo, "build.gradle"))

	msg, err := applyJavaGradle(t, repo)
	if err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	if again := readFile(t, filepath.Join(repo, "build.gradle")); again != after {
		t.Errorf("re-running rewrote the build file:\n%s\nwant it unchanged:\n%s", again, after)
	}
	for _, part := range []string{"widget", "java-gradle", "already"} {
		if !strings.Contains(msg, part) {
			t.Errorf("no-op message %q does not mention %q", msg, part)
		}
	}
}

// A repo that pulled the plugin in by hand is already on the convention,
// buildscript block and all — that's a no-op, not a conflict.
func TestApplyGradleTreatsAHandWiredPluginAsAlreadyApplied(t *testing.T) {
	contents := "buildscript {\n    dependencies {\n        classpath 'com.example.buildtools:java-conventions-plugin:1.3.0'\n    }\n}\napply plugin: 'com.example.buildtools.java-conventions'\n"
	repo := gradleRepo(t, "build.gradle", contents)

	if _, err := applyJavaGradle(t, repo); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got := readFile(t, filepath.Join(repo, "build.gradle")); got != contents {
		t.Errorf("build file was rewritten:\n%s", got)
	}
}

func TestApplyGradleRefusesAConflictingBuildscriptBlock(t *testing.T) {
	contents := "buildscript {\n    dependencies {\n        classpath 'com.other:thing:1.0.0'\n    }\n}\n" + existingBuildFile
	repo := gradleRepo(t, "build.gradle", contents)

	_, err := applyJavaGradle(t, repo)
	var manual *conventionpack.ManualEditError
	if !errors.As(err, &manual) {
		t.Fatalf("Apply error = %v, want a *ManualEditError", err)
	}
	if got := readFile(t, filepath.Join(repo, "build.gradle")); got != contents {
		t.Errorf("refused build file was rewritten anyway:\n%s", got)
	}
	if !strings.Contains(err.Error(), "build.gradle") {
		t.Errorf("refusal %q does not name the build file", err)
	}
	want := "build.gradle already has a buildscript {} block.\n" +
		"Add this dependency inside its dependencies {} block by hand, plus the apply line near the top:\n" +
		"  classpath 'com.example.buildtools:java-conventions-plugin:1.4.0'\n" +
		"  apply plugin: 'com.example.buildtools.java-conventions'"
	if got := manual.Instructions(); got != want {
		t.Errorf("Instructions() =\n%s\nwant\n%s", got, want)
	}
}

func TestApplyGradleRefusalUsesKotlinSyntaxForKotlinBuildFiles(t *testing.T) {
	repo := gradleRepo(t, "build.gradle.kts", "buildscript {\n    dependencies {\n    }\n}\n")

	_, err := applyJavaGradle(t, repo)
	var manual *conventionpack.ManualEditError
	if !errors.As(err, &manual) {
		t.Fatalf("Apply error = %v, want a *ManualEditError", err)
	}
	for _, want := range []string{
		"classpath(\"com.example.buildtools:java-conventions-plugin:1.4.0\")",
		"apply(plugin = \"com.example.buildtools.java-conventions\")",
	} {
		if !strings.Contains(manual.Instructions(), want) {
			t.Errorf("instructions %q do not carry %q", manual.Instructions(), want)
		}
	}
}

// An indented buildscript block still counts: the check is "does this file
// already open one", not "does it open one in column zero".
func TestApplyGradleRefusesAnIndentedBuildscriptBlock(t *testing.T) {
	repo := gradleRepo(t, "build.gradle", "  buildscript  {\n}\n")

	if _, err := applyJavaGradle(t, repo); err == nil {
		t.Fatal("Apply over an indented buildscript {} block succeeded, want a refusal")
	}
}

// "buildscript" inside a comment or a longer word is not a block.
func TestApplyGradleIgnoresANonBlockMentionOfBuildscript(t *testing.T) {
	repo := gradleRepo(t, "build.gradle", "// no buildscript {} here\nplugins {\n}\n")

	if _, err := applyJavaGradle(t, repo); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !strings.Contains(readFile(t, filepath.Join(repo, "build.gradle")), "classpath 'com.example") {
		t.Error("a mention of buildscript in a comment blocked the scaffold")
	}
}

func TestApplyGradleRejectsARepoWithNoBuildFile(t *testing.T) {
	repo := t.TempDir()

	_, err := applyJavaGradle(t, repo)
	if err == nil {
		t.Fatal("Apply against a non-Gradle repo succeeded, want an error")
	}
	if !strings.Contains(err.Error(), repo) || !strings.Contains(err.Error(), "build.gradle") {
		t.Errorf("error %q names neither the path nor the build file it looked for", err)
	}
}

// A half-filled pack would otherwise scaffold a coordinate made of empty
// strings into someone's build file.
func TestApplyGradleRejectsAnIncompletePack(t *testing.T) {
	repo := gradleRepo(t, "build.gradle", existingBuildFile)
	pack := conventionpack.Pack{Name: "half-baked", BuildTool: "gradle"}
	pack.Artifact.Group = "com.example"

	_, err := conventionpack.Apply(pack, conventionpack.Target{Name: "widget", Path: repo})
	if err == nil {
		t.Fatal("Apply with an incomplete pack succeeded, want an error")
	}
	for _, want := range []string{"half-baked", "artifact.id", "artifact.version", "gradle.plugin_id"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name the missing field %q", err, want)
		}
	}
	if got := readFile(t, filepath.Join(repo, "build.gradle")); got != existingBuildFile {
		t.Errorf("build file was written from an incomplete pack:\n%s", got)
	}
}

// The build file keeps whatever mode it had; the script's write-and-rename
// left it with the umask default instead.
func TestApplyGradlePreservesBuildFileMode(t *testing.T) {
	repo := gradleRepo(t, "build.gradle", existingBuildFile)
	path := filepath.Join(repo, "build.gradle")
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}

	if _, err := applyJavaGradle(t, repo); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Errorf("build file mode = %v, want 0640", got)
	}
}
