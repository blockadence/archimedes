#!/usr/bin/env bash
# The one throwaway-repo fixture the bash tests share: a bare "origin" plus
# a clone of it carrying one commit, already pushed to main — the shape that
# makes a test's fetch/rev-parse/push work against real git with no network.
# The Go tests build the same shape with internal/testrepo.
#
# It also holds the three ways a test file says what git identity it is
# running under -- isolate_git, strip_git_identity and
# unconfigure_git_identity -- which are fixtures of the same kind: what a
# repository is committed as is as much a property of the machine as what is
# in it.
#
# Sourcing this file defines those functions and nothing else: it sets no
# shell options and declares no test-reporting helpers, so a test keeps its
# own `set -e` and its own pass/fail contract.

# make_origin_and_clone_at <origin-path> <clone-path> [seed-file] [seed-content]
#
# With no seed file the initial commit is empty. The clone gets a fixed
# identity and no commit signing, so committing here — now and in whatever
# the test commits on top — doesn't depend on the machine's git config.
make_origin_and_clone_at() {
  local origin="$1" clone="$2" seed="${3:-}" content="${4:-}"

  git init -q --bare -b main "$origin"
  git clone -q "$origin" "$clone"
  git -C "$clone" config user.email t@t
  git -C "$clone" config user.name t
  git -C "$clone" config commit.gpgsign false

  if [ -n "$seed" ]; then
    printf '%s' "$content" > "$clone/$seed"
    git -C "$clone" add -A
    git -C "$clone" commit -q -m init
  else
    git -C "$clone" commit -q --allow-empty -m init
  fi

  git -C "$clone" push -q -u origin main
}

# isolate_git <scratch-dir> — give this test file a git of its own, holding
# an identity and nothing else. The bash twin of internal/testrepo's
# IsolateGit, and it is here for the same reason: a test whose subject
# creates the repository has no checkout to configure before the code under
# test commits in it, so the identity has to reach git through the
# environment or not at all. Replacing the global config also keeps a
# machine's commit signing out of the way, since that lived there too.
#
# This is where an identity belongs, and .github/workflows/test.yml is where
# it does not. A runner with no identity is the machine that first caught
# `archimedes init` assuming one; configuring the workflow around that would
# blind the only runner that reliably reproduces it.
isolate_git() {
  local dir="$1"
  printf '[user]\n\tname = t\n\temail = t@t\n' > "$dir/gitconfig"
  export GIT_CONFIG_GLOBAL="$dir/gitconfig"
  export GIT_CONFIG_SYSTEM=/dev/null
  # The environment outranks every config file, so a GIT_AUTHOR_NAME the
  # caller happened to be carrying -- strip_git_identity's, or a shell's --
  # would beat the identity just written and make this a no-op.
  unset GIT_AUTHOR_NAME GIT_AUTHOR_EMAIL GIT_COMMITTER_NAME GIT_COMMITTER_EMAIL
}

# strip_git_identity — the opposite fixture, for a test whose subject is the
# machine where git itself will not commit: a fresh container, a CI runner,
# where any commit dies with "empty ident name ... not allowed".
#
# The name is emptied rather than unset, and that is what makes this that
# machine on every machine. Unset, git derives a name from the account, and
# whether that derivation comes back usable is a property of the box -- empty
# on a CI runner, a full name on a developer's macOS one. An empty
# GIT_AUTHOR_NAME reaches git's own refusal by the route the runner takes,
# everywhere. For the other machine with nothing configured -- the one where
# git guesses and commits -- see unconfigure_git_identity.
strip_git_identity() {
  export GIT_CONFIG_GLOBAL=/dev/null
  export GIT_CONFIG_SYSTEM=/dev/null
  export GIT_AUTHOR_NAME=""
  export GIT_COMMITTER_NAME=""
}

# unconfigure_git_identity — the machine between those two: nothing configured
# anywhere, and git left free to guess an identity from the OS account the way
# it does in any repository on that box. It is the machine an operator who has
# never run `git config --global user.name` is actually on, which is most of
# them.
#
# What the guess comes back with is a property of the box -- a full name on a
# developer's macOS one, nothing on a CI runner -- so git commits here in one
# place and refuses in the other. A test using this fixture asserts what is
# true of both: that an identity nobody chose is not one to commit an
# operator's own repository under.
#
# Nothing is emptied here, only unset, EMAIL included -- git reads that one as
# a configured address in its own right, so a machine's EMAIL left standing
# would make this fixture a different machine there. Emptying is
# strip_git_identity's job, and makes a different machine again.
unconfigure_git_identity() {
  export GIT_CONFIG_GLOBAL=/dev/null
  export GIT_CONFIG_SYSTEM=/dev/null
  unset GIT_AUTHOR_NAME GIT_AUTHOR_EMAIL GIT_COMMITTER_NAME GIT_COMMITTER_EMAIL EMAIL
}
