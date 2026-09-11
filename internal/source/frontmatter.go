package source

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// SkillField is one top-level YAML frontmatter field of a SKILL.md, in
// document order. A non-scalar value keeps its YAML text.
type SkillField struct {
	Key   string
	Value string
}

// SkillDocument is one SKILL.md split into its frontmatter fields and its
// Markdown body.
type SkillDocument struct {
	Fields []SkillField
	Body   string
}

// skillFrontmatter extracts and validates the YAML frontmatter of a
// SKILL.md: a leading --- delimited block containing non-empty name and
// description strings, as required by the Agent Skills convention.
func skillFrontmatter(data []byte) (name, description string, err error) {
	block, _, ok := splitFrontmatter(data)
	if !ok {
		return "", "", fmt.Errorf("missing YAML frontmatter")
	}
	var fm struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal(block, &fm); err != nil {
		return "", "", fmt.Errorf("invalid frontmatter: %v", err)
	}
	if strings.TrimSpace(fm.Name) == "" {
		return "", "", fmt.Errorf("frontmatter name is required")
	}
	if strings.TrimSpace(fm.Description) == "" {
		return "", "", fmt.Errorf("frontmatter description is required")
	}
	return fm.Name, fm.Description, nil
}

// splitFrontmatter returns the bytes between the leading --- line and its
// closing --- line, and the remaining document body. A UTF-8 BOM before
// the opening marker is tolerated. ok is false when the block is absent.
func splitFrontmatter(data []byte) (block, body []byte, ok bool) {
	text := strings.TrimPrefix(string(data), "\ufeff")
	lines := strings.Split(text, "\n")
	if len(lines) < 3 || strings.TrimRight(lines[0], "\r") != "---" {
		return nil, []byte(text), false
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r") == "---" {
			return []byte(strings.Join(lines[1:i], "\n")), []byte(strings.Join(lines[i+1:], "\n")), true
		}
	}
	return nil, []byte(text), false
}

// ParseSkillDocument splits one SKILL.md into ordered frontmatter fields
// and its Markdown body. A missing frontmatter block is not an error.
func ParseSkillDocument(data []byte) (SkillDocument, error) {
	block, body, ok := splitFrontmatter(data)
	if !ok {
		return SkillDocument{Fields: []SkillField{}, Body: string(body)}, nil
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(block, &doc); err != nil {
		return SkillDocument{}, fmt.Errorf("invalid frontmatter: %v", err)
	}
	root := &doc
	if doc.Kind == yaml.DocumentNode {
		if len(doc.Content) == 0 {
			return SkillDocument{}, fmt.Errorf("frontmatter is not a mapping")
		}
		root = doc.Content[0]
	}
	if root.Kind != yaml.MappingNode {
		return SkillDocument{}, fmt.Errorf("frontmatter is not a mapping")
	}
	fields := make([]SkillField, 0, len(root.Content)/2)
	for i := 0; i+1 < len(root.Content); i += 2 {
		value, err := skillFieldValue(root.Content[i+1])
		if err != nil {
			return SkillDocument{}, fmt.Errorf("invalid frontmatter: %v", err)
		}
		fields = append(fields, SkillField{Key: root.Content[i].Value, Value: value})
	}
	return SkillDocument{Fields: fields, Body: strings.TrimLeft(string(body), "\r\n")}, nil
}

func skillFieldValue(n *yaml.Node) (string, error) {
	if n.Kind == yaml.ScalarNode {
		return n.Value, nil
	}
	b, err := yaml.Marshal(n)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(string(b), "\n"), nil
}
