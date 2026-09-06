package workspacemap_test

import (
	"testing"

	"github.com/blockadence/archimedes/cli/internal/manifest"
	"github.com/blockadence/archimedes/cli/internal/workspacemap"
)

// Expected outputs below were captured by running the existing
// template/scripts/render-map.sh (under gawk, since macOS's bundled awk
// chokes on a multi-line -v assignment) against identical fixtures, to
// confirm this port matches byte-for-byte.

func TestRenderReplacesExistingRepoBlock(t *testing.T) {
	existing := "# Workspace Map\n" +
		"\n" +
		"## Repos\n" +
		"\n" +
		"- [old-repo](../old-repo) — base: `main`. Dossier: [repos/old-repo.md](./repos/old-repo.md)\n" +
		"\n" +
		"## Relationships\n" +
		"\n" +
		"- service-b depends on service-a for its client SDK.\n"

	repos := []manifest.Repo{
		{Name: "service-a", Path: "../service-a", BaseBranch: "main"},
		{Name: "service-b", Path: "../service-b", BaseBranch: "develop"},
	}

	want := "# Workspace Map\n" +
		"\n" +
		"## Repos\n" +
		"\n" +
		"- [service-a](../service-a) — base: `main`. Dossier: [repos/service-a.md](./repos/service-a.md)\n" +
		"- [service-b](../service-b) — base: `develop`. Dossier: [repos/service-b.md](./repos/service-b.md)\n" +
		"\n" +
		"\n" +
		"## Relationships\n" +
		"\n" +
		"- service-b depends on service-a for its client SDK.\n"

	got := workspacemap.Render(existing, repos)
	if got != want {
		t.Errorf("Render() mismatch\n got: %q\nwant: %q", got, want)
	}
}

func TestRenderOnDefaultContent(t *testing.T) {
	repos := []manifest.Repo{
		{Name: "service-a", Path: "../service-a", BaseBranch: "main"},
		{Name: "service-b", Path: "../service-b", BaseBranch: "develop"},
	}

	want := "# Workspace Map\n" +
		"\n" +
		"## Repos\n" +
		"\n" +
		"- [service-a](../service-a) — base: `main`. Dossier: [repos/service-a.md](./repos/service-a.md)\n" +
		"- [service-b](../service-b) — base: `develop`. Dossier: [repos/service-b.md](./repos/service-b.md)\n" +
		"\n" +
		"## Relationships\n"

	got := workspacemap.Render(workspacemap.DefaultContent, repos)
	if got != want {
		t.Errorf("Render() mismatch\n got: %q\nwant: %q", got, want)
	}
}

func TestRenderWithNoRepos(t *testing.T) {
	want := "# Workspace Map\n" +
		"\n" +
		"## Repos\n" +
		"\n" +
		"\n" +
		"\n" +
		"## Relationships\n"

	got := workspacemap.Render(workspacemap.DefaultContent, nil)
	if got != want {
		t.Errorf("Render() mismatch\n got: %q\nwant: %q", got, want)
	}
}

func TestRenderLeavesContentBeforeReposUntouched(t *testing.T) {
	existing := "# Workspace Map\n" +
		"\n" +
		"Some hand-written preamble.\n" +
		"\n" +
		"## Repos\n" +
		"\n" +
		"## Relationships\n" +
		"\n" +
		"hand-written notes\n"

	repos := []manifest.Repo{{Name: "solo", Path: "../solo", BaseBranch: "main"}}

	want := "# Workspace Map\n" +
		"\n" +
		"Some hand-written preamble.\n" +
		"\n" +
		"## Repos\n" +
		"\n" +
		"- [solo](../solo) — base: `main`. Dossier: [repos/solo.md](./repos/solo.md)\n" +
		"\n" +
		"## Relationships\n" +
		"\n" +
		"hand-written notes\n"

	got := workspacemap.Render(existing, repos)
	if got != want {
		t.Errorf("Render() mismatch\n got: %q\nwant: %q", got, want)
	}
}
