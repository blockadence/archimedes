#!/usr/bin/env bash
# The tool is installed two ways and has to be the same tool both times.
#
# Standalone it is `archimedes`, built or `go install`ed, and the rest of
# this suite covers that one. This covers the other: the binary a release
# carries, which `gh extension install` downloads and runs as
# `gh archimedes`. Three things about that arrangement can rot without
# anything else noticing --
#
#   - the asset names a release publishes, which gh matches against the
#     platform it is installing onto and silently finds nothing if they
#     drift;
#   - the version the artifact reports, which is the only evidence of which
#     build it is once it is sitting on a machine with no checkout beside
#     it;
#   - whether it behaves the same when gh is the one running it.
#
# So this drives the real release build script, not a copy of its recipe.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/helpers.sh"

ROOT="$(cd "$HERE/.." && pwd)"
TAG="v9.9.9-test"
PLATFORM="$(go env GOOS)-$(go env GOARCH)"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

# Scaffolding an instance makes a commit, so this file needs an identity.
# It gets one here rather than from the CI workflow -- see isolate_git in
# tests/gitfixture.sh for why that distinction matters.
isolate_git "$WORK"

echo "the release build produces an installable, self-describing binary:"

# Only the platform this test can actually run. Cross-compiling the other
# eleven would prove nothing here that the release itself doesn't.
if ( cd "$ROOT" && ARCHIMEDES_RELEASE_PLATFORMS="$PLATFORM" \
      ARCHIMEDES_RELEASE_DIST="$WORK/dist" \
      ./.github/release-build.sh "$TAG" ) >"$WORK/build.log" 2>&1; then
  pass "the release build script runs"
else
  fail "the release build script runs"
  cat "$WORK/build.log" >&2
  report
  exit 1
fi

ASSET="$WORK/dist/gh-archimedes_${TAG}_${PLATFORM}"
assert_file_exists "$ASSET" "it builds an asset for this platform"

# gh finds the right download by matching the tail of an asset's name
# against <os>-<arch>. Anything appended after that -- a .tar.gz, a
# checksum suffix, a rename -- makes the extension uninstallable here while
# the release still looks fine on GitHub.
case "$(basename "$ASSET")" in
  *"$PLATFORM") pass "the asset name ends with the platform, which is how gh matches it" ;;
  *) fail "the asset name ends with the platform, which is how gh matches it" ;;
esac

# The point of the whole exercise: a downloaded artifact answers "which
# build is this?" with the tag it was cut from, not with a placeholder.
assert_eq "$("$ASSET" --version)" "archimedes version $TAG" \
  "the released binary reports the tag it was built from"

echo ""
echo "the same binary behaves the same however it is invoked:"

# gh installs an extension as gh-<name> and runs that, so the name on disk
# is not the name the operator typed. Nothing may depend on either.
STANDALONE="$WORK/bin/archimedes"
EXTENSION="$WORK/ext/gh-archimedes"
mkdir -p "$WORK/bin" "$WORK/ext"
cp "$ASSET" "$STANDALONE"
cp "$ASSET" "$EXTENSION"

# Scaffolding an instance is the subcommand with the most to go wrong here:
# it is the one that runs before an instance exists, and it has to build the
# whole thing out of what the binary carries, with no checkout in reach.
( cd "$WORK" && "$STANDALONE" init direct "$WORK" ) >"$WORK/direct.log" 2>&1
direct_status=$?

# GH_EXTENSION=1 is what gh sets when it dispatches an extension, and the
# arguments arrive exactly as typed after the extension name.
( cd "$WORK" && GH_EXTENSION=1 "$EXTENSION" init dispatched "$WORK" ) >"$WORK/dispatched.log" 2>&1
dispatched_status=$?

assert_eq "$direct_status" "0" "it scaffolds an instance when run standalone"
assert_eq "$dispatched_status" "0" "it scaffolds an instance when gh dispatches it"

# Compared as trees rather than as output: what an operator ends up with is
# the instance, and identical logs over different scaffolding would be the
# worse failure to miss. The instance's own git history differs by commit
# hash and timestamp, so that is excluded and its presence asserted apart.
direct_tree="$(cd "$WORK/direct" && find . -path ./.git -prune -o -print | sort)"
dispatched_tree="$(cd "$WORK/dispatched" && find . -path ./.git -prune -o -print | sort)"
assert_eq "$dispatched_tree" "$direct_tree" "both routes scaffold the same instance"
assert_eq "$(git -C "$WORK/dispatched" rev-list --count HEAD)" "1" \
  "and the one gh scaffolded got its own history too"

# The one difference between the two runs, and it is the intended one: what
# each tells the operator to run next. init's parting line is the first
# thing anybody sees, and under an extension install `archimedes bootstrap`
# is a command they have not got.
assert_contains "$(cat "$WORK/direct.log")" "&& archimedes bootstrap" \
  "standalone points at the next command as archimedes"
assert_contains "$(cat "$WORK/dispatched.log")" "&& gh archimedes bootstrap" \
  "under gh it points at the same step as gh archimedes"

echo ""
echo "help names the command the operator actually typed:"

# The one thing gh cannot forward is what it was called. An extension whose
# help says `archimedes spawn` is telling someone to run a command they have
# not got installed under that name.
standalone_help="$("$STANDALONE" spawn --help 2>&1)"
assert_contains "$standalone_help" "archimedes spawn" \
  "standalone help names the binary"
assert_not_contains "$standalone_help" "gh archimedes" \
  "and does not tell a standalone install to go through gh"

dispatched_help="$(GH_EXTENSION=1 "$EXTENSION" spawn --help 2>&1)"
assert_contains "$dispatched_help" "gh archimedes spawn" \
  "help under gh names the gh command"

report
