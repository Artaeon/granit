package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Scratchpad provides a persistent floating notepad overlay for quick capture.
// Content is automatically saved to <vaultRoot>/.granit/scratchpad.md on close
// and reloaded on open, so it survives between sessions.
type Scratchpad struct {
	active     bool
	width      int
	height     int
	vaultRoot  string
	content    []string // lines of text
	cursorLine int
	cursorCol  int
	scroll     int
	modified   bool
	clipboard  string // populated by Ctrl+A (select-all copy)
}

// NewScratchpad returns a Scratchpad in its default (inactive) state.
func NewScratchpad() Scratchpad {
	return Scratchpad{
		content: []string{""},
	}
}

// IsActive reports whether the scratchpad overlay is currently visible.
func (s Scratchpad) IsActive() bool {
	return s.active
}

// SetSize updates the available terminal dimensions for the overlay.
func (s *Scratchpad) SetSize(w, h int) {
	s.width = w
	s.height = h
}

// Open activates the scratchpad and loads persisted content from disk.
// If the scratchpad file does not yet exist it is created with empty content.
func (s *Scratchpad) Open(vaultRoot string) {
	s.active = true
	s.vaultRoot = vaultRoot
	s.cursorLine = 0
	s.cursorCol = 0
	s.scroll = 0
	s.modified = false
	s.clipboard = ""
	s.load()
}

// Close saves content to disk and deactivates the overlay.
func (s *Scratchpad) Close() {
	s.save()
	s.active = false
}

// GetContent returns the full scratchpad text as a single string with lines
// joined by newlines.
func (s Scratchpad) GetContent() string {
	return strings.Join(s.content, "\n")
}

// ---------------------------------------------------------------------------
// Persistence
// ---------------------------------------------------------------------------

// scratchpadPath returns the absolute path to the scratchpad file.
func (s *Scratchpad) scratchpadPath() string {
	return filepath.Join(s.vaultRoot, ".granit", "scratchpad.md")
}

// load reads the scratchpad file from disk. If the file does not exist it
// initialises content to a single empty line and creates the parent directory.
func (s *Scratchpad) load() {
	path := s.scratchpadPath()

	data, err := os.ReadFile(path)
	if err != nil {
		// File doesn't exist yet — start with empty content.
		s.content = []string{""}
		// Ensure the parent directory exists so save will work later.
		_ = os.MkdirAll(filepath.Dir(path), 0755)
		return
	}

	raw := strings.ReplaceAll(string(data), "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	s.content = strings.Split(raw, "\n")
	if len(s.content) == 0 {
		s.content = []string{""}
	}
}

// save writes the current content to the scratchpad file on disk.
func (s *Scratchpad) save() {
	if s.vaultRoot == "" {
		return
	}

	path := s.scratchpadPath()
	_ = os.MkdirAll(filepath.Dir(path), 0755)

	data := strings.Join(s.content, "\n")
	_ = atomicWriteState(path, []byte(data))
}

// ---------------------------------------------------------------------------
// Cursor helpers
// ---------------------------------------------------------------------------

// clampCursor ensures the cursor stays within valid content bounds.
func (s *Scratchpad) clampCursor() {
	if len(s.content) == 0 {
		s.content = []string{""}
	}
	if s.cursorLine < 0 {
		s.cursorLine = 0
	}
	if s.cursorLine >= len(s.content) {
		s.cursorLine = len(s.content) - 1
	}
	lineLen := len(s.content[s.cursorLine])
	if s.cursorCol < 0 {
		s.cursorCol = 0
	}
	if s.cursorCol > lineLen {
		s.cursorCol = lineLen
	}
}

// ensureVisible adjusts the scroll offset so the cursor line is on-screen.
func (s *Scratchpad) ensureVisible(visibleLines int) {
	if s.cursorLine < s.scroll {
		s.scroll = s.cursorLine
	}
	if s.cursorLine >= s.scroll+visibleLines {
		s.scroll = s.cursorLine - visibleLines + 1
	}
	if s.scroll < 0 {
		s.scroll = 0
	}
}

// wordCount returns the total number of words across all lines.
func (s *Scratchpad) wordCount() int {
	count := 0
	for _, line := range s.content {
		fields := strings.Fields(line)
		count += len(fields)
	}
	return count
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

// visibleHeight returns the number of content lines visible in the scratchpad.
func (s *Scratchpad) visibleHeight() int {
	h := s.height - 12
	if h < 4 {
		h = 4
	}
	return h
}

// Update handles keyboard input for the scratchpad overlay.
func (s Scratchpad) Update(msg tea.Msg) (Scratchpad, tea.Cmd) {
	if !s.active {
		return s, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			s.save()
			s.active = false
			return s, nil

		case "up":
			if s.cursorLine > 0 {
				s.cursorLine--
				s.clampCursor()
			}
			s.ensureVisible(s.visibleHeight())
			return s, nil

		case "down":
			if s.cursorLine < len(s.content)-1 {
				s.cursorLine++
				s.clampCursor()
			}
			s.ensureVisible(s.visibleHeight())
			return s, nil

		case "left":
			if s.cursorCol > 0 {
				s.cursorCol--
			} else if s.cursorLine > 0 {
				// Wrap to end of previous line.
				s.cursorLine--
				s.cursorCol = len(s.content[s.cursorLine])
			}
			s.ensureVisible(s.visibleHeight())
			return s, nil

		case "right":
			lineLen := len(s.content[s.cursorLine])
			if s.cursorCol < lineLen {
				s.cursorCol++
			} else if s.cursorLine < len(s.content)-1 {
				// Wrap to start of next line.
				s.cursorLine++
				s.cursorCol = 0
			}
			s.ensureVisible(s.visibleHeight())
			return s, nil

		case "home":
			s.cursorCol = 0
			return s, nil

		case "end":
			s.cursorCol = len(s.content[s.cursorLine])
			return s, nil

		case "pgup":
			vis := s.visibleHeight()
			s.cursorLine -= vis
			if s.cursorLine < 0 {
				s.cursorLine = 0
			}
			s.clampCursor()
			s.ensureVisible(vis)
			return s, nil

		case "pgdown":
			vis := s.visibleHeight()
			s.cursorLine += vis
			if s.cursorLine >= len(s.content) {
				s.cursorLine = len(s.content) - 1
			}
			s.clampCursor()
			s.ensureVisible(vis)
			return s, nil

		case "tab":
			s.insertChar("\t")
			return s, nil

		case "enter":
			s.insertNewline()
			s.ensureVisible(s.visibleHeight())
			return s, nil

		case "backspace":
			s.deleteBack()
			s.ensureVisible(s.visibleHeight())
			return s, nil

		case "delete":
			s.deleteForward()
			return s, nil

		case "ctrl+a":
			// Select all — copy full content to clipboard field.
			s.clipboard = strings.Join(s.content, "\n")
			return s, nil

		default:
			ch := msg.String()
			// Only insert printable single-byte ASCII or multi-byte UTF-8 runes.
			// Reject named keys like "f1", "insert", etc.
			if len(ch) == 1 && ch[0] >= 32 && ch[0] < 127 {
				s.insertChar(ch)
			} else if len(ch) > 1 && !strings.HasPrefix(ch, "ctrl+") &&
				!strings.HasPrefix(ch, "alt+") &&
				!strings.HasPrefix(ch, "shift+") &&
				!strings.HasPrefix(ch, "f") &&
				ch != "insert" && ch != "pgup" && ch != "pgdown" &&
				ch != "tab" && ch != "home" && ch != "end" &&
				ch != "up" && ch != "down" && ch != "left" && ch != "right" {
				// Multi-byte UTF-8 character.
				s.insertChar(ch)
			}
			return s, nil
		}
	}

	return s, nil
}

// insertChar inserts a string at the cursor position.
func (s *Scratchpad) insertChar(ch string) {
	line := s.content[s.cursorLine]
	before := line[:s.cursorCol]
	after := line[s.cursorCol:]
	s.content[s.cursorLine] = before + ch + after
	s.cursorCol += len(ch)
	s.modified = true
}

// insertNewline splits the current line at the cursor and creates a new line.
func (s *Scratchpad) insertNewline() {
	line := s.content[s.cursorLine]
	before := line[:s.cursorCol]
	after := line[s.cursorCol:]

	s.content[s.cursorLine] = before

	// Insert new line after current.
	newContent := make([]string, 0, len(s.content)+1)
	newContent = append(newContent, s.content[:s.cursorLine+1]...)
	newContent = append(newContent, after)
	if s.cursorLine+1 < len(s.content) {
		newContent = append(newContent, s.content[s.cursorLine+1:]...)
	}
	s.content = newContent

	s.cursorLine++
	s.cursorCol = 0
	s.modified = true
}

// deleteBack handles backspace: delete the character before the cursor, or
// merge with the previous line if at column 0.
func (s *Scratchpad) deleteBack() {
	if s.cursorCol > 0 {
		line := s.content[s.cursorLine]
		s.content[s.cursorLine] = line[:s.cursorCol-1] + line[s.cursorCol:]
		s.cursorCol--
		s.modified = true
	} else if s.cursorLine > 0 {
		// Merge current line into previous.
		prevLen := len(s.content[s.cursorLine-1])
		s.content[s.cursorLine-1] += s.content[s.cursorLine]

		newContent := make([]string, 0, len(s.content)-1)
		newContent = append(newContent, s.content[:s.cursorLine]...)
		if s.cursorLine+1 < len(s.content) {
			newContent = append(newContent, s.content[s.cursorLine+1:]...)
		}
		s.content = newContent

		s.cursorLine--
		s.cursorCol = prevLen
		s.modified = true
	}
}

// deleteForward handles the delete key: delete the character at the cursor, or
// merge the next line into the current one if at end of line.
func (s *Scratchpad) deleteForward() {
	line := s.content[s.cursorLine]
	if s.cursorCol < len(line) {
		s.content[s.cursorLine] = line[:s.cursorCol] + line[s.cursorCol+1:]
		s.modified = true
	} else if s.cursorLine < len(s.content)-1 {
		// Merge next line into current.
		s.content[s.cursorLine] += s.content[s.cursorLine+1]

		newContent := make([]string, 0, len(s.content)-1)
		newContent = append(newContent, s.content[:s.cursorLine+1]...)
		if s.cursorLine+2 < len(s.content) {
			newContent = append(newContent, s.content[s.cursorLine+2:]...)
		}
		s.content = newContent
		s.modified = true
	}
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

// View renders the scratchpad as a floating overlay panel.
func (s Scratchpad) View() string {
	// --- panel geometry ---
	panelWidth := s.width * 2 / 3
	if panelWidth > 60 {
		panelWidth = 60
	}
	if panelWidth < 30 {
		panelWidth = 30
	}

	// Inner width after border (2) and padding (2*2=4).
	innerWidth := panelWidth - 6
	if innerWidth < 20 {
		innerWidth = 20
	}

	// Use same height calculation as Update.
	contentHeight := s.visibleHeight()

	// Recompute scroll for rendering (scroll is maintained in Update).
	scroll := s.scroll
	if s.cursorLine < scroll {
		scroll = s.cursorLine
	}
	if s.cursorLine >= scroll+contentHeight {
		scroll = s.cursorLine - contentHeight + 1
	}
	if scroll < 0 {
		scroll = 0
	}

	var b strings.Builder

	// --- title bar ---
	icon := lipgloss.NewStyle().Foreground(teal).Render(IconEditChar)
	titleText := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render(" Scratchpad")
	wc := s.wordCount()
	wordInfo := lipgloss.NewStyle().
		Foreground(overlay0).
		Render(fmt.Sprintf("  %d words", wc))
	b.WriteString(icon + titleText + wordInfo)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", innerWidth)))
	b.WriteString("\n")

	// --- content area with line numbers ---
	lineNumWidth := 3
	totalLines := len(s.content)
	if totalLines >= 100 {
		lineNumWidth = 4
	}
	if totalLines >= 1000 {
		lineNumWidth = 5
	}

	end := scroll + contentHeight
	if end > len(s.content) {
		end = len(s.content)
	}

	textAreaWidth := innerWidth - lineNumWidth - 2 // 1 space + 1 separator
	if textAreaWidth < 10 {
		textAreaWidth = 10
	}

	for i := scroll; i < end; i++ {
		line := s.content[i]

		// Line number.
		numStr := fmt.Sprintf("%*s", lineNumWidth, smallNum(i+1))
		var numStyled string
		if i == s.cursorLine {
			numStyled = lipgloss.NewStyle().
				Foreground(yellow).
				Bold(true).
				Render(numStr)
		} else {
			numStyled = DimStyle.Render(numStr)
		}

		sep := DimStyle.Render("│")

		// Render text with cursor highlight if this is the cursor line.
		displayLine := s.renderLine(line, i, textAreaWidth)

		b.WriteString(numStyled + sep + displayLine)
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	// Pad remaining lines if content is shorter than visible area.
	rendered := end - scroll
	for rendered < contentHeight {
		numStr := fmt.Sprintf("%*s", lineNumWidth, "")
		sep := DimStyle.Render("│")
		b.WriteString("\n" + DimStyle.Render(numStr) + sep)
		rendered++
	}

	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", innerWidth)))
	b.WriteString("\n")

	// --- status line: line:col, word count, modified indicator ---
	posInfo := lipgloss.NewStyle().
		Foreground(subtext0).
		Render(fmt.Sprintf("Ln %d, Col %d", s.cursorLine+1, s.cursorCol+1))
	modFlag := ""
	if s.modified {
		modFlag = lipgloss.NewStyle().
			Foreground(yellow).
			Bold(true).
			Render("  [modified]")
	}
	wcInfo := lipgloss.NewStyle().
		Foreground(overlay0).
		Render(fmt.Sprintf("  %d words", wc))

	statusLeft := posInfo + modFlag
	statusRight := wcInfo

	statusPad := innerWidth - lipgloss.Width(statusLeft) - lipgloss.Width(statusRight)
	if statusPad < 1 {
		statusPad = 1
	}
	b.WriteString(statusLeft + strings.Repeat(" ", statusPad) + statusRight)

	b.WriteString("\n")

	// --- keyboard hints ---
	escKey := lipgloss.NewStyle().Foreground(blue).Bold(true).Render("Esc")
	escDesc := DimStyle.Render(": save & close  ")
	ctrlAKey := lipgloss.NewStyle().Foreground(blue).Bold(true).Render("Ctrl+A")
	ctrlADesc := DimStyle.Render(": select all")
	b.WriteString(escKey + escDesc + ctrlAKey + ctrlADesc)

	// --- border ---
	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(panelWidth).
		Background(mantle)

	return border.Render(b.String())
}

// renderLine renders a single line of content, highlighting the cursor
// character if this is the cursor line.
func (s Scratchpad) renderLine(line string, lineIdx int, maxWidth int) string {
	// Truncate the line for display if it exceeds available width.
	displayLine := line
	if len(displayLine) > maxWidth {
		displayLine = displayLine[:maxWidth]
	}

	if lineIdx != s.cursorLine {
		return " " + lipgloss.NewStyle().Foreground(text).Render(displayLine)
	}

	// This is the cursor line — highlight the character under the cursor.
	col := s.cursorCol

	// If cursor is at or beyond visible length, show cursor at end.
	if col > len(displayLine) {
		col = len(displayLine)
	}

	var result strings.Builder
	result.WriteString(" ")

	textStyle := lipgloss.NewStyle().Foreground(text)
	cursorStyle := lipgloss.NewStyle().
		Background(text).
		Foreground(surface0)

	if col < len(displayLine) {
		// Cursor is within the line.
		before := displayLine[:col]
		cursorChar := string(displayLine[col])
		after := displayLine[col+1:]

		if len(before) > 0 {
			result.WriteString(textStyle.Render(before))
		}
		result.WriteString(cursorStyle.Render(cursorChar))
		if len(after) > 0 {
			result.WriteString(textStyle.Render(after))
		}
	} else {
		// Cursor is at end of line — show a highlighted space.
		if len(displayLine) > 0 {
			result.WriteString(textStyle.Render(displayLine))
		}
		result.WriteString(cursorStyle.Render(" "))
	}

	return result.String()
}
