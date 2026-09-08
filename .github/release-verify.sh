#!/usr/bin/env bash
# Confirms a drafted release can actually be verified, then publishes it.
#
# `release.yml` asks cli/gh-extension-precompile for attestations, and
# tests/release_provenance.sh pins that we asked. Neither is evidence that
# a release got one. The action gates its attest step on
# `if: ${{ inputs.generate_attestations == 'true' }}` -- a string
# comparison against a composite-action input -- and anything that stops
# producing that exact string skips the step rather than failing it. A
# skipped step is not a red run: twelve binaries publish, the run is green,
# and the first person to find out is an operator whose
# `gh attestation verify` reports no attestation. That operator cannot tell
# that from tampering, which is the one thing the command exists to
# distinguish.
#
# So this runs the operator's own command, against the assets the run just
# built, before anyone can install them. It is worth more than an
# inspection of our own side of it precisely because it fails for the same
# reasons theirs would: a missing attestation, a digest that does not
# match, an API that will not confirm the binding.
#
# WHAT A FAILURE DOES, decided rather than left to whoever holds the tag:
# nothing gets published. `release.yml` passes `draft_release: true`, so
# the action leaves a draft -- which `gh extension install` will not install
# from, because it reads `releases/latest` and that endpoint does not return
# drafts. This script promotes the draft on its last line and only there, so
# the only release an operator can reach is one whose assets were verified.
# A failed run leaves a draft nobody promoted and a red release run. To
# recover: fix the cause, delete the draft (`gh release delete <tag> --yes`
# -- a re-run would otherwise leave a second draft on the same tag, which
# GitHub allows), and push the tag again. Promoting the draft by hand is the
# escape hatch, and it is a deliberate act rather than the default.
#
# WHAT THIS DEPENDS ON beyond this repository, because a check that fails
# for reasons nobody can act on is worse than no check:
#
#   - `gh`, preinstalled on GitHub-hosted runners.
#   - GitHub's attestations API, which `gh attestation verify` asks for the
#     attestation by artifact digest. This read happens seconds after the
#     same run wrote it, and if it is not immediately consistent the check
#     would fail a few percent of good releases -- for which the fix people
#     learn is "re-run it", which is also the fix for a real failure. The
#     retry budget below is the answer: a bounded wait absorbs propagation,
#     and an attestation that does not exist never appears no matter how
#     long you wait, so waiting cannot turn a real failure green.
#   - Sigstore's trust root, which `gh attestation verify` fetches to check
#     the signing certificate.
#
# If the retry budget turns out to be papering over a real race rather than
# a brief one, the fix is `--bundle`: verify against the bundle the attest
# step produced instead of a lookup. That needs the bundle path out of a
# step nested inside the composite action, which is why it is not what this
# does today.
#
# Usage: release-verify.sh <dist-dir> <repo> <tag>
set -euo pipefail

dist="${1:?usage: release-verify.sh <dist-dir> <repo> <tag>}"
repo="${2:?usage: release-verify.sh <dist-dir> <repo> <tag>}"
tag="${3:?usage: release-verify.sh <dist-dir> <repo> <tag>}"

# One budget for the run rather than one per asset. The delay this is
# absorbing is a property of the run -- every attestation was written in the
# same burst -- so a straggling asset should draw on what the others did not
# need. Per-asset retries would also make a genuinely missing attestation
# take twelve full waits to report, and a check nobody waits for the end of
# is a check nobody reads.
retries="${ARCHIMEDES_RELEASE_VERIFY_RETRIES:-5}"
delay="${ARCHIMEDES_RELEASE_VERIFY_DELAY:-10}"

# The release is still a draft, so there is still something to gate. The
# action decides this from `[[ "$DRAFT_RELEASE" = "true" ]]` -- the same
# shape of string comparison that can quietly stop matching as the attest
# step's can -- and if it stopped applying, the assets are already public
# and this whole script is reporting on a decision that was made without it.
# Better to say so than to verify something and promote what is already
# promoted.
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
# every assertion in it holds vacuously, and the step goes green over a
# release nobody checked. So the assets are counted before they are
# verified, and no assets is a failure.
if [ ! -d "$dist" ]; then
  echo "error: no $dist/ to verify -- the build left nothing behind." >&2
  exit 1
fi

# Newline-delimited rather than an array: macOS still ships bash 3.2, where
# expanding an empty array under `set -u` is an error, and empty is the case
# this most needs to handle.
assets="$(find "$dist" -maxdepth 1 -type f | LC_ALL=C sort)"
count=0
while IFS= read -r asset; do
  if [ -n "$asset" ]; then count=$((count + 1)); fi
done <<< "$assets"

if [ "$count" -eq 0 ]; then
  echo "error: $dist/ holds no assets to verify." >&2
  echo "       A release that published nothing verifiable must not go" >&2
  echo "       green just because there was nothing to check." >&2
  exit 1
fi

echo "verifying $count asset(s) in $dist/ against $repo:"

while IFS= read -r asset; do
  [ -n "$asset" ] || continue
  # `until`, so a failing verify is a loop condition rather than something
  # errexit acts on -- and so the retry is spelled once, around the real
  # command, with nothing between the failure and the decision to wait.
  until gh attestation verify "$asset" --repo "$repo"; do
    if [ "$retries" -le 0 ]; then
      echo "" >&2
      echo "error: $(basename "$asset") has no attestation this repository" >&2
      echo "       can verify. Nothing has been promoted: the release is" >&2
      echo "       still a draft and is not installable." >&2
      exit 1
    fi
    retries=$((retries - 1))
    echo "  $(basename "$asset"): not readable yet, $retries retry/retries left"
    sleep "$delay"
  done
done <<< "$assets"

echo ""
echo "all $count asset(s) verified; publishing $repo $tag"

# The last line, and the only place the draft becomes a release. Everything
# above is what earns it.
gh release edit "$tag" --repo "$repo" --draft=false
