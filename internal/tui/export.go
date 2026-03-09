package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type exportFormat struct {
	name string
	desc string
}

// ExportOverlay provides a note export menu supporting HTML, Plain Text, PDF,
// and full-vault HTML export.
type ExportOverlay struct {
	active    bool
	width     int
	height    int
	cursor    int
	formats   []exportFormat
	result    string
	exporting bool

	notePath    string
	noteContent string
	vaultRoot   string
}

func NewExportOverlay() ExportOverlay {
	return ExportOverlay{
		formats: []exportFormat{
			{name: "HTML", desc: "Convert to HTML file"},
			{name: "Plain Text", desc: "Strip formatting to .txt"},
			{name: "PDF", desc: "Export via pandoc (if available)"},
			{name: "Export All as HTML", desc: "Export entire vault with index"},
		},
	}
}

func (e *ExportOverlay) IsActive() bool {
	return e.active
}

func (e *ExportOverlay) Open(notePath, noteContent string, vaultRoot string) {
	e.active = true
	e.cursor = 0
	e.result = ""
	e.exporting = false
	e.notePath = notePath
	e.noteContent = noteContent
	e.vaultRoot = vaultRoot
}

func (e *ExportOverlay) Close() {
	e.active = false
}

func (e *ExportOverlay) SetSize(width, height int) {
	e.width = width
	e.height = height
}

func (e ExportOverlay) Update(msg tea.Msg) (ExportOverlay, tea.Cmd) {
	if !e.active {
		return e, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// If showing result, any key closes
		if e.result != "" {
			switch msg.String() {
			case "esc", "enter", "q":
				e.active = false
				e.result = ""
			}
			return e, nil
		}

		switch msg.String() {
		case "esc":
			e.active = false
			return e, nil
		case "up", "k":
			if e.cursor > 0 {
				e.cursor--
			}
			return e, nil
		case "down", "j":
			if e.cursor < len(e.formats)-1 {
				e.cursor++
			}
			return e, nil
		case "enter":
			if e.cursor >= 0 && e.cursor < len(e.formats) {
				e.exporting = true
				e.doExport()
				e.exporting = false
			}
			return e, nil
		}
	}
	return e, nil
}

func (e *ExportOverlay) doExport() {
	if e.cursor < 0 || e.cursor >= len(e.formats) {
		return
	}
	switch e.cursor {
	case 0: // HTML
		e.exportHTML()
	case 1: // Plain Text
		e.exportPlainText()
	case 2: // PDF
		e.exportPDF()
	case 3: // Export All as HTML
		e.exportAllHTML()
	}
}

func (e *ExportOverlay) exportHTML() {
	if e.notePath == "" {
		e.result = "Error: no note is open"
		return
	}
	html := markdownToHTML(e.noteContent)
	baseName := strings.TrimSuffix(e.notePath, filepath.Ext(e.notePath))
	outPath := filepath.Join(e.vaultRoot, baseName+".html")

	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		e.result = "Error: " + err.Error()
		return
	}

	title := strings.TrimSuffix(filepath.Base(e.notePath), filepath.Ext(e.notePath))
	fullHTML := wrapHTML(title, html)
	if err := os.WriteFile(outPath, []byte(fullHTML), 0644); err != nil {
		e.result = "Error: " + err.Error()
		return
	}
	e.result = "Exported: " + outPath
}

func (e *ExportOverlay) exportPlainText() {
	if e.notePath == "" {
		e.result = "Error: no note is open"
		return
	}
	plain := exportStripMarkdown(e.noteContent)
	baseName := strings.TrimSuffix(e.notePath, filepath.Ext(e.notePath))
	outPath := filepath.Join(e.vaultRoot, baseName+".txt")

	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		e.result = "Error: " + err.Error()
		return
	}

	if err := os.WriteFile(outPath, []byte(plain), 0644); err != nil {
		e.result = "Error: " + err.Error()
		return
	}
	e.result = "Exported: " + outPath
}

func (e *ExportOverlay) exportPDF() {
	if e.notePath == "" {
		e.result = "Error: no note is open"
		return
	}

	pandocPath, err := exec.LookPath("pandoc")
	if err != nil || pandocPath == "" {
		e.result = "Error: pandoc is not installed or not in PATH"
		return
	}

	inputPath := filepath.Join(e.vaultRoot, e.notePath)
	baseName := strings.TrimSuffix(e.notePath, filepath.Ext(e.notePath))
	outPath := filepath.Join(e.vaultRoot, baseName+".pdf")

	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		e.result = "Error: " + err.Error()
		return
	}

	cmd := exec.Command(pandocPath, inputPath, "-o", outPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		msg := err.Error()
		if len(output) > 0 {
			msg = strings.TrimSpace(string(output))
		}
		e.result = "Error: " + msg
		return
	}
	e.result = "Exported: " + outPath
}

func (e *ExportOverlay) exportAllHTML() {
	exportDir := filepath.Join(e.vaultRoot, "_export")
	if err := os.MkdirAll(exportDir, 0755); err != nil {
		e.result = "Error: " + err.Error()
		return
	}

	// Walk the vault for .md files
	var exported []string
	err := filepath.Walk(e.vaultRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			// Skip the _export directory itself and hidden directories
			name := info.Name()
			if name == "_export" || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".md") {
			return nil
		}

		relPath, _ := filepath.Rel(e.vaultRoot, path)
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}

		html := markdownToHTML(string(content))
		title := strings.TrimSuffix(filepath.Base(relPath), ".md")
		fullHTML := wrapHTML(title, html)

		outName := strings.TrimSuffix(relPath, ".md") + ".html"
		outPath := filepath.Join(exportDir, outName)
		if mkErr := os.MkdirAll(filepath.Dir(outPath), 0755); mkErr != nil {
			return nil
		}
		if wErr := os.WriteFile(outPath, []byte(fullHTML), 0644); wErr != nil {
			return nil
		}

		exported = append(exported, outName)
		return nil
	})

	if err != nil {
		e.result = "Error: " + err.Error()
		return
	}

	// Generate index.html
	var indexBody strings.Builder
	indexBody.WriteString("<h1>Vault Export</h1>\n<ul>\n")
	for _, name := range exported {
		displayName := strings.TrimSuffix(name, ".html")
		indexBody.WriteString(fmt.Sprintf("  <li><a href=\"%s\">%s</a></li>\n", name, displayName))
	}
	indexBody.WriteString("</ul>\n")

	indexHTML := wrapHTML("Vault Export", indexBody.String())
	indexPath := filepath.Join(exportDir, "index.html")
	if err := os.WriteFile(indexPath, []byte(indexHTML), 0644); err != nil {
		e.result = "Error: " + err.Error()
		return
	}

	e.result = fmt.Sprintf("Exported %d notes to %s", len(exported), exportDir)
}

func (e ExportOverlay) View() string {
	width := e.width / 2
	if width < 50 {
		width = 50
	}
	if width > 70 {
		width = 70
	}

	var b strings.Builder

	// Header
	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  " + IconSaveChar + " Export Note")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n\n")

	if e.exporting {
		b.WriteString(lipgloss.NewStyle().Foreground(yellow).Bold(true).Render("  Exporting..."))
	} else if e.result != "" {
		// Show result
		style := lipgloss.NewStyle().Foreground(green).Bold(true)
		if strings.HasPrefix(e.result, "Error:") {
			style = lipgloss.NewStyle().Foreground(red).Bold(true)
		}
		b.WriteString(style.Render("  " + e.result))
		b.WriteString("\n\n")
		b.WriteString(DimStyle.Render("  Press Esc/Enter to close"))
	} else {
		// Current note info
		if e.notePath != "" {
			noteLabel := lipgloss.NewStyle().Foreground(text).Render("  Note: ")
			noteName := lipgloss.NewStyle().Foreground(blue).Bold(true).Render(e.notePath)
			b.WriteString(noteLabel + noteName)
			b.WriteString("\n\n")
		}

		// Format menu
		for i, format := range e.formats {
			icon := lipgloss.NewStyle().Foreground(blue).Render(IconSaveChar) + " "

			if i == e.cursor {
				selected := lipgloss.NewStyle().
					Background(surface0).
					Foreground(peach).
					Bold(true)
				line := "  " + icon + format.name
				b.WriteString(selected.Width(width - 6).Render(line))
				b.WriteString("\n")
				desc := DimStyle.Render("    " + format.desc)
				b.WriteString(lipgloss.NewStyle().Background(surface0).Width(width - 6).Render(desc))
			} else {
				b.WriteString("  " + icon + NormalItemStyle.Render(format.name))
				b.WriteString("\n")
				b.WriteString(DimStyle.Render("    " + format.desc))
			}
			if i < len(e.formats)-1 {
				b.WriteString("\n")
			}
		}

		b.WriteString("\n\n")
		b.WriteString(DimStyle.Render("  j/k: navigate  Enter: export  Esc: close"))
	}

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(mauve).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

// ---------------------------------------------------------------------------
// Markdown to HTML converter
// ---------------------------------------------------------------------------

var (
	reHeading    = regexp.MustCompile(`^(#{1,6})\s+(.+)$`)
	reBold       = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reItalic     = regexp.MustCompile(`(?:^|[^*])\*([^*]+?)\*(?:[^*]|$)`)
	reInlineCode = regexp.MustCompile("`([^`]+)`")
	reWikiLink   = regexp.MustCompile(`\[\[([^\]]+)\]\]`)
	reMdLink     = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	reCheckboxOn = regexp.MustCompile(`^(\s*)- \[x\]\s+(.+)$`)
	reCheckboxOff = regexp.MustCompile(`^(\s*)- \[ \]\s+(.+)$`)
	reListItem   = regexp.MustCompile(`^(\s*)[-*+]\s+(.+)$`)
	reBlockquote = regexp.MustCompile(`^>\s*(.*)$`)
)

func markdownToHTML(md string) string {
	lines := strings.Split(md, "\n")
	var out strings.Builder
	inCodeBlock := false
	inList := false
	inBlockquote := false
	inFrontmatter := false
	frontmatterCount := 0

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// Handle YAML frontmatter (--- ... ---)
		if strings.TrimSpace(line) == "---" {
			frontmatterCount++
			if frontmatterCount == 1 && i == 0 {
				inFrontmatter = true
				continue
			}
			if inFrontmatter && frontmatterCount == 2 {
				inFrontmatter = false
				continue
			}
		}
		if inFrontmatter {
			continue
		}

		// Code blocks
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			if inCodeBlock {
				out.WriteString("</code></pre>\n")
				inCodeBlock = false
			} else {
				// Close any open list or blockquote
				if inList {
					out.WriteString("</ul>\n")
					inList = false
				}
				if inBlockquote {
					out.WriteString("</blockquote>\n")
					inBlockquote = false
				}
				lang := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "```"))
				if lang != "" {
					out.WriteString(fmt.Sprintf("<pre><code class=\"language-%s\">", lang))
				} else {
					out.WriteString("<pre><code>")
				}
				inCodeBlock = true
			}
			continue
		}
		if inCodeBlock {
			out.WriteString(htmlEscape(line))
			out.WriteString("\n")
			continue
		}

		trimmed := strings.TrimSpace(line)

		// Empty line ends lists and blockquotes
		if trimmed == "" {
			if inList {
				out.WriteString("</ul>\n")
				inList = false
			}
			if inBlockquote {
				out.WriteString("</blockquote>\n")
				inBlockquote = false
			}
			out.WriteString("\n")
			continue
		}

		// Headings
		if m := reHeading.FindStringSubmatch(line); m != nil {
			if inList {
				out.WriteString("</ul>\n")
				inList = false
			}
			if inBlockquote {
				out.WriteString("</blockquote>\n")
				inBlockquote = false
			}
			level := len(m[1])
			content := convertInline(m[2])
			out.WriteString(fmt.Sprintf("<h%d>%s</h%d>\n", level, content, level))
			continue
		}

		// Checkboxes (must check before generic list items)
		if m := reCheckboxOn.FindStringSubmatch(line); m != nil {
			if !inList {
				out.WriteString("<ul class=\"checklist\">\n")
				inList = true
			}
			out.WriteString(fmt.Sprintf("<li><input type=\"checkbox\" checked disabled> %s</li>\n", convertInline(m[2])))
			continue
		}
		if m := reCheckboxOff.FindStringSubmatch(line); m != nil {
			if !inList {
				out.WriteString("<ul class=\"checklist\">\n")
				inList = true
			}
			out.WriteString(fmt.Sprintf("<li><input type=\"checkbox\" disabled> %s</li>\n", convertInline(m[2])))
			continue
		}

		// List items
		if m := reListItem.FindStringSubmatch(line); m != nil {
			if !inList {
				out.WriteString("<ul>\n")
				inList = true
			}
			out.WriteString(fmt.Sprintf("<li>%s</li>\n", convertInline(m[2])))
			continue
		}

		// Blockquotes
		if m := reBlockquote.FindStringSubmatch(line); m != nil {
			if inList {
				out.WriteString("</ul>\n")
				inList = false
			}
			if !inBlockquote {
				out.WriteString("<blockquote>\n")
				inBlockquote = true
			}
			out.WriteString(convertInline(m[1]))
			out.WriteString("<br>\n")
			continue
		}

		// Close list/blockquote if a regular paragraph line is encountered
		if inList {
			out.WriteString("</ul>\n")
			inList = false
		}
		if inBlockquote {
			out.WriteString("</blockquote>\n")
			inBlockquote = false
		}

		// Regular paragraph line
		out.WriteString(fmt.Sprintf("<p>%s</p>\n", convertInline(trimmed)))
	}

	// Close unclosed blocks
	if inCodeBlock {
		out.WriteString("</code></pre>\n")
	}
	if inList {
		out.WriteString("</ul>\n")
	}
	if inBlockquote {
		out.WriteString("</blockquote>\n")
	}

	return out.String()
}

func convertInline(s string) string {
	s = htmlEscape(s)
	// Order matters: bold before italic
	s = reBold.ReplaceAllString(s, "<strong>$1</strong>")
	// For italic, we need a simpler approach after escaping
	s = regexp.MustCompile(`\*([^*]+?)\*`).ReplaceAllString(s, "<em>$1</em>")
	s = reInlineCode.ReplaceAllString(s, "<code>$1</code>")
	s = reWikiLink.ReplaceAllStringFunc(s, func(match string) string {
		inner := reWikiLink.FindStringSubmatch(match)
		if len(inner) < 2 {
			return match
		}
		linkTarget := inner[1]
		displayText := linkTarget
		// Handle [[target|display]] syntax
		if idx := strings.Index(linkTarget, "|"); idx >= 0 {
			displayText = linkTarget[idx+1:]
			linkTarget = linkTarget[:idx]
		}
		return fmt.Sprintf("<a href=\"%s.html\">%s</a>", linkTarget, displayText)
	})
	s = reMdLink.ReplaceAllString(s, "<a href=\"$2\">$1</a>")
	return s
}

func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

func wrapHTML(title, body string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>%s</title>
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; max-width: 800px; margin: 2rem auto; padding: 0 1rem; line-height: 1.6; color: #cdd6f4; background: #1e1e2e; }
    a { color: #89b4fa; }
    code { background: #313244; padding: 0.2em 0.4em; border-radius: 3px; font-size: 0.9em; }
    pre { background: #313244; padding: 1em; border-radius: 6px; overflow-x: auto; }
    pre code { background: none; padding: 0; }
    blockquote { border-left: 3px solid #cba6f7; margin-left: 0; padding-left: 1em; color: #a6adc8; }
    h1, h2, h3, h4, h5, h6 { color: #cba6f7; }
    ul.checklist { list-style: none; padding-left: 1.2em; }
    ul.checklist li { margin: 0.3em 0; }
    input[type="checkbox"] { margin-right: 0.5em; }
  </style>
</head>
<body>
%s
</body>
</html>`, htmlEscape(title), body)
}

// ---------------------------------------------------------------------------
// Markdown stripping for plain-text export
// ---------------------------------------------------------------------------

func exportStripMarkdown(md string) string {
	lines := strings.Split(md, "\n")
	var out strings.Builder
	inCodeBlock := false
	inFrontmatter := false
	frontmatterCount := 0

	for _, line := range lines {
		// Handle YAML frontmatter
		if strings.TrimSpace(line) == "---" {
			frontmatterCount++
			if frontmatterCount == 1 {
				inFrontmatter = true
				continue
			}
			if inFrontmatter && frontmatterCount == 2 {
				inFrontmatter = false
				continue
			}
		}
		if inFrontmatter {
			continue
		}

		// Code block fences
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inCodeBlock = !inCodeBlock
			continue
		}
		if inCodeBlock {
			out.WriteString(line)
			out.WriteString("\n")
			continue
		}

		// Strip heading markers
		if m := reHeading.FindStringSubmatch(line); m != nil {
			out.WriteString(m[2])
			out.WriteString("\n")
			continue
		}

		// Strip blockquote markers
		if m := reBlockquote.FindStringSubmatch(line); m != nil {
			out.WriteString(m[1])
			out.WriteString("\n")
			continue
		}

		// Strip checkbox markers
		if m := reCheckboxOn.FindStringSubmatch(line); m != nil {
			out.WriteString("[x] " + m[2])
			out.WriteString("\n")
			continue
		}
		if m := reCheckboxOff.FindStringSubmatch(line); m != nil {
			out.WriteString("[ ] " + m[2])
			out.WriteString("\n")
			continue
		}

		// Strip list markers but keep content
		if m := reListItem.FindStringSubmatch(line); m != nil {
			out.WriteString(m[1] + m[2])
			out.WriteString("\n")
			continue
		}

		// Strip inline formatting
		plain := line
		plain = reBold.ReplaceAllString(plain, "$1")
		plain = regexp.MustCompile(`\*([^*]+?)\*`).ReplaceAllString(plain, "$1")
		plain = reInlineCode.ReplaceAllString(plain, "$1")
		plain = reWikiLink.ReplaceAllStringFunc(plain, func(match string) string {
			inner := reWikiLink.FindStringSubmatch(match)
			if len(inner) < 2 {
				return match
			}
			linkTarget := inner[1]
			if idx := strings.Index(linkTarget, "|"); idx >= 0 {
				return linkTarget[idx+1:]
			}
			return linkTarget
		})
		plain = reMdLink.ReplaceAllString(plain, "$1")

		out.WriteString(plain)
		out.WriteString("\n")
	}

	return out.String()
}
