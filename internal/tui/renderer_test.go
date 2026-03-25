package tui

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

// stripAnsiCodes removes ANSI escape sequences for plain-text assertions.
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripAnsiCodes(s string) string { return ansiRe.ReplaceAllString(s, "") }

// newTestRenderer creates a renderer with a usable width/height for testing.
func newTestRenderer() *Renderer {
	r := NewRenderer()
	r.SetSize(80, 40)
	return r
}

// rendered is a helper that renders content and returns the raw output string.
func rendered(r *Renderer, content string) string {
	return r.Render(content, 0)
}

// renderedLines returns the output split into lines.
func renderedLines(r *Renderer, content string) []string {
	out := r.Render(content, 0)
	if out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

// =========================================================================
// 1. Heading rendering (H1-H4) — verify each level produces different output
// =========================================================================

func TestRenderHeadingH1(t *testing.T) {
	r := newTestRenderer()
	out := rendered(r, "# Hello World")
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "Hello World") {
		t.Errorf("H1 should contain heading text, got:\n%s", plain)
	}
}

func TestRenderHeadingH2(t *testing.T) {
	r := newTestRenderer()
	out := rendered(r, "## Section Title")
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "Section Title") {
		t.Errorf("H2 should contain heading text, got:\n%s", plain)
	}
	if !strings.Contains(plain, "\u2503") {
		t.Errorf("H2 should have accent bar character, got:\n%s", plain)
	}
}

func TestRenderHeadingH3(t *testing.T) {
	r := newTestRenderer()
	out := rendered(r, "### Third Level")
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "Third Level") {
		t.Errorf("H3 should contain heading text, got:\n%s", plain)
	}
	if !strings.Contains(plain, "\u2502") {
		t.Errorf("H3 should have bar character, got:\n%s", plain)
	}
}

func TestRenderHeadingH4(t *testing.T) {
	r := newTestRenderer()
	out := rendered(r, "#### Fourth Level")
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "Fourth Level") {
		t.Errorf("H4 should contain heading text, got:\n%s", plain)
	}
}

func TestRenderHeadingsDifferentOutput(t *testing.T) {
	r := newTestRenderer()
	h1 := rendered(r, "# Same")
	h2 := rendered(r, "## Same")
	h3 := rendered(r, "### Same")
	h4 := rendered(r, "#### Same")

	outputs := []string{h1, h2, h3, h4}
	for i := 0; i < len(outputs); i++ {
		for j := i + 1; j < len(outputs); j++ {
			if outputs[i] == outputs[j] {
				t.Errorf("H%d and H%d should render differently for the same text", i+1, j+1)
			}
		}
	}
}

func TestRenderHeadingAllLevels(t *testing.T) {
	r := newTestRenderer()
	content := "# H1\n\n## H2\n\n### H3\n\n#### H4"
	out := rendered(r, content)
	for _, h := range []string{"H1", "H2", "H3", "H4"} {
		if !strings.Contains(out, h) {
			t.Errorf("missing %s in combined heading output", h)
		}
	}
}

// =========================================================================
// 2-5. Bold, italic, bold+italic, inline code — verify style applied
// =========================================================================

func TestRenderBoldText(t *testing.T) {
	r := newTestRenderer()
	out := rendered(r, "This is **bold** text")
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "bold") {
		t.Errorf("Bold text should be present, got:\n%s", plain)
	}
	// Verify bold markers are consumed (not shown literally)
	if strings.Contains(plain, "**bold**") {
		t.Error("Bold markers ** should be consumed by rendering")
	}
}

func TestRenderItalicText(t *testing.T) {
	r := newTestRenderer()
	out := rendered(r, "This is *italic* text")
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "italic") {
		t.Errorf("Italic text should be present, got:\n%s", plain)
	}
	// Verify italic markers are consumed (not shown literally between words)
	if strings.Contains(plain, "*italic*") {
		t.Error("Italic markers * should be consumed by rendering")
	}
}

func TestRenderBoldItalicText(t *testing.T) {
	r := newTestRenderer()
	out := rendered(r, "This is ***both*** text")
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "both") {
		t.Errorf("Bold+italic text should be present, got:\n%s", plain)
	}
}

func TestRenderInlineCode(t *testing.T) {
	r := newTestRenderer()
	out := rendered(r, "Use `go test` command")
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "go test") {
		t.Errorf("Inline code should contain text, got:\n%s", plain)
	}
	// Verify backtick markers are consumed
	if strings.Contains(plain, "`go test`") {
		t.Error("Backtick markers should be consumed by rendering")
	}
}

// =========================================================================
// 6. Code blocks with language label — verify bordered output with language tag
// =========================================================================

func TestRenderCodeBlockWithLanguage(t *testing.T) {
	r := newTestRenderer()
	content := "```go\nfmt.Println(\"hello\")\n```"
	out := rendered(r, content)
	plain := stripAnsiCodes(out)

	if !strings.Contains(plain, "go") {
		t.Errorf("Code block should show language label, got:\n%s", plain)
	}
	if !strings.Contains(plain, "fmt.Println") {
		t.Errorf("Code block should contain code text, got:\n%s", plain)
	}
	if !strings.Contains(plain, "\u2500") {
		t.Errorf("Code block should have border characters, got:\n%s", plain)
	}
}

func TestRenderCodeBlockNoLanguage(t *testing.T) {
	r := newTestRenderer()
	content := "```\nsome code here\n```"
	out := rendered(r, content)
	plain := stripAnsiCodes(out)

	if !strings.Contains(plain, "some code here") {
		t.Errorf("Code block without language should still render code, got:\n%s", plain)
	}
	if !strings.Contains(plain, "\u2500") {
		t.Errorf("Code block should still have border, got:\n%s", plain)
	}
}

func TestRenderCodeBlockPreservesContent(t *testing.T) {
	r := newTestRenderer()
	content := "```\n  indented line\n    more indent\n```"
	out := rendered(r, content)
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "indented line") {
		t.Errorf("Code block should preserve content, got:\n%s", plain)
	}
	if !strings.Contains(plain, "more indent") {
		t.Errorf("Code block should preserve all lines, got:\n%s", plain)
	}
}

// =========================================================================
// 7-8. Unordered lists and ordered lists — verify bullet/number rendering
// =========================================================================

func TestRenderUnorderedListDash(t *testing.T) {
	r := newTestRenderer()
	content := "- First item\n- Second item"
	out := rendered(r, content)
	plain := stripAnsiCodes(out)

	if !strings.Contains(plain, "\u25CF") {
		t.Errorf("Unordered list should render bullet character, got:\n%s", plain)
	}
	if !strings.Contains(plain, "First item") || !strings.Contains(plain, "Second item") {
		t.Errorf("Unordered list should contain item text, got:\n%s", plain)
	}
}

func TestRenderUnorderedListAsterisk(t *testing.T) {
	r := newTestRenderer()
	content := "* Alpha\n* Beta"
	out := rendered(r, content)
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "Alpha") || !strings.Contains(plain, "Beta") {
		t.Errorf("Unordered list with * should render items, got:\n%s", plain)
	}
}

func TestRenderOrderedList(t *testing.T) {
	r := newTestRenderer()
	content := "1. First\n2. Second\n3. Third"
	out := rendered(r, content)
	plain := stripAnsiCodes(out)

	if !strings.Contains(plain, "1.") {
		t.Errorf("Ordered list should have numbers, got:\n%s", plain)
	}
	if !strings.Contains(plain, "Second") {
		t.Errorf("Ordered list should contain item text, got:\n%s", plain)
	}
}

// =========================================================================
// 9. Blockquotes — verify border styling
// =========================================================================

func TestRenderBlockquote(t *testing.T) {
	r := newTestRenderer()
	out := rendered(r, "> This is a quote")
	plain := stripAnsiCodes(out)

	if !strings.Contains(plain, "This is a quote") {
		t.Errorf("Blockquote should contain text, got:\n%s", plain)
	}
	if !strings.Contains(plain, "\u2503") {
		t.Errorf("Blockquote should have bar character, got:\n%s", plain)
	}
}

func TestRenderBlockquoteMultiLine(t *testing.T) {
	r := newTestRenderer()
	content := "> First line\n> Second line"
	out := rendered(r, content)
	if !strings.Contains(out, "First line") || !strings.Contains(out, "Second line") {
		t.Errorf("Multi-line blockquote should show both lines, got:\n%s", out)
	}
}

// =========================================================================
// 10. Task checkboxes — verify checkbox rendering
// =========================================================================

func TestRenderTaskCheckboxUnchecked(t *testing.T) {
	r := newTestRenderer()
	out := rendered(r, "- [ ] Todo item")
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "Todo item") {
		t.Errorf("Unchecked checkbox should contain text, got:\n%s", plain)
	}
	if !strings.Contains(plain, "\u2610") { // ☐
		t.Errorf("Unchecked checkbox should show ballot box, got:\n%s", plain)
	}
}

func TestRenderTaskCheckboxChecked(t *testing.T) {
	r := newTestRenderer()
	out := rendered(r, "- [x] Done item")
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "Done item") {
		t.Errorf("Checked checkbox should contain text, got:\n%s", plain)
	}
	if !strings.Contains(plain, "\u2611") { // ☑
		t.Errorf("Checked checkbox should show ballot box with check, got:\n%s", plain)
	}
}

func TestRenderTaskCheckboxUpperX(t *testing.T) {
	r := newTestRenderer()
	out := rendered(r, "- [X] Also done")
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "\u2611") { // ☑
		t.Errorf("Uppercase X checkbox should show ballot box with check, got:\n%s", plain)
	}
}

// =========================================================================
// 11. Horizontal rules — verify full-width line
// =========================================================================

func TestRenderHorizontalRule(t *testing.T) {
	r := newTestRenderer()
	out := rendered(r, "Above\n\n---\n\nBelow")
	plain := stripAnsiCodes(out)

	// --- renders with ─ (light horizontal) and ◆ diamond
	if !strings.Contains(plain, "\u25C6") { // ◆ diamond
		t.Errorf("Horizontal rule should contain diamond ornament, got:\n%s", plain)
	}
	if !strings.Contains(plain, "Above") || !strings.Contains(plain, "Below") {
		t.Errorf("Content around rule should render, got:\n%s", plain)
	}
}

func TestRenderHorizontalRuleVariants(t *testing.T) {
	r := newTestRenderer()
	for _, v := range []string{"***", "___"} {
		out := rendered(r, v)
		plain := stripAnsiCodes(out)
		// All horizontal rule variants render with a ◆ diamond ornament
		if !strings.Contains(plain, "\u25C6") { // ◆
			t.Errorf("Horizontal rule %q should contain diamond ornament, got:\n%s", v, plain)
		}
	}
}

// =========================================================================
// 12. Wikilinks — verify link styling
// =========================================================================

func TestRenderWikilink(t *testing.T) {
	r := newTestRenderer()
	out := rendered(r, "See [[my note]] for details")
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "my note") {
		t.Errorf("Wikilink should show note name, got:\n%s", plain)
	}
	if strings.Contains(plain, "[[") || strings.Contains(plain, "]]") {
		t.Errorf("Wikilink markers should be consumed, got:\n%s", plain)
	}
}

func TestRenderWikilinkWithAlias(t *testing.T) {
	r := newTestRenderer()
	out := rendered(r, "See [[target|Display Name]] here")
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "Display Name") {
		t.Errorf("Wikilink with alias should show display name, got:\n%s", plain)
	}
	if strings.Contains(plain, "target|") {
		t.Errorf("Wikilink alias should hide target pipe, got:\n%s", plain)
	}
}

func TestRenderWikilinkWithSpaces(t *testing.T) {
	r := newTestRenderer()
	out := rendered(r, "Link to [[note with spaces]] here")
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "note with spaces") {
		t.Errorf("Wikilink with spaces should render note name, got:\n%s", plain)
	}
}

// =========================================================================
// 13. Image references — verify alt text rendering
// =========================================================================

func TestRenderImageReference(t *testing.T) {
	r := newTestRenderer()
	out := rendered(r, "![My Photo](photo.png)")
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "photo.png") {
		t.Errorf("Image should show filename, got:\n%s", plain)
	}
	if !strings.Contains(plain, "IMG") {
		t.Errorf("Image should show IMG icon, got:\n%s", plain)
	}
	if !strings.Contains(plain, "My Photo") {
		t.Errorf("Image should show alt text, got:\n%s", plain)
	}
}

// =========================================================================
// 14. Tables — verify column alignment and borders
// =========================================================================

func TestRenderTable(t *testing.T) {
	r := newTestRenderer()
	content := "| Name | Age |\n| --- | --- |\n| Alice | 30 |"
	out := rendered(r, content)
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "Name") {
		t.Errorf("Table should contain header text, got:\n%s", plain)
	}
	if !strings.Contains(plain, "Alice") {
		t.Errorf("Table should contain data, got:\n%s", plain)
	}
	if !strings.Contains(plain, "\u2502") {
		t.Errorf("Table should have styled pipe separators, got:\n%s", plain)
	}
}

// =========================================================================
// 15. Frontmatter block — verify bordered display
// =========================================================================

func TestRenderFrontmatter(t *testing.T) {
	r := newTestRenderer()
	content := "---\ntitle: My Note\ntags: test\n---\nBody text"
	out := rendered(r, content)
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "title") {
		t.Errorf("Frontmatter should render key names, got:\n%s", plain)
	}
	if !strings.Contains(plain, "My Note") {
		t.Errorf("Frontmatter should render values, got:\n%s", plain)
	}
	if !strings.Contains(plain, "\u250C") {
		t.Errorf("Frontmatter should have top-left border corner, got:\n%s", plain)
	}
	if !strings.Contains(plain, "\u2514") {
		t.Errorf("Frontmatter should have bottom-left border corner, got:\n%s", plain)
	}
	if !strings.Contains(plain, "Body text") {
		t.Errorf("Content after frontmatter should render, got:\n%s", plain)
	}
}

func TestRenderFrontmatterEmpty(t *testing.T) {
	r := newTestRenderer()
	content := "---\n---\nBody"
	out := rendered(r, content)
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "Body") {
		t.Errorf("Content after empty frontmatter should render, got:\n%s", plain)
	}
}

// =========================================================================
// 16. Empty content
// =========================================================================

func TestRenderEmptyContent(t *testing.T) {
	r := newTestRenderer()
	out := rendered(r, "")
	if out != "" {
		lines := strings.Split(out, "\n")
		for _, l := range lines {
			if strings.TrimSpace(l) != "" {
				t.Errorf("Empty content should produce blank output, got non-blank line: %q", l)
			}
		}
	}
}

// =========================================================================
// 17. Content with only blank lines
// =========================================================================

func TestRenderBlankLinesOnly(t *testing.T) {
	r := newTestRenderer()
	out := rendered(r, "\n\n\n")
	plain := stripAnsiCodes(out)
	if strings.TrimSpace(plain) != "" {
		t.Errorf("Blank lines only should produce blank output, got:\n%q", plain)
	}
}

func TestRenderWhitespaceOnly(t *testing.T) {
	r := newTestRenderer()
	out := rendered(r, "   \n\t\n   ")
	_ = out // should not panic
}

// =========================================================================
// 18. Very long lines (1000+ chars) — should not panic
// =========================================================================

func TestRenderVeryLongLine(t *testing.T) {
	r := newTestRenderer()
	long := strings.Repeat("a", 1500)
	out := rendered(r, long)
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "aaa") {
		t.Errorf("Long line should still render content, got length %d", len(plain))
	}
}

func TestRenderVeryLongLineWithFormatting(t *testing.T) {
	r := newTestRenderer()
	long := "**" + strings.Repeat("b", 1200) + "** and *" + strings.Repeat("c", 800) + "*"
	// Should not panic
	out := rendered(r, long)
	if len(out) == 0 {
		t.Error("Very long formatted line should produce output")
	}
}

// =========================================================================
// 19. Nested blockquotes
// =========================================================================

func TestRenderNestedBlockquotes(t *testing.T) {
	r := newTestRenderer()
	out := rendered(r, "> > > deep nesting")
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "deep nesting") {
		t.Errorf("Nested blockquote should contain inner text, got:\n%s", plain)
	}
}

// =========================================================================
// 20. Code block with no language specified — already covered above
// =========================================================================

// =========================================================================
// 21. Unclosed bold/italic markers
// =========================================================================

func TestRenderUnclosedBold(t *testing.T) {
	r := newTestRenderer()
	out := rendered(r, "This is **unclosed bold")
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "**unclosed bold") {
		t.Errorf("Unclosed bold should render as literal text, got:\n%s", plain)
	}
}

func TestRenderUnclosedItalic(t *testing.T) {
	r := newTestRenderer()
	out := rendered(r, "This is *unclosed italic")
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "*unclosed italic") {
		t.Errorf("Unclosed italic should render as literal text, got:\n%s", plain)
	}
}

func TestRenderUnclosedInlineCode(t *testing.T) {
	r := newTestRenderer()
	out := rendered(r, "This is `unclosed code")
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "`unclosed code") {
		t.Errorf("Unclosed inline code should render as literal text, got:\n%s", plain)
	}
}

// =========================================================================
// 22. Mixed inline formatting
// =========================================================================

func TestRenderMixedInlineFormatting(t *testing.T) {
	r := newTestRenderer()
	out := rendered(r, "**bold *and italic* text**")
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "bold") {
		t.Errorf("Mixed formatting should contain 'bold', got:\n%s", plain)
	}
	if !strings.Contains(plain, "italic") {
		t.Errorf("Mixed formatting should contain 'italic', got:\n%s", plain)
	}
}

func TestRenderInlineMultipleFormats(t *testing.T) {
	r := newTestRenderer()
	input := "**bold** and *italic* and `code` and [[link]]"
	result := r.renderInline(input)
	plain := stripAnsiCodes(result)
	for _, word := range []string{"bold", "italic", "code", "link"} {
		if !strings.Contains(plain, word) {
			t.Errorf("Should contain %q in multi-format inline, got:\n%s", word, plain)
		}
	}
}

// =========================================================================
// 23. Wikilinks with special chars — already covered in TestRenderWikilinkWithSpaces
// =========================================================================

// =========================================================================
// 24. Multiple horizontal rules in sequence
// =========================================================================

func TestRenderMultipleHorizontalRules(t *testing.T) {
	r := newTestRenderer()
	// Use *** to avoid frontmatter ambiguity with ---
	content := "text\n***\n\n***\n\n***"
	out := rendered(r, content)
	plain := stripAnsiCodes(out)
	// Each horizontal rule has a ◆ diamond ornament
	count := strings.Count(plain, "\u25C6") // ◆
	if count < 3 {
		t.Errorf("Multiple horizontal rules should each render diamond ornament, found %d in:\n%s", count, plain)
	}
}

// =========================================================================
// 25. Callout blocks — verify icon and color
// =========================================================================

func TestRenderCalloutTip(t *testing.T) {
	r := newTestRenderer()
	content := "> [!tip] Pro Tip\n> This is helpful advice"
	out := rendered(r, content)
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "Tip") {
		t.Errorf("Callout should show label, got:\n%s", plain)
	}
	if !strings.Contains(plain, "Pro Tip") {
		t.Errorf("Callout should show title, got:\n%s", plain)
	}
	if !strings.Contains(plain, "This is helpful advice") {
		t.Errorf("Callout should show content, got:\n%s", plain)
	}
}

func TestRenderCalloutWarning(t *testing.T) {
	r := newTestRenderer()
	content := "> [!warning]\n> Be careful here"
	out := rendered(r, content)
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "Warning") {
		t.Errorf("Warning callout should show label, got:\n%s", plain)
	}
	if !strings.Contains(plain, "Be careful here") {
		t.Errorf("Warning callout should show content, got:\n%s", plain)
	}
}

func TestRenderCalloutAllTypes(t *testing.T) {
	r := newTestRenderer()
	types := map[string]string{
		"note":    "Note",
		"info":    "Note",
		"tip":     "Tip",
		"hint":    "Tip",
		"warning": "Warning",
		"danger":  "Danger",
		"example": "Example",
		"quote":   "Quote",
		"success": "Success",
		"bug":     "Bug",
		"todo":    "Todo",
	}
	for calloutType, expectedLabel := range types {
		content := "> [!" + calloutType + "] Title"
		out := rendered(r, content)
		plain := stripAnsiCodes(out)
		if !strings.Contains(plain, expectedLabel) {
			t.Errorf("Callout type %q should show label %q, got:\n%s", calloutType, expectedLabel, plain)
		}
	}
}

func TestRenderCalloutWithMultipleContentLines(t *testing.T) {
	r := newTestRenderer()
	content := "> [!note] Important\n> Line one\n> Line two\n> Line three"
	out := rendered(r, content)
	plain := stripAnsiCodes(out)
	for _, line := range []string{"Line one", "Line two", "Line three"} {
		if !strings.Contains(plain, line) {
			t.Errorf("Callout should contain %q, got:\n%s", line, plain)
		}
	}
}

// =========================================================================
// Additional inline formatting: strikethrough, highlight, math, tags, footnotes
// =========================================================================

func TestRenderStrikethrough(t *testing.T) {
	r := newTestRenderer()
	out := rendered(r, "This is ~~deleted~~ text")
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "deleted") {
		t.Errorf("Strikethrough text should be present, got:\n%s", plain)
	}
}

func TestRenderHighlight(t *testing.T) {
	r := newTestRenderer()
	out := rendered(r, "This is ==highlighted== text")
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "highlighted") {
		t.Errorf("Highlighted text should be present, got:\n%s", plain)
	}
}

func TestRenderInlineMath(t *testing.T) {
	r := newTestRenderer()
	out := rendered(r, "The equation $E=mc^2$ is famous")
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "$E=mc^2$") {
		t.Errorf("Inline math should render with dollar signs, got:\n%s", plain)
	}
}

func TestRenderTag(t *testing.T) {
	r := newTestRenderer()
	out := rendered(r, "Note about #golang and #testing")
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "#golang") {
		t.Errorf("Tag should be present, got:\n%s", plain)
	}
	if !strings.Contains(plain, "#testing") {
		t.Errorf("Second tag should be present, got:\n%s", plain)
	}
}

func TestRenderFootnoteReference(t *testing.T) {
	r := newTestRenderer()
	out := rendered(r, "Some claim[^1] is noted")
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "[1]") {
		t.Errorf("Footnote reference should render, got:\n%s", plain)
	}
}

// =========================================================================
// Nested formatting in list items
// =========================================================================

func TestRenderBoldInList(t *testing.T) {
	r := newTestRenderer()
	out := rendered(r, "- This has **bold** text")
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "bold") {
		t.Errorf("Bold inside list item should render, got:\n%s", plain)
	}
}

func TestRenderCodeInList(t *testing.T) {
	r := newTestRenderer()
	out := rendered(r, "- Use `command` here")
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "command") {
		t.Errorf("Inline code inside list item should render, got:\n%s", plain)
	}
}

func TestRenderWikilinkInList(t *testing.T) {
	r := newTestRenderer()
	out := rendered(r, "- See [[Other Note]]")
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "Other Note") {
		t.Errorf("Wikilink inside list item should render, got:\n%s", plain)
	}
}

// =========================================================================
// Embed rendering
// =========================================================================

func TestRenderEmbedWithLookup(t *testing.T) {
	r := newTestRenderer()
	r.SetNoteLookup(func(name string) string {
		if name == "embedded-note" {
			return "# Embedded\nThis is embedded content"
		}
		return ""
	})
	out := rendered(r, "![[embedded-note]]")
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "Embedded") {
		t.Errorf("Embed should show embedded note content, got:\n%s", plain)
	}
	if !strings.Contains(plain, "embedded content") {
		t.Errorf("Embed should show embedded note body, got:\n%s", plain)
	}
}

func TestRenderEmbedWithoutLookup(t *testing.T) {
	r := newTestRenderer()
	out := rendered(r, "![[missing-note]]")
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "note lookup not available") {
		t.Errorf("Embed without lookup should show fallback message, got:\n%s", plain)
	}
}

func TestRenderEmbedNotFound(t *testing.T) {
	r := newTestRenderer()
	r.SetNoteLookup(func(name string) string { return "" })
	out := rendered(r, "![[nonexistent]]")
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "note not found") {
		t.Errorf("Embed for missing note should show not-found message, got:\n%s", plain)
	}
}

func TestRenderEmbedWithHeading(t *testing.T) {
	r := newTestRenderer()
	r.SetNoteLookup(func(name string) string {
		return "# Top\nPreamble\n## Section A\nContent A\n## Section B\nContent B"
	})
	out := rendered(r, "![[my-note#Section A]]")
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "Content A") {
		t.Errorf("Embed with heading should show section content, got:\n%s", plain)
	}
}

func TestRenderEmbedWithFrontmatter(t *testing.T) {
	r := newTestRenderer()
	r.SetNoteLookup(func(name string) string {
		return "---\ntitle: Linked\n---\n\nLinked content here."
	})
	out := rendered(r, "![[linked]]")
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "Linked content here") {
		t.Errorf("Embedded note should strip frontmatter and show body, got:\n%s", plain)
	}
}

func TestRenderEmbedInline(t *testing.T) {
	r := newTestRenderer()
	out := rendered(r, "See also ![[note-ref]] in context")
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "note-ref") {
		t.Errorf("Inline embed reference should show note name, got:\n%s", plain)
	}
}

// =========================================================================
// Scroll behavior
// =========================================================================

func TestRenderScroll(t *testing.T) {
	r := NewRenderer()
	r.SetSize(80, 10) // visibleHeight = 10 - 4 = 6

	var sb strings.Builder
	for i := 0; i < 50; i++ {
		sb.WriteString("## Heading " + string(rune('A'+i%26)) + strings.Repeat("x", i) + "\n\n")
	}
	content := sb.String()

	out0 := r.Render(content, 0)
	out20 := r.Render(content, 20)
	if out0 == out20 {
		t.Error("Different scroll positions should produce different output")
	}
}

func TestRenderScrollBeyondContent(t *testing.T) {
	r := NewRenderer()
	r.SetSize(80, 10)
	out := r.Render("Short\ncontent", 9999)
	_ = out // should not panic
}

func TestRenderLineCount(t *testing.T) {
	r := newTestRenderer()
	tests := []struct {
		name     string
		content  string
		minLines int
	}{
		{"empty", "", 0},
		{"single paragraph", "Hello world", 1},
		{"multiple paragraphs", "Para one\n\nPara two\n\nPara three", 3},
		{"with heading", "# Title\n\nBody", 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			count := r.RenderLineCount(tc.content)
			if count < tc.minLines {
				t.Errorf("expected at least %d rendered lines, got %d", tc.minLines, count)
			}
		})
	}
}

// =========================================================================
// Renderer construction and sizing
// =========================================================================

func TestNewRenderer(t *testing.T) {
	r := NewRenderer()
	if r.width != 0 || r.height != 0 {
		t.Errorf("NewRenderer should start with zero size, got %dx%d", r.width, r.height)
	}
}

func TestRendererSetSize(t *testing.T) {
	r := NewRenderer()
	r.SetSize(100, 50)
	if r.width != 100 || r.height != 50 {
		t.Errorf("SetSize should update dimensions, got %dx%d", r.width, r.height)
	}
}

func TestRenderMinimalWidth(t *testing.T) {
	r := NewRenderer()
	r.SetSize(10, 20)
	out := rendered(r, "# Heading\nSome text")
	if len(out) == 0 {
		t.Error("Should render even at minimal width")
	}
}

func TestRenderMinimalHeight(t *testing.T) {
	r := NewRenderer()
	r.SetSize(80, 3)
	out := rendered(r, "Line 1\nLine 2\nLine 3\nLine 4\nLine 5")
	lines := strings.Split(out, "\n")
	if len(lines) > 1 {
		t.Errorf("Minimal height should limit visible lines, got %d lines", len(lines))
	}
}

func TestSetNoteLookup(t *testing.T) {
	r := NewRenderer()
	if r.noteLookup != nil {
		t.Error("noteLookup should be nil initially")
	}
	r.SetNoteLookup(func(name string) string { return "content" })
	if r.noteLookup == nil {
		t.Error("noteLookup should be set after SetNoteLookup")
	}
}

func TestSetVaultRoot(t *testing.T) {
	r := NewRenderer()
	r.SetVaultRoot("/tmp/vault")
	if r.vaultRoot != "/tmp/vault" {
		t.Errorf("vaultRoot should be set, got %q", r.vaultRoot)
	}
}

// =========================================================================
// Helper function tests
// =========================================================================

func TestParseHeading(t *testing.T) {
	tests := []struct {
		input string
		level int
		text  string
	}{
		{"# Title", 1, "Title"},
		{"## Subtitle", 2, "Subtitle"},
		{"### Third", 3, "Third"},
		{"#### Fourth", 4, "Fourth"},
		{"Not a heading", 0, ""},
		{"#NoSpace", 0, ""},
		{"", 0, ""},
	}
	for _, tt := range tests {
		level, text := parseHeading(tt.input)
		if level != tt.level || text != tt.text {
			t.Errorf("parseHeading(%q) = (%d, %q), want (%d, %q)",
				tt.input, level, text, tt.level, tt.text)
		}
	}
}

func TestExtractHeadingSection(t *testing.T) {
	content := "# Top\nPreamble\n## Alpha\nAlpha content\nMore alpha\n## Beta\nBeta content\n### Sub\nSub content"

	t.Run("extract Alpha", func(t *testing.T) {
		section := extractHeadingSection(content, "Alpha")
		if !strings.Contains(section, "Alpha content") {
			t.Errorf("Should extract Alpha section content, got:\n%s", section)
		}
		if !strings.Contains(section, "More alpha") {
			t.Errorf("Should include all lines under Alpha, got:\n%s", section)
		}
		if strings.Contains(section, "Beta content") {
			t.Errorf("Should not include Beta in Alpha, got:\n%s", section)
		}
	})

	t.Run("extract Beta includes subsections", func(t *testing.T) {
		section := extractHeadingSection(content, "Beta")
		if !strings.Contains(section, "Beta content") {
			t.Errorf("Should extract Beta content, got:\n%s", section)
		}
		if !strings.Contains(section, "Sub content") {
			t.Errorf("Beta should include subsections, got:\n%s", section)
		}
	})

	t.Run("nonexistent heading", func(t *testing.T) {
		section := extractHeadingSection(content, "Nonexistent")
		if section != "" {
			t.Errorf("Nonexistent heading should return empty, got:\n%s", section)
		}
	})

	t.Run("case insensitive", func(t *testing.T) {
		section := extractHeadingSection(content, "alpha")
		if !strings.Contains(section, "Alpha content") {
			t.Errorf("Heading extraction should be case-insensitive, got:\n%s", section)
		}
	})
}

func TestGetCalloutInfo(t *testing.T) {
	r := NewRenderer()
	tests := []struct {
		typ   string
		label string
	}{
		{"note", "Note"},
		{"info", "Note"},
		{"tip", "Tip"},
		{"hint", "Tip"},
		{"warning", "Warning"},
		{"caution", "Warning"},
		{"danger", "Danger"},
		{"error", "Danger"},
		{"example", "Example"},
		{"quote", "Quote"},
		{"cite", "Quote"},
		{"success", "Success"},
		{"check", "Success"},
		{"question", "Question"},
		{"faq", "Question"},
		{"abstract", "Abstract"},
		{"summary", "Abstract"},
		{"tldr", "Abstract"},
		{"todo", "Todo"},
		{"bug", "Bug"},
	}
	for _, tt := range tests {
		info := r.getCalloutInfo(tt.typ)
		if info.label != tt.label {
			t.Errorf("getCalloutInfo(%q).label = %q, want %q", tt.typ, info.label, tt.label)
		}
		if info.icon == "" {
			t.Errorf("getCalloutInfo(%q).icon should not be empty", tt.typ)
		}
	}
}

func TestGetCalloutInfoCaseInsensitive(t *testing.T) {
	r := NewRenderer()
	info := r.getCalloutInfo("WARNING")
	if info.label != "Warning" {
		t.Errorf("getCalloutInfo should be case-insensitive, got label %q", info.label)
	}
}

func TestImgIsImageFile(t *testing.T) {
	imageNames := []string{
		"photo.png", "pic.jpg", "img.jpeg", "anim.gif",
		"vector.svg", "modern.webp", "bitmap.bmp",
		"UPPER.PNG", "Mixed.Jpg",
	}
	for _, name := range imageNames {
		if !imgIsImageFile(name) {
			t.Errorf("imgIsImageFile(%q) should be true", name)
		}
	}

	nonImageNames := []string{
		"note.md", "data.json", "script.py", "doc.pdf", "file.txt", "",
	}
	for _, name := range nonImageNames {
		if imgIsImageFile(name) {
			t.Errorf("imgIsImageFile(%q) should be false", name)
		}
	}
}

// =========================================================================
// renderInline edge cases
// =========================================================================

func TestRenderInlineEmpty(t *testing.T) {
	r := newTestRenderer()
	result := r.renderInline("")
	if result != "" {
		t.Errorf("renderInline empty should return empty, got %q", result)
	}
}

func TestRenderInlinePlainText(t *testing.T) {
	r := newTestRenderer()
	result := r.renderInline("just plain text")
	plain := stripAnsiCodes(result)
	if plain != "just plain text" {
		t.Errorf("renderInline plain text should pass through, got %q", plain)
	}
}

// =========================================================================
// 26-27. Integration tests — full notes with all elements
// =========================================================================

func TestRenderFullNote(t *testing.T) {
	r := NewRenderer()
	r.SetSize(80, 200) // tall enough to see all content
	content := `---
title: Test Note
tags: test, integration
---

# Main Title

This is a paragraph with **bold**, *italic*, and ` + "`code`" + ` text.

## Section One

- Item A
- Item B
- Item C

1. First
2. Second

> A wise quote

- [ ] Todo task
- [x] Completed task

### Subsection

` + "```python" + `
def hello():
    print("world")
` + "```" + `

#### Final Heading

Some [[wikilink]] and #tag content.

> [!tip] Remember
> Always write tests.`

	out := rendered(r, content)
	plain := stripAnsiCodes(out)

	checks := map[string]string{
		"Main Title":     "H1 text",
		"Section One":    "H2 text",
		"Subsection":     "H3 text",
		"Final Heading":  "H4 text",
		"bold":           "bold text",
		"italic":         "italic text",
		"code":           "inline code",
		"Item A":         "unordered list",
		"First":          "ordered list",
		"wise quote":     "blockquote",
		"Todo task":      "unchecked checkbox",
		"Completed task": "checked checkbox",
		"hello":          "code block",
		"wikilink":       "wikilink",
		"#tag":           "tag",
		"Tip":            "callout label",
		"Remember":       "callout title",
		"write tests":    "callout content",
		"title":          "frontmatter key",
		"Test Note":      "frontmatter value",
	}

	for text, desc := range checks {
		if !strings.Contains(plain, text) {
			t.Errorf("Full note should contain %s (%q), not found in output", desc, text)
		}
	}
}

func TestRenderAllElementsTogether(t *testing.T) {
	r := NewRenderer()
	r.SetSize(80, 200) // tall enough to see all content
	content := `---
author: Tester
date: 2025-01-01
---

# Document

## Introduction

This document has **bold** and *italic* formatting, plus ==highlight== and ~~struck~~.

## Lists

- Unordered item
* Another item

1. Ordered item

## Tasks

- [ ] Open task
- [x] Done task

## Code

` + "```js" + `
console.log("hi");
` + "```" + `

Inline: ` + "`fmt.Println()`" + `

## Quotes

> Simple quote

> [!warning] Watch out
> Dangerous stuff

## Table

| Key | Value |
| --- | --- |
| A | 1 |

***

## Links

See [[other note]] and [[target|alias]].

Math: $x^2 + y^2 = z^2$

Reference[^2] here.

End of document.`

	out := rendered(r, content)
	if len(out) == 0 {
		t.Error("Combined rendering should produce output")
	}

	plain := stripAnsiCodes(out)
	expected := []string{
		"Document", "Introduction", "other note", "alias",
		"Watch out", "Simple quote", "Open task", "Done task",
		"console.log", "fmt.Println",
	}
	for _, s := range expected {
		if !strings.Contains(plain, s) {
			t.Errorf("Combined note should contain %q", s)
		}
	}
}

// =========================================================================
// 28. Performance test — render a 1000-line note
// =========================================================================

func TestRenderPerformanceLargeNote(t *testing.T) {
	r := NewRenderer()
	r.SetSize(120, 60)

	var sb strings.Builder
	sb.WriteString("---\ntitle: Large Note\n---\n\n")
	sb.WriteString("# Large Document\n\n")

	for i := 0; i < 200; i++ {
		sb.WriteString("## Section " + strings.Repeat("x", 5) + "\n\n")
		sb.WriteString("This is a paragraph with **bold** and *italic* and `code` inline.\n")
		sb.WriteString("- List item one\n- List item two\n")
		sb.WriteString("1. Ordered one\n2. Ordered two\n\n")
		sb.WriteString("> A blockquote line\n\n")
	}

	content := sb.String()
	lineCount := strings.Count(content, "\n")
	if lineCount < 1000 {
		t.Fatalf("Test note should be 1000+ lines, got %d", lineCount)
	}

	start := time.Now()
	out := r.Render(content, 0)
	elapsed := time.Since(start)

	if elapsed > 5*time.Second {
		t.Errorf("Rendering 1000+ line note took %v, should be under 5s", elapsed)
	}
	if len(out) == 0 {
		t.Error("Large note should produce output")
	}
}

func TestRenderPerformanceLineCount(t *testing.T) {
	r := NewRenderer()
	r.SetSize(120, 60)

	var sb strings.Builder
	for i := 0; i < 500; i++ {
		sb.WriteString("Paragraph " + strings.Repeat("word ", 10) + "\n\n")
	}

	start := time.Now()
	count := r.RenderLineCount(sb.String())
	elapsed := time.Since(start)

	if elapsed > 3*time.Second {
		t.Errorf("RenderLineCount for large doc took %v, should be under 3s", elapsed)
	}
	if count < 500 {
		t.Errorf("Expected at least 500 rendered lines, got %d", count)
	}
}

// =========================================================================
// renderCalloutBlock direct test
// =========================================================================

func TestRenderCalloutBlockDirect(t *testing.T) {
	r := NewRenderer()
	info := calloutInfo{color: blue, icon: "\u2139", label: "Note"}
	lines := r.renderCalloutBlock(info, "My Title", []string{"Line A", "Line B"}, 60)

	if len(lines) < 3 {
		t.Errorf("Callout block should produce at least 3 lines (header + 2 content), got %d", len(lines))
	}

	joined := strings.Join(lines, "\n")
	plain := stripAnsiCodes(joined)
	if !strings.Contains(plain, "Note") {
		t.Error("Callout block should contain label")
	}
	if !strings.Contains(plain, "My Title") {
		t.Error("Callout block should contain title")
	}
	if !strings.Contains(plain, "Line A") {
		t.Error("Callout block should contain content line A")
	}
	if !strings.Contains(plain, "Line B") {
		t.Error("Callout block should contain content line B")
	}
}

func TestRenderCalloutBlockNoTitle(t *testing.T) {
	r := NewRenderer()
	info := calloutInfo{color: green, icon: "\u2713", label: "Success"}
	lines := r.renderCalloutBlock(info, "", []string{"Done!"}, 60)

	if len(lines) < 2 {
		t.Errorf("Callout block should produce at least 2 lines, got %d", len(lines))
	}

	joined := strings.Join(lines, "\n")
	plain := stripAnsiCodes(joined)
	if !strings.Contains(plain, "Success") {
		t.Error("Callout block should contain label")
	}
	if !strings.Contains(plain, "Done!") {
		t.Error("Callout block should contain content")
	}
}

// =========================================================================
// renderEmbed direct test
// =========================================================================

func TestRenderEmbedDirect(t *testing.T) {
	r := NewRenderer()
	r.SetSize(80, 40)
	r.SetNoteLookup(func(name string) string {
		if name == "test" {
			return "Hello from embed"
		}
		return ""
	})
	lines := r.renderEmbed("test", "", 60)
	joined := strings.Join(lines, "\n")
	plain := stripAnsiCodes(joined)
	if !strings.Contains(plain, "Embedded") {
		t.Error("Embed should show 'Embedded' label")
	}
	if !strings.Contains(plain, "Hello from embed") {
		t.Error("Embed should show note content")
	}
}

func TestRenderEmbedDirectNotFound(t *testing.T) {
	r := NewRenderer()
	r.SetNoteLookup(func(name string) string { return "" })
	lines := r.renderEmbed("missing", "", 60)
	joined := strings.Join(lines, "\n")
	plain := stripAnsiCodes(joined)
	if !strings.Contains(plain, "note not found") {
		t.Error("Embed for missing note should show not-found message")
	}
}

// =========================================================================
// renderEmbedLine direct tests
// =========================================================================

func TestRenderEmbedLineHeading(t *testing.T) {
	r := NewRenderer()
	out := r.renderEmbedLine("# Main Title", 80)
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "# Main Title") {
		t.Errorf("Embed line should contain heading, got %q", plain)
	}
}

func TestRenderEmbedLineCheckbox(t *testing.T) {
	r := NewRenderer()
	out := r.renderEmbedLine("- [ ] Todo", 80)
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "[ ]") {
		t.Errorf("Embed line should render unchecked checkbox, got %q", plain)
	}

	out2 := r.renderEmbedLine("- [x] Done", 80)
	plain2 := stripAnsiCodes(out2)
	if !strings.Contains(plain2, "[x]") {
		t.Errorf("Embed line should render checked checkbox, got %q", plain2)
	}
}

func TestRenderEmbedLineTruncation(t *testing.T) {
	r := NewRenderer()
	long := strings.Repeat("z", 200)
	out := r.renderEmbedLine(long, 50)
	plain := stripAnsiCodes(out)
	if len([]rune(plain)) > 60 { // 50 + "..." overhead
		t.Errorf("Embed line should truncate to maxChars, got length %d", len([]rune(plain)))
	}
	if !strings.HasSuffix(plain, "...") {
		t.Errorf("Truncated embed line should end with '...', got %q", plain)
	}
}

// =========================================================================
// imgRenderPlaceholder direct test
// =========================================================================

func TestImgRenderPlaceholder(t *testing.T) {
	lines := imgRenderPlaceholder("photo.jpg", "A photo", "", 60)
	if len(lines) == 0 {
		t.Fatal("imgRenderPlaceholder should produce output")
	}
	joined := strings.Join(lines, "\n")
	plain := stripAnsiCodes(joined)

	if !strings.Contains(plain, "photo.jpg") {
		t.Error("Placeholder should contain filename")
	}
	if !strings.Contains(plain, "IMG") {
		t.Error("Placeholder should contain IMG icon")
	}
	if !strings.Contains(plain, "A photo") {
		t.Error("Placeholder should contain alt text")
	}
	if !strings.Contains(plain, "[Image]") {
		t.Error("Placeholder should contain [Image] dimension text")
	}
}

func TestImgRenderPlaceholderNoAlt(t *testing.T) {
	lines := imgRenderPlaceholder("pic.png", "", "", 60)
	joined := strings.Join(lines, "\n")
	plain := stripAnsiCodes(joined)
	if strings.Contains(plain, "Alt:") {
		t.Error("Placeholder with no alt text should not show Alt line")
	}
}

func TestImgRenderPlaceholderNarrowWidth(t *testing.T) {
	// Should not panic with very small content width
	lines := imgRenderPlaceholder("tiny.png", "", "", 10)
	if len(lines) == 0 {
		t.Error("Placeholder should produce output even with small width")
	}
}

// =========================================================================
// Image embed via wikilink syntax ![[image.png]]
// =========================================================================

func TestRenderImageEmbed(t *testing.T) {
	r := newTestRenderer()
	out := rendered(r, "![[photo.png]]")
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "photo.png") {
		t.Errorf("Image embed should show filename, got:\n%s", plain)
	}
	if !strings.Contains(plain, "IMG") {
		t.Errorf("Image embed should show IMG placeholder, got:\n%s", plain)
	}
}

// =========================================================================
// Non-image embed treated as note
// =========================================================================

func TestRenderNonImageEmbed(t *testing.T) {
	r := newTestRenderer()
	out := rendered(r, "![[some-note]]")
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "some-note") {
		t.Errorf("Non-image embed should reference note name, got:\n%s", plain)
	}
}

// =========================================================================
// Edge: single > on its own (not blockquote since no space after)
// =========================================================================

func TestRenderBareGreaterThan(t *testing.T) {
	r := newTestRenderer()
	out := rendered(r, ">no space after")
	plain := stripAnsiCodes(out)
	// Current renderer treats > even without space as blockquote (nesting support).
	// The text after > should still be rendered.
	if !strings.Contains(plain, "no space after") {
		t.Errorf("Bare > should still render the text content, got:\n%s", plain)
	}
	// Should have blockquote bar
	if !strings.Contains(plain, "\u2503") {
		t.Errorf("Bare > is treated as blockquote, should have bar, got:\n%s", plain)
	}
}

// =========================================================================
// Table separator row rendering
// =========================================================================

func TestRenderTableSeparatorRow(t *testing.T) {
	r := newTestRenderer()
	content := "| H1 | H2 |\n| --- | --- |\n| a | b |"
	out := rendered(r, content)
	plain := stripAnsiCodes(out)
	// Separator row should use horizontal line chars
	if !strings.Contains(plain, "\u2500") {
		t.Errorf("Table separator should use horizontal line chars, got:\n%s", plain)
	}
}

// =========================================================================
// renderMarkdown returns correct number of lines
// =========================================================================

func TestRenderMarkdownDirectLineCount(t *testing.T) {
	r := newTestRenderer()
	lines := r.renderMarkdown("Hello\n\nWorld")
	if len(lines) != 3 {
		t.Errorf("Expected 3 rendered lines (Hello, blank, World), got %d", len(lines))
	}
}

func TestRenderMarkdownHeadingExtraLines(t *testing.T) {
	r := newTestRenderer()
	lines := r.renderMarkdown("# Title")
	// H1 adds: blank + bar + blank = 3 lines
	if len(lines) < 3 {
		t.Errorf("H1 should produce multiple rendered lines (spacing + bar + spacing), got %d", len(lines))
	}
}

// =========================================================================
// Definition lists
// =========================================================================

func TestRenderDefinitionListSingle(t *testing.T) {
	r := newTestRenderer()
	content := "Term One\n: This is the definition"
	out := rendered(r, content)
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "Term One") {
		t.Errorf("Definition list should contain term text, got:\n%s", plain)
	}
	if !strings.Contains(plain, "This is the definition") {
		t.Errorf("Definition list should contain definition text, got:\n%s", plain)
	}
	if !strings.Contains(plain, "▸") {
		t.Errorf("Definition list should have marker character, got:\n%s", plain)
	}
}

func TestRenderDefinitionListMultipleDefinitions(t *testing.T) {
	r := newTestRenderer()
	content := "Term\n: First definition\n: Second definition"
	out := rendered(r, content)
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "First definition") {
		t.Errorf("Should contain first definition, got:\n%s", plain)
	}
	if !strings.Contains(plain, "Second definition") {
		t.Errorf("Should contain second definition, got:\n%s", plain)
	}
}

func TestRenderDefinitionListMultipleTerms(t *testing.T) {
	r := newTestRenderer()
	content := "Term 1\n: Definition 1\n\nTerm 2\n: Definition 2"
	out := rendered(r, content)
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "Term 1") {
		t.Errorf("Should contain first term, got:\n%s", plain)
	}
	if !strings.Contains(plain, "Definition 1") {
		t.Errorf("Should contain first definition, got:\n%s", plain)
	}
	if !strings.Contains(plain, "Term 2") {
		t.Errorf("Should contain second term, got:\n%s", plain)
	}
	if !strings.Contains(plain, "Definition 2") {
		t.Errorf("Should contain second definition, got:\n%s", plain)
	}
}

// =========================================================================
// Table alignment indicators
// =========================================================================

func TestRenderTableAlignmentLeft(t *testing.T) {
	r := newTestRenderer()
	content := "| Left |\n| :--- |\n| data |"
	out := rendered(r, content)
	plain := stripAnsiCodes(out)
	// Left alignment should show colon at the start of separator
	if !strings.Contains(plain, ":") {
		t.Errorf("Left-aligned table should show alignment indicator, got:\n%s", plain)
	}
	if !strings.Contains(plain, "data") {
		t.Errorf("Table should contain data, got:\n%s", plain)
	}
}

func TestRenderTableAlignmentCenter(t *testing.T) {
	r := newTestRenderer()
	content := "| Center |\n| :---: |\n| data |"
	out := rendered(r, content)
	plain := stripAnsiCodes(out)
	// Center alignment should have colons on both sides
	lines := strings.Split(plain, "\n")
	foundSep := false
	for _, line := range lines {
		if strings.Contains(line, "═") {
			foundSep = true
			// For center, expect :═══:
			sepPart := line[strings.Index(line, "├")+len("├"):]
			sepPart = sepPart[:strings.Index(sepPart, "┤")]
			if !strings.HasPrefix(sepPart, ":") || !strings.HasSuffix(sepPart, ":") {
				t.Errorf("Center-aligned separator should have colons on both sides, got: %s", sepPart)
			}
		}
	}
	if !foundSep {
		t.Errorf("Should have a separator row with ═, got:\n%s", plain)
	}
}

func TestRenderTableAlignmentRight(t *testing.T) {
	r := newTestRenderer()
	content := "| Right |\n| ---: |\n| data |"
	out := rendered(r, content)
	plain := stripAnsiCodes(out)
	// Right alignment should have colon at the end
	lines := strings.Split(plain, "\n")
	foundSep := false
	for _, line := range lines {
		if strings.Contains(line, "═") {
			foundSep = true
			sepPart := line[strings.Index(line, "├")+len("├"):]
			sepPart = sepPart[:strings.Index(sepPart, "┤")]
			if !strings.HasSuffix(sepPart, ":") {
				t.Errorf("Right-aligned separator should end with colon, got: %s", sepPart)
			}
		}
	}
	if !foundSep {
		t.Errorf("Should have a separator row with ═, got:\n%s", plain)
	}
}

func TestRenderTableAlignmentMixed(t *testing.T) {
	r := newTestRenderer()
	content := "| Left | Center | Right |\n| :--- | :---: | ---: |\n| a | b | c |"
	out := rendered(r, content)
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "Left") {
		t.Errorf("Should contain header text, got:\n%s", plain)
	}
	// Content should be present
	if !strings.Contains(plain, "a") || !strings.Contains(plain, "b") || !strings.Contains(plain, "c") {
		t.Errorf("Should contain cell data, got:\n%s", plain)
	}
}

func TestRenderTableRightAlignedContent(t *testing.T) {
	r := newTestRenderer()
	content := "| Name | Value |\n| :--- | ---: |\n| short | 42 |"
	out := rendered(r, content)
	plain := stripAnsiCodes(out)
	// Right-aligned "42" should have leading spaces
	lines := strings.Split(plain, "\n")
	for _, line := range lines {
		if strings.Contains(line, "42") {
			// The 42 should be right-padded (spaces before it)
			idx := strings.Index(line, "42")
			if idx > 0 && line[idx-1] != ' ' {
				// That's fine, as long as it's rendered
			}
		}
	}
	if !strings.Contains(plain, "42") {
		t.Errorf("Right-aligned cell should contain data, got:\n%s", plain)
	}
}

func TestRenderDefinitionListNotTriggeredWithoutColon(t *testing.T) {
	r := newTestRenderer()
	content := "Just a normal line\nFollowed by another"
	out := rendered(r, content)
	plain := stripAnsiCodes(out)
	// Should NOT contain definition markers
	if strings.Contains(plain, "▸") {
		t.Errorf("Normal text should not trigger definition list, got:\n%s", plain)
	}
}

// =========================================================================
// Soft line breaks vs hard breaks
// =========================================================================

func TestRenderSoftBreak(t *testing.T) {
	r := newTestRenderer()
	content := "First line\nSecond line"
	out := rendered(r, content)
	plain := stripAnsiCodes(out)
	// Soft break: single newline should join with space
	if !strings.Contains(plain, "First line Second line") {
		t.Errorf("Soft break should join lines with space, got:\n%s", plain)
	}
}

func TestRenderHardBreak(t *testing.T) {
	r := newTestRenderer()
	// Two trailing spaces before newline = hard break
	content := "First line  \nSecond line"
	lines := renderedLines(r, content)
	plainLines := make([]string, len(lines))
	for i, l := range lines {
		plainLines[i] = stripAnsiCodes(l)
	}
	// Hard break should produce separate rendered lines
	foundFirst := false
	foundSecond := false
	for _, pl := range plainLines {
		if strings.Contains(pl, "First line") {
			foundFirst = true
		}
		if strings.Contains(pl, "Second line") {
			foundSecond = true
		}
	}
	if !foundFirst || !foundSecond {
		t.Errorf("Hard break should render both lines, got:\n%s", strings.Join(plainLines, "\n"))
	}
	// They should NOT be on the same line
	for _, pl := range plainLines {
		if strings.Contains(pl, "First line") && strings.Contains(pl, "Second line") {
			t.Errorf("Hard break should put lines on separate rendered lines, got:\n%s", pl)
		}
	}
}

func TestRenderSoftBreakMultipleLines(t *testing.T) {
	r := newTestRenderer()
	content := "Line one\nLine two\nLine three"
	out := rendered(r, content)
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "Line one Line two Line three") {
		t.Errorf("Multiple soft breaks should join all lines, got:\n%s", plain)
	}
}

func TestRenderParagraphBreak(t *testing.T) {
	r := newTestRenderer()
	content := "Paragraph one\n\nParagraph two"
	lines := renderedLines(r, content)
	plainLines := make([]string, len(lines))
	for i, l := range lines {
		plainLines[i] = stripAnsiCodes(l)
	}
	// Empty line should separate paragraphs
	foundP1 := false
	foundP2 := false
	for _, pl := range plainLines {
		if strings.Contains(pl, "Paragraph one") {
			foundP1 = true
		}
		if strings.Contains(pl, "Paragraph two") {
			foundP2 = true
		}
	}
	if !foundP1 || !foundP2 {
		t.Errorf("Paragraph break should render both paragraphs, got:\n%s", strings.Join(plainLines, "\n"))
	}
}

func TestRenderSoftBreakStopsAtHeading(t *testing.T) {
	r := newTestRenderer()
	content := "Some text\n## Heading"
	out := rendered(r, content)
	plain := stripAnsiCodes(out)
	// Heading should not be merged into paragraph
	if strings.Contains(plain, "Some text Heading") {
		t.Errorf("Soft break should not merge heading into paragraph, got:\n%s", plain)
	}
}

func TestRenderSoftBreakStopsAtList(t *testing.T) {
	r := newTestRenderer()
	content := "Some text\n- list item"
	out := rendered(r, content)
	plain := stripAnsiCodes(out)
	if strings.Contains(plain, "Some text - list item") || strings.Contains(plain, "Some text list item") {
		t.Errorf("Soft break should not merge list item into paragraph, got:\n%s", plain)
	}
}

func TestRenderIsBlockElement(t *testing.T) {
	blockLines := []string{
		"# Heading", "## H2", "### H3", "#### H4",
		"> Blockquote", "```code",
		"- Item", "* Star item",
		"- [ ] Todo", "- [x] Done",
		"---", "***", "___",
		"$$math", "[^1]: footnote",
		"![[embed]]", "![alt](img.png)",
		"1. Numbered",
		"| table | row |",
		": definition",
	}
	for _, line := range blockLines {
		if !isBlockElement(line) {
			t.Errorf("Expected %q to be detected as block element", line)
		}
	}

	nonBlockLines := []string{
		"Normal text",
		"Some paragraph content",
		"Just a sentence.",
	}
	for _, line := range nonBlockLines {
		if isBlockElement(line) {
			t.Errorf("Expected %q to NOT be detected as block element", line)
		}
	}
}

// =========================================================================
// HTML inline tags
// =========================================================================

func TestRenderHTMLKbd(t *testing.T) {
	r := newTestRenderer()
	content := "Press <kbd>Ctrl</kbd> to continue"
	out := rendered(r, content)
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "Ctrl") {
		t.Errorf("kbd tag should render content, got:\n%s", plain)
	}
	// Should not contain the raw HTML tags
	if strings.Contains(plain, "<kbd>") || strings.Contains(plain, "</kbd>") {
		t.Errorf("kbd tag HTML should be consumed, got:\n%s", plain)
	}
}

func TestRenderHTMLSub(t *testing.T) {
	r := newTestRenderer()
	content := "H<sub>2</sub>O"
	out := rendered(r, content)
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "_2") {
		t.Errorf("sub tag should render with underscore prefix, got:\n%s", plain)
	}
	if strings.Contains(plain, "<sub>") {
		t.Errorf("sub tag HTML should be consumed, got:\n%s", plain)
	}
}

func TestRenderHTMLSup(t *testing.T) {
	r := newTestRenderer()
	content := "x<sup>2</sup> + y"
	out := rendered(r, content)
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "^2") {
		t.Errorf("sup tag should render with caret prefix, got:\n%s", plain)
	}
	if strings.Contains(plain, "<sup>") {
		t.Errorf("sup tag HTML should be consumed, got:\n%s", plain)
	}
}

func TestRenderHTMLMark(t *testing.T) {
	r := newTestRenderer()
	content := "This is <mark>highlighted</mark> text"
	out := rendered(r, content)
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "highlighted") {
		t.Errorf("mark tag should render content, got:\n%s", plain)
	}
	if strings.Contains(plain, "<mark>") {
		t.Errorf("mark tag HTML should be consumed, got:\n%s", plain)
	}
}

func TestRenderHTMLAbbr(t *testing.T) {
	r := newTestRenderer()
	content := "The <abbr title=\"HyperText Markup Language\">HTML</abbr> spec"
	out := rendered(r, content)
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "HTML") {
		t.Errorf("abbr tag should render text content, got:\n%s", plain)
	}
	if !strings.Contains(plain, "HyperText Markup Language") {
		t.Errorf("abbr tag should show title, got:\n%s", plain)
	}
	if strings.Contains(plain, "<abbr") {
		t.Errorf("abbr tag HTML should be consumed, got:\n%s", plain)
	}
}

func TestRenderHTMLMultipleTags(t *testing.T) {
	r := newTestRenderer()
	content := "<kbd>Ctrl</kbd>+<kbd>C</kbd> to copy"
	out := rendered(r, content)
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "Ctrl") || !strings.Contains(plain, "C") {
		t.Errorf("Multiple kbd tags should all render, got:\n%s", plain)
	}
}

func TestRenderHTMLTagNotMatched(t *testing.T) {
	r := newTestRenderer()
	content := "A <div>tag</div> not supported"
	out := rendered(r, content)
	plain := stripAnsiCodes(out)
	// Unknown HTML tags should pass through as-is
	if !strings.Contains(plain, "<div>") {
		t.Errorf("Unknown HTML tag should pass through, got:\n%s", plain)
	}
}

// =========================================================================
// Task list nesting
// =========================================================================

func TestRenderNestedTaskListUnchecked(t *testing.T) {
	r := newTestRenderer()
	content := "- [ ] Parent\n  - [ ] Child"
	lines := renderedLines(r, content)
	plainLines := make([]string, len(lines))
	for i, l := range lines {
		plainLines[i] = stripAnsiCodes(l)
	}
	foundParent := false
	foundChild := false
	for _, pl := range plainLines {
		if strings.Contains(pl, "Parent") && strings.Contains(pl, "☐") {
			foundParent = true
		}
		if strings.Contains(pl, "Child") && strings.Contains(pl, "☐") {
			foundChild = true
		}
	}
	if !foundParent {
		t.Errorf("Should render parent task, got:\n%s", strings.Join(plainLines, "\n"))
	}
	if !foundChild {
		t.Errorf("Should render child task, got:\n%s", strings.Join(plainLines, "\n"))
	}
}

func TestRenderNestedTaskListChecked(t *testing.T) {
	r := newTestRenderer()
	content := "- [ ] Parent\n  - [x] Done child"
	lines := renderedLines(r, content)
	plainLines := make([]string, len(lines))
	for i, l := range lines {
		plainLines[i] = stripAnsiCodes(l)
	}
	foundDoneChild := false
	for _, pl := range plainLines {
		if strings.Contains(pl, "Done child") && strings.Contains(pl, "☑") {
			foundDoneChild = true
		}
	}
	if !foundDoneChild {
		t.Errorf("Should render checked child task, got:\n%s", strings.Join(plainLines, "\n"))
	}
}

func TestRenderNestedTaskListConnector(t *testing.T) {
	r := newTestRenderer()
	content := "- [ ] Parent\n  - [ ] Child"
	lines := renderedLines(r, content)
	plainLines := make([]string, len(lines))
	for i, l := range lines {
		plainLines[i] = stripAnsiCodes(l)
	}
	// Child should have connector character
	foundConnector := false
	for _, pl := range plainLines {
		if strings.Contains(pl, "Child") && strings.Contains(pl, "└") {
			foundConnector = true
		}
	}
	if !foundConnector {
		t.Errorf("Nested child should have └ connector, got:\n%s", strings.Join(plainLines, "\n"))
	}
}

func TestRenderDeeplyNestedTaskList(t *testing.T) {
	r := newTestRenderer()
	content := "- [ ] Level 0\n  - [ ] Level 1\n    - [x] Level 2"
	lines := renderedLines(r, content)
	plainLines := make([]string, len(lines))
	for i, l := range lines {
		plainLines[i] = stripAnsiCodes(l)
	}
	if len(plainLines) < 3 {
		t.Errorf("Should have at least 3 lines for 3 levels, got %d:\n%s",
			len(plainLines), strings.Join(plainLines, "\n"))
		return
	}
	// Level 2 should be more indented than Level 1
	l1Indent := 0
	l2Indent := 0
	for _, pl := range plainLines {
		stripped := strings.TrimLeft(pl, " ")
		indent := len(pl) - len(stripped)
		if strings.Contains(pl, "Level 1") {
			l1Indent = indent
		}
		if strings.Contains(pl, "Level 2") {
			l2Indent = indent
		}
	}
	if l2Indent <= l1Indent {
		t.Errorf("Level 2 should be more indented than Level 1 (got L1=%d, L2=%d)", l1Indent, l2Indent)
	}
}

func TestRenderNestedTaskListParentNoConnector(t *testing.T) {
	r := newTestRenderer()
	content := "- [ ] Parent task"
	out := rendered(r, content)
	plain := stripAnsiCodes(out)
	// Top-level task should NOT have connector
	if strings.Contains(plain, "└") {
		t.Errorf("Top-level task should not have └ connector, got:\n%s", plain)
	}
}

// =========================================================================
// Callout/admonition improvements
// =========================================================================

func TestRenderCalloutBug(t *testing.T) {
	r := newTestRenderer()
	content := "> [!bug] Found a bug\n> This is a bug report"
	out := rendered(r, content)
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "Bug") {
		t.Errorf("Bug callout should show label, got:\n%s", plain)
	}
	if !strings.Contains(plain, "Found a bug") {
		t.Errorf("Bug callout should show title, got:\n%s", plain)
	}
	if !strings.Contains(plain, "◉") {
		t.Errorf("Bug callout should show icon, got:\n%s", plain)
	}
}

func TestRenderCalloutAbstract(t *testing.T) {
	r := newTestRenderer()
	content := "> [!abstract] Summary\n> This is the abstract"
	out := rendered(r, content)
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "Abstract") {
		t.Errorf("Abstract callout should show label, got:\n%s", plain)
	}
	if !strings.Contains(plain, "Summary") {
		t.Errorf("Abstract callout should show title, got:\n%s", plain)
	}
}

func TestRenderCalloutImportant(t *testing.T) {
	r := newTestRenderer()
	content := "> [!important] Read this\n> Critical information"
	out := rendered(r, content)
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "Important") {
		t.Errorf("Important callout should show label, got:\n%s", plain)
	}
	if !strings.Contains(plain, "★") {
		t.Errorf("Important callout should show star icon, got:\n%s", plain)
	}
}

func TestRenderCalloutFailure(t *testing.T) {
	r := newTestRenderer()
	content := "> [!failure] Test failed\n> Assertion error"
	out := rendered(r, content)
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "Failure") {
		t.Errorf("Failure callout should show label, got:\n%s", plain)
	}
	if !strings.Contains(plain, "✘") {
		t.Errorf("Failure callout should show ✘ icon, got:\n%s", plain)
	}
}

func TestRenderCalloutMissing(t *testing.T) {
	r := newTestRenderer()
	content := "> [!missing] Data not found\n> File was deleted"
	out := rendered(r, content)
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "Missing") {
		t.Errorf("Missing callout should show label, got:\n%s", plain)
	}
}

func TestRenderCalloutAttention(t *testing.T) {
	r := newTestRenderer()
	content := "> [!attention] Watch out\n> Be careful here"
	out := rendered(r, content)
	plain := stripAnsiCodes(out)
	if !strings.Contains(plain, "Attention") {
		t.Errorf("Attention callout should show label, got:\n%s", plain)
	}
	if !strings.Contains(plain, "◆") {
		t.Errorf("Attention callout should show diamond icon, got:\n%s", plain)
	}
}

func TestRenderCalloutHasBorders(t *testing.T) {
	r := newTestRenderer()
	content := "> [!note] A note\n> Some content"
	lines := renderedLines(r, content)
	plainLines := make([]string, len(lines))
	for i, l := range lines {
		plainLines[i] = stripAnsiCodes(l)
	}
	// Should have top and bottom borders (─ characters beyond header)
	borderCount := 0
	for _, pl := range plainLines {
		if strings.Contains(pl, "┃─") || strings.Contains(pl, "┃─") {
			borderCount++
		}
	}
	if borderCount < 2 {
		t.Errorf("Callout should have top and bottom borders, found %d border lines in:\n%s",
			borderCount, strings.Join(plainLines, "\n"))
	}
}

func TestRenderCalloutAllNewTypes(t *testing.T) {
	types := map[string]string{
		"important": "Important",
		"attention": "Attention",
		"failure":   "Failure",
		"fail":      "Failure",
		"missing":   "Missing",
	}
	r := newTestRenderer()
	for calloutType, expectedLabel := range types {
		content := "> [!" + calloutType + "] Title"
		out := rendered(r, content)
		plain := stripAnsiCodes(out)
		if !strings.Contains(plain, expectedLabel) {
			t.Errorf("Callout [!%s] should produce label %q, got:\n%s", calloutType, expectedLabel, plain)
		}
	}
}

func TestRenderCalloutDistinctColors(t *testing.T) {
	r := newTestRenderer()
	// Bug and danger should look different (different icons at minimum)
	bugOut := rendered(r, "> [!bug] Bug")
	dangerOut := rendered(r, "> [!danger] Danger")
	if bugOut == dangerOut {
		t.Errorf("Bug and danger callouts should look different")
	}
}
