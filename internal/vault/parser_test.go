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

// ---------------------------------------------------------------------------
// frontmatterBounds — strict-mode regression coverage
// ---------------------------------------------------------------------------

func TestFrontmatterBounds_RequiresOpeningNewline(t *testing.T) {
	// "---" without a newline (e.g. content begins with literal "---title")
	// must not be treated as frontmatter.
	content := "---title: oops\n---\nbody"
	_, _, _, ok := frontmatterBounds(content)
	if ok {
		t.Error("frontmatter without proper '---\\n' opening should be rejected")
	}
}

func TestFrontmatterBounds_AcceptsValidFrontmatter(t *testing.T) {
	content := "---\ntitle: Test\ntags: [a]\n---\nbody"
	bs, be, body, ok := frontmatterBounds(content)
	if !ok {
		t.Fatal("valid frontmatter rejected")
	}
	block := content[bs:be]
	if !contains(block, "title: Test") {
		t.Errorf("block missing title field: %q", block)
	}
	if content[body:] != "body" {
		t.Errorf("body wrong: %q", content[body:])
	}
}

func TestFrontmatterBounds_RejectsBlockWithoutKeyValue(t *testing.T) {
	// Two horizontal rules around prose with no "key: value" line.
	content := "---\njust some prose between rules\n---\nbody"
	_, _, _, ok := frontmatterBounds(content)
	if ok {
		t.Error("block without key:value should not be frontmatter")
	}
}

func TestFrontmatterBounds_AcceptsClosingFenceAtEOF(t *testing.T) {
	content := "---\ntitle: x\n---"
	_, _, _, ok := frontmatterBounds(content)
	if !ok {
		t.Error("closing --- at EOF should be accepted")
	}
}

func TestFrontmatterBounds_RejectsTextAfterClosingFence(t *testing.T) {
	// "---abc" must NOT be treated as a closing fence.
	content := "---\ntitle: x\n---abc"
	_, _, _, ok := frontmatterBounds(content)
	if ok {
		t.Error("'---abc' should not match closing fence")
	}
}

func TestParseFrontmatter_ArrayValue(t *testing.T) {
	content := "---\ntags: [alpha, beta, gamma]\n---\nbody"
	fm := ParseFrontmatter(content)
	tags, ok := fm["tags"].([]string)
	if !ok {
		t.Fatalf("tags not parsed as []string, got %T", fm["tags"])
	}
	if len(tags) != 3 || tags[0] != "alpha" || tags[2] != "gamma" {
		t.Errorf("tags wrong: %v", tags)
	}
}

func TestParseFrontmatter_ValueWithColon(t *testing.T) {
	// SplitN limit 2 means a value containing ':' is preserved.
	content := "---\ntitle: foo: bar\n---\nbody"
	fm := ParseFrontmatter(content)
	if fm["title"] != "foo: bar" {
		t.Errorf("expected 'foo: bar', got %v", fm["title"])
	}
}

func TestParseFrontmatter_KeyWithDashOrUnderscore(t *testing.T) {
	content := "---\nlast_modified: 2026-04-10\nrelated-notes: [a, b]\n---\nbody"
	fm := ParseFrontmatter(content)
	if fm["last_modified"] != "2026-04-10" {
		t.Errorf("underscore key not parsed: %v", fm["last_modified"])
	}
	if _, ok := fm["related-notes"].([]string); !ok {
		t.Errorf("hyphen key not parsed: %v", fm["related-notes"])
	}
}

// helper used by frontmatter tests
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
