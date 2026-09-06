package status

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const sampleStatusMD = `# my-slug

| repo | branch | worktree | note | pr |
|---|---|---|---|---|
| service-a | my-slug | /work/service-a-worktrees/my-slug | based on main | - |
| service-b | my-slug | /work/service-b-worktrees/my-slug | stacked on service-a:my-slug | - |
`

func TestParseFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "status.md")
	if err := os.WriteFile(path, []byte(sampleStatusMD), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ParseFile(path, "my-slug")
	if err != nil {
		t.Fatalf("ParseFile returned error: %v", err)
	}

	want := []Entry{
		{Slug: "my-slug", Repo: "service-a", Branch: "my-slug", Worktree: "/work/service-a-worktrees/my-slug", Note: "based on main"},
		{Slug: "my-slug", Repo: "service-b", Branch: "my-slug", Worktree: "/work/service-b-worktrees/my-slug", Note: "stacked on service-a:my-slug"},
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseFile mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestParseFileSkipsBlankAndMalformedRows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "status.md")
	content := "# my-slug\n\n| repo | branch | worktree | note | pr |\n|---|---|---|---|---|\n\n| service-a | my-slug | /wt | a note | - |\nnot a table row\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ParseFile(path, "my-slug")
	if err != nil {
		t.Fatalf("ParseFile returned error: %v", err)
	}

	want := []Entry{
		{Slug: "my-slug", Repo: "service-a", Branch: "my-slug", Worktree: "/wt", Note: "a note"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseFile mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestParseFileCollapsesInternalWhitespace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "status.md")
	content := "# my-slug\n\n| repo | branch | worktree | note | pr |\n|---|---|---|---|---|\n" +
		"|  service-a  |  my-slug  |  /wt  |  based   on    main  | - |\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ParseFile(path, "my-slug")
	if err != nil {
		t.Fatalf("ParseFile returned error: %v", err)
	}

	want := []Entry{
		{Slug: "my-slug", Repo: "service-a", Branch: "my-slug", Worktree: "/wt", Note: "based on main"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseFile mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestParseFileMissingErrors(t *testing.T) {
	if _, err := ParseFile(filepath.Join(t.TempDir(), "missing.md"), "slug"); err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestDiscover(t *testing.T) {
	dir := t.TempDir()
	workDir := filepath.Join(dir, "work")

	writeStatus := func(slug, content string) {
		slugDir := filepath.Join(workDir, slug)
		if err := os.MkdirAll(slugDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(slugDir, "status.md"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	rowFor := func(slug string) string {
		return "# " + slug + "\n\n| repo | branch | worktree | note | pr |\n|---|---|---|---|---|\n| service-a | " + slug + " | /wt | note | - |\n"
	}

	writeStatus("alpha", rowFor("alpha"))
	writeStatus("beta", rowFor("beta"))

	t.Run("all slugs", func(t *testing.T) {
		got, err := Discover(workDir, "")
		if err != nil {
			t.Fatalf("Discover returned error: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("expected 2 entries, got %d: %#v", len(got), got)
		}
	})

	t.Run("filtered to one slug", func(t *testing.T) {
		got, err := Discover(workDir, "alpha")
		if err != nil {
			t.Fatalf("Discover returned error: %v", err)
		}
		if len(got) != 1 || got[0].Slug != "alpha" {
			t.Fatalf("expected 1 entry for alpha, got %#v", got)
		}
	})

	t.Run("no status.md files yet", func(t *testing.T) {
		got, err := Discover(filepath.Join(dir, "nonexistent"), "")
		if err != nil {
			t.Fatalf("Discover returned error: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("expected no entries, got %#v", got)
		}
	})
}
