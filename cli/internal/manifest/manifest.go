// Package manifest reads an instance's repos.yaml, the source of truth for
// which repos it knows about and how they relate.
package manifest

import (
	"fmt"
	"os"

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
