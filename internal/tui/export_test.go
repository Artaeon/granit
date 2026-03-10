package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// ---------------------------------------------------------------------------
// NewExportOverlay — initial state
// ---------------------------------------------------------------------------

func TestExportNewOverlay(t *testing.T) {
	e := NewExportOverlay()
	if e.IsActive() {
		t.Error("new overlay should not be active")
	}
	if len(e.formats) != 4 {
		t.Errorf("expected 4 formats, got %d", len(e.formats))
	}
	if e.cursor != 0 {
		t.Errorf("expected cursor 0, got %d", e.cursor)
	}
	if e.notePath != "" {
		t.Error("notePath should be empty")
	}
	if e.noteContent != "" {
		t.Error("noteContent should be empty")
	}
}

// ---------------------------------------------------------------------------
// Open / Close / IsActive — state transitions
// ---------------------------------------------------------------------------

func TestExportOpenCloseIsActive(t *testing.T) {
	e := NewExportOverlay()

	if e.IsActive() {
		t.Error("should not be active before Open")
	}

	e.Open("notes/hello.md", "# Hello", "/tmp/vault")
	if !e.IsActive() {
		t.Error("should be active after Open")
	}
	if e.notePath != "notes/hello.md" {
		t.Errorf("unexpected notePath: %s", e.notePath)
	}
	if e.noteContent != "# Hello" {
		t.Errorf("unexpected noteContent: %s", e.noteContent)
	}
	if e.vaultRoot != "/tmp/vault" {
		t.Errorf("unexpected vaultRoot: %s", e.vaultRoot)
	}
	if e.cursor != 0 {
		t.Errorf("cursor should reset to 0 on Open, got %d", e.cursor)
	}

	e.Close()
	if e.IsActive() {
		t.Error("should not be active after Close")
	}
}

func TestExportOpenResetsState(t *testing.T) {
	e := NewExportOverlay()
	e.cursor = 3
	e.result = "some old result"
	e.exporting = true

	e.Open("note.md", "content", "/vault")
	if e.cursor != 0 {
		t.Errorf("cursor should reset on Open, got %d", e.cursor)
	}
	if e.result != "" {
		t.Errorf("result should reset on Open, got %q", e.result)
	}
	if e.exporting {
		t.Error("exporting should reset on Open")
	}
}

// ---------------------------------------------------------------------------
// SetSize — dimensions
// ---------------------------------------------------------------------------

func TestExportSetSize(t *testing.T) {
	e := NewExportOverlay()
	e.SetSize(120, 40)
	if e.width != 120 {
		t.Errorf("expected width 120, got %d", e.width)
	}
	if e.height != 40 {
		t.Errorf("expected height 40, got %d", e.height)
	}
}

// ---------------------------------------------------------------------------
// Format selection — navigate between options
// ---------------------------------------------------------------------------

func TestExportNavigateFormats(t *testing.T) {
	e := NewExportOverlay()
	e.Open("note.md", "hello", "/tmp/vault")

	// Navigate down through all formats
	for i := 0; i < 3; i++ {
		e, _ = e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	}
	if e.cursor != 3 {
		t.Errorf("expected cursor at 3, got %d", e.cursor)
	}

	// Cursor should not go beyond last format
	e, _ = e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if e.cursor != 3 {
		t.Errorf("cursor should clamp at 3, got %d", e.cursor)
	}

	// Navigate back up
	e, _ = e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if e.cursor != 2 {
		t.Errorf("expected cursor at 2, got %d", e.cursor)
	}

	// Navigate to top
	for i := 0; i < 5; i++ {
		e, _ = e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	}
	if e.cursor != 0 {
		t.Errorf("cursor should clamp at 0, got %d", e.cursor)
	}
}

func TestExportNavigateArrowKeys(t *testing.T) {
	e := NewExportOverlay()
	e.Open("note.md", "hello", "/tmp/vault")

	e, _ = e.Update(tea.KeyMsg{Type: tea.KeyDown})
	if e.cursor != 1 {
		t.Errorf("expected cursor 1 after down, got %d", e.cursor)
	}

	e, _ = e.Update(tea.KeyMsg{Type: tea.KeyUp})
	if e.cursor != 0 {
		t.Errorf("expected cursor 0 after up, got %d", e.cursor)
	}
}

// ---------------------------------------------------------------------------
// SetNote via Open — sets the note to export
// ---------------------------------------------------------------------------

func TestExportSetNote(t *testing.T) {
	e := NewExportOverlay()
	e.Open("docs/readme.md", "# Readme\n\nSome content", "/vault")

	if e.notePath != "docs/readme.md" {
		t.Errorf("unexpected notePath: %s", e.notePath)
	}
	if e.noteContent != "# Readme\n\nSome content" {
		t.Errorf("unexpected noteContent: %s", e.noteContent)
	}
}

// ---------------------------------------------------------------------------
// Export to text — produces plain text output (strips markdown)
// ---------------------------------------------------------------------------

func TestExportPlainTextStripsMarkdown(t *testing.T) {
	input := "# Hello World\n\nThis is **bold** and *italic*.\n\n- item one\n- item two\n"
	result := exportStripMarkdown(input)

	if strings.Contains(result, "#") {
		t.Error("plain text should strip heading markers")
	}
	if strings.Contains(result, "**") {
		t.Error("plain text should strip bold markers")
	}
	if strings.Contains(result, "- ") {
		t.Error("plain text should strip list markers")
	}
	if !strings.Contains(result, "Hello World") {
		t.Error("plain text should contain heading text")
	}
	if !strings.Contains(result, "item one") {
		t.Error("plain text should contain list item text")
	}
}

func TestExportPlainTextStripsFrontmatter(t *testing.T) {
	input := "---\ntitle: Test\ntags: [a, b]\n---\n# Body\n"
	result := exportStripMarkdown(input)

	if strings.Contains(result, "title: Test") {
		t.Error("plain text should strip frontmatter")
	}
	if !strings.Contains(result, "Body") {
		t.Error("plain text should keep body")
	}
}

func TestExportPlainTextStripsWikiLinks(t *testing.T) {
	input := "See [[Other Note]] for more.\n"
	result := exportStripMarkdown(input)

	if strings.Contains(result, "[[") {
		t.Error("plain text should strip wikilink brackets")
	}
	if !strings.Contains(result, "Other Note") {
		t.Error("plain text should keep link display text")
	}
}

func TestExportPlainTextStripsWikiLinkWithAlias(t *testing.T) {
	input := "See [[Other Note|alias text]] for more.\n"
	result := exportStripMarkdown(input)

	if strings.Contains(result, "[[") {
		t.Error("plain text should strip wikilink brackets")
	}
	if !strings.Contains(result, "alias text") {
		t.Error("plain text should show alias text")
	}
}

func TestExportPlainTextPreservesCodeBlocks(t *testing.T) {
	input := "```go\nfunc main() {}\n```\n"
	result := exportStripMarkdown(input)

	if !strings.Contains(result, "func main() {}") {
		t.Error("plain text should preserve code block content")
	}
	if strings.Contains(result, "```") {
		t.Error("plain text should strip code fences")
	}
}

// ---------------------------------------------------------------------------
// Export to HTML — produces HTML with proper tags
// ---------------------------------------------------------------------------

func TestExportHTMLProducesHeadings(t *testing.T) {
	input := "# Title\n## Subtitle\n"
	html := markdownToHTML(input)

	if !strings.Contains(html, "<h1>Title</h1>") {
		t.Errorf("expected <h1> tag, got: %s", html)
	}
	if !strings.Contains(html, "<h2>Subtitle</h2>") {
		t.Errorf("expected <h2> tag, got: %s", html)
	}
}

func TestExportHTMLProducesBoldItalic(t *testing.T) {
	input := "This is **bold** text.\n"
	html := markdownToHTML(input)

	if !strings.Contains(html, "<strong>bold</strong>") {
		t.Errorf("expected <strong> tag, got: %s", html)
	}
}

func TestExportHTMLProducesLists(t *testing.T) {
	input := "- item one\n- item two\n"
	html := markdownToHTML(input)

	if !strings.Contains(html, "<ul>") {
		t.Errorf("expected <ul> tag, got: %s", html)
	}
	if !strings.Contains(html, "<li>item one</li>") {
		t.Errorf("expected list items, got: %s", html)
	}
}

func TestExportHTMLProducesBlockquote(t *testing.T) {
	input := "> quoted text\n"
	html := markdownToHTML(input)

	if !strings.Contains(html, "<blockquote>") {
		t.Errorf("expected <blockquote> tag, got: %s", html)
	}
	if !strings.Contains(html, "quoted text") {
		t.Errorf("expected quoted text, got: %s", html)
	}
}

func TestExportHTMLProducesCodeBlock(t *testing.T) {
	input := "```go\nfmt.Println()\n```\n"
	html := markdownToHTML(input)

	if !strings.Contains(html, `<pre><code class="language-go">`) {
		t.Errorf("expected code block with language, got: %s", html)
	}
}

func TestExportHTMLProducesCheckboxes(t *testing.T) {
	input := "- [x] done\n- [ ] todo\n"
	html := markdownToHTML(input)

	if !strings.Contains(html, `checked disabled`) {
		t.Errorf("expected checked checkbox, got: %s", html)
	}
	if !strings.Contains(html, `<input type="checkbox" disabled>`) {
		t.Errorf("expected unchecked checkbox, got: %s", html)
	}
}

func TestExportHTMLWikiLinks(t *testing.T) {
	input := "See [[Other Note]] here.\n"
	html := markdownToHTML(input)

	if !strings.Contains(html, `<a href="Other Note.html">Other Note</a>`) {
		t.Errorf("expected wiki link conversion, got: %s", html)
	}
}

func TestExportHTMLSkipsFrontmatter(t *testing.T) {
	input := "---\ntitle: Test\n---\n# Body\n"
	html := markdownToHTML(input)

	if strings.Contains(html, "title: Test") {
		t.Error("HTML should skip frontmatter")
	}
	if !strings.Contains(html, "<h1>Body</h1>") {
		t.Errorf("expected body heading, got: %s", html)
	}
}

func TestExportHTMLEscapesSpecialChars(t *testing.T) {
	input := "Use <div> & \"quotes\"\n"
	html := markdownToHTML(input)

	if !strings.Contains(html, "&lt;div&gt;") {
		t.Errorf("expected escaped HTML entities, got: %s", html)
	}
	if !strings.Contains(html, "&amp;") {
		t.Errorf("expected escaped ampersand, got: %s", html)
	}
}

func TestExportWrapHTML(t *testing.T) {
	body := "<h1>Title</h1>"
	result := wrapHTML("My Title", body)

	if !strings.Contains(result, "<!DOCTYPE html>") {
		t.Error("expected DOCTYPE")
	}
	if !strings.Contains(result, "<title>My Title</title>") {
		t.Error("expected title tag")
	}
	if !strings.Contains(result, body) {
		t.Error("expected body content in output")
	}
}

// ---------------------------------------------------------------------------
// Navigation — up/down between format options
// ---------------------------------------------------------------------------

func TestExportUpdateInactive(t *testing.T) {
	e := NewExportOverlay()
	// Update while inactive should be a no-op
	e2, cmd := e.Update(tea.KeyMsg{Type: tea.KeyDown})
	if cmd != nil {
		t.Error("expected nil cmd when inactive")
	}
	if e2.cursor != 0 {
		t.Error("cursor should not change when inactive")
	}
}

func TestExportEscCloses(t *testing.T) {
	e := NewExportOverlay()
	e.Open("note.md", "content", "/vault")

	e, _ = e.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if e.IsActive() {
		t.Error("Esc should close the overlay")
	}
}

func TestExportResultDismissesOnEnter(t *testing.T) {
	e := NewExportOverlay()
	e.Open("note.md", "content", "/vault")
	e.result = "Exported: /some/path"

	e, _ = e.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if e.IsActive() {
		t.Error("Enter on result screen should close overlay")
	}
}

func TestExportResultDismissesOnEsc(t *testing.T) {
	e := NewExportOverlay()
	e.Open("note.md", "content", "/vault")
	e.result = "Exported: /some/path"

	e, _ = e.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if e.IsActive() {
		t.Error("Esc on result screen should close overlay")
	}
}

// ---------------------------------------------------------------------------
// Empty note — exports empty content without crash
// ---------------------------------------------------------------------------

func TestExportEmptyContent(t *testing.T) {
	html := markdownToHTML("")
	if html == "" {
		// Empty input can produce empty output — that's fine
	}
	// Should not panic
	plain := exportStripMarkdown("")
	_ = plain
}

func TestExportEmptyContentPlainText(t *testing.T) {
	result := exportStripMarkdown("")
	// Should be empty or just a newline
	trimmed := strings.TrimSpace(result)
	if trimmed != "" {
		t.Errorf("expected empty result for empty input, got %q", trimmed)
	}
}

// ---------------------------------------------------------------------------
// Export file write integration tests (using temp directories)
// ---------------------------------------------------------------------------

func TestExportHTMLWritesFile(t *testing.T) {
	tmpDir := t.TempDir()
	e := NewExportOverlay()
	e.Open("test.md", "# Hello\n\nWorld\n", tmpDir)

	// Trigger HTML export (cursor at 0)
	e.doExport()

	outPath := filepath.Join(tmpDir, "test.html")
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("expected HTML file at %s: %v", outPath, err)
	}
	content := string(data)
	if !strings.Contains(content, "<!DOCTYPE html>") {
		t.Error("expected DOCTYPE in exported HTML")
	}
	if !strings.Contains(content, "<h1>Hello</h1>") {
		t.Error("expected heading in exported HTML")
	}
	if !strings.Contains(e.result, "Exported:") {
		t.Errorf("expected success result, got: %s", e.result)
	}
}

func TestExportPlainTextWritesFile(t *testing.T) {
	tmpDir := t.TempDir()
	e := NewExportOverlay()
	e.Open("test.md", "# Hello\n\n**Bold** text\n", tmpDir)
	e.cursor = 1 // Plain Text

	e.doExport()

	outPath := filepath.Join(tmpDir, "test.txt")
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("expected text file at %s: %v", outPath, err)
	}
	content := string(data)
	if strings.Contains(content, "#") {
		t.Error("plain text should not contain heading markers")
	}
	if !strings.Contains(content, "Hello") {
		t.Error("expected heading text in plain text output")
	}
	if !strings.Contains(e.result, "Exported:") {
		t.Errorf("expected success result, got: %s", e.result)
	}
}

func TestExportNoNotePathError(t *testing.T) {
	e := NewExportOverlay()
	e.Open("", "content", "/vault")

	// HTML export with empty path
	e.cursor = 0
	e.doExport()
	if e.result != "Error: no note is open" {
		t.Errorf("expected error for empty note path, got: %s", e.result)
	}

	// Plain text export with empty path
	e.result = ""
	e.cursor = 1
	e.doExport()
	if e.result != "Error: no note is open" {
		t.Errorf("expected error for empty note path, got: %s", e.result)
	}

	// PDF export with empty path
	e.result = ""
	e.cursor = 2
	e.doExport()
	if e.result != "Error: no note is open" {
		t.Errorf("expected error for empty note path, got: %s", e.result)
	}
}

func TestExportViewRenders(t *testing.T) {
	e := NewExportOverlay()
	e.SetSize(120, 40)
	e.Open("note.md", "content", "/vault")

	// Should not panic
	view := e.View()
	if view == "" {
		t.Error("expected non-empty view")
	}
}

func TestExportViewWithResult(t *testing.T) {
	e := NewExportOverlay()
	e.SetSize(120, 40)
	e.Open("note.md", "content", "/vault")
	e.result = "Exported: /some/path"

	view := e.View()
	if view == "" {
		t.Error("expected non-empty view with result")
	}
}
