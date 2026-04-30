package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// pdfRenderOptions controls how a note gets rendered to PDF via pandoc.
// Values are intentionally exposed instead of buried inside exportPDF so
// future callers (CLI subcommand, batch export, the publish pipeline)
// can override individual fields without re-creating the whole flag set.
type pdfRenderOptions struct {
	// PandocPath is the resolved pandoc binary location. Caller is
	// responsible for `exec.LookPath("pandoc")` so we can return a
	// friendly error before constructing arguments.
	PandocPath string

	// PDFEngine selects the LaTeX backend. "xelatex" is the default —
	// it handles Unicode (German umlauts, Greek letters in notes,
	// Polish diacritics) without the user needing to babysit
	// inputenc / fontenc settings the way pdflatex does.
	PDFEngine string

	// IncludeTOC adds a "Contents" page after the title for notes
	// long enough to benefit (>= 1500 words by default; the caller
	// can force on/off).
	IncludeTOC bool

	// Title overrides the document title. Empty falls back to the
	// note's frontmatter title or filename.
	Title string

	// Author appears under the title. Empty omits the line.
	Author string

	// Date appears next to the author. Empty falls back to today.
	Date string
}

// renderNoteToPDF converts a markdown note's body to a PDF using pandoc
// with a clean, opinionated set of typography flags. The output looks
// closer to a polished document than to default-pandoc — narrow margins,
// 11pt body, KP-Serif/Latin Modern by default (whichever xelatex picks),
// soft link colors, sober heading scale.
//
// Steps:
//  1. Strip frontmatter, capture title/author/date overrides
//  2. Pre-process the body: wikilinks → italicized targets, task-marker
//     emojis → ASCII fallbacks, granit-internal symbols stripped
//  3. Build a complete pandoc invocation (yaml metadata block + flags)
//  4. Shell out, surface stderr on failure
//
// The intermediate file is written to a temp path so the original .md
// is never modified, even if pandoc gets confused by our preprocessing.
func renderNoteToPDF(notePath, body, outPath string, opts pdfRenderOptions) error {
	if opts.PandocPath == "" {
		p, err := exec.LookPath("pandoc")
		if err != nil {
			return fmt.Errorf("pandoc is not installed or not in PATH")
		}
		opts.PandocPath = p
	}
	if opts.PDFEngine == "" {
		opts.PDFEngine = "xelatex"
	}

	// 1) Strip frontmatter so pandoc doesn't render it as a literal
	// block, and pull out title/author/date so we can emit a proper
	// pandoc title block ourselves with cleaner formatting.
	cleanBody, fm := splitFrontmatterForPDF(body)

	title := opts.Title
	if title == "" {
		if t, ok := fm["title"]; ok {
			title = t
		}
	}
	if title == "" {
		base := filepath.Base(notePath)
		title = strings.TrimSuffix(base, filepath.Ext(base))
	}
	author := opts.Author
	if author == "" {
		if a, ok := fm["author"]; ok {
			author = a
		}
	}
	date := opts.Date
	if date == "" {
		if d, ok := fm["date"]; ok {
			date = d
		}
	}

	// 2) Pre-process body: wikilinks to readable form, task-marker
	// emojis to ASCII fallbacks. Without this the wikilinks print
	// raw "[[Note]]" in the PDF and emoji print as missing-glyph
	// boxes since the default LaTeX fonts lack emoji coverage.
	cleanBody = preprocessForPDF(cleanBody)

	// 3) Decide whether to include a TOC.
	includeTOC := opts.IncludeTOC
	if !includeTOC && countH2Plus(cleanBody) >= 4 && wordCountForPDF(cleanBody) >= 1500 {
		// Auto-on for substantial notes — saves the user from
		// flipping a flag for the obvious case.
		includeTOC = true
	}

	// 4) Construct the input: a YAML metadata block (pandoc reads
	// these and emits a styled title block) followed by the body.
	var meta strings.Builder
	meta.WriteString("---\n")
	if title != "" {
		meta.WriteString("title: " + pdfYamlEscape(title) + "\n")
	}
	if author != "" {
		meta.WriteString("author: " + pdfYamlEscape(author) + "\n")
	}
	if date != "" {
		meta.WriteString("date: " + pdfYamlEscape(date) + "\n")
	}
	meta.WriteString("---\n\n")

	// Write to a temp file in the same directory as the output so
	// pandoc can resolve any future relative-path image references
	// the same way it would from the source location.
	tmp, err := os.CreateTemp(filepath.Dir(outPath), "granit-pdf-*.md")
	if err != nil {
		return fmt.Errorf("create temp markdown: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.WriteString(meta.String() + cleanBody); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp markdown: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp markdown: %w", err)
	}

	// 5) Build pandoc args. Variables override the default LaTeX
	// template — we don't ship a custom .tex template (keeps the
	// install footprint tiny) so the variables below are how we
	// achieve the "clean" look.
	args := []string{
		tmpPath, "-o", outPath,
		"--from=gfm+yaml_metadata_block",
		"--pdf-engine=" + opts.PDFEngine,
		"--standalone",
		// Margins: 2.2cm all-round is the print-design sweet spot
		// for A4 — narrower than pandoc's 1in default, wider than
		// "report" templates that go too tight.
		"-V", "geometry:margin=2.2cm",
		"-V", "geometry:a4paper",
		// Typography: 11pt body, 1.4 line stretch reads as the
		// "documentation-grade" rhythm without going essay-thick.
		"-V", "fontsize=11pt",
		"-V", "linestretch=1.35",
		// xelatex defaults to Latin Modern Roman + Latin Modern Mono,
		// both bundled with texlive-fontsrecommended. We deliberately
		// don't specify monofont here — fontspec errors hard if the
		// requested font isn't installed, and Latin Modern Mono is
		// the safe universal fallback. Users who want a specific
		// code font can drop --metadata-file with a custom monofont
		// override (documented in PDF export reference).
		// Heading scale + spacing — the default LaTeX article
		// class is verbose; KOMA-script "scrartcl" gives a more
		// modern look but needs koma-script package. Stick with
		// article for portability and tune the scale instead.
		"-V", "documentclass=article",
		// Colors: subtle teal links, no highlighting boxes. The
		// "blue!50!black" shade is dark enough for accessible
		// contrast on white paper while still reading as a link.
		"-V", "colorlinks=true",
		"-V", "linkcolor=blue!50!black",
		"-V", "urlcolor=blue!50!black",
		"-V", "citecolor=blue!50!black",
		"-V", "toccolor=black",
		// Section numbering off — most knowledge notes don't
		// benefit from "1.1.1.2" prefixes, and pandoc's default
		// outputs unnumbered headings cleanly.
		// Code highlighting style — "tango" is high-contrast
		// without screaming color, works well in print.
		"--highlight-style=tango",
	}
	if includeTOC {
		args = append(args, "--toc", "--toc-depth=3")
	}

	cmd := exec.Command(opts.PandocPath, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		// Surface a more actionable error when the user is missing
		// the LaTeX engine — pandoc's own message is a multi-line
		// dump; ours points at the fix.
		if strings.Contains(msg, "xelatex not found") || strings.Contains(msg, "pdflatex not found") {
			return fmt.Errorf("PDF engine %q not installed — install texlive-xetex (Arch) or basictex (mac) and retry", opts.PDFEngine)
		}
		return fmt.Errorf("pandoc: %s", msg)
	}
	return nil
}

// splitFrontmatterForPDF separates the YAML front matter from a markdown
// body and returns the body + a flat string→string map of fields. We
// only handle the simple `key: value` form; arrays and nested values
// fall through and stay in the body — that's safe because pandoc would
// also struggle with them.
func splitFrontmatterForPDF(body string) (string, map[string]string) {
	out := map[string]string{}
	if !strings.HasPrefix(body, "---\n") && !strings.HasPrefix(body, "---\r\n") {
		return body, out
	}
	rest := body[4:]
	if strings.HasPrefix(body, "---\r\n") {
		rest = body[5:]
	}
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return body, out
	}
	front := rest[:end]
	bodyOut := rest[end+4:]
	bodyOut = strings.TrimLeft(bodyOut, "\r\n")
	for _, line := range strings.Split(front, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, ":")
		if idx <= 0 {
			continue
		}
		k := strings.TrimSpace(line[:idx])
		v := strings.TrimSpace(line[idx+1:])
		// Strip surrounding quotes if present.
		if len(v) >= 2 {
			if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
				v = v[1 : len(v)-1]
			}
		}
		// Strip trailing inline comments after a "  #" run.
		// Without this, a frontmatter line like
		// `title: Notes  # private` would carry the comment text
		// into the PDF title block — visible in the rendered
		// output. Mirrors the same step in parseFlatFrontmatter.
		if hashIdx := strings.Index(v, "  #"); hashIdx >= 0 {
			v = strings.TrimSpace(v[:hashIdx])
		}
		out[k] = v
	}
	return bodyOut, out
}

// rePDFWikiLink + rePDFTaskMarkerEmoji — pre-compiled regexes used by
// preprocessForPDF. Kept module-scoped so the cost is amortized across
// repeated exports.
var (
	rePDFWikiLink         = regexp.MustCompile(`\[\[([^\]|]+)(\|([^\]]+))?\]\]`)
	rePDFCheckboxOff      = regexp.MustCompile(`(?m)^(\s*)- \[ \] `)
	rePDFCheckboxOn       = regexp.MustCompile(`(?m)^(\s*)- \[[xX]\] `)
	rePDFTaskMarkerEmoji  = regexp.MustCompile(`📅 *|🔺|⏫|🔼|🔽|⏬|🔁 *|🛫 *|⏳ *|✅ *|⏰ *|📆 *`)
	rePDFTrailingMetaTime = regexp.MustCompile(`\s*⏰\s*\d{1,2}:\d{2}(-\d{1,2}:\d{2})?`)
)

// preprocessForPDF transforms wiki-style and granit-internal markdown
// constructs into forms a generic pandoc → LaTeX pipeline can render
// cleanly. Specifically:
//
//   - [[Note Name]]            → *Note Name*       (italic for emphasis)
//   - [[Note|Display]]         → *Display*
//   - "- [ ] task"             → "- ☐ task"
//   - "- [x] task"             → "- ☑ task"
//   - 📅 / 🔼 / 🔁 etc.        → stripped (LaTeX default fonts have
//                                 no emoji glyphs; without this you
//                                 get missing-glyph boxes everywhere)
//
// Everything else passes through untouched so pandoc's GFM parser can
// handle tables, code blocks, fenced highlights, footnotes, etc.
func preprocessForPDF(body string) string {
	// Wikilinks first — italicize the display text. Without an HTML
	// target to link to, italic communicates "this references
	// something else" without the broken-link feel of "[[X]]" on a
	// printed page.
	body = rePDFWikiLink.ReplaceAllStringFunc(body, func(match string) string {
		sub := rePDFWikiLink.FindStringSubmatch(match)
		display := sub[1]
		if len(sub) >= 4 && sub[3] != "" {
			display = sub[3]
		}
		return "*" + strings.TrimSpace(display) + "*"
	})
	// Task checkboxes → Unicode glyphs. The boxes ☐ / ☑ are present
	// in DejaVu Sans Mono and most CJK-aware Latin fonts that
	// xelatex falls back to.
	body = rePDFCheckboxOff.ReplaceAllString(body, "$1- ☐ ")
	body = rePDFCheckboxOn.ReplaceAllString(body, "$1- ☑ ")
	// Strip task-marker emojis. Order matters: ⏰ HH:MM-HH:MM is a
	// composite that should be removed wholesale, not just the emoji.
	body = rePDFTrailingMetaTime.ReplaceAllString(body, "")
	body = rePDFTaskMarkerEmoji.ReplaceAllString(body, "")
	// Collapse runs of spaces only on the INTERIOR of each line —
	// leading whitespace carries list-indentation semantics in
	// markdown (two spaces in front of "-" means a nested list
	// item) and must be preserved.
	body = collapseInteriorSpaces(body)
	return body
}

// collapseInteriorSpaces walks each line, preserves leading whitespace
// verbatim, and squeezes runs of spaces in the rest of the line down to
// a single space. Used by preprocessForPDF after emoji stripping leaves
// double spaces between words. Splitting on lines avoids the previous
// regex-based collapse that flattened nested-list indentation.
func collapseInteriorSpaces(body string) string {
	var b strings.Builder
	lines := strings.Split(body, "\n")
	reInteriorRun := regexp.MustCompile(`  +`)
	for i, line := range lines {
		// Find the index where leading spaces end.
		j := 0
		for j < len(line) && (line[j] == ' ' || line[j] == '\t') {
			j++
		}
		b.WriteString(line[:j])
		b.WriteString(reInteriorRun.ReplaceAllString(line[j:], " "))
		if i < len(lines)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// pdfYamlEscape returns a YAML-safe string suitable for embedding in a
// `key: value` line. Wraps the value in single quotes and escapes
// embedded single quotes by doubling them — the pandoc YAML reader
// honours this without parsing it as a multiline string.
func pdfYamlEscape(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// countH2Plus counts the number of "##" or deeper headings in body —
// used as the heuristic for "is this note long enough to benefit from a
// table of contents?".
func countH2Plus(body string) int {
	count := 0
	inFence := false
	for _, line := range strings.Split(body, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if strings.HasPrefix(t, "## ") {
			count++
		}
	}
	return count
}

// wordCountForPDF approximates the body word count, used together with
// countH2Plus to decide whether to auto-include a TOC. Strips fenced
// code blocks first because those words inflate the count without
// telling us anything about reader-attention need.
func wordCountForPDF(body string) int {
	var b strings.Builder
	inFence := false
	for _, line := range strings.Split(body, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return len(strings.Fields(b.String()))
}
