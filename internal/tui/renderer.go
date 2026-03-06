package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type Renderer struct {
	width  int
	height int
}

func NewRenderer() Renderer {
	return Renderer{}
}

func (r *Renderer) SetSize(width, height int) {
	r.width = width
	r.height = height
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

	for i, line := range lines {
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
						Foreground(lipgloss.Color("#6C7086")).
						Render("  ┌" + strings.Repeat("─", contentWidth-4) + "┐")
					result = append(result, fmBorder)
					for _, fl := range fmLines {
						parts := strings.SplitN(fl, ":", 2)
						if len(parts) == 2 {
							key := lipgloss.NewStyle().Foreground(lipgloss.Color("#89B4FA")).Bold(true).Render(strings.TrimSpace(parts[0]))
							val := lipgloss.NewStyle().Foreground(lipgloss.Color("#CDD6F4")).Render(strings.TrimSpace(parts[1]))
							fmLine := "  │ " + key + ": " + val
							result = append(result, fmLine)
						}
					}
					fmBorderBottom := lipgloss.NewStyle().
						Foreground(lipgloss.Color("#6C7086")).
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
				if codeBlockLang != "" {
					langLabel := lipgloss.NewStyle().
						Foreground(lipgloss.Color("#6C7086")).
						Italic(true).
						Render("  " + codeBlockLang)
					result = append(result, langLabel)
				}
				codeBorder := lipgloss.NewStyle().
					Foreground(lipgloss.Color("#45475A")).
					Render("  " + strings.Repeat("─", contentWidth-4))
				result = append(result, codeBorder)
				continue
			} else {
				inCodeBlock = false
				codeBlockLang = ""
				codeBorder := lipgloss.NewStyle().
					Foreground(lipgloss.Color("#45475A")).
					Render("  " + strings.Repeat("─", contentWidth-4))
				result = append(result, codeBorder)
				continue
			}
		}

		if inCodeBlock {
			codeLine := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#A6E3A1")).
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
				Foreground(lipgloss.Color("#45475A")).
				Render("  " + strings.Repeat("━", contentWidth-4))
			result = append(result, rule)
			continue
		}

		// Headings
		if strings.HasPrefix(trimmed, "# ") {
			text := strings.TrimPrefix(trimmed, "# ")
			// Big heading with underline
			styled := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#CBA6F7")).
				Bold(true).
				Render("  " + text)
			underline := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#CBA6F7")).
				Render("  " + strings.Repeat("═", len(text)))
			result = append(result, "")
			result = append(result, styled)
			result = append(result, underline)
			result = append(result, "")
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			text := strings.TrimPrefix(trimmed, "## ")
			styled := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#89B4FA")).
				Bold(true).
				Render("  " + text)
			underline := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#45475A")).
				Render("  " + strings.Repeat("─", len(text)))
			result = append(result, "")
			result = append(result, styled)
			result = append(result, underline)
			continue
		}
		if strings.HasPrefix(trimmed, "### ") {
			text := strings.TrimPrefix(trimmed, "### ")
			styled := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#74C7EC")).
				Bold(true).
				Render("  " + text)
			result = append(result, "")
			result = append(result, styled)
			continue
		}
		if strings.HasPrefix(trimmed, "#### ") {
			text := strings.TrimPrefix(trimmed, "#### ")
			styled := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#94E2D5")).
				Bold(true).
				Render("  " + text)
			result = append(result, styled)
			continue
		}

		// Blockquote
		if strings.HasPrefix(trimmed, "> ") {
			text := strings.TrimPrefix(trimmed, "> ")
			bar := lipgloss.NewStyle().Foreground(lipgloss.Color("#CBA6F7")).Render("  ┃ ")
			quote := lipgloss.NewStyle().Foreground(lipgloss.Color("#7F849C")).Italic(true).Render(text)
			result = append(result, bar+quote)
			continue
		}

		// Checkboxes
		if strings.HasPrefix(trimmed, "- [x] ") || strings.HasPrefix(trimmed, "- [X] ") {
			text := trimmed[6:]
			checkbox := lipgloss.NewStyle().Foreground(lipgloss.Color("#A6E3A1")).Render("  ✓ ")
			styledText := lipgloss.NewStyle().Foreground(lipgloss.Color("#6C7086")).Strikethrough(true).Render(text)
			result = append(result, checkbox+styledText)
			continue
		}
		if strings.HasPrefix(trimmed, "- [ ] ") {
			text := trimmed[6:]
			checkbox := lipgloss.NewStyle().Foreground(lipgloss.Color("#F9E2AF")).Render("  ○ ")
			styledText := lipgloss.NewStyle().Foreground(lipgloss.Color("#CDD6F4")).Render(text)
			result = append(result, checkbox+styledText)
			continue
		}

		// Unordered list
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			text := trimmed[2:]
			indent := strings.Repeat(" ", len(line)-len(trimmed))
			bullet := lipgloss.NewStyle().Foreground(lipgloss.Color("#FAB387")).Render("  " + indent + "● ")
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
						numStyled := lipgloss.NewStyle().Foreground(lipgloss.Color("#FAB387")).Bold(true).Render("  " + num + ". ")
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
					tableLine += lipgloss.NewStyle().Foreground(lipgloss.Color("#45475A")).Render(strings.Repeat("─", len(part)+2))
				} else {
					tableLine += lipgloss.NewStyle().Foreground(lipgloss.Color("#CDD6F4")).Render(" " + part + " ")
				}
				if pi < len(parts)-1 {
					tableLine += lipgloss.NewStyle().Foreground(lipgloss.Color("#45475A")).Render("│")
				}
			}
			result = append(result, tableLine)
			continue
		}

		// Normal paragraph
		result = append(result, "  "+r.renderInline(trimmed))
	}

	return result
}

func (r Renderer) renderInline(text string) string {
	if text == "" {
		return ""
	}

	var result strings.Builder
	runes := []rune(text)
	n := len(runes)
	i := 0

	for i < n {
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
					Foreground(lipgloss.Color("#89B4FA")).
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
					Foreground(lipgloss.Color("#A6E3A1")).
					Background(lipgloss.Color("#313244")).
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
					Foreground(lipgloss.Color("#CDD6F4")).
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
					Foreground(lipgloss.Color("#BAC2DE")).
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
					Foreground(lipgloss.Color("#11111B")).
					Background(lipgloss.Color("#89B4FA")).
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
