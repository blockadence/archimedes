// Package manifest reads an instance's repos.yaml, the source of truth for
// which repos it knows about and how they relate.
package manifest

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Repo is one entry under repos.yaml's top-level "repos" list.
//
// Bootstrap fills in Name, Path and BaseBranch from what the forge already
// knows. The rest are the operator's to declare afterwards — bootstrap
// scaffolds them present-but-unset so the keys are there to edit — and are
// empty until then.
type Repo struct {
	Name       string `yaml:"name"`
	Path       string `yaml:"path"`
	BaseBranch string `yaml:"base_branch"`
	// DependsOn names the other repos in this instance a unit of work
	// here tends to reach into.
	DependsOn []string `yaml:"depends_on"`
	// ContextModeledSHA records the commit the repo's dossier was last
	// written against.
	ContextModeledSHA string `yaml:"context_modeled_sha"`
	// ConventionPack names the shared build/lint convention this repo
	// follows (convention-packs/<name>.yaml).
	ConventionPack string `yaml:"convention_pack"`
	// Driver names the context-mapping driver to run for this repo
	// (drivers/<name>), overriding the instance-wide default.
	Driver string `yaml:"driver"`
}

// Manifest is the parsed contents of repos.yaml.
type Manifest struct {
	Repos []Repo `yaml:"repos"`
}

// Find looks up a repo by name. The second return value is false if no
// repo with that name exists.
func (m *Manifest) Find(name string) (Repo, bool) {
	for _, r := range m.Repos {
		if r.Name == name {
			return r, true
		}
	}
	return Repo{}, false
}

// Load reads and parses repos.yaml at path.
func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing yaml: %w", err)
	}

	return &m, nil
}

// RepoPath resolves name's local path, relative to root (the instance
// directory containing repos.yaml). Mirrors lib.sh's repo_path helper.
func (m *Manifest) RepoPath(root, name string) (string, error) {
	r, ok := m.Find(name)
	if !ok {
		return "", fmt.Errorf("unknown repo: %s", name)
	}
	return filepath.Join(root, r.Path), nil
}
