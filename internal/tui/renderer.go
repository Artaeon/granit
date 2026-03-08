package tui

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/vault"
)

type Renderer struct {
	width      int
	height     int
	noteLookup func(name string) string // returns content for a note name
	vaultNotes map[string]*vault.Note   // for dataview queries
	vaultRoot  string                   // root path of the vault for resolving images
}

// calloutInfo maps callout type keywords to their color and icon.
type calloutInfo struct {
	color lipgloss.Color
	icon  string
	label string
}

// calloutRe matches the opening line of a callout: > [!type] optional title
var calloutRe = regexp.MustCompile(`^>\s*\[!(\w+)\]\s*(.*)$`)

// embedRe matches note embedding syntax: ![[note-name]] or ![[note-name#heading]]
var embedRe = regexp.MustCompile(`!\[\[([^\]]+)\]\]`)

func NewRenderer() Renderer {
	return Renderer{}
}

func (r *Renderer) SetSize(width, height int) {
	r.width = width
	r.height = height
}

func (r *Renderer) SetNoteLookup(fn func(string) string) {
	r.noteLookup = fn
}

func (r *Renderer) SetVaultNotes(notes map[string]*vault.Note) {
	r.vaultNotes = notes
}

func (r *Renderer) SetVaultRoot(root string) {
	r.vaultRoot = root
}

// imgMarkdownImageRe matches standard markdown images: ![alt text](path/to/image.png)
var imgMarkdownImageRe = regexp.MustCompile(`^!\[([^\]]*)\]\(([^)]+)\)$`)

// imgExtensions is the set of file extensions recognized as images.
var imgExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
	".svg": true, ".webp": true, ".bmp": true,
}

// imgIsImageFile returns true if the filename has an image extension.
func imgIsImageFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return imgExtensions[ext]
}

// imgGetDimensions tries to read image dimensions from disk.
// Returns width, height, ok.
func imgGetDimensions(vaultRoot, filename string) (int, int, bool) {
	if vaultRoot == "" {
		return 0, 0, false
	}
	// Try the filename as-is (relative to vault root), then common subdirectories.
	candidates := []string{
		filepath.Join(vaultRoot, filename),
		filepath.Join(vaultRoot, "attachments", filename),
		filepath.Join(vaultRoot, "assets", filename),
		filepath.Join(vaultRoot, "images", filename),
	}
	for _, path := range candidates {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		cfg, _, err := image.DecodeConfig(f)
		f.Close()
		if err != nil {
			continue
		}
		return cfg.Width, cfg.Height, true
	}
	return 0, 0, false
}

// imgRenderPlaceholder renders an image inline.  When the terminal supports
// truecolor it first attempts to render actual pixel art using half-block
// characters (via renderImageTerminal in imageview.go).  If that fails or the
// terminal lacks truecolor, it falls back to a styled placeholder box showing
// the filename and dimensions.
func imgRenderPlaceholder(filename, altText, vaultRoot string, contentWidth int) []string {
	// Try terminal image rendering when supported.
	if isTerminalImageCapable() && vaultRoot != "" {
		absPath := resolveImagePath(vaultRoot, filename)
		if absPath != "" {
			maxW := contentWidth * 3 / 4
			if maxW < 10 {
				maxW = 10
			}
			if maxW > 120 {
				maxW = 120
			}
			maxH := 20 // terminal rows
			rendered, err := renderImageTerminal(absPath, maxW, maxH)
			if err == nil && rendered != "" {
				var out []string
				// Caption line above the image.
				captionStyle := lipgloss.NewStyle().Foreground(overlay0).Italic(true)
				caption := "  " + captionStyle.Render(filename)
				if altText != "" {
					caption += captionStyle.Render(" \u2014 " + altText)
				}
				out = append(out, caption)
				out = append(out, strings.Split(rendered, "\n")...)
				return out
			}
		}
	}

	// Fallback: styled placeholder box.
	var out []string

	boxWidth := contentWidth - 4
	if boxWidth < 20 {
		boxWidth = 20
	}

	borderStyle := lipgloss.NewStyle().Foreground(surface1)
	bgStyle := lipgloss.NewStyle().Background(surface0)
	fileStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(overlay0)

	// Top border
	topLine := "  " + borderStyle.Render("\u256D"+strings.Repeat("\u2500", boxWidth)+"\u256E")
	out = append(out, topLine)

	// Icon + filename line
	iconAndName := "  IMG  " + fileStyle.Render(filename)
	padded := bgStyle.Render(iconAndName)
	line1 := "  " + borderStyle.Render("\u2502") + " " + padded
	out = append(out, line1)

	// Alt text line (if provided)
	if altText != "" {
		altLine := "  " + borderStyle.Render("\u2502") + "  " + bgStyle.Render(dimStyle.Render("Alt: "+altText))
		out = append(out, altLine)
	}

	// Dimensions line
	w, h, ok := imgGetDimensions(vaultRoot, filename)
	var dimText string
	if ok {
		dimText = fmt.Sprintf("[Image: %dx%d]", w, h)
	} else {
		dimText = "[Image]"
	}
	dimLine := "  " + borderStyle.Render("\u2502") + "  " + bgStyle.Render(dimStyle.Render(dimText))
	out = append(out, dimLine)

	// Bottom border
	bottomLine := "  " + borderStyle.Render("\u2570"+strings.Repeat("\u2500", boxWidth)+"\u256F")
	out = append(out, bottomLine)

	return out
}

func (r Renderer) getCalloutInfo(typ string) calloutInfo {
	typ = strings.ToLower(typ)
	switch typ {
	case "note", "info":
		return calloutInfo{color: blue, icon: "\u2139", label: "Note"}
	case "tip", "hint":
		return calloutInfo{color: teal, icon: "\u2726", label: "Tip"}
	case "warning", "caution":
		return calloutInfo{color: yellow, icon: "\u26A0", label: "Warning"}
	case "danger", "error":
		return calloutInfo{color: red, icon: "\u2716", label: "Danger"}
	case "example":
		return calloutInfo{color: peach, icon: "\u25B8", label: "Example"}
	case "quote", "cite":
		return calloutInfo{color: overlay1, icon: "\u275D", label: "Quote"}
	case "success", "check":
		return calloutInfo{color: green, icon: "\u2713", label: "Success"}
	case "question", "faq":
		return calloutInfo{color: sapphire, icon: "?", label: "Question"}
	case "abstract", "summary", "tldr":
		return calloutInfo{color: teal, icon: "\u2261", label: "Abstract"}
	case "todo":
		return calloutInfo{color: yellow, icon: "\u25CB", label: "Todo"}
	case "bug":
		return calloutInfo{color: red, icon: "\u25C9", label: "Bug"}
	default:
		return calloutInfo{color: blue, icon: "\u2139", label: titleCase(typ)}
	}
}

// renderCalloutBlock renders a collected callout block (header line + content lines).
func (r Renderer) renderCalloutBlock(info calloutInfo, title string, contentLines []string, contentWidth int) []string {
	var out []string

	barStyle := lipgloss.NewStyle().Foreground(info.color)
	iconLabelStyle := lipgloss.NewStyle().Foreground(info.color).Bold(true)
	bgStyle := lipgloss.NewStyle().Background(surface0)

	// Header line: ┃ icon Label: Optional Title
	header := "  " + barStyle.Render("\u2503") + " " + iconLabelStyle.Render(info.icon+" "+info.label)
	if title != "" {
		header += lipgloss.NewStyle().Foreground(info.color).Bold(true).Render(": " + title)
	}
	out = append(out, bgStyle.Render(header))

	// Content lines
	for _, cl := range contentLines {
		line := "  " + barStyle.Render("\u2503") + "   " + lipgloss.NewStyle().Foreground(text).Render(cl)
		out = append(out, bgStyle.Render(line))
	}

	return out
}

// renderMathBlock renders a block math expression in a styled box.
func (r Renderer) renderMathBlock(mathLines []string, contentWidth int) []string {
	var out []string

	boxWidth := contentWidth - 4
	if boxWidth < 20 {
		boxWidth = 20
	}

	borderStyle := lipgloss.NewStyle().Foreground(teal)
	labelStyle := lipgloss.NewStyle().Foreground(teal).Italic(true)
	mathStyle := lipgloss.NewStyle().Foreground(teal).Italic(true)

	// Top border with "math" label
	label := " math "
	topPad := boxWidth - len([]rune(label)) - 2
	if topPad < 0 {
		topPad = 0
	}
	topLine := "  " + borderStyle.Render("╭"+label) + borderStyle.Render(strings.Repeat("─", topPad)+"╮")
	_ = labelStyle // label is rendered inline
	out = append(out, topLine)

	// Math content lines
	for _, ml := range mathLines {
		ml = strings.TrimSpace(ml)
		// Center the math content within the box
		padding := (boxWidth - len([]rune(ml)) - 4) / 2
		if padding < 0 {
			padding = 0
		}
		contentLine := "  " + borderStyle.Render("│") +
			strings.Repeat(" ", padding+1) +
			mathStyle.Render(ml) +
			strings.Repeat(" ", maxInt(1, boxWidth-len([]rune(ml))-padding-3)) +
			borderStyle.Render("│")
		out = append(out, contentLine)
	}

	// Bottom border
	bottomLine := "  " + borderStyle.Render("╰"+strings.Repeat("─", boxWidth)+"╯")
	out = append(out, bottomLine)

	return out
}

// parseTableCells splits a markdown table row into cells (trimming outer pipes).
func parseTableCells(row string) []string {
	row = strings.TrimSpace(row)
	if strings.HasPrefix(row, "|") {
		row = row[1:]
	}
	if strings.HasSuffix(row, "|") {
		row = row[:len(row)-1]
	}
	parts := strings.Split(row, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

// isSeparatorRow returns true if the row is a markdown table separator (e.g. |---|:---:|---:|).
func isSeparatorRow(cells []string) bool {
	for _, c := range cells {
		c = strings.TrimSpace(c)
		c = strings.TrimLeft(c, ":")
		c = strings.TrimRight(c, ":")
		if len(c) == 0 || strings.Trim(c, "-") != "" {
			return false
		}
	}
	return true
}

// tableAlignment returns 'l', 'c', or 'r' based on separator cell content.
func tableAlignment(cell string) byte {
	cell = strings.TrimSpace(cell)
	leftColon := strings.HasPrefix(cell, ":")
	rightColon := strings.HasSuffix(cell, ":")
	if leftColon && rightColon {
		return 'c'
	}
	if rightColon {
		return 'r'
	}
	return 'l'
}

// renderTable renders collected markdown table rows with box-drawing borders.
func (r Renderer) renderTable(rows []string, contentWidth int) []string {
	var out []string

	if len(rows) == 0 {
		return out
	}

	// Parse all rows into cells
	var allCells [][]string
	sepIdx := -1
	var alignments []byte

	for ri, row := range rows {
		cells := parseTableCells(row)
		if sepIdx < 0 && isSeparatorRow(cells) {
			sepIdx = ri
			// Extract alignments from separator
			for _, c := range cells {
				alignments = append(alignments, tableAlignment(c))
			}
			continue
		}
		allCells = append(allCells, cells)
	}

	if len(allCells) == 0 {
		return out
	}

	// Determine number of columns
	numCols := 0
	for _, cells := range allCells {
		if len(cells) > numCols {
			numCols = len(cells)
		}
	}
	if numCols == 0 {
		return out
	}

	// Pad alignments to match column count
	for len(alignments) < numCols {
		alignments = append(alignments, 'l')
	}

	// Calculate column widths (max content width per column)
	colWidths := make([]int, numCols)
	for _, cells := range allCells {
		for ci := 0; ci < numCols; ci++ {
			if ci < len(cells) {
				w := len([]rune(cells[ci]))
				if w > colWidths[ci] {
					colWidths[ci] = w
				}
			}
		}
	}
	// Ensure minimum column width
	for ci := range colWidths {
		if colWidths[ci] < 3 {
			colWidths[ci] = 3
		}
	}

	borderStyle := lipgloss.NewStyle().Foreground(surface1)
	headerStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	cellStyle := lipgloss.NewStyle().Foreground(text)
	altCellStyle := lipgloss.NewStyle().Foreground(text).Background(surface0)

	// Helper: align text in a cell
	alignCell := func(content string, width int, align byte) string {
		runes := []rune(content)
		contentLen := len(runes)
		if contentLen >= width {
			return string(runes[:width])
		}
		pad := width - contentLen
		switch align {
		case 'c':
			left := pad / 2
			right := pad - left
			return strings.Repeat(" ", left) + content + strings.Repeat(" ", right)
		case 'r':
			return strings.Repeat(" ", pad) + content
		default:
			return content + strings.Repeat(" ", pad)
		}
	}

	// Build horizontal border lines
	buildBorder := func(left, mid, right, fill string) string {
		var sb strings.Builder
		sb.WriteString("  ")
		sb.WriteString(left)
		for ci := 0; ci < numCols; ci++ {
			sb.WriteString(strings.Repeat(fill, colWidths[ci]+2))
			if ci < numCols-1 {
				sb.WriteString(mid)
			}
		}
		sb.WriteString(right)
		return borderStyle.Render(sb.String())
	}

	// Top border: ┌─┬─┐
	out = append(out, buildBorder("┌", "┬", "┐", "─"))

	for ri, cells := range allCells {
		// Pad cells to numCols
		for len(cells) < numCols {
			cells = append(cells, "")
		}

		var sb strings.Builder
		sb.WriteString("  ")
		sb.WriteString(borderStyle.Render("│"))
		for ci := 0; ci < numCols; ci++ {
			content := alignCell(cells[ci], colWidths[ci], alignments[ci])
			if ri == 0 && sepIdx >= 0 {
				// Header row
				sb.WriteString(" " + headerStyle.Render(content) + " ")
			} else if ri%2 == 0 {
				sb.WriteString(" " + altCellStyle.Render(content) + " ")
			} else {
				sb.WriteString(" " + cellStyle.Render(content) + " ")
			}
			if ci < numCols-1 {
				sb.WriteString(borderStyle.Render("│"))
			}
		}
		sb.WriteString(borderStyle.Render("│"))
		out = append(out, sb.String())

		// After header row, draw a thick separator: ├═┼═┤
		if ri == 0 && sepIdx >= 0 {
			out = append(out, buildBorder("├", "┼", "┤", "═"))
		}
	}

	// Bottom border: └─┴─┘
	out = append(out, buildBorder("└", "┴", "┘", "─"))

	return out
}

// renderEmbed renders the embedded note preview box.
func (r Renderer) renderEmbed(noteName string, heading string, contentWidth int) []string {
	var out []string

	boxWidth := contentWidth - 4
	if boxWidth < 20 {
		boxWidth = 20
	}

	label := noteName
	if heading != "" {
		label += " > " + heading
	}

	topLabel := "\u2500 \U0001F4CE Embedded: " + label + " "
	topPad := boxWidth - len([]rune(topLabel)) - 2
	if topPad < 0 {
		topPad = 0
	}
	topLine := "  \u256D" + topLabel + strings.Repeat("\u2500", topPad) + "\u256E"

	dimStyle := lipgloss.NewStyle().Foreground(overlay0)
	embedBorderStyle := lipgloss.NewStyle().Foreground(sapphire)

	out = append(out, embedBorderStyle.Render(topLine))

	if r.noteLookup == nil {
		contentLine := "  \u2502  " + dimStyle.Render("[note lookup not available]")
		pad := boxWidth - len([]rune("[note lookup not available]")) - 4
		if pad < 0 {
			pad = 0
		}
		contentLine += strings.Repeat(" ", pad) + embedBorderStyle.Render("\u2502")
		out = append(out, contentLine)
	} else {
		noteContent := r.noteLookup(noteName)
		if noteContent == "" {
			contentLine := "  " + embedBorderStyle.Render("\u2502") + "  " + dimStyle.Render("[note not found]")
			out = append(out, contentLine+embedBorderStyle.Render("  \u2502"))
		} else {
			// If heading is specified, find the section
			if heading != "" {
				noteContent = extractHeadingSection(noteContent, heading)
			}

			noteLines := strings.Split(noteContent, "\n")
			// Strip frontmatter from embedded content
			if len(noteLines) > 0 && strings.TrimSpace(noteLines[0]) == "---" {
				for fi := 1; fi < len(noteLines); fi++ {
					if strings.TrimSpace(noteLines[fi]) == "---" {
						noteLines = noteLines[fi+1:]
						break
					}
				}
			}
			// Trim leading blank lines
			for len(noteLines) > 0 && strings.TrimSpace(noteLines[0]) == "" {
				noteLines = noteLines[1:]
			}
			maxLines := 12
			truncated := len(noteLines) > maxLines
			if truncated {
				noteLines = noteLines[:maxLines]
			}
			maxChars := boxWidth - 6
			if maxChars < 10 {
				maxChars = 10
			}
			for _, nl := range noteLines {
				nl = strings.TrimSpace(nl)
				if nl == "" {
					nl = " "
				}
				// Render basic markdown in embedded content
				styled := r.renderEmbedLine(nl, maxChars)
				contentLine := "  " + embedBorderStyle.Render("\u2502") + "  " + styled
				out = append(out, contentLine)
			}
			if truncated {
				moreMsg := dimStyle.Render("... more lines omitted")
				out = append(out, "  "+embedBorderStyle.Render("\u2502")+"  "+moreMsg)
			}
		}
	}

	bottomLine := "  \u2570" + strings.Repeat("\u2500", boxWidth) + "\u256F"
	out = append(out, embedBorderStyle.Render(bottomLine))

	return out
}

// renderEmbedLine renders a single line within an embedded note with basic markdown formatting.
func (r Renderer) renderEmbedLine(line string, maxChars int) string {
	// Truncate
	runes := []rune(line)
	if len(runes) > maxChars {
		line = string(runes[:maxChars]) + "..."
	}

	// Headings
	if strings.HasPrefix(line, "# ") {
		return lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(line)
	}
	if strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "### ") {
		return lipgloss.NewStyle().Foreground(mauve).Render(line)
	}

	// List items
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
		return lipgloss.NewStyle().Foreground(text).Render(line)
	}

	// Checkboxes
	if strings.HasPrefix(line, "- [ ] ") {
		return lipgloss.NewStyle().Foreground(yellow).Bold(true).Render("☐ ") +
			lipgloss.NewStyle().Foreground(text).Render(line[6:])
	}
	if strings.HasPrefix(line, "- [x] ") {
		return lipgloss.NewStyle().Foreground(green).Bold(true).Render("☑ ") +
			lipgloss.NewStyle().Foreground(overlay0).Strikethrough(true).Render(line[6:])
	}

	return lipgloss.NewStyle().Foreground(text).Render(line)
}

// extractHeadingSection returns the content under a given heading until the next heading of same or higher level.
func extractHeadingSection(content string, heading string) string {
	lines := strings.Split(content, "\n")
	heading = strings.ToLower(strings.TrimSpace(heading))
	inSection := false
	sectionLevel := 0
	var sectionLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inSection {
			// Check if this line is the target heading
			hLevel, hText := parseHeading(trimmed)
			if hLevel > 0 && strings.ToLower(hText) == heading {
				inSection = true
				sectionLevel = hLevel
				continue
			}
		} else {
			// Check if we hit another heading of same or higher level
			hLevel, _ := parseHeading(strings.TrimSpace(line))
			if hLevel > 0 && hLevel <= sectionLevel {
				break
			}
			sectionLines = append(sectionLines, line)
		}
	}

	if len(sectionLines) == 0 {
		return ""
	}
	return strings.Join(sectionLines, "\n")
}

// parseHeading returns the heading level and text, or 0 if not a heading.
func parseHeading(line string) (int, string) {
	if strings.HasPrefix(line, "#### ") {
		return 4, strings.TrimPrefix(line, "#### ")
	}
	if strings.HasPrefix(line, "### ") {
		return 3, strings.TrimPrefix(line, "### ")
	}
	if strings.HasPrefix(line, "## ") {
		return 2, strings.TrimPrefix(line, "## ")
	}
	if strings.HasPrefix(line, "# ") {
		return 1, strings.TrimPrefix(line, "# ")
	}
	return 0, ""
}

func (r Renderer) Render(content string, scroll int) string {
	lines := r.renderMarkdown(content)

	// Apply scroll
	if scroll >= len(lines) {
		scroll = maxInt(0, len(lines)-1)
	}

	visibleHeight := r.height - 4
	if visibleHeight < 1 {
		visibleHeight = 1
	}

	end := scroll + visibleHeight
	if end > len(lines) {
		end = len(lines)
	}

	visible := lines[scroll:end]
	return strings.Join(visible, "\n")
}

func (r Renderer) RenderLineCount(content string) int {
	return len(r.renderMarkdown(content))
}

func (r Renderer) renderMarkdown(content string) []string {
	var result []string
	contentWidth := r.width - 6
	if contentWidth < 20 {
		contentWidth = 20
	}

	lines := strings.Split(content, "\n")
	inFrontmatter := false
	fmDone := false
	inCodeBlock := false
	codeBlockLang := ""

	// Footnote definitions collected during rendering
	var footnoteOrder []string
	footnoteMap := map[string]string{}

	// First pass: collect frontmatter
	var fmLines []string
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		inFrontmatter = true
	}

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Frontmatter handling
		if i == 0 && trimmed == "---" {
			inFrontmatter = true
			continue
		}
		if inFrontmatter && !fmDone {
			if trimmed == "---" {
				fmDone = true
				inFrontmatter = false
				// Render collected frontmatter as a styled block
				if len(fmLines) > 0 {
					fmBorder := lipgloss.NewStyle().
						Foreground(overlay0).
						Render("  ┌" + strings.Repeat("─", contentWidth-4) + "┐")
					result = append(result, fmBorder)
					for _, fl := range fmLines {
						parts := strings.SplitN(fl, ":", 2)
						if len(parts) == 2 {
							key := lipgloss.NewStyle().Foreground(blue).Bold(true).Render(strings.TrimSpace(parts[0]))
							val := lipgloss.NewStyle().Foreground(text).Render(strings.TrimSpace(parts[1]))
							fmLine := "  │ " + key + ": " + val
							result = append(result, fmLine)
						}
					}
					fmBorderBottom := lipgloss.NewStyle().
						Foreground(overlay0).
						Render("  └" + strings.Repeat("─", contentWidth-4) + "┘")
					result = append(result, fmBorderBottom)
					result = append(result, "")
				}
				continue
			}
			fmLines = append(fmLines, line)
			continue
		}

		// Code blocks
		if strings.HasPrefix(trimmed, "```") {
			if !inCodeBlock {
				inCodeBlock = true
				codeBlockLang = strings.TrimPrefix(trimmed, "```")

				// Dataview query blocks: collect content and render inline
				if codeBlockLang == "query" && r.vaultNotes != nil {
					var queryLines []string
					for i++; i < len(lines); i++ {
						if strings.TrimSpace(lines[i]) == "```" {
							break
						}
						queryLines = append(queryLines, lines[i])
					}
					queryBlock := strings.Join(queryLines, "\n")
					query := ParseDataviewQuery(queryBlock)
					if query != nil {
						results := ExecuteDataviewQuery(query, r.vaultNotes)
						rendered := RenderDataviewResults(results, query.Fields, contentWidth)
						for _, rl := range strings.Split(rendered, "\n") {
							result = append(result, rl)
						}
					}
					inCodeBlock = false
					codeBlockLang = ""
					continue
				}

				// Mermaid diagram blocks: render as ASCII art
				if codeBlockLang == "mermaid" {
					var mermaidLines []string
					for i++; i < len(lines); i++ {
						if strings.TrimSpace(lines[i]) == "```" {
							break
						}
						mermaidLines = append(mermaidLines, lines[i])
					}
					mermaidSrc := strings.Join(mermaidLines, "\n")
					rendered := RenderMermaidASCII(mermaidSrc, contentWidth)
					result = append(result, rendered...)
					inCodeBlock = false
					codeBlockLang = ""
					continue
				}

				// Custom diagram blocks: render via diagram engine
				if codeBlockLang == "diagram" {
					var diagramLines []string
					for i++; i < len(lines); i++ {
						if strings.TrimSpace(lines[i]) == "```" {
							break
						}
						diagramLines = append(diagramLines, lines[i])
					}
					diagramSrc := strings.Join(diagramLines, "\n")
					rendered := RenderDiagramASCII(diagramSrc, contentWidth)
					result = append(result, rendered...)
					inCodeBlock = false
					codeBlockLang = ""
					continue
				}

				if codeBlockLang != "" {
					langLabel := lipgloss.NewStyle().
						Foreground(overlay0).
						Italic(true).
						Render("  " + codeBlockLang)
					result = append(result, langLabel)
				}
				codeBorder := lipgloss.NewStyle().
					Foreground(surface1).
					Render("  " + strings.Repeat("─", contentWidth-4))
				result = append(result, codeBorder)
				continue
			} else {
				inCodeBlock = false
				codeBlockLang = ""
				codeBorder := lipgloss.NewStyle().
					Foreground(surface1).
					Render("  " + strings.Repeat("─", contentWidth-4))
				result = append(result, codeBorder)
				continue
			}
		}

		if inCodeBlock {
			codeLine := lipgloss.NewStyle().
				Foreground(green).
				Render("    " + line)
			result = append(result, codeLine)
			continue
		}

		// Empty line
		if trimmed == "" {
			result = append(result, "")
			continue
		}

		// Block math $$...$$
		if strings.HasPrefix(trimmed, "$$") {
			// Check if it's a single-line block math: $$...$$ on one line
			if strings.HasSuffix(trimmed, "$$") && len(trimmed) > 4 {
				mathContent := trimmed[2 : len(trimmed)-2]
				mathLines := r.renderMathBlock([]string{mathContent}, contentWidth)
				result = append(result, mathLines...)
				continue
			}
			// Multi-line block math: collect until closing $$
			var mathContentLines []string
			for i+1 < len(lines) {
				i++
				nextTrimmed := strings.TrimSpace(lines[i])
				if nextTrimmed == "$$" || strings.HasSuffix(nextTrimmed, "$$") {
					if nextTrimmed != "$$" {
						mathContentLines = append(mathContentLines, strings.TrimSuffix(nextTrimmed, "$$"))
					}
					break
				}
				mathContentLines = append(mathContentLines, lines[i])
			}
			mathLines := r.renderMathBlock(mathContentLines, contentWidth)
			result = append(result, mathLines...)
			continue
		}

		// Horizontal rule
		if trimmed == "---" || trimmed == "***" || trimmed == "___" {
			hrChar := "─"
			if trimmed == "***" {
				hrChar = "═"
			} else if trimmed == "___" {
				hrChar = "━"
			}
			leftStyle := lipgloss.NewStyle().Foreground(surface1)
			accentStyle := lipgloss.NewStyle().Foreground(overlay0)
			diamond := accentStyle.Render(" ◆ ")
			halfWidth := (contentWidth - 8) / 2
			if halfWidth < 2 {
				halfWidth = 2
			}
			rule := "  " + leftStyle.Render(strings.Repeat(hrChar, halfWidth)) + diamond + leftStyle.Render(strings.Repeat(hrChar, halfWidth))
			result = append(result, rule)
			continue
		}

		// Headings
		if strings.HasPrefix(trimmed, "# ") {
			hText := strings.TrimPrefix(trimmed, "# ")
			upper := strings.ToUpper(hText)
			// Full-width background bar for H1
			barStyle := lipgloss.NewStyle().
				Foreground(crust).
				Background(mauve).
				Bold(true).
				Width(contentWidth).
				Padding(0, 2)
			underline := lipgloss.NewStyle().
				Foreground(mauve).
				Render("  " + strings.Repeat("━", contentWidth-4))
			result = append(result, "")
			result = append(result, "")
			result = append(result, barStyle.Render(upper))
			result = append(result, underline)
			result = append(result, "")
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			hText := strings.TrimPrefix(trimmed, "## ")
			// Accent left border + bold text for H2
			bar := lipgloss.NewStyle().Foreground(blue).Bold(true).Render("┃ ")
			styled := lipgloss.NewStyle().
				Foreground(blue).
				Bold(true).
				Render(hText)
			underline := lipgloss.NewStyle().
				Foreground(surface1).
				Render("  " + strings.Repeat("─", contentWidth-4))
			result = append(result, "")
			result = append(result, "  "+bar+styled)
			result = append(result, underline)
			result = append(result, "")
			continue
		}
		if strings.HasPrefix(trimmed, "### ") {
			hText := strings.TrimPrefix(trimmed, "### ")
			bar := lipgloss.NewStyle().Foreground(sapphire).Render("│ ")
			styled := lipgloss.NewStyle().
				Foreground(sapphire).
				Bold(true).
				Render(hText)
			result = append(result, "")
			result = append(result, "  "+bar+styled)
			continue
		}
		if strings.HasPrefix(trimmed, "#### ") {
			hText := strings.TrimPrefix(trimmed, "#### ")
			styled := lipgloss.NewStyle().
				Foreground(teal).
				Bold(true).
				Italic(true).
				Render("  " + hText)
			result = append(result, "")
			result = append(result, styled)
			continue
		}

		// Callout detection — must come before generic blockquote handling
		if calloutRe.MatchString(trimmed) {
			matches := calloutRe.FindStringSubmatch(trimmed)
			calloutType := matches[1]
			calloutTitle := strings.TrimSpace(matches[2])
			info := r.getCalloutInfo(calloutType)

			// Collect content lines (subsequent lines starting with "> ")
			var calloutContent []string
			for i+1 < len(lines) {
				nextTrimmed := strings.TrimSpace(lines[i+1])
				if strings.HasPrefix(nextTrimmed, "> ") {
					calloutContent = append(calloutContent, strings.TrimPrefix(nextTrimmed, "> "))
					i++
				} else if nextTrimmed == ">" {
					calloutContent = append(calloutContent, "")
					i++
				} else {
					break
				}
			}

			rendered := r.renderCalloutBlock(info, calloutTitle, calloutContent, contentWidth)
			result = append(result, rendered...)
			continue
		}

		// Blockquote (with nesting support)
		if strings.HasPrefix(trimmed, ">") {
			// Count nesting depth and extract text
			depth := 0
			remainder := trimmed
			for strings.HasPrefix(remainder, ">") {
				depth++
				remainder = strings.TrimPrefix(remainder, ">")
				remainder = strings.TrimPrefix(remainder, " ")
			}

			// Colors per nesting depth level
			quoteColors := []lipgloss.Color{mauve, blue, teal, peach, pink}
			textColors := []lipgloss.Color{overlay1, subtext0, subtext1, overlay2, overlay0}

			var bar string
			bar = "  "
			for d := 0; d < depth; d++ {
				colorIdx := d % len(quoteColors)
				barChar := "┃"
				if d > 0 {
					barChar = "│"
				}
				bar += lipgloss.NewStyle().Foreground(quoteColors[colorIdx]).Render(barChar) + " "
			}

			textColorIdx := (depth - 1) % len(textColors)
			quote := lipgloss.NewStyle().Foreground(textColors[textColorIdx]).Italic(true).Render(remainder)
			result = append(result, bar+quote)
			continue
		}

		// Checkboxes with Unicode styling
		if strings.HasPrefix(trimmed, "- [x] ") || strings.HasPrefix(trimmed, "- [X] ") {
			doneText := trimmed[6:]
			checkbox := lipgloss.NewStyle().Foreground(crust).Background(green).Bold(true).Render(" ☑ ")
			styledText := lipgloss.NewStyle().Foreground(overlay0).Strikethrough(true).Render(" " + doneText)
			result = append(result, "  "+checkbox+styledText)
			continue
		}
		if strings.HasPrefix(trimmed, "- [ ] ") {
			todoText := trimmed[6:]
			checkbox := lipgloss.NewStyle().Foreground(yellow).Bold(true).Render(" ☐ ")
			styledText := lipgloss.NewStyle().Foreground(text).Render(" " + todoText)
			result = append(result, "  "+checkbox+styledText)
			continue
		}

		// Unordered list
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			text := trimmed[2:]
			indent := strings.Repeat(" ", len(line)-len(trimmed))
			bullet := lipgloss.NewStyle().Foreground(peach).Render("  " + indent + "● ")
			result = append(result, bullet+r.renderInline(text))
			continue
		}

		// Numbered list
		isNumbered := false
		for idx, ch := range trimmed {
			if ch == '.' && idx > 0 && idx < 4 {
				if idx+1 < len(trimmed) && trimmed[idx+1] == ' ' {
					allDigits := true
					for j := 0; j < idx; j++ {
						if trimmed[j] < '0' || trimmed[j] > '9' {
							allDigits = false
							break
						}
					}
					if allDigits {
						num := trimmed[:idx]
						text := trimmed[idx+2:]
						numStyled := lipgloss.NewStyle().Foreground(peach).Bold(true).Render("  " + num + ". ")
						result = append(result, numStyled+r.renderInline(text))
						isNumbered = true
					}
				}
				break
			}
			if ch < '0' || ch > '9' {
				break
			}
		}
		if isNumbered {
			continue
		}

		// Table detection — collect all consecutive table rows and render as a box table
		if strings.Contains(trimmed, "|") && strings.Count(trimmed, "|") >= 2 {
			var tableRows []string
			tableRows = append(tableRows, trimmed)
			for i+1 < len(lines) {
				nextTrimmed := strings.TrimSpace(lines[i+1])
				if strings.Contains(nextTrimmed, "|") && strings.Count(nextTrimmed, "|") >= 2 {
					tableRows = append(tableRows, nextTrimmed)
					i++
				} else {
					break
				}
			}
			rendered := r.renderTable(tableRows, contentWidth)
			result = append(result, rendered...)
			continue
		}

		// Standard markdown image: ![alt text](path/to/image.png)
		if imgMatch := imgMarkdownImageRe.FindStringSubmatch(trimmed); imgMatch != nil {
			imgAlt := imgMatch[1]
			imgPath := imgMatch[2]
			if imgIsImageFile(imgPath) {
				imgName := filepath.Base(imgPath)
				placeholder := imgRenderPlaceholder(imgName, imgAlt, r.vaultRoot, contentWidth)
				result = append(result, placeholder...)
				continue
			}
		}

		// Note/image embedding detection: line contains ![[...]]
		if embedRe.MatchString(trimmed) {
			// Check if the entire line is just the embed
			fullMatch := embedRe.FindString(trimmed)
			if fullMatch == trimmed {
				// Standalone embed line — render as embedded box
				inner := embedRe.FindStringSubmatch(trimmed)
				ref := inner[1]
				noteName := ref
				heading := ""
				if hashIdx := strings.Index(ref, "#"); hashIdx >= 0 {
					noteName = ref[:hashIdx]
					heading = ref[hashIdx+1:]
				}
				// If it's an image file, render image placeholder instead of note embed
				if imgIsImageFile(noteName) {
					imgName := filepath.Base(noteName)
					placeholder := imgRenderPlaceholder(imgName, "", r.vaultRoot, contentWidth)
					result = append(result, placeholder...)
					continue
				}
				embedLines := r.renderEmbed(noteName, heading, contentWidth)
				result = append(result, embedLines...)
				continue
			}
			// Mixed line: render inline with embed replaced by a styled link
			result = append(result, "  "+r.renderInline(trimmed))
			continue
		}

		// Footnote definition: [^id]: text
		if strings.HasPrefix(trimmed, "[^") {
			closeBracket := strings.Index(trimmed, "]:")
			if closeBracket > 2 {
				fnID := trimmed[2:closeBracket]
				fnText := strings.TrimSpace(trimmed[closeBracket+2:])
				if _, exists := footnoteMap[fnID]; !exists {
					footnoteOrder = append(footnoteOrder, fnID)
				}
				footnoteMap[fnID] = fnText
				continue
			}
		}

		// Normal paragraph
		result = append(result, "  "+r.renderInline(trimmed))
	}

	// Render collected footnotes at the bottom
	if len(footnoteOrder) > 0 {
		result = append(result, "")
		borderStyle := lipgloss.NewStyle().Foreground(surface1)
		result = append(result, borderStyle.Render("  "+strings.Repeat("─", contentWidth-4)))
		labelStyle := lipgloss.NewStyle().Foreground(overlay0).Italic(true)
		result = append(result, "  "+labelStyle.Render("Footnotes"))
		result = append(result, "")

		fnIDStyle := lipgloss.NewStyle().Foreground(sapphire).Bold(true)
		fnTextStyle := lipgloss.NewStyle().Foreground(subtext0)
		for _, fnID := range footnoteOrder {
			fnText := footnoteMap[fnID]
			line := "  " + fnIDStyle.Render("["+fnID+"]") + " " + fnTextStyle.Render(fnText)
			result = append(result, line)
		}
	}

	return result
}

func (r Renderer) renderInline(input string) string {
	if input == "" {
		return ""
	}

	var result strings.Builder
	runes := []rune(input)
	n := len(runes)
	i := 0

	for i < n {
		// Embed links ![[...]] rendered inline as styled reference
		if runes[i] == '!' && i+2 < n && runes[i+1] == '[' && runes[i+2] == '[' {
			end := -1
			for j := i + 3; j+1 < n; j++ {
				if runes[j] == ']' && runes[j+1] == ']' {
					end = j + 1
					break
				}
			}
			if end != -1 {
				ref := string(runes[i+3 : end-1])
				displayName := ref
				if hashIdx := strings.Index(ref, "#"); hashIdx >= 0 {
					displayName = ref[:hashIdx] + " > " + ref[hashIdx+1:]
				}
				styled := lipgloss.NewStyle().
					Foreground(sapphire).
					Italic(true).
					Render("\U0001F4CE " + displayName)
				result.WriteString(styled)
				i = end + 1
				continue
			}
		}

		// WikiLinks [[...]]
		if i+1 < n && runes[i] == '[' && runes[i+1] == '[' {
			end := -1
			for j := i + 2; j+1 < n; j++ {
				if runes[j] == ']' && runes[j+1] == ']' {
					end = j + 1
					break
				}
			}
			if end != -1 {
				linkContent := string(runes[i+2 : end-1])
				// Handle aliases [[target|display]]
				displayName := linkContent
				if pipeIdx := strings.Index(linkContent, "|"); pipeIdx >= 0 {
					displayName = linkContent[pipeIdx+1:]
				}
				styled := lipgloss.NewStyle().
					Foreground(blue).
					Underline(true).
					Render(displayName)
				result.WriteString(styled)
				i = end + 1
				continue
			}
		}

		// Inline code `...`
		if runes[i] == '`' {
			end := -1
			for j := i + 1; j < n; j++ {
				if runes[j] == '`' {
					end = j
					break
				}
			}
			if end != -1 {
				code := string(runes[i+1 : end])
				styled := lipgloss.NewStyle().
					Foreground(green).
					Background(surface0).
					Render(" " + code + " ")
				result.WriteString(styled)
				i = end + 1
				continue
			}
		}

		// Bold **...**
		if i+1 < n && runes[i] == '*' && runes[i+1] == '*' {
			end := -1
			for j := i + 2; j+1 < n; j++ {
				if runes[j] == '*' && runes[j+1] == '*' {
					end = j + 1
					break
				}
			}
			if end != -1 {
				bold := string(runes[i+2 : end-1])
				styled := lipgloss.NewStyle().
					Foreground(text).
					Bold(true).
					Render(bold)
				result.WriteString(styled)
				i = end + 1
				continue
			}
		}

		// Strikethrough ~~...~~
		if i+1 < n && runes[i] == '~' && runes[i+1] == '~' {
			end := -1
			for j := i + 2; j+1 < n; j++ {
				if runes[j] == '~' && runes[j+1] == '~' {
					end = j + 1
					break
				}
			}
			if end != -1 {
				struck := string(runes[i+2 : end-1])
				styled := lipgloss.NewStyle().
					Foreground(overlay0).
					Strikethrough(true).
					Render(struck)
				result.WriteString(styled)
				i = end + 1
				continue
			}
		}

		// Highlight ==...==
		if i+1 < n && runes[i] == '=' && runes[i+1] == '=' {
			end := -1
			for j := i + 2; j+1 < n; j++ {
				if runes[j] == '=' && runes[j+1] == '=' {
					end = j + 1
					break
				}
			}
			if end != -1 {
				highlighted := string(runes[i+2 : end-1])
				styled := lipgloss.NewStyle().
					Foreground(crust).
					Background(yellow).
					Bold(true).
					Render(highlighted)
				result.WriteString(styled)
				i = end + 1
				continue
			}
		}

		// Inline math $...$
		if runes[i] == '$' && (i+1 < n && runes[i+1] != '$') {
			end := -1
			for j := i + 1; j < n; j++ {
				if runes[j] == '$' {
					end = j
					break
				}
			}
			if end != -1 && end > i+1 {
				math := string(runes[i+1 : end])
				styled := lipgloss.NewStyle().
					Foreground(teal).
					Italic(true).
					Render("$" + math + "$")
				result.WriteString(styled)
				i = end + 1
				continue
			}
		}

		// Footnote references [^n]
		if runes[i] == '[' && i+2 < n && runes[i+1] == '^' {
			end := -1
			for j := i + 2; j < n; j++ {
				if runes[j] == ']' {
					end = j
					break
				}
			}
			if end != -1 {
				fnID := string(runes[i+2 : end])
				styled := lipgloss.NewStyle().
					Foreground(sapphire).
					Bold(true).
					Render("[" + fnID + "]")
				result.WriteString(styled)
				i = end + 1
				continue
			}
		}

		// Italic *...*
		if runes[i] == '*' && (i+1 < n && runes[i+1] != '*') {
			end := -1
			for j := i + 1; j < n; j++ {
				if runes[j] == '*' {
					end = j
					break
				}
			}
			if end != -1 && end > i+1 {
				italic := string(runes[i+1 : end])
				styled := lipgloss.NewStyle().
					Foreground(subtext1).
					Italic(true).
					Render(italic)
				result.WriteString(styled)
				i = end + 1
				continue
			}
		}

		// Tags #tag
		if runes[i] == '#' && (i == 0 || runes[i-1] == ' ') {
			end := i + 1
			for end < n && runes[end] != ' ' && runes[end] != '\t' && runes[end] != ',' {
				end++
			}
			if end > i+1 {
				tag := string(runes[i:end])
				styled := lipgloss.NewStyle().
					Foreground(crust).
					Background(blue).
					Render(" " + tag + " ")
				result.WriteString(styled)
				i = end
				continue
			}
		}

		result.WriteRune(runes[i])
		i++
	}

	return result.String()
}
