package vault

import (
	"testing"
)

func TestParseFrontmatter_NoFrontmatter(t *testing.T) {
	// Content without --- delimiters should return an empty map.
	tests := []struct {
		name    string
		content string
	}{
		{"plain text", "Hello world, no frontmatter here."},
		{"heading only", "# My Note\n\nSome body text."},
		{"dashes in middle", "Some text\n---\ntitle: not frontmatter\n---\nMore text"},
		{"single dash line", "-\nNot frontmatter"},
		{"double dash", "--\nNot frontmatter\n--"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm := ParseFrontmatter(tt.content)
			if len(fm) != 0 {
				t.Errorf("expected empty map, got %d entries: %v", len(fm), fm)
			}
		})
	}
}

func TestParseFrontmatter_EmptyFrontmatter(t *testing.T) {
	// Two --- with nothing between them should return an empty map.
	tests := []struct {
		name    string
		content string
	}{
		{"empty block", "---\n---\nBody text"},
		{"whitespace only", "---\n   \n\t\n---\nBody text"},
		{"newlines only", "---\n\n\n---\nBody text"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm := ParseFrontmatter(tt.content)
			if len(fm) != 0 {
				t.Errorf("expected empty map, got %d entries: %v", len(fm), fm)
			}
		})
	}
}

func TestParseFrontmatter_MalformedYAML(t *testing.T) {
	// Random text between --- delimiters that does not follow key: value format.
	tests := []struct {
		name    string
		content string
		wantLen int
	}{
		{
			name:    "random words no colons",
			content: "---\nhello world\nfoo bar baz\n---\nBody",
			wantLen: 0,
		},
		{
			name:    "only numbers",
			content: "---\n12345\n67890\n---\nBody",
			wantLen: 0,
		},
		{
			name:    "mixed valid and invalid lines",
			content: "---\ntitle: Valid\nthis is garbage\nalso garbage\nauthor: Also Valid\n---\nBody",
			wantLen: 2, // only lines with colons get parsed
		},
		{
			name:    "empty lines between valid entries",
			content: "---\ntitle: A\n\n\nauthor: B\n---\nBody",
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm := ParseFrontmatter(tt.content)
			if len(fm) != tt.wantLen {
				t.Errorf("expected %d entries, got %d: %v", tt.wantLen, len(fm), fm)
			}
		})
	}
}

func TestParseFrontmatter_BinaryContent(t *testing.T) {
	// Non-text bytes in content should not crash the parser.
	content := "---\ntitle: Binary Test\n---\n\x00\x01\x02\x03\xff\xfe\xfd"
	fm := ParseFrontmatter(content)

	if fm["title"] != "Binary Test" {
		t.Errorf("expected 'Binary Test', got '%v'", fm["title"])
	}

	// Also test binary content within frontmatter — should not panic.
	binaryFm := "---\ntitle: \x00\x01\x02\n---\nBody"
	fm2 := ParseFrontmatter(binaryFm)
	if len(fm2) != 1 {
		t.Errorf("expected 1 entry, got %d", len(fm2))
	}
}

func TestParseFrontmatter_NestedArrays(t *testing.T) {
	// Tags with brackets [a, b, c] — the parser's simple bracket handling.
	content := "---\ntags: [alpha, beta, gamma]\n---\nBody"
	fm := ParseFrontmatter(content)

	tags, ok := fm["tags"].([]string)
	if !ok {
		t.Fatalf("expected []string, got %T", fm["tags"])
	}
	if len(tags) != 3 {
		t.Fatalf("expected 3 tags, got %d: %v", len(tags), tags)
	}
	expected := []string{"alpha", "beta", "gamma"}
	for i, exp := range expected {
		if tags[i] != exp {
			t.Errorf("tags[%d] = %q, want %q", i, tags[i], exp)
		}
	}
}

func TestParseFrontmatter_QuotedCommas(t *testing.T) {
	// Value with commas in quotes — the parser treats the whole value as a string
	// since it does not start with [.
	content := "---\ndescription: \"hello, world\"\n---\nBody"
	fm := ParseFrontmatter(content)

	desc, ok := fm["description"].(string)
	if !ok {
		t.Fatalf("expected string, got %T", fm["description"])
	}
	if desc != "\"hello, world\"" {
		t.Errorf("expected '\"hello, world\"', got '%s'", desc)
	}
}

func TestParseWikiLinks_NestedBrackets(t *testing.T) {
	// [[note[0]]] — known limitation but should not crash.
	content := "[[note[0]]]"
	// Should not panic.
	links := ParseWikiLinks(content)
	// The regex may or may not match this; the important thing is no crash.
	_ = links
}

func TestParseWikiLinks_Escaped(t *testing.T) {
	// \[\[not a link\]\] should not match as a wikilink.
	// Note: the regex does not handle backslash escaping, so this documents
	// current behavior rather than ideal behavior.
	content := `\[\[not a link\]\]`
	links := ParseWikiLinks(content)
	// The regex won't match because the brackets are escaped with backslashes.
	if len(links) != 0 {
		t.Logf("parser matched escaped brackets (known limitation): %v", links)
	}
}

func TestParseWikiLinks_WithAlias(t *testing.T) {
	// [[note|display text]] should extract "note" only.
	content := "See [[my-note|My Display Text]]"
	links := ParseWikiLinks(content)

	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d: %v", len(links), links)
	}
	if links[0] != "my-note" {
		t.Errorf("expected 'my-note', got '%s'", links[0])
	}
}

func TestParseWikiLinks_WithHeading(t *testing.T) {
	// [[note#heading]] should extract "note#heading".
	content := "Go to [[my-note#section-two]]"
	links := ParseWikiLinks(content)

	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d: %v", len(links), links)
	}
	if links[0] != "my-note#section-two" {
		t.Errorf("expected 'my-note#section-two', got '%s'", links[0])
	}
}

func TestParseWikiLinks_MultipleOnOneLine(t *testing.T) {
	// Multiple links on a single line should all be extracted.
	content := "See [[Alpha]] and [[Beta]] and [[Gamma]]"
	links := ParseWikiLinks(content)

	if len(links) != 3 {
		t.Fatalf("expected 3 links, got %d: %v", len(links), links)
	}
	expected := []string{"Alpha", "Beta", "Gamma"}
	for i, exp := range expected {
		if links[i] != exp {
			t.Errorf("links[%d] = %q, want %q", i, links[i], exp)
		}
	}
}

func TestParseWikiLinks_InsideCodeBlock(t *testing.T) {
	// Links inside ``` blocks — the parser does NOT skip code blocks.
	// This test documents the current behavior.
	content := "Normal [[outside]]\n```\n[[inside-code]]\n```\nAfter [[also-outside]]"
	links := ParseWikiLinks(content)

	// Parser extracts all links regardless of code blocks.
	if len(links) != 3 {
		t.Fatalf("expected 3 links (parser does not skip code blocks), got %d: %v", len(links), links)
	}
	expected := []string{"outside", "inside-code", "also-outside"}
	for i, exp := range expected {
		if links[i] != exp {
			t.Errorf("links[%d] = %q, want %q", i, links[i], exp)
		}
	}
}

func TestStripFrontmatter_NormalContent(t *testing.T) {
	// Strips frontmatter and returns only the body.
	content := "---\ntitle: My Title\nauthor: Jane\ntags: [x, y]\n---\n# Heading\n\nParagraph text."
	body := StripFrontmatter(content)

	expected := "# Heading\n\nParagraph text."
	if body != expected {
		t.Errorf("expected %q, got %q", expected, body)
	}
}

func TestStripFrontmatter_NoFrontmatter(t *testing.T) {
	// Content without frontmatter is returned unchanged.
	tests := []struct {
		name    string
		content string
	}{
		{"plain text", "Just some text without frontmatter."},
		{"heading", "# My Heading\n\nParagraph."},
		{"empty string", ""},
		{"dashes in middle", "Text before\n---\nText after"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StripFrontmatter(tt.content)
			if result != tt.content {
				t.Errorf("expected content unchanged %q, got %q", tt.content, result)
			}
		})
	}
}
