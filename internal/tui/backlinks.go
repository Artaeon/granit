package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// BacklinkItem represents a single backlink with context about where the link appears.
type BacklinkItem struct {
	Path    string // file path (e.g. "meeting-notes.md")
	Context string // the line/paragraph containing the link
	LineNum int    // line number of the link
}

type Backlinks struct {
	incoming []BacklinkItem
	outgoing []BacklinkItem
	cursor   int
	focused  bool
	height   int
	width    int
	mode     int // 0=incoming, 1=outgoing
	scroll   int
}

func NewBacklinks() Backlinks {
	return Backlinks{}
}

func (bl *Backlinks) SetSize(width, height int) {
	bl.width = width
	bl.height = height
}

func (bl *Backlinks) SetLinks(incoming, outgoing []BacklinkItem) {
	bl.incoming = incoming
	bl.outgoing = outgoing
	bl.cursor = 0
	bl.scroll = 0
}

// Selected returns the file path of the currently selected backlink item.
func (bl *Backlinks) Selected() string {
	items := bl.currentItems()
	if len(items) == 0 || bl.cursor >= len(items) {
		return ""
	}
	return items[bl.cursor].Path
}

func (bl *Backlinks) currentItems() []BacklinkItem {
	if bl.mode == 0 {
		return bl.incoming
	}
	return bl.outgoing
}

func (bl Backlinks) Update(msg tea.Msg) (Backlinks, tea.Cmd) {
	if !bl.focused {
		return bl, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		items := bl.currentItems()
		switch msg.String() {
		case "up", "k":
			if bl.cursor > 0 {
				bl.cursor--
				if bl.cursor < bl.scroll {
					bl.scroll = bl.cursor
				}
			}
		case "down", "j":
			if bl.cursor < len(items)-1 {
				bl.cursor++
				visibleHeight := bl.visibleItemCount()
				if bl.cursor >= bl.scroll+visibleHeight {
					bl.scroll = bl.cursor - visibleHeight + 1
				}
			}
		case "tab":
			bl.mode = (bl.mode + 1) % 2
			bl.cursor = 0
			bl.scroll = 0
		}
	}
	return bl, nil
}

// showContext returns true if the panel is wide enough to show context lines.
func (bl Backlinks) showContext() bool {
	return bl.width >= 25
}

// visibleItemCount returns how many backlink items fit in the visible area,
// accounting for context lines taking extra vertical space.
func (bl Backlinks) visibleItemCount() int {
	available := bl.height - 8
	if available < 1 {
		available = 1
	}
	if bl.showContext() {
		// Each item takes 2 lines (name + context)
		count := available / 2
		if count < 1 {
			count = 1
		}
		return count
	}
	return available
}

func (bl Backlinks) View() string {
	var b strings.Builder
	contentWidth := bl.width - 4
	if contentWidth < 10 {
		contentWidth = 10
	}

	// Tab header with pill style
	inCount := len(bl.incoming)
	outCount := len(bl.outgoing)

	activeTabStyle := lipgloss.NewStyle().
		Foreground(base).
		Background(mauve).
		Bold(true).
		Padding(0, 1)

	inactiveTabStyle := lipgloss.NewStyle().
		Foreground(overlay0).
		Background(surface0).
		Padding(0, 1)

	var inTab, outTab string
	if bl.mode == 0 {
		inTab = activeTabStyle.Render(formatTabLabel("Backlinks", inCount))
		outTab = inactiveTabStyle.Render(formatTabLabel("Outgoing", outCount))
	} else {
		inTab = inactiveTabStyle.Render(formatTabLabel("Backlinks", inCount))
		outTab = activeTabStyle.Render(formatTabLabel("Outgoing", outCount))
	}

	b.WriteString(inTab + " " + outTab)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", contentWidth)))
	b.WriteString("\n")

	items := bl.currentItems()
	if len(items) == 0 {
		b.WriteString("\n")
		emptyIcon := DimStyle.Render("  ")
		emptyText := DimStyle.Render(" No links found")
		b.WriteString(emptyIcon + emptyText)
		return b.String()
	}

	visibleHeight := bl.visibleItemCount()

	end := bl.scroll + visibleHeight
	if end > len(items) {
		end = len(items)
	}

	showCtx := bl.showContext()
	contextStyle := lipgloss.NewStyle().Foreground(overlay0)
	linkHighlightStyle := lipgloss.NewStyle().Foreground(blue)

	for i := bl.scroll; i < end; i++ {
		item := items[i]
		displayName := strings.TrimSuffix(item.Path, ".md")

		// Truncate name if needed
		maxLen := contentWidth - 6
		if maxLen < 5 {
			maxLen = 5
		}
		if len(displayName) > maxLen {
			displayName = displayName[:maxLen-3] + "..."
		}

		icon := lipgloss.NewStyle().Foreground(blue).Render(" ")

		if i == bl.cursor && bl.focused {
			line := "  " + icon + " " + displayName
			padLen := contentWidth - lipgloss.Width(line)
			if padLen < 0 {
				padLen = 0
			}
			highlighted := lipgloss.NewStyle().
				Background(surface0).
				Foreground(peach).
				Bold(true).
				Width(contentWidth).
				Render("  " + icon + " " + displayName + strings.Repeat(" ", padLen))
			b.WriteString(highlighted)
		} else {
			b.WriteString("  " + icon + " " + NormalItemStyle.Render(displayName))
		}

		// Show context line below the filename if we have context and enough width
		if showCtx && item.Context != "" {
			b.WriteString("\n")
			ctxLine := formatContextLine(item.Context, contentWidth-5, contextStyle, linkHighlightStyle)
			b.WriteString("     " + ctxLine)
		}

		if i < end-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}

// formatContextLine renders a context line with [[...]] parts highlighted in blue.
// The line is trimmed and truncated to fit within maxWidth characters.
func formatContextLine(context string, maxWidth int, dimStyle, highlightStyle lipgloss.Style) string {
	if maxWidth < 5 {
		maxWidth = 5
	}

	// Clean up the context: trim whitespace, collapse internal whitespace
	line := strings.TrimSpace(context)
	if line == "" {
		return ""
	}

	// Truncate the raw text (before styling) to fit the width.
	// We need to measure without the [[ ]] markup to get true visual length.
	plainText := strings.ReplaceAll(strings.ReplaceAll(line, "[[", ""), "]]", "")
	if len(plainText) > maxWidth {
		// Truncate the original line proportionally
		line = truncateWithLinks(line, maxWidth)
	}

	// Now render with [[...]] highlighted
	var result strings.Builder
	remaining := line
	for {
		openIdx := strings.Index(remaining, "[[")
		if openIdx == -1 {
			// No more links, render the rest dimmed
			result.WriteString(dimStyle.Render(remaining))
			break
		}
		closeIdx := strings.Index(remaining[openIdx:], "]]")
		if closeIdx == -1 {
			// No closing bracket, render all dimmed
			result.WriteString(dimStyle.Render(remaining))
			break
		}
		closeIdx += openIdx // adjust to absolute position

		// Render text before the link as dimmed
		if openIdx > 0 {
			result.WriteString(dimStyle.Render(remaining[:openIdx]))
		}
		// Render the link part (including brackets) highlighted
		linkPart := remaining[openIdx : closeIdx+2]
		result.WriteString(highlightStyle.Render(linkPart))
		remaining = remaining[closeIdx+2:]
	}

	return result.String()
}

// truncateWithLinks truncates a line to maxWidth visible characters,
// appending "..." if truncated. It counts [[ and ]] as visible characters.
func truncateWithLinks(line string, maxWidth int) string {
	if maxWidth <= 3 {
		return "..."
	}
	target := maxWidth - 3 // leave room for "..."
	if len(line) <= maxWidth {
		return line
	}
	// Simple rune-based truncation
	runes := []rune(line)
	if len(runes) <= maxWidth {
		return line
	}
	return string(runes[:target]) + "..."
}

func formatTabLabel(label string, count int) string {
	if count == 0 {
		return label
	}
	c := string(rune('0' + count%10))
	if count >= 10 {
		c = string(rune('0'+count/10)) + c
	}
	return label + " " + c
}
