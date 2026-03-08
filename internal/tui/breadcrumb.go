package tui

import (
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
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

	// Select mode — lets the user navigate breadcrumb segments with arrow keys
	selectMode      bool
	selectedSegment int      // index into selectSegments
	selectSegments  []string // display segments (e.g. ["vault", "Projects", "web-app", "README"])
	selectActiveNote string  // the activeNote path used to build selectSegments
	selectedFolder  string   // consumed-once: full path selected by the user
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

// ---------------------------------------------------------------------------
// Select-mode: navigate breadcrumb segments with arrow keys
// ---------------------------------------------------------------------------

// breadcrumbSegments builds the display segments for a given activeNote path.
// The result mirrors what renderBreadcrumb produces:
// ["vault", ...folder parts..., filename_without_ext].
func breadcrumbSegments(activeNote string) []string {
	if activeNote == "" {
		return nil
	}
	clean := filepath.ToSlash(activeNote)
	parts := strings.Split(clean, "/")

	var segments []string
	segments = append(segments, "vault")
	for i, p := range parts {
		if i < len(parts)-1 {
			segments = append(segments, p)
		} else {
			segments = append(segments, strings.TrimSuffix(p, ".md"))
		}
	}
	return segments
}

// EnterSelectMode activates breadcrumb select mode and positions the cursor
// on the last segment (the current note).
func (bc *Breadcrumb) EnterSelectMode(activeNote string) {
	segs := breadcrumbSegments(activeNote)
	if len(segs) == 0 {
		return
	}
	bc.selectMode = true
	bc.selectSegments = segs
	bc.selectActiveNote = activeNote
	bc.selectedSegment = len(segs) - 1
	bc.selectedFolder = ""
}

// IsSelectMode reports whether the breadcrumb is in segment-select mode.
func (bc Breadcrumb) IsSelectMode() bool {
	return bc.selectMode
}

// ExitSelectMode deactivates select mode without choosing a segment.
func (bc *Breadcrumb) ExitSelectMode() {
	bc.selectMode = false
	bc.selectedSegment = 0
	bc.selectSegments = nil
	bc.selectActiveNote = ""
	bc.selectedFolder = ""
}

// UpdateSelect handles keyboard input while in select mode.
// Left/right arrows move between segments; enter confirms the selection and
// returns the relative folder path (or the note path for the last segment);
// esc exits select mode. The returned string is the selected path (empty when
// not confirmed) and the bool indicates whether the selection was confirmed.
func (bc *Breadcrumb) UpdateSelect(msg tea.KeyMsg) (string, bool) {
	if !bc.selectMode || len(bc.selectSegments) == 0 {
		return "", false
	}

	switch msg.String() {
	case "left", "h":
		if bc.selectedSegment > 0 {
			bc.selectedSegment--
		}
	case "right", "l":
		if bc.selectedSegment < len(bc.selectSegments)-1 {
			bc.selectedSegment++
		}
	case "enter":
		folder := bc.segmentPath(bc.selectedSegment)
		bc.selectedFolder = folder
		bc.selectMode = false
		return folder, true
	case "esc":
		bc.ExitSelectMode()
		return "", false
	}
	return "", false
}

// segmentPath maps a segment index back to a vault-relative path.
//   - Segment 0 ("vault") → "" (vault root)
//   - Segment 1..n-2 (folders) → joined folder path (e.g. "Projects/web-app")
//   - Segment n-1 (filename) → the full activeNote path as-is
func (bc *Breadcrumb) segmentPath(idx int) string {
	if idx <= 0 {
		// Vault root
		return ""
	}
	if idx >= len(bc.selectSegments)-1 {
		// Last segment is the note itself
		return bc.selectActiveNote
	}
	// Folder segments: segments[1..idx] joined
	clean := filepath.ToSlash(bc.selectActiveNote)
	parts := strings.Split(clean, "/")
	// parts[0..idx-1] are the folder components (segment 1 = parts[0], etc.)
	if idx > len(parts) {
		return bc.selectActiveNote
	}
	return strings.Join(parts[:idx], "/")
}

// ViewWithSelect renders the folder-path breadcrumb with the currently
// selected segment highlighted in mauve+bold+underline. Non-selected segments
// use subtext0 for text and surface1 for separator arrows. The output matches
// the same layout as renderBreadcrumb.
func (bc *Breadcrumb) ViewWithSelect(width int) string {
	if len(bc.selectSegments) == 0 {
		return ""
	}

	// Styles
	selectedStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true).Underline(true)
	normalStyle := lipgloss.NewStyle().Foreground(subtext0)
	sepStyle := lipgloss.NewStyle().Foreground(surface1)
	bgStyle := lipgloss.NewStyle().Background(surface0).Width(width)

	sep := sepStyle.Render(" > ")
	sepWidth := lipgloss.Width(sep)

	prefix := "  "
	prefixWidth := 2

	renderSeg := func(i int, s string) string {
		if i == bc.selectedSegment {
			return selectedStyle.Render(s)
		}
		return normalStyle.Render(s)
	}

	// Full render attempt
	var rendered []string
	totalWidth := prefixWidth
	for i, s := range bc.selectSegments {
		r := renderSeg(i, s)
		rendered = append(rendered, r)
		totalWidth += lipgloss.Width(r)
		if i < len(bc.selectSegments)-1 {
			totalWidth += sepWidth
		}
	}

	if totalWidth <= width {
		line := prefix + strings.Join(rendered, sep)
		return bgStyle.Render(line)
	}

	// Truncate from the left, keeping the selected segment visible
	ellipsis := normalStyle.Render("...")
	ellipsisWidth := lipgloss.Width(ellipsis)

	for start := 1; start < len(bc.selectSegments)-1; start++ {
		var truncRendered []string
		truncRendered = append(truncRendered, ellipsis)
		tw := prefixWidth + ellipsisWidth + sepWidth

		for i := start; i < len(bc.selectSegments); i++ {
			r := renderSeg(i, bc.selectSegments[i])
			truncRendered = append(truncRendered, r)
			tw += lipgloss.Width(r)
			if i < len(bc.selectSegments)-1 {
				tw += sepWidth
			}
		}

		if tw <= width {
			line := prefix + strings.Join(truncRendered, sep)
			return bgStyle.Render(line)
		}
	}

	// Last resort: "... > last_segment"
	last := renderSeg(len(bc.selectSegments)-1, bc.selectSegments[len(bc.selectSegments)-1])
	line := prefix + strings.Join([]string{ellipsis, last}, sep)
	return bgStyle.Render(line)
}

// GetSelectedFolder returns the path selected by the user via enter in select
// mode. It is consumed once: the value is cleared after the first read.
// The bool is true only when a valid selection is available.
func (bc *Breadcrumb) GetSelectedFolder() (string, bool) {
	if bc.selectedFolder == "" && !bc.selectMode {
		return "", false
	}
	if bc.selectedFolder == "" {
		return "", false
	}
	folder := bc.selectedFolder
	bc.selectedFolder = ""
	return folder, true
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
