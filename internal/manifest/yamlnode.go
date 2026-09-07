package manifest

import (
	"bytes"

	"gopkg.in/yaml.v3"
)

// The plumbing both of this package's writers share. repos.yaml is
// hand-edited — it ships with explanatory comments and accumulates the
// operator's own — so AppendRepo and SetRepoField both edit it as a node
// tree rather than by re-marshaling a Manifest, which would drop comments,
// key order, and any field this package doesn't model.

// mapValue returns the value node for key in a mapping node, or nil.
func mapValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

// nodeField reads a scalar field off a mapping node, or "" if it has no
// such key.
func nodeField(node *yaml.Node, key string) string {
	if value := mapValue(node, key); value != nil {
		return value.Value
	}
	return ""
}

// setMapValue sets key to a plain string scalar in a mapping node,
// appending the key if it isn't there yet.
func setMapValue(node *yaml.Node, key, value string) {
	if existing := mapValue(node, key); existing != nil {
		existing.Kind = yaml.ScalarNode
		existing.Tag = "!!str"
		existing.Style = 0
		existing.Value = value
		existing.Content = nil
		return
	}
	node.Content = append(node.Content, scalarNode("!!str", key), scalarNode("!!str", value))
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

// encode serializes a document node back to YAML at repos.yaml's two-space
// indentation (yaml.v3 defaults to four).
func encode(doc *yaml.Node) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
