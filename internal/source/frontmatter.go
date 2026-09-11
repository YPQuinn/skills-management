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

// SetScalarField sets the top-level scalar frontmatter field key to value
// (written unquoted as "key: value"), or removes it when remove is true,
// preserving every other line of the SKILL.md document byte-for-byte. A
// SKILL.md without a frontmatter block is an error: the Agent Skills
// convention requires name and description, so a managed document always has
// one. changed is false when the requested state already holds, so the
// caller can skip a needless Store write.
func SetScalarField(data []byte, key, value string, remove bool) (out []byte, changed bool, err error) {
	bom := ""
	text := string(data)
	if strings.HasPrefix(text, "\ufeff") {
		bom, text = "\ufeff", strings.TrimPrefix(text, "\ufeff")
	}
	lines := strings.Split(text, "\n")
	if len(lines) < 3 || strings.TrimRight(lines[0], "\r") != "---" {
		return nil, false, fmt.Errorf("missing YAML frontmatter")
	}
	closeIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r") == "---" {
			closeIdx = i
			break
		}
	}
	if closeIdx == -1 {
		return nil, false, fmt.Errorf("missing YAML frontmatter")
	}
	keyIdx := -1
	for i := 1; i < closeIdx; i++ {
		if k, ok := topLevelKey(strings.TrimRight(lines[i], "\r")); ok && k == key {
			keyIdx = i
			break
		}
	}
	newLine := key + ": " + value
	switch {
	case remove:
		if keyIdx == -1 {
			return data, false, nil
		}
		lines = append(lines[:keyIdx], lines[keyIdx+1:]...)
	case keyIdx != -1:
		if strings.TrimRight(lines[keyIdx], "\r") == newLine {
			return data, false, nil
		}
		lines[keyIdx] = newLine
	default:
		lines = append(lines[:closeIdx], append([]string{newLine}, lines[closeIdx:]...)...)
	}
	return []byte(bom + strings.Join(lines, "\n")), true, nil
}

// topLevelKey returns the key of a top-level scalar mapping line ("key:" or
// "key: value" at column zero), rejecting indented keys, list items, and
// comments so a nested or commented occurrence never matches.
func topLevelKey(line string) (string, bool) {
	if line == "" {
		return "", false
	}
	switch line[0] {
	case ' ', '\t', '#', '-':
		return "", false
	}
	ci := strings.IndexByte(line, ':')
	if ci < 0 {
		return "", false
	}
	return strings.TrimSpace(line[:ci]), true
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
