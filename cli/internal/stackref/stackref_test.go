package stackref_test

import (
	"testing"

	"github.com/blockadence/archimedes/cli/internal/stackref"
)

func TestParseFlag(t *testing.T) {
	tests := []struct {
		in   string
		want stackref.Ref
	}{
		{"", stackref.Ref{}},
		{"target:widget-fix", stackref.Ref{Repo: "target", Slug: "widget-fix"}},
		{"target", stackref.Ref{Repo: "target"}},
		{"target:widget:fix", stackref.Ref{Repo: "target", Slug: "widget:fix"}},
	}

	for _, tt := range tests {
		if got := stackref.ParseFlag(tt.in); got != tt.want {
			t.Errorf("ParseFlag(%q) = %+v, want %+v", tt.in, got, tt.want)
		}
	}
}

func TestNoteRoundTrips(t *testing.T) {
	ref := stackref.Ref{Repo: "service-a", Slug: "auth-api"}

	note := stackref.Note(ref)
	if want := "stacked on service-a:auth-api"; note != want {
		t.Fatalf("Note() = %q, want %q", note, want)
	}

	got, ok := stackref.ParseNote(note)
	if !ok || got != ref {
		t.Errorf("ParseNote(%q) = %+v, %v; want %+v, true", note, got, ok, ref)
	}
}

func TestParseNoteRejectsEverythingElse(t *testing.T) {
	tests := []struct {
		name string
		note string
	}{
		{"plain base note", "based on main"},
		{"empty note", ""},
		{"missing colon", "stacked on service-a"},
		{"missing repo", "stacked on :auth-api"},
		{"missing slug", "stacked on service-a:"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, ok := stackref.ParseNote(tt.note); ok {
				t.Errorf("ParseNote(%q) = %+v, true; want false", tt.note, got)
			}
		})
	}
}

func TestParseNoteToleratesSurroundingWhitespace(t *testing.T) {
	want := stackref.Ref{Repo: "service-a", Slug: "auth-api"}
	if got, ok := stackref.ParseNote("  stacked on service-a:auth-api  "); !ok || got != want {
		t.Errorf("ParseNote() = %+v, %v; want %+v, true", got, ok, want)
	}
}

func TestStringIsTheRepoSlugPair(t *testing.T) {
	if got, want := (stackref.Ref{Repo: "service-a", Slug: "widget-fix"}).String(), "service-a:widget-fix"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
