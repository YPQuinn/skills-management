package source

import (
	"strings"
	"testing"
)

func TestParseSkillDocument(t *testing.T) {
	cases := []struct {
		name    string
		data    string
		want    SkillDocument
		wantErr string
	}{
		{
			name: "field order with extra key",
			data: "---\nname: alpha\ndescription: first\nlicense: MIT\n---\n\n# Alpha\n",
			want: SkillDocument{
				Fields: []SkillField{
					{Key: "name", Value: "alpha"},
					{Key: "description", Value: "first"},
					{Key: "license", Value: "MIT"},
				},
				Body: "# Alpha\n",
			},
		},
		{
			name: "folded description",
			data: "---\nname: alpha\ndescription: >-\n  First line\n  second line\n---\n\n# Alpha\n",
			want: SkillDocument{
				Fields: []SkillField{
					{Key: "name", Value: "alpha"},
					{Key: "description", Value: "First line second line"},
				},
				Body: "# Alpha\n",
			},
		},
		{
			name: "sequence value",
			data: "---\nname: alpha\ndescription: desc\nallowed-tools: [Read, Grep]\n---\n# Body\n",
			want: SkillDocument{
				Fields: []SkillField{
					{Key: "name", Value: "alpha"},
					{Key: "description", Value: "desc"},
					{Key: "allowed-tools", Value: "[Read, Grep]"},
				},
				Body: "# Body\n",
			},
		},
		{
			name: "no frontmatter",
			data: "# Just markdown\n\nHello\n",
			want: SkillDocument{
				Fields: []SkillField{},
				Body:   "# Just markdown\n\nHello\n",
			},
		},
		{
			name:    "invalid YAML",
			data:    "---\nname: [unterminated\n---\n# Body\n",
			wantErr: "invalid frontmatter",
		},
		{
			name:    "frontmatter is not a mapping",
			data:    "---\n- just\n- a list\n---\n# Body\n",
			wantErr: "frontmatter is not a mapping",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseSkillDocument([]byte(c.data))
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("error: got %v, want substring %q", err, c.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Body != c.want.Body {
				t.Fatalf("body: got %q, want %q", got.Body, c.want.Body)
			}
			if len(got.Fields) != len(c.want.Fields) {
				t.Fatalf("fields: got %+v, want %+v", got.Fields, c.want.Fields)
			}
			for i := range c.want.Fields {
				if got.Fields[i] != c.want.Fields[i] {
					t.Fatalf("field[%d]: got %+v, want %+v", i, got.Fields[i], c.want.Fields[i])
				}
			}
		})
	}
}
