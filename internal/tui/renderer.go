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
		return calloutInfo{color: blue, icon: "\u2139", label: strings.Title(typ)}
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
		return lipgloss.NewStyle().Foreground(overlay0).Render("[ ] ") +
			lipgloss.NewStyle().Foreground(text).Render(line[6:])
	}
	if strings.HasPrefix(line, "- [x] ") {
		return lipgloss.NewStyle().Foreground(green).Render("[x] ") +
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

		// Horizontal rule
		if trimmed == "---" || trimmed == "***" || trimmed == "___" {
			rule := lipgloss.NewStyle().
				Foreground(surface1).
				Render("  " + strings.Repeat("━", contentWidth-4))
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

		// Blockquote
		if strings.HasPrefix(trimmed, "> ") {
			text := strings.TrimPrefix(trimmed, "> ")
			bar := lipgloss.NewStyle().Foreground(mauve).Render("  ┃ ")
			quote := lipgloss.NewStyle().Foreground(overlay1).Italic(true).Render(text)
			result = append(result, bar+quote)
			continue
		}

		// Checkboxes
		if strings.HasPrefix(trimmed, "- [x] ") || strings.HasPrefix(trimmed, "- [X] ") {
			doneText := trimmed[6:]
			checkbox := lipgloss.NewStyle().Foreground(green).Render("  ✓ ")
			styledText := lipgloss.NewStyle().Foreground(overlay0).Strikethrough(true).Render(doneText)
			result = append(result, checkbox+styledText)
			continue
		}
		if strings.HasPrefix(trimmed, "- [ ] ") {
			todoText := trimmed[6:]
			checkbox := lipgloss.NewStyle().Foreground(yellow).Render("  ○ ")
			styledText := lipgloss.NewStyle().Foreground(text).Render(todoText)
			result = append(result, checkbox+styledText)
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

		// Table detection (basic)
		if strings.Contains(trimmed, "|") && strings.Count(trimmed, "|") >= 2 {
			// Simple: just style the pipe separators
			tableLine := "  "
			parts := strings.Split(trimmed, "|")
			for pi, part := range parts {
				part = strings.TrimSpace(part)
				if strings.Repeat("-", len(part)) == part && len(part) > 0 {
					// Separator row
					tableLine += lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("─", len(part)+2))
				} else {
					tableLine += lipgloss.NewStyle().Foreground(text).Render(" " + part + " ")
				}
				if pi < len(parts)-1 {
					tableLine += lipgloss.NewStyle().Foreground(surface1).Render("│")
				}
			}
			result = append(result, tableLine)
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

		// Normal paragraph
		result = append(result, "  "+r.renderInline(trimmed))
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
