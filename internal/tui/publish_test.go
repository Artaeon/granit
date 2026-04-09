package tui

import (
	"strings"
	"testing"
)

func TestSplitPatterns_Basic(t *testing.T) {
	result := splitPatterns("*.md, *.txt, *.html")
	if len(result) != 3 {
		t.Fatalf("expected 3 patterns, got %d", len(result))
	}
	if result[0] != "*.md" || result[1] != "*.txt" || result[2] != "*.html" {
		t.Errorf("unexpected patterns: %v", result)
	}
}

func TestSplitPatterns_Empty(t *testing.T) {
	if result := splitPatterns(""); result != nil {
		t.Errorf("expected nil for empty, got %v", result)
	}
	if result := splitPatterns("   "); result != nil {
		t.Errorf("expected nil for whitespace, got %v", result)
	}
}

func TestSplitPatterns_SingleItem(t *testing.T) {
	result := splitPatterns("*.md")
	if len(result) != 1 || result[0] != "*.md" {
		t.Errorf("expected [*.md], got %v", result)
	}
}

func TestPublishHTMLEscape(t *testing.T) {
	got := publishHTMLEscape(`<script>alert("xss")</script> & more`)
	if strings.Contains(got, "<script>") {
		t.Error("should escape angle brackets")
	}
	if !strings.Contains(got, "&amp;") {
		t.Error("should escape ampersand")
	}
	if !strings.Contains(got, "&quot;") {
		t.Error("should escape quotes")
	}
}

func TestPublishParseFrontmatter_Basic(t *testing.T) {
	content := "---\ntitle: My Note\ntags: [go, tui]\ndate: 2026-04-09\n---\n\n# Content"
	fm := publishParseFrontmatter(content)

	if fm["title"] != "My Note" {
		t.Errorf("expected title 'My Note', got %v", fm["title"])
	}
	if fm["date"] != "2026-04-09" {
		t.Errorf("expected date, got %v", fm["date"])
	}
	tags, ok := fm["tags"].([]string)
	if !ok || len(tags) != 2 {
		t.Errorf("expected 2 tags, got %v", fm["tags"])
	}
}

func TestPublishParseFrontmatter_NoFrontmatter(t *testing.T) {
	fm := publishParseFrontmatter("# Just content")
	if len(fm) != 0 {
		t.Errorf("expected empty map, got %v", fm)
	}
}

func TestPublishParseFrontmatter_MalformedNoClose(t *testing.T) {
	fm := publishParseFrontmatter("---\ntitle: Test\nno closing")
	if len(fm) != 0 {
		t.Errorf("expected empty for unclosed frontmatter, got %v", fm)
	}
}

func TestPublishStripFrontmatter(t *testing.T) {
	content := "---\ntitle: Test\n---\n\n# Hello World"
	result := publishStripFrontmatter(content)
	if result != "# Hello World" {
		t.Errorf("expected '# Hello World', got %q", result)
	}
}

func TestPublishStripFrontmatter_NoFrontmatter(t *testing.T) {
	content := "# Just content"
	result := publishStripFrontmatter(content)
	if result != content {
		t.Errorf("expected unchanged content, got %q", result)
	}
}

func TestPublishUniqueStrings(t *testing.T) {
	result := publishUniqueStrings([]string{"a", "b", "a", "c", "b"})
	if len(result) != 3 {
		t.Errorf("expected 3 unique strings, got %d", len(result))
	}
}

func TestPublishUniqueStrings_Empty(t *testing.T) {
	result := publishUniqueStrings(nil)
	if len(result) != 0 {
		t.Errorf("expected empty, got %v", result)
	}
}

func TestPublishMarkdownToHTML_Headings(t *testing.T) {
	html := publishMarkdownToHTML("# Title\n\nParagraph.\n")
	if !strings.Contains(html, "<h1") {
		t.Error("expected h1 tag")
	}
	if !strings.Contains(html, "Paragraph") {
		t.Error("expected paragraph content")
	}
}

func TestPublishMarkdownToHTML_WikiLinks(t *testing.T) {
	html := publishMarkdownToHTML("See [[My Note]] for details.\n")
	// Wiki links should be converted to HTML links
	if strings.Contains(html, "[[") {
		t.Error("wiki links should be converted, not left as-is")
	}
}

func TestPublishConvertInline_Bold(t *testing.T) {
	result := publishConvertInline("This is **bold** text")
	if !strings.Contains(result, "<strong>bold</strong>") {
		t.Errorf("expected bold conversion, got %q", result)
	}
}

func TestPublishConvertInline_Italic(t *testing.T) {
	result := publishConvertInline("This is *italic* text")
	if !strings.Contains(result, "<em>italic</em>") {
		t.Errorf("expected italic conversion, got %q", result)
	}
}

func TestPublishConvertInline_Code(t *testing.T) {
	result := publishConvertInline("Use `fmt.Println` here")
	if !strings.Contains(result, "<code>") {
		t.Errorf("expected code conversion, got %q", result)
	}
}
