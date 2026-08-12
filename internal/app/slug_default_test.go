package app

import (
	"strings"
	"testing"
)

func TestDefaultSlug(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"Hello World", "hello-world"},
		{"Hello, World!", "hello-world"},
		{"Code_Review v2", "code-review-v2"},
		{"  Leading and trailing  ", "leading-and-trailing"},
		{"UPPERCASE", "uppercase"},
		{"123 Abc", "123-abc"},
		{"single", "single"},
		{"a--b", "a-b"},
		{"-leading", "leading"},
		{"trailing-", "trailing"},
		{"A - B", "a-b"},
	}
	for _, tc := range cases {
		got, err := defaultSlug(tc.name)
		if err != nil {
			t.Fatalf("defaultSlug(%q): %v", tc.name, err)
		}
		if got != tc.want {
			t.Fatalf("defaultSlug(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestDefaultSlugRejectsEmptyResult(t *testing.T) {
	for _, name := range []string{"", "---", "!!!", "中文", "Ü"} {
		_, err := defaultSlug(name)
		if err == nil || !strings.Contains(err.Error(), "explicit slug") {
			t.Fatalf("defaultSlug(%q) must fail with explicit-slug guidance: %v", name, err)
		}
	}
}

// TestDefaultSlugDropsNonASCII proves non-ASCII characters are not
// transliterated: they collapse like any separator and the remaining ASCII
// letters are preserved.
func TestDefaultSlugDropsNonASCII(t *testing.T) {
	got, err := defaultSlug("Über Cool")
	if err != nil {
		t.Fatal(err)
	}
	if got != "ber-cool" {
		t.Fatalf("non-ASCII must be dropped, got %q", got)
	}
}

func TestDefaultSlugLengthBoundary(t *testing.T) {
	long := strings.Repeat("a", 65)
	if _, err := defaultSlug(long); err == nil || !strings.Contains(err.Error(), "explicit slug") {
		t.Fatalf("65-character slug must be rejected with guidance: %v", err)
	}
	// a separator run collapsing over 64 characters is rejected too
	sep := strings.Join([]string{strings.Repeat("a", 40), strings.Repeat("b", 40)}, " ")
	if _, err := defaultSlug(sep); err == nil {
		t.Fatal("collapsed slug over 64 must be rejected")
	}
	boundary := strings.Repeat("a", 64)
	got, err := defaultSlug(boundary)
	if err != nil || got != boundary {
		t.Fatalf("64-character slug: %q, %v", got, err)
	}
}
