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
doc_section() { # <file> <pattern>
  awk -v pat="$2" '
    /^```/ { infence = !infence; if (inblock) print; next }
    !inblock && $0 ~ pat { inblock = 1; print; next }
    !inblock { next }
    !infence && /^#+ / { exit }
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
release_notes="$(doc_section "$CLI_DOC" '^### Cutting a release')"
assert_contains "$release_notes" "gh attestation verify" \
  "cutting a release documents what the run publishes to be verified against"

report
