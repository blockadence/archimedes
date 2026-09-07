package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testPack = `name: java-gradle
language: java
build_tool: gradle
artifact:
  group: com.example.buildtools
  id: java-conventions-plugin
  version: "1.4.0"
gradle:
  plugin_id: com.example.buildtools.java-conventions
`

// conventionInstance builds an instance root whose one repo declares pack
// (empty for none) and is cloned unless cloned is false, with java-gradle
// defined in the instance's own convention-packs/.
func conventionInstance(t *testing.T, pack string, cloned bool) string {
	t.Helper()
	root := t.TempDir()

	packs := filepath.Join(root, "convention-packs")
	if err := os.MkdirAll(packs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packs, "java-gradle.yaml"), []byte(testPack), 0o644); err != nil {
		t.Fatal(err)
	}

	reposYAML := "repos:\n  - name: widget\n    path: ./widget\n    base_branch: main\n    convention_pack: " + pack + "\n"
	if err := os.WriteFile(filepath.Join(root, "repos.yaml"), []byte(reposYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	if cloned {
		repo := filepath.Join(root, "widget")
		if err := os.MkdirAll(repo, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repo, "build.gradle"), []byte("plugins {\n    id 'java'\n}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	return root
}

func buildFile(t *testing.T, root string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "widget", "build.gradle"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestApplyConventionPackWiresUpTheDeclaredPack(t *testing.T) {
	root := conventionInstance(t, "java-gradle", true)

	var out bytes.Buffer
	if err := runApplyConventionPack(&out, &bytes.Buffer{}, root, "widget"); err != nil {
		t.Fatalf("runApplyConventionPack: %v", err)
	}

	after := buildFile(t, root)
	if !strings.Contains(after, "classpath 'com.example.buildtools:java-conventions-plugin:1.4.0'") {
		t.Errorf("build.gradle did not gain the pack's coordinate:\n%s", after)
	}
	if !strings.Contains(out.String(), "Review the diff") {
		t.Errorf("output %q does not tell the operator to review the diff", out.String())
	}
}

func TestApplyConventionPackIsANoOpTheSecondTime(t *testing.T) {
	root := conventionInstance(t, "java-gradle", true)

	var first bytes.Buffer
	if err := runApplyConventionPack(&first, &bytes.Buffer{}, root, "widget"); err != nil {
		t.Fatalf("first run: %v", err)
	}
	after := buildFile(t, root)

	var second bytes.Buffer
	if err := runApplyConventionPack(&second, &bytes.Buffer{}, root, "widget"); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if again := buildFile(t, root); again != after {
		t.Errorf("re-running rewrote build.gradle:\n%s", again)
	}
	if !strings.Contains(second.String(), "already on java-gradle") {
		t.Errorf("second run said %q, want it to report the repo as already on the pack", second.String())
	}
}

func TestApplyConventionPackRefusesAConflictingBuildFile(t *testing.T) {
	root := conventionInstance(t, "java-gradle", true)
	path := filepath.Join(root, "widget", "build.gradle")
	contents := "buildscript {\n    dependencies {\n        classpath 'com.other:thing:1.0.0'\n    }\n}\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	err := runApplyConventionPack(&out, &errOut, root, "widget")
	if err == nil {
		t.Fatal("run over a conflicting build file succeeded, want a refusal")
	}
	for _, want := range []string{
		"already has a buildscript {} block",
		"by hand",
		"classpath 'com.example.buildtools:java-conventions-plugin:1.4.0'",
		"apply plugin: 'com.example.buildtools.java-conventions'",
	} {
		if !strings.Contains(errOut.String(), want) {
			t.Errorf("hand-edit instructions %q do not carry %q", errOut.String(), want)
		}
	}
	if got := buildFile(t, root); got != contents {
		t.Errorf("refused build file was rewritten anyway:\n%s", got)
	}
}

func TestApplyConventionPackRejectsAnUnknownRepo(t *testing.T) {
	root := conventionInstance(t, "java-gradle", true)

	err := runApplyConventionPack(&bytes.Buffer{}, &bytes.Buffer{}, root, "gadget")
	if err == nil {
		t.Fatal("run against an unlisted repo succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "gadget") || !strings.Contains(err.Error(), "repos.yaml") {
		t.Errorf("error %q does not name the repo and where it should be listed", err)
	}
}

func TestApplyConventionPackRejectsARepoThatIsNotClonedYet(t *testing.T) {
	root := conventionInstance(t, "java-gradle", false)

	err := runApplyConventionPack(&bytes.Buffer{}, &bytes.Buffer{}, root, "widget")
	if err == nil {
		t.Fatal("run against an uncloned repo succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "bootstrap") {
		t.Errorf("error %q does not point at bootstrap", err)
	}
}

func TestApplyConventionPackRejectsARepoDeclaringNoPack(t *testing.T) {
	root := conventionInstance(t, "", true)

	err := runApplyConventionPack(&bytes.Buffer{}, &bytes.Buffer{}, root, "widget")
	if err == nil {
		t.Fatal("run against a repo with no convention_pack succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "convention_pack") {
		t.Errorf("error %q does not name the unset field", err)
	}
}

// The pack name comes from repos.yaml and the definition from the
// instance's convention-packs/ — naming one that isn't defined there fails
// against that directory rather than falling back elsewhere.
func TestApplyConventionPackRejectsAnUndefinedPack(t *testing.T) {
	root := conventionInstance(t, "rust-cargo", true)

	err := runApplyConventionPack(&bytes.Buffer{}, &bytes.Buffer{}, root, "widget")
	if err == nil {
		t.Fatal("run against an undefined pack succeeded, want an error")
	}
	want := filepath.Join(root, "convention-packs", "rust-cargo.yaml")
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error %q does not name %s", err, want)
	}
}
