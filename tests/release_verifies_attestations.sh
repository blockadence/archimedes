#!/usr/bin/env bash
# A release run has to prove the attestation exists before anyone can
# install what it attests to.
#
# tests/release_provenance.sh reads release.yml and pins that we *asked*
# for attestations -- the input is set, the grants are scoped, the verify
# command is documented. Every one of those assertions is about a file, and
# all of them pass on a repository that has never produced an attestation.
# The way that goes wrong is silent: the action gates its attest step on a
# string comparison against a composite-action input, and anything that
# stops producing that exact string skips the step instead of failing. The
# run is green, twelve binaries ship, and the first person to find out is an
# operator whose `gh attestation verify` reports no attestation -- which is
# indistinguishable, to them, from the tampering that command exists to
# catch.
#
# `.github/release-verify.sh` closes that by running the operator's own
# command against the built assets before the drafted release is promoted,
# and this file drives that script against a stub `gh`. The stub is what
# makes the failure paths reachable: an attestation that is missing, a
# lookup that is not readable yet, a dist/ that a glob expanded to nothing,
# and a draft flag that quietly stopped applying are all outcomes you cannot
# ask a real release to produce on demand -- and "check it fails on purpose
# once" is worth more as a test that keeps checking than as something
# somebody did by hand in a branch that is gone.
#
# What this cannot prove, and must not be read as claiming: that GitHub
# mints a real attestation or that its API serves one back. It proves that
# when the answer is no, nothing gets promoted.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/helpers.sh"

ROOT="$(cd "$HERE/.." && pwd)"
VERIFY="$ROOT/.github/release-verify.sh"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

REPO="blockadence/gh-archimedes"
TAG="v9.9.9-test"
DIST="$WORK/dist"
STUB_BIN="$WORK/bin"
mkdir -p "$STUB_BIN"

# Stands in for the `gh` on the runner. Records every invocation -- what
# the script asks of GitHub, and in what order, is the whole of what this
# file is checking -- then answers the way the GH_STUB_* knobs say.
#
# Deliberately not a mock of gh's output beyond the one field the script
# reads: a stub that reproduced `gh attestation verify`'s report would be
# asserting on our imitation of it rather than on our use of it.
cat > "$STUB_BIN/gh" <<'STUB'
#!/usr/bin/env bash
set -uo pipefail
printf '%s\n' "$*" >> "$GH_STUB_LOG"
case "${1:-} ${2:-}" in
  "release view")
    # gh release view <tag> --repo <repo> --json isDraft --jq .isDraft
    echo "${GH_STUB_IS_DRAFT:-true}"
    ;;
  "release edit")
    [ "${GH_STUB_EDIT_FAILS:-0}" = "1" ] && { echo "gh: could not edit release" >&2; exit 1; }
    echo "https://github.com/$GH_STUB_REPO/releases/tag/$3"
    ;;
  "attestation verify")
    asset="$3"
    base="$(basename "$asset")"
    # An asset GitHub has no attestation for, however long you wait.
    case "${GH_STUB_MISSING:-}" in
      "") ;;
      *) case "$base" in
           *"$GH_STUB_MISSING"*)
             echo "gh: no attestation found for $base" >&2; exit 1 ;;
         esac ;;
    esac
    # A lookup that is not readable yet: the first try at each asset fails,
    # every try after it succeeds. Per asset rather than per run, so a
    # retry budget shared across assets can be told from one spent on the
    # first asset alone.
    if [ "${GH_STUB_FIRST_TRY_FAILS:-0}" = "1" ]; then
      seen="$GH_STUB_STATE/$base"
      if [ ! -e "$seen" ]; then
        : > "$seen"
        echo "gh: failed to fetch attestations: 404" >&2; exit 1
      fi
    fi
    echo "Loaded 1 attestation from GitHub API"
    ;;
  *)
    echo "gh: unexpected stub invocation: $*" >&2; exit 1 ;;
esac
exit 0
STUB
chmod +x "$STUB_BIN/gh"

export PATH="$STUB_BIN:$PATH"
export GH_STUB_LOG="$WORK/gh.log"
export GH_STUB_STATE="$WORK/state"
export GH_STUB_REPO="$REPO"

# The assets a release build leaves behind, named the way
# .github/release-build.sh names them -- three platforms rather than twelve,
# which is enough to tell "verified the first one" from "verified all of
# them" and is what the loop under test actually varies over.
ASSETS="gh-archimedes_${TAG}_linux-amd64
gh-archimedes_${TAG}_darwin-arm64
gh-archimedes_${TAG}_windows-amd64.exe"

# One run of the script under a fresh stub. Every knob is cleared here
# rather than by each case, so a case that forgets to set one gets the
# healthy default instead of the previous case's failure. STATUS and RUN
# are what the assertions read.
run_verify() { # [dist-dir]
  local dist="${1:-$DIST}"
  rm -rf "$GH_STUB_STATE"; mkdir -p "$GH_STUB_STATE"
  : > "$GH_STUB_LOG"
  # Zero delay: the wait between retries is the script's answer to
  # propagation, not something this file should sit through.
  ARCHIMEDES_RELEASE_VERIFY_DELAY=0 \
  ARCHIMEDES_RELEASE_VERIFY_RETRIES="${RETRIES:-5}" \
  GH_STUB_IS_DRAFT="${IS_DRAFT:-true}" \
  GH_STUB_MISSING="${MISSING:-}" \
  GH_STUB_FIRST_TRY_FAILS="${FIRST_TRY_FAILS:-0}" \
  GH_STUB_EDIT_FAILS="${EDIT_FAILS:-0}" \
    "$VERIFY" "$dist" "$REPO" "$TAG" >"$WORK/run.log" 2>&1
  STATUS=$?
  RUN="$(cat "$WORK/run.log")"
  GH_CALLS="$(cat "$GH_STUB_LOG")"
}

fresh_dist() {
  rm -rf "$DIST"; mkdir -p "$DIST"
  while IFS= read -r a; do
    [ -n "$a" ] && echo "not really a binary" > "$DIST/$a"
  done <<< "$ASSETS"
}

# The promotion is the act everything here gates, so "did it happen" is
# asked of the stub's log rather than inferred from an exit status.
assert_promoted() { # <label>
  assert_contains "$GH_CALLS" "release edit $TAG" "$1"
}
assert_not_promoted() { # <label>
  assert_not_contains "$GH_CALLS" "release edit" "$1"
}

assert_file_exists "$VERIFY" "there is a script the release run checks itself with"
[ -x "$VERIFY" ] && pass "and the workflow can execute it" || fail "and the workflow can execute it"

echo ""
echo "a release whose assets all verify is published:"

fresh_dist
run_verify
assert_eq "$STATUS" "0" "the check passes when every asset has an attestation"
assert_promoted "and the drafted release is promoted out of draft"

# Every asset, not just the first. A loop that checked one and reported for
# twelve would pass every assertion above and is the plausible way to write
# this.
while IFS= read -r a; do
  [ -n "$a" ] || continue
  assert_contains "$GH_CALLS" "attestation verify $DIST/$a --repo $REPO" \
    "it runs the operator's own command against $a"
done <<< "$ASSETS"

# ...and it is the operator's command, not an inspection of our own side of
# it. A check that read the workflow, or the attest step's exit status,
# would not fail for the reasons an operator's does.
assert_contains "$GH_CALLS" "--repo $REPO" \
  "and scopes the check to this repository, the way the documented one does"

echo ""
echo "a release whose assets do not verify is not published:"

fresh_dist
MISSING="darwin-arm64" run_verify
assert_eq "$STATUS" "1" "an asset with no attestation fails the run"
assert_not_promoted "and nothing is promoted, so nothing unverified is installable"
assert_contains "$RUN" "darwin-arm64" "and the failure names the asset that could not be verified"

echo ""
echo "a check that cannot fail is not a check:"

# The hazard the issue names first: a glob that expands to nothing leaves
# the loop running zero times and the step reporting success. This is the
# one failure mode that would restore exactly the silence being fixed.
rm -rf "$DIST"; mkdir -p "$DIST"
run_verify
assert_eq "$STATUS" "1" "an empty dist/ fails rather than verifying nothing"
assert_not_promoted "and an empty dist/ promotes nothing"
assert_not_contains "$GH_CALLS" "attestation verify" \
  "and it says so without asking GitHub about assets that are not there"

run_verify "$WORK/no-such-dist"
assert_eq "$STATUS" "1" "a missing dist/ fails too"
assert_not_promoted "and a missing dist/ promotes nothing"

# The other half of "nothing unverified is installable" is that the release
# was never installable to begin with. `draft_release` is gated inside the
# action on the same kind of string comparison as the attest step, so the
# script confirms the draft applied rather than trusting that it did --
# without this, a draft flag that quietly stopped working would leave the
# assets public for the whole length of the check.
fresh_dist
IS_DRAFT="false" run_verify
assert_eq "$STATUS" "1" "a release that is already public fails the check"
assert_not_contains "$GH_CALLS" "attestation verify" \
  "and it stops before verifying, because the answer no longer gates anything"

echo ""
echo "a lookup that is not readable yet is waited on, within a budget:"

# The race worth taking seriously: `gh attestation verify` asks GitHub's
# API for the attestation by digest, seconds after the same run created it.
# If that read is not immediately consistent, a check with no retry fails a
# few percent of good releases -- and the fix people learn ("re-run it") is
# also the fix for a real failure, which is worse than not having the check.
fresh_dist
FIRST_TRY_FAILS=1 RETRIES=3 run_verify
assert_eq "$STATUS" "0" "a lookup that succeeds on a retry passes"
assert_promoted "and the release is promoted once it does"

# The budget is for the run, not for each asset. Per-asset retries would
# make a genuinely missing attestation take twelve full waits to report,
# and this is the assertion that pins which one it is: three assets each
# needing one retry need three from the budget.
fresh_dist
FIRST_TRY_FAILS=1 RETRIES=2 run_verify
assert_eq "$STATUS" "1" "the retries are one budget shared across the assets, not one each"
assert_not_promoted "and an exhausted budget promotes nothing"

fresh_dist
MISSING="darwin-arm64" RETRIES=0 run_verify
assert_eq "$STATUS" "1" "and waiting is never the way a missing attestation passes"
assert_not_promoted "and still promotes nothing"

echo ""
echo "the publish either finishes or is red:"

# A promotion that fails leaves a draft nobody promoted, which is the
# failure this design chose. What it must not leave is a green run: the
# tag would look released and nothing would be installable.
fresh_dist
EDIT_FAILS=1 run_verify
assert_eq "$STATUS" "1" "a promotion that fails fails the run"

echo ""
echo "the check is not quietly disarmed:"

# The three ways a step like this stays in place while reporting success
# regardless. Read from the source, because none of them changes what the
# passing cases above do.
src="$(cat "$VERIFY")"
assert_not_contains "$src" "|| true" "no failure is swallowed with || true"
assert_not_contains "$src" "set +e" "errors are not turned off partway through"
assert_contains "$src" "set -euo pipefail" "and the script exits on the first thing that fails"

report
