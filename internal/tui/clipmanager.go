package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// ClipManager — clipboard history overlay
// ---------------------------------------------------------------------------

const maxClips = 50

// clipCategory classifies a clip entry.
type clipCategory int

const (
	clipCatText clipCategory = iota
	clipCatCode
	clipCatLink
)

func (c clipCategory) String() string {
	switch c {
	case clipCatCode:
		return "Code"
	case clipCatLink:
		return "Link"
	default:
		return "Text"
	}
}

func (c clipCategory) color() lipgloss.Color {
	switch c {
	case clipCatCode:
		return green
	case clipCatLink:
		return blue
	default:
		return text
	}
}

// clipEntry is a single item in the clipboard history.
type clipEntry struct {
	Text      string
	Timestamp time.Time
	Source    string // note path where it was copied from
	Pinned   bool   // pinned clips stay at top
}

// category auto-detects the clip type.
func (ce clipEntry) category() clipCategory {
	t := strings.TrimSpace(ce.Text)
	if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "    ") || strings.HasPrefix(t, "\t") {
		return clipCatCode
	}
	if strings.Contains(t, "http://") || strings.Contains(t, "https://") || strings.Contains(t, "[[") {
		return clipCatLink
	}
	return clipCatText
}

// preview returns a single-line preview of the clip text (max chars).
func (ce clipEntry) preview(maxLen int) string {
	line := strings.ReplaceAll(ce.Text, "\n", " ")
	line = strings.ReplaceAll(line, "\r", "")
	line = strings.Join(strings.Fields(line), " ")
	return TruncateDisplay(line, maxLen)
}

// ClipManager provides a clipboard history overlay for the TUI.
type ClipManager struct {
	active bool
	width  int
	height int

	clips  []clipEntry
	cursor int
	scroll int

	// Search
	searching bool
	searchBuf string
	filtered  []int // indices into clips

	// Result
	selectedText string
	hasResult    bool
}

// NewClipManager returns a zero-value ClipManager ready for use.
func NewClipManager() ClipManager {
	return ClipManager{}
}

// IsActive reports whether the overlay is currently visible.
func (cm ClipManager) IsActive() bool {
	return cm.active
}

// Open activates the clipboard manager overlay.
func (cm *ClipManager) Open() {
	cm.active = true
	cm.cursor = 0
	cm.scroll = 0
	cm.searching = false
	cm.searchBuf = ""
	cm.filtered = nil
	cm.hasResult = false
	cm.selectedText = ""
	cm.rebuildFiltered()
}

// Close hides the overlay.
func (cm *ClipManager) Close() {
	cm.active = false
	cm.searching = false
}

// SetSize updates the available dimensions for the overlay.
func (cm *ClipManager) SetSize(w, h int) {
	cm.width = w
	cm.height = h
}

// AddClip records a new clip in the history. Duplicates are moved to the
// front rather than inserted again. The list is capped at maxClips.
// Pinned state is preserved when a duplicate is re-added.
func (cm *ClipManager) AddClip(text, source string) {
	if strings.TrimSpace(text) == "" {
		return
	}

	// Deduplicate — remove any existing entry with the same text,
	// preserving its pinned state.
	wasPinned := false
	for i := 0; i < len(cm.clips); i++ {
		if cm.clips[i].Text == text {
			wasPinned = cm.clips[i].Pinned
			cm.clips = append(cm.clips[:i], cm.clips[i+1:]...)
			break
		}
	}

	entry := clipEntry{
		Text:      text,
		Timestamp: time.Now(),
		Source:    source,
		Pinned:   wasPinned,
	}

	// Prepend.
	cm.clips = append([]clipEntry{entry}, cm.clips...)

	// Cap at maxClips, but never drop pinned clips.
	if len(cm.clips) > maxClips {
		for len(cm.clips) > maxClips {
			// Remove the last unpinned clip
			removed := false
			for i := len(cm.clips) - 1; i >= 0; i-- {
				if !cm.clips[i].Pinned {
					cm.clips = append(cm.clips[:i], cm.clips[i+1:]...)
					removed = true
					break
				}
			}
			if !removed {
				// All clips are pinned — hard cap
				cm.clips = cm.clips[:maxClips]
				break
			}
		}
	}
}

// GetResult returns the selected clip text (consumed-once pattern).
func (cm *ClipManager) GetResult() (string, bool) {
	if cm.hasResult {
		cm.hasResult = false
		t := cm.selectedText
		cm.selectedText = ""
		return t, true
	}
	return "", false
}

// ClipCount returns the number of clips currently stored.
func (cm ClipManager) ClipCount() int {
	return len(cm.clips)
}

// ---------------------------------------------------------------------------
// Sorting helpers — pinned clips always come first
// ---------------------------------------------------------------------------

// sortedClips returns indices into cm.clips with pinned first, then by time.
func (cm *ClipManager) sortedClips() []int {
	pinned := make([]int, 0)
	unpinned := make([]int, 0)
	for i := range cm.clips {
		if cm.clips[i].Pinned {
			pinned = append(pinned, i)
		} else {
			unpinned = append(unpinned, i)
		}
	}
	return append(pinned, unpinned...)
}

// rebuildFiltered updates the filtered list based on search query.
func (cm *ClipManager) rebuildFiltered() {
	sorted := cm.sortedClips()
	if cm.searchBuf == "" {
		cm.filtered = sorted
		return
	}
	query := strings.ToLower(cm.searchBuf)
	cm.filtered = nil
	for _, idx := range sorted {
		if strings.Contains(strings.ToLower(cm.clips[idx].Text), query) ||
			strings.Contains(strings.ToLower(cm.clips[idx].Source), query) {
			cm.filtered = append(cm.filtered, idx)
		}
	}
}

// currentClip returns the clipEntry under the cursor, if any.
func (cm *ClipManager) currentClip() *clipEntry {
	if len(cm.filtered) == 0 || cm.cursor >= len(cm.filtered) {
		return nil
	}
	idx := cm.filtered[cm.cursor]
	if idx < 0 || idx >= len(cm.clips) {
		return nil
	}
	return &cm.clips[idx]
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

// Update processes messages for the clipboard manager overlay.
func (cm ClipManager) Update(msg tea.Msg) (ClipManager, tea.Cmd) {
	if !cm.active {
		return cm, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if cm.searching {
			return cm.updateSearch(msg)
		}
		return cm.updateNormal(msg)
	}
	return cm, nil
}

func (cm ClipManager) updateSearch(msg tea.KeyMsg) (ClipManager, tea.Cmd) {
	switch msg.String() {
	case "esc":
		cm.searching = false
		cm.searchBuf = ""
		cm.rebuildFiltered()
		cm.cursor = 0
		cm.scroll = 0
	case "enter":
		cm.searching = false
		// Keep filtered results, switch to normal nav
	case "backspace":
		if len(cm.searchBuf) > 0 {
			cm.searchBuf = dropLastRune(cm.searchBuf)
			cm.rebuildFiltered()
			cm.cursor = 0
			cm.scroll = 0
		}
	default:
		ch := msg.String()
		if len(ch) == 1 {
			cm.searchBuf += ch
			cm.rebuildFiltered()
			cm.cursor = 0
			cm.scroll = 0
		}
	}
	return cm, nil
}

func (cm ClipManager) updateNormal(msg tea.KeyMsg) (ClipManager, tea.Cmd) {
	switch msg.String() {
	case "esc":
		cm.active = false
		return cm, nil

	case "/":
		cm.searching = true
		cm.searchBuf = ""
		return cm, nil

	case "up", "k":
		if cm.cursor > 0 {
			cm.cursor--
			if cm.cursor < cm.scroll {
				cm.scroll = cm.cursor
			}
		}

	case "down", "j":
		if cm.cursor < len(cm.filtered)-1 {
			cm.cursor++
			visH := cm.listHeight()
			if cm.cursor >= cm.scroll+visH {
				cm.scroll = cm.cursor - visH + 1
			}
		}

	case "enter":
		ce := cm.currentClip()
		if ce != nil {
			cm.selectedText = ce.Text
			cm.hasResult = true
			cm.active = false
		}

	case "d":
		if len(cm.filtered) > 0 && cm.cursor < len(cm.filtered) {
			idx := cm.filtered[cm.cursor]
			if idx >= 0 && idx < len(cm.clips) {
				cm.clips = append(cm.clips[:idx], cm.clips[idx+1:]...)
				cm.rebuildFiltered()
				if cm.cursor >= len(cm.filtered) && cm.cursor > 0 {
					cm.cursor--
				}
			}
		}

	case "p":
		ce := cm.currentClip()
		if ce != nil {
			ce.Pinned = !ce.Pinned
			cm.rebuildFiltered()
		}
	}

	return cm, nil
}

// listHeight returns the number of visible lines available for the clip list.
func (cm ClipManager) listHeight() int {
	// overlay height minus header (3), separator (1), preview section (~40%),
	// footer (2), border/padding (4)
	h := cm.height/2 - 10
	listH := h * 6 / 10
	if listH < 3 {
		listH = 3
	}
	return listH
}

// previewHeight returns the number of lines for the preview pane.
func (cm ClipManager) previewHeight() int {
	h := cm.height/2 - 10
	prevH := h * 4 / 10
	if prevH < 3 {
		prevH = 3
	}
	return prevH
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

// View renders the clipboard manager overlay.
func (cm ClipManager) View() string {
	width := cm.width / 2
	if width < 56 {
		width = 56
	}
	if width > 80 {
		width = 80
	}
	innerW := width - 6

	var b strings.Builder

	// Title
	titleStr := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  Clipboard Manager")
	b.WriteString(titleStr)
	b.WriteString("\n")

	// Summary line
	countStr := lipgloss.NewStyle().Foreground(text).Render(
		fmt.Sprintf("  %d clips", len(cm.clips)))
	helpHints := DimStyle.Render("  /: search  p: pin")
	b.WriteString(countStr + helpHints)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", innerW-4)))
	b.WriteString("\n")

	// Search bar (if active)
	if cm.searching {
		prompt := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("  / ")
		cursor := lipgloss.NewStyle().Foreground(green).Bold(true).Render("\u2588")
		searchVal := lipgloss.NewStyle().Foreground(text).Render(cm.searchBuf + cursor)
		b.WriteString(prompt + searchVal)
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", innerW-4)))
		b.WriteString("\n")
	}

	// Clip list
	if len(cm.filtered) == 0 {
		if cm.searchBuf != "" {
			b.WriteString(DimStyle.Render("  No clips matching \"" + cm.searchBuf + "\""))
		} else {
			b.WriteString(DimStyle.Render("  No clips yet"))
		}
		b.WriteString("\n")
	} else {
		visH := cm.listHeight()
		end := cm.scroll + visH
		if end > len(cm.filtered) {
			end = len(cm.filtered)
		}

		// Track whether we've printed section headers
		inPinned := false
		inRecent := false

		for i := cm.scroll; i < end; i++ {
			idx := cm.filtered[i]
			if idx < 0 || idx >= len(cm.clips) {
				continue
			}
			ce := cm.clips[idx]

			// Section headers
			if ce.Pinned && !inPinned {
				inPinned = true
				header := lipgloss.NewStyle().Foreground(yellow).Bold(true).Render("  PIN Pinned")
				b.WriteString(header)
				b.WriteString("\n")
			}
			if !ce.Pinned && !inRecent {
				inRecent = true
				header := lipgloss.NewStyle().Foreground(blue).Bold(true).Render("  Recent")
				b.WriteString(header)
				b.WriteString("\n")
			}

			// Clip line
			previewLen := innerW - 18
			if previewLen < 20 {
				previewLen = 20
			}
			preview := ce.preview(previewLen)
			timeStr := cm.formatAge(ce.Timestamp)

			catLabel := ce.category().String()
			catColor := ce.category().color()
			catTag := lipgloss.NewStyle().Foreground(catColor).Render("[" + catLabel + "]")

			if i == cm.cursor {
				line := fmt.Sprintf("  > \"%s\"", preview)
				padded := line
				tlen := len(timeStr) + len(catLabel) + 5
				if len(padded)+tlen < innerW {
					padded += strings.Repeat(" ", innerW-len(padded)-tlen)
				}
				padded += " " + catTag + " "

				selected := lipgloss.NewStyle().
					Background(surface0).
					Foreground(peach).
					Bold(true).
					Width(innerW).
					Render(padded + DimStyle.Render(timeStr))
				b.WriteString(selected)
			} else {
				line := fmt.Sprintf("    \"%s\"", preview)
				tlen := len(timeStr) + len(catLabel) + 5
				if len(line)+tlen < innerW {
					line += strings.Repeat(" ", innerW-len(line)-tlen)
				}
				line += " " + catTag + " "

				b.WriteString(NormalItemStyle.Render(line) + DimStyle.Render(timeStr))
			}
			b.WriteString("\n")
		}
	}

	// Separator
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", innerW-4)))
	b.WriteString("\n")

	// Preview pane
	previewLabel := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("  Preview:")
	b.WriteString(previewLabel)
	b.WriteString("\n")

	ce := cm.currentClip()
	if ce != nil {
		prevH := cm.previewHeight()
		lines := strings.Split(ce.Text, "\n")
		if len(lines) > prevH {
			lines = lines[:prevH]
		}
		contentStyle := lipgloss.NewStyle().Foreground(text)
		for _, line := range lines {
			rendered := contentStyle.Render("  " + truncateClip(line, innerW-4))
			b.WriteString(rendered)
			b.WriteString("\n")
		}

		// Source info
		if ce.Source != "" {
			srcLabel := DimStyle.Render("  from: " + truncateClip(ce.Source, innerW-10))
			b.WriteString(srcLabel)
			b.WriteString("\n")
		}
	} else {
		b.WriteString(DimStyle.Render("  (no clip selected)"))
		b.WriteString("\n")
	}

	// Footer
	b.WriteString("\n")
	helpKeys := lipgloss.NewStyle().Foreground(surface0).Background(overlay0).Padding(0, 1)
	helpDesc := DimStyle
	b.WriteString("  ")
	b.WriteString(helpKeys.Render("Enter") + helpDesc.Render(" paste") + "  ")
	b.WriteString(helpKeys.Render("d") + helpDesc.Render(" delete") + "  ")
	b.WriteString(helpKeys.Render("p") + helpDesc.Render(" pin") + "  ")
	b.WriteString(helpKeys.Render("Esc") + helpDesc.Render(" close"))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

// ---------------------------------------------------------------------------
// Time formatting
// ---------------------------------------------------------------------------

func (cm ClipManager) formatAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		mins := int(d.Minutes())
		return fmt.Sprintf("%dm ago", mins)
	case d < 24*time.Hour:
		hours := int(d.Hours())
		return fmt.Sprintf("%dh ago", hours)
	default:
		days := int(d.Hours() / 24)
		return fmt.Sprintf("%dd ago", days)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// truncateClip shortens s to at most maxLen characters with ellipsis.
// This is a local helper to avoid collision with clipboard.go's truncate.
func truncateClip(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
