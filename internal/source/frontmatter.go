package source

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// skillFrontmatter extracts and validates the YAML frontmatter of a
// SKILL.md: a leading --- delimited block containing non-empty name and
// description strings, as required by the Agent Skills convention.
func skillFrontmatter(data []byte) (name, description string, err error) {
	block, ok := frontmatterBlock(data)
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

// frontmatterBlock returns the bytes between the leading --- line and its
// closing --- line. A UTF-8 BOM before the opening marker is tolerated.
func frontmatterBlock(data []byte) ([]byte, bool) {
	text := strings.TrimPrefix(string(data), "\ufeff")
	lines := strings.Split(text, "\n")
	if len(lines) < 3 || strings.TrimRight(lines[0], "\r") != "---" {
		return nil, false
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r") == "---" {
			return []byte(strings.Join(lines[1:i], "\n")), true
		}
	}
	return nil, false
}
