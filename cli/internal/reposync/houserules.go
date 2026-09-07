package reposync

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/aymanbagabas/go-udiff"

	"github.com/blockadence/archimedes/cli/internal/dossier"
	"github.com/blockadence/archimedes/cli/internal/gitutil"
	"github.com/blockadence/archimedes/cli/internal/manifest"
)

// The branch, commit, and pull request the house-rules sync opens in the
// target repo. Fixed, so re-running updates the same pull request.
const (
	HouseRulesBranch        = "archimedes-sync-house-rules"
	houseRulesCommitMessage = "Sync house rules from Archimedes"
	houseRulesPRTitle       = "Sync house rules from Archimedes"
)

// HouseRulesOptions is one house-rules-sync request.
type HouseRulesOptions struct {
	// Root is the instance directory holding repos.yaml and repos/.
	Root string
	// Repo is the target repo's name in repos.yaml.
	Repo string
	// DryRun prints the pending change without committing, pushing, or
	// opening a pull request.
	DryRun bool
}

// RenderHouseRules turns a dossier's House rules section into the file
// committed to the target repo. It ends in a newline, where the shell
// version's command substitution stripped one — so a repo synced by the old
// script sees a single whitespace-only change on its next sync, and none
// after that (see sameContent).
func RenderHouseRules(rules string) string {
	return "# House rules\n\n" + rules + "\n"
}

// SyncHouseRules pushes one repo's house rules — the "## House rules"
// section of its dossier, which is the single source of truth — into that
// repo as a durably committed HOUSE_RULES.md, via a pull request.
//
// Unlike the templates fan-out this isn't a multi-repo job: the content is
// specific to one repo, so it works directly on that repo's own local clone
// (already present from bootstrap) with plain git plus gh.
//
// progress receives git's and gh's own output; out receives the lines meant
// for the caller.
func SyncHouseRules(opts HouseRulesOptions, out, progress io.Writer, run ExecFunc) error {
	root, m, err := manifest.LoadInstance(opts.Root)
	if err != nil {
		return err
	}

	repo, ok := m.Find(opts.Repo)
	if !ok {
		return fmt.Errorf("unknown repo: %s", opts.Repo)
	}
	repoPath := filepath.Join(root, repo.Path)
	if info, err := os.Stat(repoPath); err != nil || !info.IsDir() {
		return fmt.Errorf("unknown repo checkout: %s (run bootstrap first)", repoPath)
	}

	// The dossier parse is shared with the per-worktree delivery in
	// internal/spawn, so a house rule only ever needs editing in one place.
	dossierDir := filepath.Join(root, "repos")
	rules, err := dossier.HouseRules(dossierDir, opts.Repo)
	if err != nil {
		return err
	}
	if rules == "" {
		return fmt.Errorf("no house rules recorded for %s (%s); nothing to sync",
			opts.Repo, dossier.Path(dossierDir, opts.Repo))
	}
	want := RenderHouseRules(rules)

	// Compare against current remote state, not a possibly-stale checkout,
	// so an already-merged sync is recognized as a no-op.
	if err := gitutil.RunOut(repoPath, progress, "fetch", "origin"); err != nil {
		return err
	}
	if err := gitutil.RunOut(repoPath, progress, "checkout", repo.BaseBranch); err != nil {
		return err
	}
	if err := gitutil.RunOut(repoPath, progress, "pull", "--ff-only", "origin", repo.BaseBranch); err != nil {
		return err
	}

	targetFile := filepath.Join(repoPath, dossier.HouseRulesFileName)
	current, err := os.ReadFile(targetFile)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", targetFile, err)
	}

	if sameContent(string(current), want) {
		fmt.Fprintf(out, "%s: %s already current on %s.\n", opts.Repo, dossier.HouseRulesFileName, repo.BaseBranch)
		return nil
	}

	if opts.DryRun {
		fmt.Fprintf(out, "%s: would update %s:\n", opts.Repo, dossier.HouseRulesFileName)
		fmt.Fprint(out, unifiedDiff(string(current), want))
		return nil
	}

	if err := gitutil.RunOut(repoPath, progress, "checkout", "-B", HouseRulesBranch); err != nil {
		return err
	}

	// From here on the clone is off its base branch, so every exit path has
	// to put it back — including the failure ones.
	syncErr := commitAndPush(repoPath, targetFile, want, progress)
	if syncErr == nil {
		syncErr = createPR(repo, repoPath, dossierDir, run, out, progress)
	}

	if err := gitutil.RunOut(repoPath, progress, "checkout", repo.BaseBranch); err != nil && syncErr == nil {
		syncErr = err
	}
	return syncErr
}

// sameContent compares the committed copy with the one the dossier would
// produce, ignoring trailing newlines so a file that only differs by its
// final newline isn't re-synced forever.
func sameContent(current, want string) bool {
	return strings.TrimRight(current, "\n") == strings.TrimRight(want, "\n")
}

// unifiedDiff renders the pending change the way the dry run of the shell
// script did (`diff -u`), so the operator sees what a real run would commit.
// A repo with no HOUSE_RULES.md yet passes current as "", which diffs as a
// clean addition.
func unifiedDiff(current, want string) string {
	return udiff.Unified(
		dossier.HouseRulesFileName+" (in repo)",
		dossier.HouseRulesFileName+" (from dossier)",
		current, want)
}

func commitAndPush(repoPath, targetFile, content string, progress io.Writer) error {
	if err := os.WriteFile(targetFile, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", targetFile, err)
	}
	if err := gitutil.RunOut(repoPath, progress, "add", dossier.HouseRulesFileName); err != nil {
		return err
	}
	if err := gitutil.RunOut(repoPath, progress, "commit", "-q", "-m", houseRulesCommitMessage); err != nil {
		return err
	}
	return gitutil.RunOut(repoPath, progress, "push", "-q", "-u", "origin", HouseRulesBranch, "--force-with-lease")
}

// createPR opens the pull request for the pushed branch. A failure here is
// reported rather than returned: the usual cause is a pull request for this
// branch already existing, which means the sync has done its job and the
// push just updated it.
func createPR(repo manifest.Repo, repoPath, dossierDir string, run ExecFunc, out, progress io.Writer) error {
	slug, err := gitutil.GHSlug(repoPath)
	if err != nil {
		return err
	}

	body := fmt.Sprintf("Updates %s from this repo's dossier in Archimedes (%s). "+
		"Edit the dossier, not this file, and re-run `archimedes sync-house-rules %s`.",
		dossier.HouseRulesFileName, dossier.Path(dossierDir, repo.Name), repo.Name)

	args := []string{"pr", "create",
		"--repo", slug,
		"--base", repo.BaseBranch,
		"--head", HouseRulesBranch,
		"--title", houseRulesPRTitle,
		"--body", body,
	}
	// gh prints the new pull request's URL on stdout — the one result of a
	// successful run worth capturing — so that stream goes to out while its
	// diagnostics join the rest of the progress output.
	if err := run("gh", args, out, progress); err != nil {
		fmt.Fprintf(progress, "%s: PR create failed or a PR for %s already exists; check manually. (%v)\n",
			repo.Name, HouseRulesBranch, err)
	}
	return nil
}
