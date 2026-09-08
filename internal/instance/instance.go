// Package instance scaffolds a new Archimedes instance: the starting data
// on its own fresh git history, so instance-specific (possibly sensitive)
// content never lives in this repo's history.
//
// "On its own history" is as far as it goes where nobody has configured git
// an identity to commit under, which is every fresh laptop, container and CI
// runner and every operator who has not got round to it: the instance is
// written and its repository initialized, and the first commit waits for
// them. Create says which of the two happened so its caller can pass that
// on. It will not close the gap with an identity nobody chose — an instance
// is the operator's own repository and its first commit is in that history
// forever, so an author they never picked is worse there than an instance
// that is merely uncommitted.
//
// That holds even where git would have committed. Given no configuration git
// guesses an identity from the OS account and uses it if the guess comes
// back usable, which on a developer's macOS box it does; the guess is a name
// and a hostname, and it would sit in the instance's first commit for good.
// Whether that counts as an author the operator chose was the question, and
// the answer is no: it is git's right answer for a commit somebody typed and
// the wrong one for a commit this package makes on their behalf. So the bar
// is an identity in config or in the environment (gitutil's
// HasConfiguredIdentity), and the caller has to say that this is stricter
// than git, or an operator on that box reads a skipped commit as a bug.
//
// A commit git was asked for and refused ends the same way, and Create says
// which of those two happened as well: an identity somebody configured, and
// signing set up with no key that works on this machine, or a hook that says
// no. Nothing is being decided on the operator's behalf there — git simply
// will not do what their own configuration asks — so what comes back is the
// instance and git's own words about the commit, for a caller that has to
// tell them what to fix. What is not done is asking again with that
// configuration turned off: an unsigned commit under a policy they set is
// the same objection as an author they never chose, one file of their
// permanent history away.
//
// What lands in an instance is data and nothing else — a manifest, dossier
// and work directories, scaffolding it owns from here on, and an empty
// drivers/ for whatever drivers it comes to own. Not one file of it is a
// program: the tooling that acts on an instance is the archimedes binary,
// and so are the drivers it ships (see internal/driver's Set). So there is
// nothing in an instance to keep in step with this repo, nothing to
// re-vendor into it, and no file mode to restore on the way in.
//
// The template is a filesystem the caller supplies rather than a path this
// package goes looking for: cmd hands it the copy embedded in the binary
// (see the archimedes root package), which is what lets a machine with no
// clone of this repo create an instance at all.
package instance

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/blockadence/gh-archimedes/internal/gitutil"
)

// Result is the instance Create made: where it is, and how it stands with
// respect to its first commit. Three states, and Path is set in all of them
// — an instance that is not committed is still an instance:
//
//   - Committed: the scaffolding is the instance's first commit.
//   - Not committed, CommitErr nil: nobody had configured an identity to
//     commit under, so no commit was attempted and there is nothing of
//     git's to report.
//   - Not committed, CommitErr set: git was asked and refused, and
//     CommitErr is why — in git's own words, via gitutil.Reason.
//
// The caller has something different to say in each case, which is why the
// second and third are distinguishable at all rather than one "no commit"
// flag: one is a setting to make, the other is a machine to fix.
type Result struct {
	// Path is the instance directory that was created.
	Path string

	// Committed reports whether the scaffolding is the instance's first
	// commit.
	Committed bool

	// CommitErr is git's refusal of that commit, where one was attempted.
	// Always nil when Committed.
	CommitErr error
}

// CommitSubject is the subject Create gives an instance's first commit. It
// is exported for the caller that has to tell an operator how to make that
// commit by hand, so what they are told to type is the line itself rather
// than a paraphrase of it that drifts from it.
func CommitSubject(name string) string {
	return "Scaffold " + name + " from Archimedes template"
}

// Create scaffolds an instance named name under destParent from the
// template tree src.
//
// Its two halves end differently, and that difference is the whole of the
// cleanup rule. Writing the instance — the files, and the repository they
// sit in — is Create's own work: if any of it fails, what is on disk is not
// an instance, so dest goes back and the error is returned. The first commit
// is the operator's, made on their behalf: if it does not happen, the
// instance is there and usable, so it is reported in Result rather than
// returned as a failure.
//
// So the rule is not "an error means the directory goes back" but the
// question behind it — is there an instance here? Nothing was at dest a
// moment ago, so everything under it is ours to take back, and a half-built
// one left behind would make the retry fail with "already exists" instead of
// with whatever actually went wrong. Once the instance is whole, taking it
// back over a commit costs the operator the valuable half to punish them for
// the half that needs them, and a retry after they have fixed their machine
// would produce the identical directory.
func Create(src fs.FS, name, destParent string) (Result, error) {
	dest := filepath.Join(destParent, name)
	if _, err := os.Stat(dest); err == nil {
		return Result{}, fmt.Errorf("%s already exists", dest)
	} else if !os.IsNotExist(err) {
		return Result{}, fmt.Errorf("checking %s: %w", dest, err)
	}

	if err := materialize(src, dest); err != nil {
		return discard(dest, err)
	}
	if _, err := gitutil.Run(dest, "init", "-q"); err != nil {
		return discard(dest, err)
	}

	res := Result{Path: dest}

	// Asked after `git init` rather than before, so an identity set on this
	// repository alone counts the same as a global one — and asked at all
	// because the alternative is either git's "Please tell me who you are"
	// as the first thing this tool ever says to an operator, or a first
	// commit authored by whoever git guessed they were.
	//
	// Nothing is staged on the way out. What such an operator is told to run
	// is `git add -A && git commit`, and an index left half-filled here would
	// make that line quietly wrong about what it commits.
	if !gitutil.HasConfiguredIdentity(dest) {
		return res, nil
	}

	// Git may still refuse: signing configured with no key it can use here,
	// a hook that says no, a full disk. The refusal is kept whole rather than
	// summarized, because what an operator has to fix is in git's own words
	// and this package cannot know which of those it is. What is not done is
	// asking again with the configuration turned off — `--no-gpg-sign` past a
	// broken key writes something into the instance's permanent history that
	// contradicts what its owner asked for, which is the same objection that
	// rules out an author nobody chose.
	//
	// Staging counts as part of the commit rather than as Create's own work,
	// so a failure in either is the same news: it is what the commit is made
	// of, the instance is equally whole and equally uncommitted whichever of
	// the two would not run, and the line the operator is given repeats both.
	//
	// The index is left as git left it, staged. Unlike the skipped commit
	// above there is no wrong impression to avoid: `git add -A && git
	// commit`, which is what the operator is told to run either way, stages
	// exactly what is already there.
	for _, args := range [][]string{
		{"add", "-A"},
		{"commit", "-q", "-m", CommitSubject(name)},
	} {
		if _, err := gitutil.Run(dest, args...); err != nil {
			res.CommitErr = err
			return res, nil
		}
	}
	res.Committed = true
	return res, nil
}

// discard takes dest back and reports err, for the failures that leave no
// instance behind them.
func discard(dest string, err error) (Result, error) {
	_ = os.RemoveAll(dest)
	return Result{}, err
}

// materialize writes every file in src under dest.
func materialize(src fs.FS, dest string) error {
	return fs.WalkDir(src, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dest, filepath.FromSlash(path))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := fs.ReadFile(src, path)
		if err != nil {
			return fmt.Errorf("reading template %s: %w", path, err)
		}
		return os.WriteFile(target, data, 0o644)
	})
}
