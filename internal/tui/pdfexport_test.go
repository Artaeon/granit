package tui

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// lookForBinary returns the absolute path of name on $PATH or an error
// if missing. Wraps exec.LookPath in a single helper so the integration
// test can clearly skip when pandoc / xelatex / pdftotext aren't
// installed (CI environments without LaTeX, slim Docker images).
func lookForBinary(name string) (string, error) {
	p, err := exec.LookPath(name)
	if err != nil {
		return "", errors.New(name + " not on PATH")
	}
	return p, nil
}

// preprocessForPDF must turn wikilinks into italicized text so they
// render meaningfully on a printed page (no broken-link blue underline,
// no literal "[[name]]" bracket noise). Both the bare and aliased
// forms have to work.
func TestPreprocessForPDF_WikilinksItalicized(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"See [[Energie Perg]] for context.", "See *Energie Perg* for context."},
		{"See [[Energie Perg|the Perg cooperative]] for context.", "See *the Perg cooperative* for context."},
		{"Two links: [[A]] and [[B]].", "Two links: *A* and *B*."},
		// Section anchor: trailing #section drops; we don't want
		// raw # in italic prose.
		{"Plain paragraph with no link.", "Plain paragraph with no link."},
	}
	for _, c := range cases {
		got := preprocessForPDF(c.in)
		if got != c.want {
			t.Errorf("input %q\n  got:  %q\n  want: %q", c.in, got, c.want)
		}
	}
}

// Task-marker emojis (📅 due dates, 🔼 priorities, 🔁 recurrence, ⏰
// time blocks) MUST be stripped before pandoc renders. Without this
// they print as missing-glyph boxes since LaTeX's default fonts have
// no emoji coverage.
func TestPreprocessForPDF_StripsTaskEmoji(t *testing.T) {
	in := "- [ ] Write report 📅 2026-04-15 ⏫ ~30m #work\n" +
		"- [ ] Daily standup 🔁 daily ⏰ 09:00-09:15"
	got := preprocessForPDF(in)
	for _, banned := range []string{"📅", "⏫", "🔁", "⏰"} {
		if strings.Contains(got, banned) {
			t.Errorf("emoji %q should be stripped, got:\n%s", banned, got)
		}
	}
	// And the actual text should survive.
	if !strings.Contains(got, "Write report") {
		t.Errorf("text body lost: %s", got)
	}
}

// Checkbox lines convert to Unicode glyphs that DejaVu Sans Mono +
// xelatex's default fallback fonts both render correctly, instead of
// the literal "[ ]" / "[x]" which look like ASCII art on the page.
func TestPreprocessForPDF_CheckboxesToUnicode(t *testing.T) {
	in := "- [ ] open task\n- [x] done task\n  - [ ] indented sub"
	got := preprocessForPDF(in)
	if !strings.Contains(got, "- ☐ open task") {
		t.Errorf("open checkbox not converted: %s", got)
	}
	if !strings.Contains(got, "- ☑ done task") {
		t.Errorf("done checkbox not converted: %s", got)
	}
	if !strings.Contains(got, "  - ☐ indented sub") {
		t.Errorf("indented checkbox not converted: %s", got)
	}
}

// Frontmatter splits cleanly so pandoc receives a body it can parse
// and we keep the title/date/author for the YAML metadata block we
// emit on top of the temp file.
func TestSplitFrontmatterForPDF(t *testing.T) {
	in := "---\ntitle: Big Document\ndate: 2026-04-15\nauthor: Jane Doe\n---\n\n# H1\nbody.\n"
	body, fm := splitFrontmatterForPDF(in)
	if fm["title"] != "Big Document" {
		t.Errorf("title: got %q", fm["title"])
	}
	if fm["date"] != "2026-04-15" {
		t.Errorf("date: got %q", fm["date"])
	}
	if fm["author"] != "Jane Doe" {
		t.Errorf("author: got %q", fm["author"])
	}
	if !strings.HasPrefix(body, "# H1") {
		t.Errorf("body should start with the H1, got: %q", body)
	}
}

// No-frontmatter input must pass through unchanged so notes without
// YAML headers still export.
func TestSplitFrontmatterForPDF_None(t *testing.T) {
	in := "# H1\nbody only.\n"
	body, fm := splitFrontmatterForPDF(in)
	if body != in {
		t.Errorf("body changed without frontmatter: %q", body)
	}
	if len(fm) != 0 {
		t.Errorf("empty frontmatter map expected, got %v", fm)
	}
}

// pdfYamlEscape protects single-quoted YAML strings against embedded
// quotes by doubling them — required because we stuff potentially-
// untrusted note titles into the temp file's metadata block.
func TestPDFYamlEscape(t *testing.T) {
	cases := []struct{ in, want string }{
		{"plain", "'plain'"},
		{"", "''"},
		{"It's working", "'It''s working'"},
		{"a 'b' c", "'a ''b'' c'"},
	}
	for _, c := range cases {
		if got := pdfYamlEscape(c.in); got != c.want {
			t.Errorf("pdfYamlEscape(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// The TOC heuristic auto-enables for substantial notes: 4+ H2 sections
// AND >= 1500 words. Below either threshold we omit the TOC (a
// 2-section / 200-word note doesn't need a table of contents).
func TestCountH2Plus_IgnoresFencedCode(t *testing.T) {
	in := "## Real heading\n```\n## Not a heading inside code\n```\n## Another real one"
	if got := countH2Plus(in); got != 2 {
		t.Errorf("expected 2 H2 (ignoring fenced ##), got %d", got)
	}
}

// Integration test: drives the full pandoc pipeline if pandoc + a LaTeX
// engine are installed. Skipped when not — keeps CI green on slim
// images. Run locally with:
//
//	go test ./internal/tui/ -run TestPDFExportIntegration -v
//
// Verifies: process exits 0, output PDF exists with non-trivial size,
// pdftotext (if available) extracts the expected title/headings/body.
func TestPDFExportIntegration(t *testing.T) {
	pandoc, err := lookForBinary("pandoc")
	if err != nil {
		t.Skipf("pandoc not installed — skipping integration test (%v)", err)
	}
	if _, err := lookForBinary("xelatex"); err != nil {
		t.Skipf("xelatex not installed — skipping integration test (%v)", err)
	}

	tmpDir := t.TempDir()
	notePath := "Showcase.md"
	body := `---
title: Granit PDF Export Showcase
date: 2026-04-28
author: Test Suite
---

# Granit PDF Export Showcase

This note exercises every preprocessor path: wikilinks, task-marker
emojis, checkboxes, headings, tables, code, and inline formatting.

## Wikilinks

A reference to [[Important Note]] and an aliased [[Source|the original]] should
both italicize cleanly without leaving raw brackets in the PDF.

## Tasks

- [ ] Open task with due date 📅 2026-04-30 ⏫ ~30m #urgent
- [x] Completed daily routine 🔁 daily ⏰ 09:00-09:15
- [ ] Nested:
  - [ ] sub-task one
  - [x] sub-task two

## Table

| Topic | Status | Effort |
|-------|--------|--------|
| Layout | Done | 2h |
| Typography | In progress | 4h |
| Code blocks | Pending | 1h |

## Code

` + "```go\nfunc main() {\n    fmt.Println(\"hello, PDF\")\n}\n```" + `

## Inline formatting

Some **bold**, some *italic*, some ` + "`inline code`" + `, and an
[external link](https://example.com) for completeness.
`

	outPath := filepath.Join(tmpDir, "showcase.pdf")
	err = renderNoteToPDF(notePath, body, outPath, pdfRenderOptions{
		PandocPath: pandoc,
	})
	if err != nil {
		t.Fatalf("renderNoteToPDF failed: %v", err)
	}

	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("output PDF not created: %v", err)
	}
	if info.Size() < 4000 {
		t.Errorf("PDF suspiciously small (%d bytes) — render may have produced an empty document", info.Size())
	}
	// First 4 bytes of any PDF are "%PDF".
	header := make([]byte, 4)
	f, err := os.Open(outPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Read(header); err != nil {
		t.Fatal(err)
	}
	if string(header) != "%PDF" {
		t.Errorf("output is not a PDF — first 4 bytes = %q", string(header))
	}

	// If pdftotext is available, sanity-check the extracted text.
	pdftotext, err := lookForBinary("pdftotext")
	if err != nil {
		t.Logf("pdftotext not installed — skipping content extraction (PDF header check is enough)")
		return
	}
	textPath := filepath.Join(tmpDir, "showcase.txt")
	cmd := exec.Command(pdftotext, "-layout", outPath, textPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("pdftotext failed: %v: %s", err, out)
	}
	text, err := os.ReadFile(textPath)
	if err != nil {
		t.Fatal(err)
	}
	s := string(text)
	for _, want := range []string{
		"Granit PDF Export Showcase", // title
		"Test Suite",                 // author
		"Wikilinks",                  // section heading
		"Important Note",             // wikilink target → italic
		"the original",               // aliased wikilink display
		"hello, PDF",                 // code block content
		"Topic",                      // table header
	} {
		if !strings.Contains(s, want) {
			t.Errorf("rendered PDF text missing %q\n--- text dump ---\n%s", want, s)
		}
	}
	// Negative checks: emoji must be stripped, raw [[...]] must NOT appear.
	for _, banned := range []string{"📅", "⏫", "🔁", "⏰", "[[Important Note]]"} {
		if strings.Contains(s, banned) {
			t.Errorf("rendered PDF text still contains %q (preprocessing failed)", banned)
		}
	}
}
