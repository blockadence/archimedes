#!/usr/bin/env bash
# Confirms a drafted release can actually be verified, then publishes it.
#
# `release.yml` asks cli/gh-extension-precompile for attestations, and
# tests/release_provenance.sh pins that we asked. Neither is evidence that a
# release got one: the action gates its attest step on a string comparison
# against a composite-action input, and anything that stops producing that
# exact string skips the step rather than failing it. A skipped step is a
# green run over twelve unattested binaries. This runs the operator's own
# command instead, so it fails for the same reasons theirs would.
#
# WHAT A FAILURE DOES, decided rather than left to whoever holds the tag:
# nothing is published. `release.yml` passes `draft_release: true`, so the
# action leaves a draft -- which `gh extension install` will not install
# from, because it reads `releases/latest` and that endpoint does not return
# drafts. This script promotes the draft on its last line and only there.
#
# To recover from a failed run: fix the cause and re-run. The action's
# build_and_release.sh does `gh release view "$TAG"` before it creates
# anything, and gh's release lookup falls back to finding a *draft* by tag
# name, so a re-run finds the existing draft and re-uploads into it with
# `--clobber` rather than leaving a second one. Nothing needs deleting
# first. Promoting the draft by hand stays available and stays a deliberate
# act.
#
# WHAT THIS DEPENDS ON beyond this repository, because a check that fails
# for reasons nobody can act on is worse than no check:
#
#   - `gh`, preinstalled on GitHub-hosted runners.
#   - GitHub's attestations API, which `gh attestation verify` asks for the
#     attestation by artifact digest -- a read happening seconds after the
#     same run wrote it.
#   - Sigstore's trust root, which `gh attestation verify` fetches to check
#     the signing certificate.
#
# THE RACE, AND WHAT FOUR RELEASES SAID ABOUT IT. If that read is not
# immediately consistent, a check with no wait would fail a few percent of
# good releases -- and the fix people learn ("re-run it") is also the fix
# for a real failure, which is worse than not checking at all. So the retry
# budget below went in as a hedge rather than a finding, instrumented
# rather than silent: every retry prints, and a run that needed any says so
# in a line written to be grepped for. That was the measurement.
#
# It has now been taken, and no run has needed a retry. v0.1.0-rc.1 through
# -rc.4 verified twelve assets each and printed no marker line. Be exact
# about what that does and does not measure:
#
#   - Each run's loop *started* 0.36-0.57s after the attest step logged
#     "Attestation created for 12 subjects" (measured to this script's own
#     banner, printed immediately before the first iteration). So the first
#     lookup of each run is the tightest timing a release produces, and it
#     succeeded first try, four for four.
#   - The loops then ran about 40s each, so the twelfth lookup is roughly
#     40s after the write, not under a second. Only the first read is a
#     test of immediate consistency; the other eleven are progressively
#     weaker ones.
#   - For -rc.1 to -rc.3 "no retries" was inferred from the per-run count
#     check rather than read off per-lookup timestamps, because `gh` prints
#     nothing on success -- which is the other thing those runs
#     established, and what the per-asset line below now fixes. -rc.4 is
#     the first run whose twelve lookups are individually timestamped: the
#     first completed 4.7s after the attestation was uploaded, they ran
#     3.1-4.3s apart, and none retried.
#
# What that supports: the read was immediately consistent on the four
# occasions it was tested hardest, so `--bundle` is not needed now. What it
# does not support: a general claim about GitHub's consistency. Four runs,
# two commits, inside two hours, on one runner pool, is a small sample
# against a failure mode whose whole danger is that it is intermittent.
#
# So the budget stays, and it is no longer only a hedge -- it is the thing
# that keeps a rare slow read from becoming a red release whose documented
# fix ("re-run it") is indistinguishable from the fix for a real failure.
# It costs nothing on a healthy run; four spent none of it. `--bundle`
# stays the answer if the marker line starts showing up.
#
# A bounded wait cannot turn a real failure green either way: an attestation
# that does not exist never appears, however long you wait.
#
# Usage: release-verify.sh <dist-dir> <repo> <tag>
set -euo pipefail

dist="${1:?usage: release-verify.sh <dist-dir> <repo> <tag>}"
repo="${2:?usage: release-verify.sh <dist-dir> <repo> <tag>}"
tag="${3:?usage: release-verify.sh <dist-dir> <repo> <tag>}"

# One budget for the run rather than one per asset. The delay it absorbs is
# a property of the run -- every attestation was written in the same burst
# -- so a straggling asset should draw on what the others did not need.
# Per-asset retries would also make a genuinely missing attestation take
# twelve full waits to report.
retries="${ARCHIMEDES_RELEASE_VERIFY_RETRIES:-5}"
delay="${ARCHIMEDES_RELEASE_VERIFY_DELAY:-10}"
retries_used=0

# The release is still a draft, so there is still something to gate.
# `draft_release` is decided inside the action by `[[ "$DRAFT_RELEASE" =
# "true" ]]` -- the same shape of string comparison that can quietly stop
# matching as the attest step's can. If it stopped applying, the assets are
# already installable and this script is reporting on a decision made
# without it. Better to say so than to verify something and promote what is
# already promoted.
draft="$(gh release view "$tag" --repo "$repo" --json isDraft --jq .isDraft)"
if [ "$draft" != "true" ]; then
  echo "error: $repo $tag is not a draft (isDraft=$draft)." >&2
  echo "       Its assets are already installable, so verifying them now" >&2
  echo "       gates nothing. Check that release.yml still passes" >&2
  echo "       draft_release: true and that the action still honours it." >&2
  exit 1
fi

# A glob that expands to nothing is the one failure mode that would restore
# exactly the silence this exists to end: the loop below runs zero times,
# every check in it holds vacuously, and the step goes green over a release
# nobody verified. So the assets are counted before they are verified, and
# the count is checked again afterwards.
if [ ! -d "$dist" ]; then
  echo "error: no $dist/ to verify -- the build left nothing behind." >&2
  exit 1
fi

# Newline-delimited rather than an array: macOS still ships bash 3.2, where
# expanding an empty array under `set -u` is an error, and empty is the case
# this most needs to handle.
assets="$(find "$dist" -maxdepth 1 -type f | LC_ALL=C sort)"
if [ -z "$assets" ]; then
  echo "error: $dist/ holds no assets to verify." >&2
  echo "       A release that published nothing verifiable must not go" >&2
  echo "       green just because there was nothing to check." >&2
  exit 1
fi
expected="$(printf '%s\n' "$assets" | wc -l | tr -d ' ')"

echo "verifying $expected asset(s) in $dist/ against $repo:"

verified=0
while IFS= read -r asset; do
  [ -n "$asset" ] || continue
  name="$(basename "$asset")"
  # `until`, so a failing verify is a loop condition rather than something
  # errexit acts on. stdin is closed to it because this loop is fed by a
  # here-string on the same descriptor: a subprocess that read from stdin
  # would swallow the remaining assets and end the loop early, verifying
  # one and reporting for twelve. The count check after the loop catches
  # that even if this redirect is ever dropped.
  until gh attestation verify "$asset" --repo "$repo" </dev/null; do
    if [ "$retries" -le 0 ]; then
      echo "" >&2
      echo "error: $name has no attestation this repository" >&2
      echo "       can verify, and the run's retry budget is spent." >&2
      echo "       Nothing has been promoted: the release is still a draft" >&2
      echo "       and is not installable." >&2
      exit 1
    fi
    retries=$((retries - 1))
    retries_used=$((retries_used + 1))
    echo "  $name: not readable yet, $retries retry/retries left"
    sleep "$delay"
  done
  # One line per asset, printed here rather than left to `gh`, and shaped
  # like the retry line above it so that both of an asset's possible
  # outcomes grep by the same leading name.
  #
  # The first real releases established why this has to be ours:
  # `gh attestation verify` prints nothing whatsoever on success when
  # stdout is not a terminal -- its report is gated on an interactive one
  # -- so twelve verifies left the job log holding only the count lines
  # around this loop. The count check below is what actually catches a loop
  # that ended early, but a reader could not see it working: "verified one
  # and reported twelve" and the truth looked identical in the log. Now
  # they do not.
  echo "  $name: verified"
  verified=$((verified + 1))
done <<< "$assets"

# The loop ran over everything it said it would. Anything that ends it early
# -- a subprocess eating stdin, a read error -- would otherwise leave a
# green step that checked a fraction of the release.
if [ "$verified" -ne "$expected" ]; then
  echo "error: verified $verified of $expected asset(s); the check did not" >&2
  echo "       cover the release and nothing has been promoted." >&2
  exit 1
fi

# The measurement described in the header. One line, phrased so that
# "did any release need a retry?" is a search rather than a reading.
if [ "$retries_used" -gt 0 ]; then
  echo ""
  echo "ATTESTATION-LOOKUP-RETRIES=$retries_used -- the attestation read was" \
       "not immediately consistent. If this line shows up on most releases," \
       "switch to --bundle; see the header of .github/release-verify.sh."
fi

echo ""
echo "all $verified asset(s) verified; publishing $repo $tag"

# The last line, and the only place the draft becomes a release. Everything
# above is what earns it.
gh release edit "$tag" --repo "$repo" --draft=false
