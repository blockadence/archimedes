package conventionpack_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blockadence/gh-archimedes/internal/conventionpack"
)

// javaGradlePack is template/convention-packs/java-gradle.yaml, the shipped
// pack this port has to keep working.
const javaGradlePack = `name: java-gradle
language: java
build_tool: gradle
description: >
  Shared Checkstyle/Spotless/JaCoCo/Error Prone conventions for Java repos,
  applied as a Gradle convention plugin from one published artifact.
artifact:
  group: com.example.buildtools
  id: java-conventions-plugin
  version: "1.4.0"
gradle:
  plugin_id: com.example.buildtools.java-conventions
`

// packsDir writes one pack file into a fresh convention-packs directory.
func packsDir(t *testing.T, name, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name+".yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLoadReadsPackDefinition(t *testing.T) {
	dir := packsDir(t, "java-gradle", javaGradlePack)

	pack, err := conventionpack.Load(dir, "java-gradle")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if pack.Name != "java-gradle" || pack.Language != "java" || pack.BuildTool != "gradle" {
		t.Errorf("got %+v, want the java-gradle pack's generic core", pack)
	}
	if pack.Artifact != (conventionpack.Artifact{
		Group:   "com.example.buildtools",
		ID:      "java-conventions-plugin",
		Version: "1.4.0",
	}) {
		t.Errorf("Artifact = %+v, want the pack file's artifact block", pack.Artifact)
	}
}

// The build-tool-named block stays undecoded until a scaffolder asks for
// it in its own shape, so a pack for a build tool nothing supports yet
// still loads, and a new one costs no field on Pack.
func TestDecodeToolHandsTheBuildToolsOwnBlockToTheScaffolder(t *testing.T) {
	pack, err := conventionpack.Load(packsDir(t, "java-gradle", javaGradlePack), "java-gradle")
	if err != nil {
		t.Fatal(err)
	}

	var cfg struct {
		PluginID string `yaml:"plugin_id"`
	}
	if err := pack.DecodeTool(&cfg); err != nil {
		t.Fatalf("DecodeTool: %v", err)
	}
	if got, want := cfg.PluginID, "com.example.buildtools.java-conventions"; got != want {
		t.Errorf("plugin_id = %q, want %q", got, want)
	}
}

// A pack for a build tool this binary has no scaffolder for still parses:
// its block is carried, not rejected.
func TestLoadKeepsAnUnrecognizedBuildToolsBlock(t *testing.T) {
	body := "name: rust-cargo\nbuild_tool: cargo\nartifact:\n  group: acme\n  id: lints\n  version: \"2.0.0\"\ncargo:\n  feature: strict\n"
	pack, err := conventionpack.Load(packsDir(t, "rust-cargo", body), "rust-cargo")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	var cfg struct {
		Feature string `yaml:"feature"`
	}
	if err := pack.DecodeTool(&cfg); err != nil {
		t.Fatalf("DecodeTool: %v", err)
	}
	if cfg.Feature != "strict" {
		t.Errorf("cargo block decoded to %+v, want its feature carried through", cfg)
	}
}

// A pack with no block for its own build tool leaves the scaffolder's
// config zeroed, so the scaffolder's own missing-field check is what
// reports it — by field name, once.
func TestDecodeToolIsANoOpWhenThePackHasNoBlockForItsBuildTool(t *testing.T) {
	pack, err := conventionpack.Load(packsDir(t, "bare", "name: bare\nbuild_tool: gradle\n"), "bare")
	if err != nil {
		t.Fatal(err)
	}

	cfg := struct {
		PluginID string `yaml:"plugin_id"`
	}{PluginID: "untouched"}
	if err := pack.DecodeTool(&cfg); err != nil {
		t.Fatalf("DecodeTool: %v", err)
	}
	if cfg.PluginID != "untouched" {
		t.Errorf("PluginID = %q, want it left alone", cfg.PluginID)
	}
}

func TestLoadUnknownPackNamesTheMissingFile(t *testing.T) {
	dir := packsDir(t, "java-gradle", javaGradlePack)

	_, err := conventionpack.Load(dir, "rust-cargo")
	if err == nil {
		t.Fatal("Load of an undefined pack succeeded, want an error")
	}
	want := filepath.Join(dir, "rust-cargo.yaml")
	if !strings.Contains(err.Error(), "rust-cargo") || !strings.Contains(err.Error(), want) {
		t.Errorf("error %q names neither the pack nor %s", err, want)
	}
}

func TestLoadMalformedPackBlamesTheFileNotTheName(t *testing.T) {
	dir := packsDir(t, "java-gradle", "name: java-gradle\n  build_tool: [oops\n")

	_, err := conventionpack.Load(dir, "java-gradle")
	if err == nil {
		t.Fatal("Load of a malformed pack succeeded, want an error")
	}
	if strings.Contains(err.Error(), "unknown convention pack") {
		t.Errorf("error %q blames the pack name, want it to blame the file's contents", err)
	}
}

// A pack whose build_tool nothing scaffolds yet must say so, and say where
// the missing case goes, rather than silently doing nothing.
func TestApplyUnknownBuildToolPointsAtWhereTheCaseGoes(t *testing.T) {
	pack := conventionpack.Pack{Name: "rust-cargo", BuildTool: "cargo"}

	_, err := conventionpack.Apply(pack, conventionpack.Target{Name: "widget", Path: t.TempDir()})
	if err == nil {
		t.Fatal("Apply with an unscaffolded build tool succeeded, want an error")
	}
	for _, want := range []string{"cargo", "rust-cargo", "convention-packs/README.md"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// Every build tool's scaffolder is reached through the same dispatch, so a
// second one is a registration rather than a rewrite of Apply.
func TestApplyDispatchesOnBuildTool(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "build.gradle"), []byte("plugins {\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pack := loadJavaGradle(t)

	if _, err := conventionpack.Apply(pack, conventionpack.Target{Name: "widget", Path: repo}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !strings.Contains(readFile(t, filepath.Join(repo, "build.gradle")), "buildscript {") {
		t.Error("gradle pack applied through Apply left the build file untouched")
	}
}

// The refusal path is a typed error so callers can tell "this needs a human"
// apart from "this broke".
func TestManualEditErrorIsDistinguishable(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "build.gradle"), []byte("buildscript {\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := conventionpack.Apply(loadJavaGradle(t), conventionpack.Target{Name: "widget", Path: repo})
	var manual *conventionpack.ManualEditError
	if !errors.As(err, &manual) {
		t.Fatalf("Apply error = %v, want a *ManualEditError", err)
	}
}

func loadJavaGradle(t *testing.T) conventionpack.Pack {
	t.Helper()
	pack, err := conventionpack.Load(packsDir(t, "java-gradle", javaGradlePack), "java-gradle")
	if err != nil {
		t.Fatal(err)
	}
	return pack
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
