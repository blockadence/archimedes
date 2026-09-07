package manifest

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// AppendRepo adds r to the end of repos.yaml's "repos" list, reporting
// whether it actually added anything: a repo already listed under that name
// leaves the file byte-for-byte untouched, which is what makes re-running
// bootstrap against an existing instance add only what's missing.
//
// The file is edited as a YAML node tree rather than by round-tripping
// through Manifest, so comments, key order, and any fields this package
// doesn't model survive the rewrite. (The one cosmetic exception: yaml.v3
// re-emits a multi-line comment's continuation lines at column zero, so a
// hand-indented comment block loses its hanging indent the first time an
// entry is appended. The text itself is preserved.)
func AppendRepo(path string, r Repo) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return false, fmt.Errorf("parsing %s: %w", path, err)
	}

	repos, err := reposSequence(&doc)
	if err != nil {
		return false, fmt.Errorf("%s: %w", path, err)
	}

	for _, entry := range repos.Content {
		if nodeField(entry, "name") == r.Name {
			return false, nil
		}
	}

	// An empty list parses as flow style ("[]"); appending to it would
	// otherwise emit the new entry inline on one line.
	repos.Style = 0
	repos.Content = append(repos.Content, repoNode(r))

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return false, fmt.Errorf("encoding %s: %w", path, err)
	}
	if err := enc.Close(); err != nil {
		return false, fmt.Errorf("encoding %s: %w", path, err)
	}

	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return false, err
	}

	return true, nil
}

// reposSequence returns doc's top-level "repos" list as a node ready to
// append to, creating it (and the document's root mapping) when the
// manifest is still empty or has no such key yet.
func reposSequence(doc *yaml.Node) (*yaml.Node, error) {
	if len(doc.Content) == 0 {
		doc.Kind = yaml.DocumentNode
		doc.Content = []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}
	}

	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("expected a mapping at the top level, got %s", root.Tag)
	}

	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != "repos" {
			continue
		}
		value := root.Content[i+1]
		switch {
		case value.Kind == yaml.SequenceNode:
			return value, nil
		case value.Tag == "!!null":
			// "repos:" with nothing under it yet.
			*value = yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
			return value, nil
		default:
			return nil, fmt.Errorf("expected repos to be a list, got %s", value.Tag)
		}
	}

	seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	root.Content = append(root.Content, scalarNode("!!str", "repos"), seq)
	return seq, nil
}

// repoNode renders one repos.yaml entry. Every per-repo field is written,
// including the ones bootstrap has no value for, so the keys an operator
// fills in later (notably convention_pack and driver) are already there to
// edit rather than having to be remembered.
func repoNode(r Repo) *yaml.Node {
	entry := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	set := func(key string, value *yaml.Node) {
		entry.Content = append(entry.Content, scalarNode("!!str", key), value)
	}

	set("name", scalarNode("!!str", r.Name))
	set("path", scalarNode("!!str", r.Path))
	set("base_branch", scalarNode("!!str", r.BaseBranch))
	set("depends_on", stringsNode(r.DependsOn))
	set("context_modeled_sha", stringOrNull(r.ContextModeledSHA))
	set("convention_pack", stringOrNull(r.ConventionPack))
	set("driver", stringOrNull(r.Driver))

	return entry
}

func scalarNode(tag, value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value}
}

// stringOrNull writes an unset field as an explicit null rather than an
// empty string, so the key reads as "nothing declared here yet".
func stringOrNull(value string) *yaml.Node {
	if value == "" {
		return scalarNode("!!null", "null")
	}
	return scalarNode("!!str", value)
}

// stringsNode renders a list of names, kept on one line so a repo entry
// stays scannable.
func stringsNode(values []string) *yaml.Node {
	node := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: yaml.FlowStyle}
	for _, v := range values {
		node.Content = append(node.Content, scalarNode("!!str", v))
	}
	return node
}

// nodeField reads a scalar field off a mapping node, or "" if it has no
// such key.
func nodeField(node *yaml.Node, key string) string {
	if node.Kind != yaml.MappingNode {
		return ""
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1].Value
		}
	}
	return ""
}
