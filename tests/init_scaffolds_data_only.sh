#!/usr/bin/env bash
# A scaffolded instance is data and nothing else: a manifest, dossier and
# work directories, the scaffolding it owns from there on, and an empty
# drivers/ shelf for the drivers it comes to own. The tooling that acts on
# it is the globally installed `archimedes` binary -- and so are the drivers
# it ships -- so not one file of an instance is a program, and there is
# nothing to re-vendor later.
#
# This is the guarantee that decays quietly: adding one convenience script
# back to template/ would go unnoticed until an instance somewhere had a
# stale copy of it.
#
# Scaffolding is itself the binary's job now (`archimedes init`), so this
# runs it the way a first-time user meets it: a copy of the binary on its
# own, with no checkout of this repo in reach.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/helpers.sh"

ROOT="$(cd "$HERE/.." && pwd)"

build_archimedes || exit 1

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

# Scaffolding an instance makes a commit, so this file needs an identity.
# It gets one here rather than from the CI workflow -- see isolate_git in
# tests/gitfixture.sh for why that distinction matters.
isolate_git "$WORK"

echo "a scaffolded instance holds data only:"

# The binary is copied out of the repo and run from elsewhere, so a pass
# here can't be coming from a checkout it found next to itself.
INSTALLED="$WORK/bin/archimedes"
mkdir -p "$WORK/bin"
cp "$ARCHIMEDES_BIN" "$INSTALLED"

if ( cd "$WORK" && "$INSTALLED" init widgets "$WORK" ) >"$WORK/init.log" 2>&1; then
  pass "the installed binary scaffolds an instance with no checkout in reach"
else
  fail "the installed binary scaffolds an instance with no checkout in reach"
  cat "$WORK/init.log" >&2
  report
  exit 1
fi

INSTANCE="$WORK/widgets"

assert_dir_missing "$INSTANCE/scripts" "no scripts/ directory is vendored into the instance"

# Nothing at all, not even under drivers/. The drivers an instance can run
# ride in the binary; its own drivers/ starts empty, holding only whatever
# the operator later writes or adopts into it.
strays="$(cd "$INSTANCE" && find . -path ./.git -prune -o -type f \
  \( -name '*.sh' -o -perm -u+x \) -print | sort | tr '\n' ' ')"
assert_eq "$strays" "" "nothing executable or shell-shaped is copied into the instance at all"

for path in repos.yaml WORKSPACE-MAP.md AGENTS.md README.md; do
  assert_file_exists "$INSTANCE/$path" "the instance carries its $path"
done
for path in repos work drivers scaffolding convention-packs; do
  assert_dir_exists "$INSTANCE/$path" "the instance carries its $path/"
done

# A driver seeded into an instance would be a driver no fix could ever
# reach again, which is precisely what this whole arrangement exists to
# avoid. The shelf arrives empty.
seeded="$(find "$INSTANCE/drivers" -mindepth 1 -maxdepth 1 -type d | tr '\n' ' ')"
assert_eq "$seeded" "" "no driver is seeded into the instance, so none of them is beyond the reach of a fix"

# The instance owns its drivers and scaffolding from here, so there is no
# refresh-from-Archimedes step in either direction. Asserted as the absence
# of the thing itself rather than of the words: the README names the retired
# script on purpose, telling operators of older instances what became of it.
revendor="$(find "$ROOT" -name .git -prune -o -name 'update-from-archimedes*' -print | tr '\n' ' ')"
assert_eq "$revendor" "" "the re-vendoring script is gone from the repo"
assert_file_missing "$INSTANCE/scripts/update-from-archimedes.sh" \
  "and a fresh instance is not handed one"

# Scaffolding from a checkout was the same job done a second way, and a
# second way drifts. There is one now.
assert_file_missing "$ROOT/scripts/init.sh" "scaffolding from a checkout is gone from the repo"

commits="$(git -C "$INSTANCE" rev-list --count HEAD)"
assert_eq "$commits" "1" "the instance starts on its own fresh history"

# Refusing an occupied destination is what keeps a mistyped name from
# merging a template into somebody's existing directory.
mkdir -p "$WORK/taken"
if err="$( "$INSTALLED" init taken "$WORK" 2>&1 >/dev/null )"; then
  fail "init refuses a destination that already exists"
else
  assert_contains "$err" "already exists" "init refuses a destination that already exists"
fi

report
