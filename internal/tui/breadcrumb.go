package tui

import (
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Breadcrumb implements breadcrumb navigation (note history trail) and pinned
// tabs. It is not an overlay; it renders a bar intended for the status area.
type Breadcrumb struct {
	history    []string // stack of visited note paths
	maxHistory int      // default 50
	position   int      // current position in history

	// Pinned tabs
	pinned    []string // paths of pinned notes
	maxPinned int      // default 5
}

// NewBreadcrumb creates a Breadcrumb with sensible defaults.
func NewBreadcrumb() *Breadcrumb {
	return &Breadcrumb{
		maxHistory: 50,
		maxPinned:  5,
	}
}

// Push adds a note to the history stack. If the user has navigated back and
// then opens a new note, forward history is truncated.
func (bc *Breadcrumb) Push(path string) {
	// Avoid duplicating the current entry.
	if bc.position >= 0 && bc.position < len(bc.history) && bc.history[bc.position] == path {
		return
	}

	// Truncate forward history when navigating to a new note after going back.
	if bc.position < len(bc.history)-1 {
		bc.history = bc.history[:bc.position+1]
	}

	bc.history = append(bc.history, path)

	// Enforce max history size by trimming the oldest entries.
	if len(bc.history) > bc.maxHistory {
		excess := len(bc.history) - bc.maxHistory
		bc.history = bc.history[excess:]
	}

	bc.position = len(bc.history) - 1
}

// Back moves one step back in history and returns the path. Returns an empty
// string if already at the start.
func (bc *Breadcrumb) Back() string {
	if !bc.CanGoBack() {
		return ""
	}
	bc.position--
	return bc.history[bc.position]
}

// Forward moves one step forward in history and returns the path. Returns an
// empty string if already at the end.
func (bc *Breadcrumb) Forward() string {
	if !bc.CanGoForward() {
		return ""
	}
	bc.position++
	return bc.history[bc.position]
}

// Current returns the current note in history, or an empty string if the
// history is empty.
func (bc *Breadcrumb) Current() string {
	if bc.position < 0 || bc.position >= len(bc.history) {
		return ""
	}
	return bc.history[bc.position]
}

// CanGoBack reports whether there is a previous entry in history.
func (bc *Breadcrumb) CanGoBack() bool {
	return bc.position > 0
}

// CanGoForward reports whether there is a next entry in history.
func (bc *Breadcrumb) CanGoForward() bool {
	return bc.position < len(bc.history)-1
}

// Trail returns the last maxLen items ending at (and including) the current
// position, suitable for display.
func (bc *Breadcrumb) Trail(maxLen int) []string {
	if len(bc.history) == 0 || bc.position < 0 {
		return nil
	}
	end := bc.position + 1 // exclusive upper bound
	start := end - maxLen
	if start < 0 {
		start = 0
	}
	trail := make([]string, end-start)
	copy(trail, bc.history[start:end])
	return trail
}

// Pin adds a note path to the pinned tabs. If the note is already pinned it
// is a no-op. Pinned tabs are capped at maxPinned.
func (bc *Breadcrumb) Pin(path string) {
	if bc.IsPinned(path) {
		return
	}
	if len(bc.pinned) >= bc.maxPinned {
		return
	}
	bc.pinned = append(bc.pinned, path)
}

// Unpin removes a note path from the pinned tabs.
func (bc *Breadcrumb) Unpin(path string) {
	for i, p := range bc.pinned {
		if p == path {
			bc.pinned = append(bc.pinned[:i], bc.pinned[i+1:]...)
			return
		}
	}
}

// IsPinned reports whether the given path is currently pinned.
func (bc *Breadcrumb) IsPinned(path string) bool {
	for _, p := range bc.pinned {
		if p == path {
			return true
		}
	}
	return false
}

// Pinned returns the list of pinned note paths.
func (bc *Breadcrumb) Pinned() []string {
	out := make([]string, len(bc.pinned))
	copy(out, bc.pinned)
	return out
}

// RenderBar renders the breadcrumb bar with pinned tabs and the navigation
// trail as a styled string that fits within the given width.
//
// Layout:
//
//	pinned_tab | pinned_tab  <-  Crumb > Crumb > Current Note
func (bc *Breadcrumb) RenderBar(width int, activeNote string) string {
	// --- Styles ---
	pinnedStyle := lipgloss.NewStyle().Foreground(text).Background(surface0).Padding(0, 1)
	pinnedActiveStyle := lipgloss.NewStyle().Foreground(mauve).Background(surface1).Bold(true).Padding(0, 1)
	separatorStyle := lipgloss.NewStyle().Foreground(overlay0)
	crumbDimStyle := lipgloss.NewStyle().Foreground(overlay0)
	crumbCurrentStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	navStyle := lipgloss.NewStyle().Foreground(overlay0)
	barBg := lipgloss.NewStyle().Background(crust).Foreground(overlay0)

	var parts []string

	// --- Pinned tabs ---
	if len(bc.pinned) > 0 {
		pinIcon := lipgloss.NewStyle().Foreground(peach).Render("\u2759") // pin icon
		var tabs []string
		for _, p := range bc.pinned {
			name := noteName(p)
			if p == activeNote {
				tabs = append(tabs, pinnedActiveStyle.Render(name))
			} else {
				tabs = append(tabs, pinnedStyle.Render(name))
			}
		}
		pinnedSection := pinIcon + " " + strings.Join(tabs, separatorStyle.Render(" | "))
		parts = append(parts, pinnedSection)
	}

	// --- Navigation arrows ---
	navArrows := ""
	if bc.CanGoBack() {
		navArrows += "\u2190" // left arrow
	}
	if bc.CanGoForward() {
		if navArrows != "" {
			navArrows += " "
		}
		navArrows += "\u2192" // right arrow
	}
	if navArrows != "" {
		parts = append(parts, navStyle.Render(navArrows))
	}

	// --- Breadcrumb trail ---
	trail := bc.Trail(5)
	if len(trail) > 0 {
		var crumbs []string
		for i, p := range trail {
			name := noteName(p)
			if i == len(trail)-1 {
				// Current note
				crumbs = append(crumbs, crumbCurrentStyle.Render(name))
			} else {
				crumbs = append(crumbs, crumbDimStyle.Render(name))
			}
		}
		crumbSection := strings.Join(crumbs, separatorStyle.Render(" > "))
		parts = append(parts, crumbSection)
	}

	content := strings.Join(parts, "  ")
	contentWidth := lipgloss.Width(content)

	// Pad or truncate to fill width.
	if contentWidth < width {
		gap := width - contentWidth
		content = content + barBg.Render(strings.Repeat(" ", gap))
	}

	return barBg.Width(width).Render(content)
}

// noteName extracts a display-friendly name from a file path: the base name
// without extension.
func noteName(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	if ext != "" {
		base = strings.TrimSuffix(base, ext)
	}
	return base
}

// renderBreadcrumb renders a folder-path breadcrumb for the active note.
// Example output: "  vault > Research > AI > transformers"
// The last segment (filename without .md) is bold mauve, folder segments use
// overlay0, and the root "vault" segment uses blue. If the rendered string
// exceeds the available width, segments are truncated from the left and
// replaced with "...". Returns an empty string if activeNote is empty.
func renderBreadcrumb(activeNote string, width int) string {
	if activeNote == "" {
		return ""
	}

	// Styles
	sepStyle := lipgloss.NewStyle().Foreground(overlay0)
	folderStyle := lipgloss.NewStyle().Foreground(overlay0)
	rootStyle := lipgloss.NewStyle().Foreground(blue)
	fileStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	bgStyle := lipgloss.NewStyle().Background(surface0).Width(width)

	sep := sepStyle.Render(" > ")
	sepWidth := lipgloss.Width(sep)

	// Split the path into segments: folders + filename
	clean := filepath.ToSlash(activeNote)
	parts := strings.Split(clean, "/")

	// Build segments: ["vault", ...folders..., filename]
	var segments []string
	segments = append(segments, "vault")
	for i, p := range parts {
		if i < len(parts)-1 {
			// folder segment
			segments = append(segments, p)
		} else {
			// filename: strip .md extension
			name := strings.TrimSuffix(p, ".md")
			segments = append(segments, name)
		}
	}

	// Render each segment with appropriate styling
	renderSegment := func(i int, s string) string {
		if i == 0 {
			return rootStyle.Render(s)
		}
		if i == len(segments)-1 {
			return fileStyle.Render(s)
		}
		return folderStyle.Render(s)
	}

	prefix := "  " // left padding
	prefixWidth := 2

	// Try full render first
	var rendered []string
	totalWidth := prefixWidth
	for i, s := range segments {
		r := renderSegment(i, s)
		rendered = append(rendered, r)
		totalWidth += lipgloss.Width(r)
		if i < len(segments)-1 {
			totalWidth += sepWidth
		}
	}

	if totalWidth <= width {
		line := prefix + strings.Join(rendered, sep)
		return bgStyle.Render(line)
	}

	// Truncate from the left: drop leading segments and replace with "..."
	ellipsis := folderStyle.Render("...")
	ellipsisWidth := lipgloss.Width(ellipsis)

	for start := 1; start < len(segments)-1; start++ {
		var truncRendered []string
		truncRendered = append(truncRendered, ellipsis)
		tw := prefixWidth + ellipsisWidth + sepWidth

		for i := start; i < len(segments); i++ {
			r := renderSegment(i, segments[i])
			truncRendered = append(truncRendered, r)
			tw += lipgloss.Width(r)
			if i < len(segments)-1 {
				tw += sepWidth
			}
		}

		if tw <= width {
			line := prefix + strings.Join(truncRendered, sep)
			return bgStyle.Render(line)
		}
	}

	// Last resort: just "... > filename"
	fileName := renderSegment(len(segments)-1, segments[len(segments)-1])
	line := prefix + strings.Join([]string{ellipsis, fileName}, sep)
	return bgStyle.Render(line)
}
