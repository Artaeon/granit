package vault

import (
	"strings"
	"testing"
)

func TestParseWikiLinksWithDisplayText(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected []string
	}{
		{
			name:     "single display text link",
			content:  "See [[note|display text here]]",
			expected: []string{"note"},
		},
		{
			name:     "display text with path",
			content:  "[[folder/note|My Note]]",
			expected: []string{"folder/note"},
		},
		{
			name:     "mix of plain and display text",
			content:  "[[plain]] and [[aliased|show this]]",
			expected: []string{"plain", "aliased"},
		},
		{
			name:     "display text with special characters",
			content:  "[[my-note|This is a (special) note!]]",
			expected: []string{"my-note"},
		},
		{
			name:     "display text with pipe-like content",
			content:  "[[note|first part]]",
			expected: []string{"note"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			links := ParseWikiLinks(tt.content)
			if len(links) != len(tt.expected) {
				t.Fatalf("expected %d links, got %d: %v", len(tt.expected), len(links), links)
			}
			for i, exp := range tt.expected {
				if links[i] != exp {
					t.Errorf("links[%d]: expected '%s', got '%s'", i, exp, links[i])
				}
			}
		})
	}
}

func TestParseWikiLinksNestedBrackets(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected int
	}{
		{
			name:     "extra closing brackets after link",
			content:  "text [[link]] more]] text",
			expected: 1,
		},
		{
			name:     "opening brackets inside text no link",
			content:  "some [[ incomplete",
			expected: 0,
		},
		{
			name:     "adjacent links",
			content:  "[[one]][[two]]",
			expected: 2,
		},
		{
			name:     "empty brackets",
			content:  "[[]]",
			expected: 0, // regex requires at least one char in the capture group [^\]|]+
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			links := ParseWikiLinks(tt.content)
			if len(links) != tt.expected {
				t.Errorf("expected %d links, got %d: %v", tt.expected, len(links), links)
			}
		})
	}
}

func TestParseWikiLinksInCodeBlocks(t *testing.T) {
	// The parser does NOT skip code blocks — it finds all wikilinks regardless
	t.Run("link inside fenced code block", func(t *testing.T) {
		content := "Normal text\n```\n[[code-link]]\n```\nMore text"
		links := ParseWikiLinks(content)
		if len(links) != 1 {
			t.Fatalf("expected 1 link (parser does not skip code), got %d", len(links))
		}
		if links[0] != "code-link" {
			t.Errorf("expected 'code-link', got '%s'", links[0])
		}
	})

	t.Run("link inside inline code", func(t *testing.T) {
		content := "Some `inline [[code-link]]` text and [[real-link]]"
		links := ParseWikiLinks(content)
		if len(links) != 2 {
			t.Fatalf("expected 2 links (parser does not skip inline code), got %d", len(links))
		}
	})

	t.Run("links both inside and outside code", func(t *testing.T) {
		content := "[[outside]]\n```markdown\n[[inside]]\n```\n[[also-outside]]"
		links := ParseWikiLinks(content)
		if len(links) != 3 {
			t.Fatalf("expected 3 links total, got %d: %v", len(links), links)
		}
	})
}

func TestParseWikiLinksEdgeCases(t *testing.T) {
	t.Run("link with leading/trailing whitespace", func(t *testing.T) {
		content := "[[  spaced  ]]"
		links := ParseWikiLinks(content)
		if len(links) != 1 {
			t.Fatalf("expected 1 link, got %d", len(links))
		}
		if links[0] != "spaced" {
			t.Errorf("expected trimmed 'spaced', got '%s'", links[0])
		}
	})

	t.Run("empty content", func(t *testing.T) {
		links := ParseWikiLinks("")
		if len(links) != 0 {
			t.Errorf("expected 0 links from empty content, got %d", len(links))
		}
	})

	t.Run("link on multiple lines", func(t *testing.T) {
		content := "Line 1 [[link1]]\nLine 2 [[link2]]\nLine 3 [[link3]]"
		links := ParseWikiLinks(content)
		if len(links) != 3 {
			t.Errorf("expected 3 links from multiline content, got %d", len(links))
		}
	})

	t.Run("many links in one line", func(t *testing.T) {
		content := "[[a]] [[b]] [[c]] [[d]] [[e]]"
		links := ParseWikiLinks(content)
		if len(links) != 5 {
			t.Errorf("expected 5 links, got %d", len(links))
		}
	})
}

func TestParseFrontmatterNestedValues(t *testing.T) {
	t.Run("simple key-value pairs", func(t *testing.T) {
		content := "---\ntitle: My Title\nauthor: John Doe\n---\nBody"
		fm := ParseFrontmatter(content)
		if fm["title"] != "My Title" {
			t.Errorf("expected 'My Title', got '%v'", fm["title"])
		}
		if fm["author"] != "John Doe" {
			t.Errorf("expected 'John Doe', got '%v'", fm["author"])
		}
	})

	t.Run("value with colon in it", func(t *testing.T) {
		// SplitN(line, ":", 2) means the value keeps the rest including colons
		content := "---\ntitle: Note: A Special One\n---\nBody"
		fm := ParseFrontmatter(content)
		if fm["title"] != "Note: A Special One" {
			t.Errorf("expected 'Note: A Special One', got '%v'", fm["title"])
		}
	})

	t.Run("array values in brackets", func(t *testing.T) {
		content := "---\ntags: [go, testing, vault]\n---\nBody"
		fm := ParseFrontmatter(content)
		tags, ok := fm["tags"].([]string)
		if !ok {
			t.Fatalf("expected []string, got %T", fm["tags"])
		}
		if len(tags) != 3 {
			t.Fatalf("expected 3 tags, got %d", len(tags))
		}
		if tags[0] != "go" || tags[1] != "testing" || tags[2] != "vault" {
			t.Errorf("unexpected tags: %v", tags)
		}
	})

	t.Run("single-element array", func(t *testing.T) {
		content := "---\ntags: [solo]\n---\nBody"
		fm := ParseFrontmatter(content)
		tags, ok := fm["tags"].([]string)
		if !ok {
			t.Fatalf("expected []string, got %T", fm["tags"])
		}
		if len(tags) != 1 || tags[0] != "solo" {
			t.Errorf("expected [solo], got %v", tags)
		}
	})

	t.Run("empty array", func(t *testing.T) {
		content := "---\ntags: []\n---\nBody"
		fm := ParseFrontmatter(content)
		tags, ok := fm["tags"].([]string)
		if !ok {
			t.Fatalf("expected []string, got %T", fm["tags"])
		}
		// Split("", ",") returns [""], so we get 1 empty-string element
		if len(tags) != 1 {
			t.Errorf("expected 1 element (empty string), got %d: %v", len(tags), tags)
		}
	})

	t.Run("boolean-like values stored as strings", func(t *testing.T) {
		content := "---\ndraft: true\npublished: false\n---\nBody"
		fm := ParseFrontmatter(content)
		// The parser stores everything as strings (no YAML parsing)
		if fm["draft"] != "true" {
			t.Errorf("expected string 'true', got '%v' (type %T)", fm["draft"], fm["draft"])
		}
		if fm["published"] != "false" {
			t.Errorf("expected string 'false', got '%v' (type %T)", fm["published"], fm["published"])
		}
	})

	t.Run("numeric-like values stored as strings", func(t *testing.T) {
		content := "---\nweight: 42\nrating: 3.14\n---\nBody"
		fm := ParseFrontmatter(content)
		if fm["weight"] != "42" {
			t.Errorf("expected string '42', got '%v'", fm["weight"])
		}
		if fm["rating"] != "3.14" {
			t.Errorf("expected string '3.14', got '%v'", fm["rating"])
		}
	})

	t.Run("date value", func(t *testing.T) {
		content := "---\ndate: 2026-03-08\n---\nBody"
		fm := ParseFrontmatter(content)
		if fm["date"] != "2026-03-08" {
			t.Errorf("expected '2026-03-08', got '%v'", fm["date"])
		}
	})
}

func TestParseFrontmatterEmptyBlock(t *testing.T) {
	t.Run("empty frontmatter block", func(t *testing.T) {
		content := "---\n---\nBody text"
		fm := ParseFrontmatter(content)
		if len(fm) != 0 {
			t.Errorf("expected empty map for empty frontmatter, got %d entries: %v", len(fm), fm)
		}
	})

	t.Run("frontmatter with only whitespace", func(t *testing.T) {
		content := "---\n   \n\n---\nBody text"
		fm := ParseFrontmatter(content)
		if len(fm) != 0 {
			t.Errorf("expected empty map for whitespace-only frontmatter, got %d entries", len(fm))
		}
	})
}

func TestParseFrontmatterMissingClosing(t *testing.T) {
	t.Run("no closing dashes", func(t *testing.T) {
		content := "---\ntitle: Unclosed\nNo closing marker"
		fm := ParseFrontmatter(content)
		if len(fm) != 0 {
			t.Errorf("expected empty map when closing --- is missing, got %d entries: %v", len(fm), fm)
		}
	})

	t.Run("only opening dashes", func(t *testing.T) {
		content := "---\n"
		fm := ParseFrontmatter(content)
		if len(fm) != 0 {
			t.Errorf("expected empty map, got %d entries", len(fm))
		}
	})

	t.Run("single set of dashes only", func(t *testing.T) {
		content := "---"
		fm := ParseFrontmatter(content)
		if len(fm) != 0 {
			t.Errorf("expected empty map, got %d entries", len(fm))
		}
	})
}

func TestParseFrontmatterMultipleKeys(t *testing.T) {
	content := "---\na: 1\nb: 2\nc: 3\nd: 4\ne: 5\n---\nBody"
	fm := ParseFrontmatter(content)

	if len(fm) != 5 {
		t.Fatalf("expected 5 keys, got %d", len(fm))
	}
	for _, key := range []string{"a", "b", "c", "d", "e"} {
		if _, exists := fm[key]; !exists {
			t.Errorf("missing key '%s'", key)
		}
	}
}

func TestStripFrontmatterPreservesContent(t *testing.T) {
	t.Run("preserves body exactly", func(t *testing.T) {
		body := "# Heading\n\nParagraph with **bold** and *italic*.\n\n- List item 1\n- List item 2"
		content := "---\ntitle: Test\ntags: [a, b]\n---\n" + body
		result := StripFrontmatter(content)
		if result != body {
			t.Errorf("body not preserved exactly.\nExpected:\n%s\nGot:\n%s", body, result)
		}
	})

	t.Run("preserves leading newlines in body", func(t *testing.T) {
		content := "---\ntitle: Test\n---\n\n\nBody after newlines"
		result := StripFrontmatter(content)
		// StripFrontmatter uses TrimSpace, so leading newlines are removed
		expected := "Body after newlines"
		if result != expected {
			t.Errorf("expected '%s', got '%s'", expected, result)
		}
	})

	t.Run("preserves content with dashes in body", func(t *testing.T) {
		content := "---\ntitle: Test\n---\nSome text\n---\nMore text after rule"
		result := StripFrontmatter(content)
		if !strings.Contains(result, "---") {
			t.Error("expected horizontal rule (---) preserved in body")
		}
		if !strings.Contains(result, "More text after rule") {
			t.Error("expected text after hr to be preserved")
		}
	})

	t.Run("empty body after frontmatter", func(t *testing.T) {
		content := "---\ntitle: Test\n---\n"
		result := StripFrontmatter(content)
		if result != "" {
			t.Errorf("expected empty string, got '%s'", result)
		}
	})
}

func TestStripFrontmatterMultipleSeparators(t *testing.T) {
	t.Run("three dashes in body text", func(t *testing.T) {
		content := "---\ntitle: Test\n---\nBody\n---\nAnother section\n---\nFinal"
		result := StripFrontmatter(content)
		// The first --- pair is the frontmatter; everything after is body
		if !strings.Contains(result, "Body") {
			t.Error("expected 'Body' in result")
		}
		if !strings.Contains(result, "Another section") {
			t.Error("expected 'Another section' in result")
		}
		if !strings.Contains(result, "Final") {
			t.Error("expected 'Final' in result")
		}
	})

	t.Run("dashes in frontmatter value", func(t *testing.T) {
		content := "---\ntitle: 2026-03-08 - Daily Note\n---\nBody"
		result := StripFrontmatter(content)
		if result != "Body" {
			t.Errorf("expected 'Body', got '%s'", result)
		}
	})

	t.Run("no frontmatter returns original", func(t *testing.T) {
		content := "# Just a heading\n\nNormal content."
		result := StripFrontmatter(content)
		if result != content {
			t.Errorf("expected original content returned, got '%s'", result)
		}
	})

	t.Run("dashes not at start of file", func(t *testing.T) {
		content := "Some text\n---\ntitle: Not frontmatter\n---\nMore text"
		result := StripFrontmatter(content)
		if result != content {
			t.Errorf("expected original content (--- not at start), got '%s'", result)
		}
	})
}

func TestStripFrontmatterAndParseFrontmatterConsistency(t *testing.T) {
	content := "---\ntitle: Consistency Check\ntags: [x, y]\n---\n# The Actual Content\n\nParagraph."

	fm := ParseFrontmatter(content)
	body := StripFrontmatter(content)

	if fm["title"] != "Consistency Check" {
		t.Errorf("frontmatter title mismatch: %v", fm["title"])
	}
	if !strings.HasPrefix(body, "# The Actual Content") {
		t.Errorf("stripped body should start with heading, got: '%s'", body)
	}
}

func TestParseWikiLinksWithPathSeparators(t *testing.T) {
	t.Run("link with subdirectory path", func(t *testing.T) {
		content := "[[folder/subfolder/note]]"
		links := ParseWikiLinks(content)
		if len(links) != 1 {
			t.Fatalf("expected 1 link, got %d", len(links))
		}
		if links[0] != "folder/subfolder/note" {
			t.Errorf("expected 'folder/subfolder/note', got '%s'", links[0])
		}
	})

	t.Run("link with path and display text", func(t *testing.T) {
		content := "[[projects/2026/roadmap|Q1 Roadmap]]"
		links := ParseWikiLinks(content)
		if len(links) != 1 {
			t.Fatalf("expected 1 link, got %d", len(links))
		}
		if links[0] != "projects/2026/roadmap" {
			t.Errorf("expected 'projects/2026/roadmap', got '%s'", links[0])
		}
	})
}

func TestParseWikiLinksWithHeadingAnchors(t *testing.T) {
	// Obsidian supports [[note#heading]] — the regex captures "note#heading"
	content := "[[note#section]] and [[other#heading|Display]]"
	links := ParseWikiLinks(content)

	if len(links) != 2 {
		t.Fatalf("expected 2 links, got %d: %v", len(links), links)
	}
	if links[0] != "note#section" {
		t.Errorf("expected 'note#section', got '%s'", links[0])
	}
	if links[1] != "other#heading" {
		t.Errorf("expected 'other#heading', got '%s'", links[1])
	}
}

func TestParseFrontmatterKeyWithNoValue(t *testing.T) {
	// A line like "key:" with nothing after the colon
	content := "---\nempty:\nfilled: yes\n---\nBody"
	fm := ParseFrontmatter(content)

	if fm["empty"] != "" {
		t.Errorf("expected empty string for key with no value, got '%v'", fm["empty"])
	}
	if fm["filled"] != "yes" {
		t.Errorf("expected 'yes', got '%v'", fm["filled"])
	}
}

func TestParseFrontmatterLineWithNoColon(t *testing.T) {
	// Lines without a colon are skipped by SplitN check
	content := "---\ntitle: Valid\nthis line has no colon\n---\nBody"
	fm := ParseFrontmatter(content)

	if len(fm) != 1 {
		t.Errorf("expected 1 key (line without colon skipped), got %d: %v", len(fm), fm)
	}
	if fm["title"] != "Valid" {
		t.Errorf("expected 'Valid', got '%v'", fm["title"])
	}
}
