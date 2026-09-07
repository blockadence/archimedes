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
	// here tends to reach into. A context-mapping pass also reads it as
	// ordering: a repo is mapped after the repos it depends on, so each
	// session can be primed with their maps.
	DependsOn []string `yaml:"depends_on"`
	// ContextModeledSHA records the base-branch commit the repo's context
	// map was last built against; empty means never mapped.
	ContextModeledSHA string `yaml:"context_modeled_sha"`
	// ConventionPack names the shared build/lint convention this repo
	// follows (convention-packs/<name>.yaml).
	ConventionPack string `yaml:"convention_pack"`
	// Driver names the context-mapping driver to run for this repo
	// (drivers/<name>), overriding the instance-wide Manifest.Driver.
	Driver string `yaml:"driver"`
}

// FieldContextModeledSHA is Repo.ContextModeledSHA's key in repos.yaml,
// named here because SetRepoField addresses fields by their YAML key
// rather than through the struct.
const FieldContextModeledSHA = "context_modeled_sha"

// Manifest is the parsed contents of repos.yaml.
type Manifest struct {
	// Driver is the instance-wide default context-mapping driver, used for
	// any repo that doesn't name one of its own.
	Driver string `yaml:"driver"`
	Repos  []Repo `yaml:"repos"`
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

// SetRepoField writes value to one repo's field in the repos.yaml at path:
// the port of lib.sh's set_repo_field, which did the same with `yq -i`. The
// field is added if that repo doesn't carry it yet.
//
// repos.yaml is hand-edited — it ships with explanatory comments and
// accumulates the operator's own — so the rewrite goes through a yaml.Node
// round-trip, preserving comments and per-node style, rather than
// re-marshaling the parsed Manifest struct (which would drop both, plus
// every field this package doesn't model).
func SetRepoField(path, repoName, field, value string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}

	repo, err := findRepoNode(&doc, repoName)
	if err != nil {
		return err
	}
	setMapValue(repo, field, value)

	out, err := encode(&doc)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, info.Mode().Perm())
}

// findRepoNode locates the mapping node for repoName under the document's
// top-level "repos" sequence.
func findRepoNode(doc *yaml.Node, repoName string) (*yaml.Node, error) {
	unknown := fmt.Errorf("unknown repo: %s", repoName)
	if len(doc.Content) == 0 {
		return nil, unknown
	}
	repos := mapValue(doc.Content[0], "repos")
	if repos == nil {
		return nil, unknown
	}
	for _, entry := range repos.Content {
		if name := mapValue(entry, "name"); name != nil && name.Value == repoName {
			return entry, nil
		}
	}
	return nil, unknown
}
