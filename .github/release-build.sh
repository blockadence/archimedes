#!/usr/bin/env bash
# Builds the binaries a release carries, one per platform, into dist/ for
# cli/gh-extension-precompile to attach to the release.
#
# The action builds Go extensions itself and needs no script -- this one
# exists for two things its own build cannot do:
#
#   - Our main package is ./cmd/archimedes, not the repository root. The
#     root is a library package (template.go, carrying the embedded trees),
#     so the default build has nothing to compile.
#   - The version has to be linked in. A downloaded release artifact has no
#     checkout beside it, so the tag is the only evidence of which build it
#     is; the action's `go_build_options` is passed to `go build` as a
#     single word, which cannot carry both a -ldflags value and a package.
#
# Everything else here mirrors the action's own build deliberately: the same
# platform list, the same `go tool dist list` guard, the same -trimpath and
# -s -w. Divergence there would be an accident, not a decision.
#
# Usage: release-build.sh <tag>   (the action passes the release tag)
set -euo pipefail

tag="${1:?usage: release-build.sh <tag>}"

# Both overridable for tests/gh_extension_packaging.sh, which builds the one
# platform it is going to run rather than cross-compiling twelve, and puts
# it in its own temp directory rather than in the dist/ a release uses --
# so a test run can neither clobber nor leave behind an operator's build.
# The action passes neither, and gets the release's own defaults.
dist="${ARCHIMEDES_RELEASE_DIST:-dist}"
platforms="${ARCHIMEDES_RELEASE_PLATFORMS:-
  darwin-amd64 darwin-arm64
  freebsd-386 freebsd-amd64 freebsd-arm64
  linux-386 linux-amd64 linux-arm linux-arm64
  windows-386 windows-amd64 windows-arm64
}"

# The one thing a release artifact knows about itself that nothing else can
# tell it later. internal/version reads this back.
ldflags="-s -w -X github.com/blockadence/gh-archimedes/internal/version.stamped=${tag}"

supported="$(go tool dist list)"

mkdir -p "$dist"
for p in $platforms; do
  goos="${p%-*}"
  goarch="${p#*-}"

  if ! grep -qx "${goos}/${goarch}" <<<"$supported"; then
    echo "warning: skipping platform $p, unsupported by this Go toolchain" >&2
    continue
  fi

  ext=""
  if [ "$goos" = "windows" ]; then
    ext=".exe"
  fi

  # gh picks an asset out of a release by matching the tail of its name
  # against the platform it is installing onto, so <os>-<arch> has to be
  # last. The rest is for whoever downloads one by hand.
  out="$dist/gh-archimedes_${tag}_${p}${ext}"

  echo "building $out"
  GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
    go build -trimpath -ldflags="$ldflags" -o "$out" ./cmd/archimedes
done
