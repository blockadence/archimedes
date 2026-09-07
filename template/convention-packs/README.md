# Convention packs

A convention pack is a named, shared build/lint/static-analysis convention
that a repo can declare it follows. A repo declares one by name (the
`convention_pack` field in `repos.yaml`); the actual definition — language,
build tool, and the shared artifact/version that carries the config — lives
here as one YAML file per pack, named `<pack-name>.yaml`.

This is documentation + one-time-scaffold only. Archimedes never distributes
or re-syncs raw config files into a repo on an ongoing basis — each repo
pulls the shared artifact via its own build tool's normal dependency
mechanism (a published Gradle plugin, a Maven shared-config artifact, an npm
`eslint-config-*` package, ...), the same way it pulls any other dependency.

## Shape

Every pack file has this generic core, regardless of language:

```yaml
name: <pack-name>              # matches the filename, and repos.yaml's convention_pack value
language: <language>           # e.g. java, javascript, python, go
build_tool: <build-tool>       # e.g. gradle, maven, npm, pip, go
description: <one line>
artifact:
  group: <group/namespace>     # e.g. Maven/Gradle group, npm scope, PyPI project owner
  id: <artifact/package name>
  version: <version>
```

A `build_tool`-named block below `artifact` carries whatever extra detail
that build tool's own dependency mechanism needs to actually pull the
artifact in (for example Gradle's plugin ID, applied by binary coordinate so
it resolves a privately-published artifact without assuming Plugin Portal
registration). See `java-gradle.yaml` for a worked example.

## Adding a pack for a new language/build tool

1. Add `<pack-name>.yaml` here following the shape above.
2. Teach `archimedes apply-convention-pack` how to scaffold that
   `build_tool`. It dispatches on the field, so this is one new case: a
   scaffolder registered in the CLI's `internal/conventionpack` (`gradle.go`
   is the worked example to copy). Nothing above that dispatch should ever
   assume Java, Gradle, or any one language/build tool.
3. Point a repo at it by setting `convention_pack: <pack-name>` in its
   `repos.yaml` entry, then run `archimedes apply-convention-pack <repo>`.
