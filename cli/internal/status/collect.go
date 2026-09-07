package status

import (
	"fmt"
	"path/filepath"

	"github.com/blockadence/archimedes/cli/internal/manifest"
)

// Collect builds the whole report for the instance at root: every row of
// every work/<slug>/status.md (narrowed to one slug when slugFilter is
// set), each row's live PR state, and the guardrail verdict.
//
// src carries the report's PR, ref, and merged-state sources; its Repos is
// filled in here, since resolving a row's repo name needs root's manifest,
// which only this layer loads. Every way of asking for a status report
// comes through here, so none of them can disagree about what the instance
// currently looks like.
func Collect(root, slugFilter string, src Sources, guardrailMax int) (Report, error) {
	manifestPath := filepath.Join(root, "repos.yaml")
	m, err := manifest.Load(manifestPath)
	if err != nil {
		return Report{}, fmt.Errorf("loading %s: %w", manifestPath, err)
	}
	src.Repos = ManifestRepos(m, root)

	entries, err := Discover(filepath.Join(root, "work"), slugFilter)
	if err != nil {
		return Report{}, fmt.Errorf("discovering status files: %w", err)
	}

	return BuildReport(entries, src, guardrailMax), nil
}
