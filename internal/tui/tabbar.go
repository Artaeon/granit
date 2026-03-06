package tui

import (
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// TabEntry represents a single open tab in the tab bar.
type TabEntry struct {
	Path     string
	Modified bool // has unsaved changes
	Pinned   bool
}

// TabBar is a visual tab bar component that shows open/recent notes as
// clickable tabs at the top of the editor area. It is a helper component
// (not a tea.Model) — callers invoke its methods directly.
type TabBar struct {
	tabs      []TabEntry
	maxTabs   int // default 8
	activeIdx int // which tab is currently active
}

// NewTabBar creates a TabBar with sensible defaults.
func NewTabBar() *TabBar {
	return &TabBar{
		maxTabs:   8,
		activeIdx: -1,
	}
}

// findTab returns the index of the tab with the given path, or -1.
func (tb *TabBar) findTab(path string) int {
	for i, t := range tb.tabs {
		if t.Path == path {
			return i
		}
	}
	return -1
}

// AddTab adds a tab for the given path. If the tab already exists it is
// activated instead. When the tab bar is at capacity the oldest unpinned tab
// is removed to make room.
func (tb *TabBar) AddTab(path string) {
	if idx := tb.findTab(path); idx >= 0 {
		tb.activeIdx = idx
		return
	}

	// Evict oldest unpinned tab if at max capacity.
	if len(tb.tabs) >= tb.maxTabs {
		evicted := false
		for i, t := range tb.tabs {
			if !t.Pinned {
				tb.tabs = append(tb.tabs[:i], tb.tabs[i+1:]...)
				if tb.activeIdx >= i && tb.activeIdx > 0 {
					tb.activeIdx--
				}
				evicted = true
				break
			}
		}
		// If all tabs are pinned we still need room — do nothing (cannot evict).
		if !evicted {
			return
		}
	}

	tb.tabs = append(tb.tabs, TabEntry{Path: path})
	tb.activeIdx = len(tb.tabs) - 1
}

// RemoveTab closes the tab with the given path. Pinned tabs are not removed.
func (tb *TabBar) RemoveTab(path string) {
	idx := tb.findTab(path)
	if idx < 0 {
		return
	}
	if tb.tabs[idx].Pinned {
		return
	}
	tb.tabs = append(tb.tabs[:idx], tb.tabs[idx+1:]...)

	// Adjust active index.
	if len(tb.tabs) == 0 {
		tb.activeIdx = -1
	} else if tb.activeIdx >= len(tb.tabs) {
		tb.activeIdx = len(tb.tabs) - 1
	} else if tb.activeIdx > idx {
		tb.activeIdx--
	}
}

// SetActive marks the tab with the given path as active.
func (tb *TabBar) SetActive(path string) {
	if idx := tb.findTab(path); idx >= 0 {
		tb.activeIdx = idx
	}
}

// SetModified marks (or unmarks) a tab as having unsaved changes.
func (tb *TabBar) SetModified(path string, modified bool) {
	if idx := tb.findTab(path); idx >= 0 {
		tb.tabs[idx].Modified = modified
	}
}

// PinTab pins a tab so it cannot be auto-removed.
func (tb *TabBar) PinTab(path string) {
	if idx := tb.findTab(path); idx >= 0 {
		tb.tabs[idx].Pinned = true
	}
}

// UnpinTab removes the pin from a tab.
func (tb *TabBar) UnpinTab(path string) {
	if idx := tb.findTab(path); idx >= 0 {
		tb.tabs[idx].Pinned = false
	}
}

// GetActive returns the path of the currently active tab, or an empty string.
func (tb *TabBar) GetActive() string {
	if tb.activeIdx >= 0 && tb.activeIdx < len(tb.tabs) {
		return tb.tabs[tb.activeIdx].Path
	}
	return ""
}

// NextTab cycles to the next tab and returns its path. Wraps around.
func (tb *TabBar) NextTab() string {
	if len(tb.tabs) == 0 {
		return ""
	}
	tb.activeIdx = (tb.activeIdx + 1) % len(tb.tabs)
	return tb.tabs[tb.activeIdx].Path
}

// PrevTab cycles to the previous tab and returns its path. Wraps around.
func (tb *TabBar) PrevTab() string {
	if len(tb.tabs) == 0 {
		return ""
	}
	tb.activeIdx--
	if tb.activeIdx < 0 {
		tb.activeIdx = len(tb.tabs) - 1
	}
	return tb.tabs[tb.activeIdx].Path
}

// Tabs returns a copy of all tab entries.
func (tb *TabBar) Tabs() []TabEntry {
	out := make([]TabEntry, len(tb.tabs))
	copy(out, tb.tabs)
	return out
}

// HasTab reports whether a tab with the given path is open.
func (tb *TabBar) HasTab(path string) bool {
	return tb.findTab(path) >= 0
}

// ---------------------------------------------------------------------------
// Rendering
// ---------------------------------------------------------------------------

// tbBaseName extracts the file basename without extension.
func tbBaseName(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	if ext != "" {
		base = strings.TrimSuffix(base, ext)
	}
	return base
}

// tbTruncName truncates a name to maxLen characters, appending "…" if needed.
func tbTruncName(name string, maxLen int) string {
	runes := []rune(name)
	if len(runes) <= maxLen {
		return name
	}
	return string(runes[:maxLen-1]) + "\u2026"
}

// tbRenderTab renders a single tab label with the appropriate styling.
func tbRenderTab(entry TabEntry, isActive bool) string {
	name := tbTruncName(tbBaseName(entry.Path), 15)

	var label string

	// Pin indicator before the name.
	if entry.Pinned {
		pinStyle := lipgloss.NewStyle().Foreground(peach)
		label += pinStyle.Render("\u2503") + " "
	}

	// Tab name with active/inactive styling.
	if isActive {
		activeStyle := lipgloss.NewStyle().
			Foreground(mauve).
			Background(surface0).
			Bold(true).
			Padding(0, 1)
		label += activeStyle.Render(name)
	} else {
		inactiveStyle := lipgloss.NewStyle().
			Foreground(overlay0).
			Background(mantle).
			Padding(0, 1)
		label += inactiveStyle.Render(name)
	}

	// Modified indicator after the name.
	if entry.Modified {
		dotStyle := lipgloss.NewStyle().Foreground(yellow)
		label += dotStyle.Render(" \u25cf")
	}

	return label
}

// Render renders the tab bar as a single-line styled string that fits within
// the given width. activeNote is used to highlight the currently open note.
func (tb *TabBar) Render(width int, activeNote string) string {
	barBg := lipgloss.NewStyle().Background(crust).Foreground(overlay0)
	dividerStyle := lipgloss.NewStyle().Foreground(surface1)
	divider := dividerStyle.Render("\u2502")

	if len(tb.tabs) == 0 {
		return barBg.Width(width).Render(strings.Repeat(" ", width))
	}

	// Ensure activeIdx is consistent with activeNote.
	if activeNote != "" {
		if idx := tb.findTab(activeNote); idx >= 0 {
			tb.activeIdx = idx
		}
	}

	// Render tabs one by one, tracking total width.
	var rendered []string
	totalWidth := 0
	hiddenCount := 0

	// Pre-compute the overflow indicator so we know how much space to reserve.
	// We will use a placeholder like "... +3" which we compute once we know
	// the hidden count, but we need at least a rough budget.
	overflowBudget := 8 // enough for " ... +NN"

	for i, entry := range tb.tabs {
		isActive := i == tb.activeIdx
		tabStr := tbRenderTab(entry, isActive)
		tabWidth := lipgloss.Width(tabStr)

		// Account for divider before this tab (except the first).
		extraWidth := 0
		if len(rendered) > 0 {
			extraWidth = lipgloss.Width(divider)
		}

		needed := tabWidth + extraWidth
		// Check whether this tab fits. Always include the active tab.
		if totalWidth+needed > width-overflowBudget && !isActive && len(rendered) > 0 {
			hiddenCount++
			continue
		}

		if len(rendered) > 0 {
			rendered = append(rendered, divider)
			totalWidth += extraWidth
		}
		rendered = append(rendered, tabStr)
		totalWidth += tabWidth
	}

	content := strings.Join(rendered, "")

	// Append overflow indicator if any tabs were hidden.
	if hiddenCount > 0 {
		overflowStyle := lipgloss.NewStyle().Foreground(overlay0).Background(crust)
		overflow := overflowStyle.Render(" \u2026 +" + tbItoa(hiddenCount))
		content += overflow
		totalWidth += lipgloss.Width(overflow)
	}

	contentWidth := lipgloss.Width(content)

	// Pad to fill the full width.
	if contentWidth < width {
		gap := width - contentWidth
		content += barBg.Render(strings.Repeat(" ", gap))
	}

	return barBg.Width(width).Render(content)
}

// tbItoa is a minimal int-to-string without importing strconv.
func tbItoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}
