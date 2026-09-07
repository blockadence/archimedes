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
type Repo struct {
	Name       string `yaml:"name"`
	Path       string `yaml:"path"`
	BaseBranch string `yaml:"base_branch"`
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

// Resolve looks name up and returns its entry with Path resolved against
// root (the instance directory holding repos.yaml), so callers get a
// usable checkout path instead of the relative one the file records.
// Mirrors lib.sh's repo_path helper, plus the rest of the entry callers
// need alongside it.
func (m *Manifest) Resolve(root, name string) (Repo, error) {
	r, ok := m.Find(name)
	if !ok {
		return Repo{}, fmt.Errorf("unknown repo: %s", name)
	}
	r.Path = filepath.Join(root, r.Path)
	return r, nil
}
