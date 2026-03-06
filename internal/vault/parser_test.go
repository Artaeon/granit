package vault

import (
	"testing"
)

func TestParseWikiLinks(t *testing.T) {
	content := "Some text with [[link1]] and [[link2|display]].\n"

	links := ParseWikiLinks(content)

	if len(links) != 2 {
		t.Fatalf("expected 2 links, got %d", len(links))
	}
	if links[0] != "link1" {
		t.Errorf("expected first link 'link1', got '%s'", links[0])
	}
	if links[1] != "link2" {
		t.Errorf("expected second link 'link2', got '%s'", links[1])
	}
}

func TestParseWikiLinksDeduplicate(t *testing.T) {
	content := "[[same]] and [[same]] again"

	links := ParseWikiLinks(content)

	if len(links) != 1 {
		t.Errorf("expected 1 unique link, got %d", len(links))
	}
}

func TestParseFrontmatter(t *testing.T) {
	content := "---\ntitle: My Note\ndate: 2024-01-01\ntags: [a, b, c]\n---\nBody text"

	fm := ParseFrontmatter(content)

	if fm["title"] != "My Note" {
		t.Errorf("expected title 'My Note', got '%v'", fm["title"])
	}
	if fm["date"] != "2024-01-01" {
		t.Errorf("expected date '2024-01-01', got '%v'", fm["date"])
	}

	tags, ok := fm["tags"].([]string)
	if !ok {
		t.Fatalf("expected tags to be []string, got %T", fm["tags"])
	}
	if len(tags) != 3 {
		t.Errorf("expected 3 tags, got %d", len(tags))
	}
}

func TestParseFrontmatterEmpty(t *testing.T) {
	content := "# No frontmatter here\n\nJust text."

	fm := ParseFrontmatter(content)

	if len(fm) != 0 {
		t.Errorf("expected empty frontmatter map, got %d entries", len(fm))
	}
}

func TestStripFrontmatter(t *testing.T) {
	content := "---\ntitle: Test\n---\nBody text here"

	body := StripFrontmatter(content)

	if body != "Body text here" {
		t.Errorf("expected 'Body text here', got '%s'", body)
	}
}

func TestStripFrontmatterNoFrontmatter(t *testing.T) {
	content := "# Simple Note\n\nJust some text."

	body := StripFrontmatter(content)

	if body != content {
		t.Errorf("expected content unchanged, got '%s'", body)
	}
}
