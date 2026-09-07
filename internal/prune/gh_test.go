package prune

import "testing"

func TestParsePRListStateFirstEntry(t *testing.T) {
	got := parsePRListState([]byte(`[{"state":"MERGED"},{"state":"OPEN"}]`))
	if got != "MERGED" {
		t.Errorf("got %q, want MERGED", got)
	}
}

func TestParsePRListStateNoEntries(t *testing.T) {
	got := parsePRListState([]byte(`[]`))
	if got != "NONE" {
		t.Errorf("got %q, want NONE", got)
	}
}

func TestParsePRListStateMalformed(t *testing.T) {
	got := parsePRListState([]byte(`not json`))
	if got != "NONE" {
		t.Errorf("got %q, want NONE", got)
	}
}
