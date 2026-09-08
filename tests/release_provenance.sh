#!/usr/bin/env bash
# A downloaded release asset has to be checkable against this repository.
#
# `gh extension install` fetches a binary over TLS to github.com and runs
# it. TLS says the bytes came from GitHub; it says nothing about whether
# they are the bytes this repository's workflow built, so an asset replaced
# by anyone with write access is indistinguishable from a real one after
# the fact. What closes that is an attestation the release run produces
# itself -- and, because it is inert if nobody ever runs the verify
# command, an install section that tells an operator how.
#
# All of that lives in a workflow file and two documents, and every way it
# rots is quiet: the input gets dropped in a refactor, the permissions the
# signing needs get hoisted to the top of the file where they also apply to
# the test gate, or the verify command ends up only in the release notes an
# operator installing the tool never opens. Nothing else in this suite
# would notice, so this file reads them and pins the shape.
#
# It cannot prove GitHub mints a real attestation, which is not in doubt.
# It proves we asked for one, scoped what it costs, and wrote down the
# check.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/helpers.sh"

ROOT="$(cd "$HERE/.." && pwd)"
RELEASE_WF="$ROOT/.github/workflows/release.yml"
README="$ROOT/README.md"
CLI_DOC="$ROOT/docs/cli.md"

# The workflow's own comments discuss `id-token` and `attestations` at
# length, and a substring match cannot tell a grant from a sentence about
# one -- which would pass hardest in exactly the case worth catching, the
# grant deleted and the prose about it left behind. So the permission
# assertions below read the YAML with the comments taken out.
# (ci_gates_release.sh's gap_note reasons about the same hazard from the
# other side, where only comment lines count.)
uncommented() { grep -v -E '^[[:space:]]*#'; }

# Everything above `jobs:` -- the file-level defaults, which apply to every
# job in the file including the gate.
workflow_header() { # <workflow-file>
  awk '/^jobs:/ { exit } { print }' "$1"
}

# A markdown section: from the line matching <pattern> up to the next
# heading. Fences are tracked from the top of the file rather than from the
# match, because the install this anchors on is itself inside one -- and a
# `# comment` in a code block is not a heading to stop at.
#
# <depth> is how shallow a heading has to be to end the section: 3 reads a
# `###` section including its `####` subsections, and the default of 6 ends
# it at any heading at all. A section whose material is organised into
# subsections is still that section, and a reader looking for what it says
# should not have to know which of its paragraphs got a heading.
doc_section() { # <file> <pattern> [depth]
  awk -v pat="$2" -v depth="${3:-6}" '
    /^```/ { infence = !infence; if (inblock) print; next }
    !inblock && $0 ~ pat { inblock = 1; print; next }
    !inblock { next }
    !infence && /^#+ / {
      match($0, /^#+/)
      if (RLENGTH <= depth) exit
    }
    { print }
  ' "$1"
}

echo "a release asset carries evidence of where it came from:"

assert_file_exists "$RELEASE_WF" "there is a workflow that publishes a release"

release_job="$(workflow_job_block "$RELEASE_WF" release)"
release_grants="$(printf '%s\n' "$release_job" | uncommented)"

# The evidence itself, produced by the run that builds the assets rather
# than by someone attaching a file afterwards.
assert_contains "$release_job" "generate_attestations: true" \
  "the release run attests to the assets it built"

# ...which needs two grants beyond the `contents: write` that publishing
# already takes.
assert_contains "$release_grants" "id-token: write" \
  "the release job can mint the OIDC token the attestation is signed with"
assert_contains "$release_grants" "attestations: write" \
  "the release job can write the attestation back to the repository"

# And they stop there. A token that can sign on this repository's behalf
# must not reach the gate, which runs the suite and an `npm install -g`
# with whatever it is handed -- and it reaches the gate from two directions:
# the file header, which every job inherits, and the gate's own job.
# Checking only the header would miss the leak written where it is most
# likely to be written.
header="$(workflow_header "$RELEASE_WF" | uncommented)"
assert_not_contains "$header" "write" \
  "no grant is handed to every job in the file"

job_names="$(workflow_job_names "$RELEASE_WF")"
assert_contains "$job_names" "test" \
  "the workflow's jobs can be read at all (the gate is among them)"
while IFS= read -r job; do
  [ -n "$job" ] || continue
  [ "$job" = "release" ] && continue
  body="$(workflow_job_block "$RELEASE_WF" "$job" | uncommented)"
  assert_not_contains "$body" "write" "the $job job holds no write grant of its own"
done <<< "$job_names"

# The other half of the trade was GPG, and the file has to say it was
# decided rather than missed -- the action takes a `gpg_fingerprint` and
# this workflow does not pass one.
decision_note="$(grep -E '^[[:space:]]*#.*gpg_fingerprint' "$RELEASE_WF")"
assert_contains "$decision_note" "gpg_fingerprint" \
  "the workflow says why it signs no GPG signature"

# And it really is not passed -- which the comment alone cannot say, and
# which the verify step below now depends on. The action attests
# `dist/*` and uploads that same set, *except* that a `gpg_fingerprint`
# makes it also generate `checksums.txt` and `checksums.txt.sig` outside
# dist/ and attach those too. Those two would be published assets that
# nothing attested and nothing verified, which is the exact gap the
# verification exists to close, reopened by an input whose comment says it
# is about GPG.
assert_not_contains "$release_grants" "gpg_fingerprint" \
  "and does not pass one, so dist/ is the whole of what a release publishes"

# ...and the run confirms it got one, rather than trusting that it asked.
# Everything above this point is satisfied by a repository that has never
# produced an attestation: the input is a string in a file, and the action
# gates its attest step on a string comparison that skips rather than fails.
# What closes that is a step that runs the operator's own command against
# the built assets, and the assertions below pin that the step exists, that
# it runs on what was published rather than before it, and that it is the
# thing standing between the assets and an install.
assert_contains "$release_job" "release-verify.sh" \
  "the release run verifies the attestations it asked for"

# Before the check runs, nothing is installable: `gh extension install`
# reads `releases/latest`, which does not return drafts. This is the input
# that makes a failed check mean "a draft nobody promoted" instead of "a
# release nobody can trust", and it is half of the decision recorded below.
assert_contains "$release_job" "draft_release: true" \
  "the release is drafted first, so nothing unverified is ever installable"

# Order, not just presence. A verify placed before the action would run on
# a dist/ that does not exist yet and pass over nothing -- the same silent
# green this file exists to end, wearing the check's name.
step_line() { # <needle> -> the line number of that step within the job block
  printf '%s\n' "$release_job" | grep -n "$1" | head -1 | cut -d: -f1
}
publishes_at="$(step_line 'cli/gh-extension-precompile')"
verifies_at="$(step_line 'release-verify.sh')"
if [ -n "$publishes_at" ] && [ -n "$verifies_at" ] && [ "$verifies_at" -gt "$publishes_at" ]; then
  pass "and verifies after the assets exist, not before"
else
  fail "and verifies after the assets exist, not before (publish at [$publishes_at], verify at [$verifies_at])"
fi

# The ways a step stays in the file while reporting success regardless.
# `continue-on-error` is the one that gets added later, by someone stopping
# a flake, and it leaves every assertion above still passing.
assert_not_contains "$release_grants" "continue-on-error" \
  "no step in the release job is allowed to fail quietly"

# The decision the issue asked to be made on purpose -- draft-then-promote
# rather than publish-then-go-red -- recorded where the gpg_fingerprint
# decision is, so whoever changes it knows they are changing it. Only
# comment lines count, so the `draft_release:` input above cannot satisfy
# this on its own.
draft_note="$(grep -E '^[[:space:]]*#' "$RELEASE_WF" | grep -i 'draft')"
assert_contains "$draft_note" "draft" \
  "the workflow says why it drafts the release rather than publishing it"

echo ""
echo "and an operator can perform the check:"

# Where an operator meets the install, not only in the release notes: the
# stretch of README between the `gh extension install` line and the next
# heading.
install_section="$(doc_section "$README" 'gh extension install blockadence/gh-archimedes')"
assert_contains "$install_section" "gh attestation verify" \
  "the install section says how to verify a downloaded asset"
assert_contains "$install_section" "--repo blockadence/gh-archimedes" \
  "the documented check names the repository the asset must come from"

# ...and points at the downloaded asset rather than at the installed copy.
# Retargeting it reads like an improvement -- it is the file you actually
# run -- and is wrong: gh ad-hoc codesigns an extension binary in place on
# Apple Silicon, so that copy stops hashing to what was attested and a
# genuine build fails the check. A verification that cries wolf is worse
# than none.
assert_not_contains "$install_section" ".local/share/gh/extensions/gh-archimedes" \
  "the documented check is not pointed at the copy gh rewrites as it installs"

# Unchanged for the nearly everyone who verifies nothing: the line an
# operator copies is still that command alone, with no verify step bolted
# onto it. Asking whether the section contains the string would be vacuous
# -- doc_section found the section *by* that string -- so this counts lines
# in the file that are exactly it.
bare_installs="$(grep -cx 'gh extension install blockadence/gh-archimedes' "$README")"
assert_eq "$bare_installs" "1" \
  "the install itself is still one bare command"

# The check has to be the operator's option, not a step they are told they
# must perform, or the criterion above is satisfied only on paper.
assert_contains "$install_section" "optional" \
  "the install section says verifying is optional"

# The design note, for whoever is changing release.yml rather than
# installing the tool.
release_notes="$(doc_section "$CLI_DOC" '^### Cutting a release' 3)"
assert_contains "$release_notes" "gh attestation verify" \
  "cutting a release documents what the run publishes to be verified against"

# What a failed confirmation does to the release, written down rather than
# left to whoever is holding the tag. The word to look for is the outcome
# an operator or a maintainer would meet -- a draft that was not promoted.
assert_contains "$release_notes" "draft" \
  "cutting a release says what a failed verification leaves behind"

# And what the check leans on outside this repository, so that a red run
# nobody can act on is a documented possibility rather than a surprise.
assert_contains "$release_notes" "Sigstore" \
  "cutting a release names what the check depends on beyond this repository"

report
