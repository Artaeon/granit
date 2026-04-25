package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// TabKind discriminates between note tabs (carry a vault note
// path; render the editor) and feature tabs (carry a feature ID;
// render that feature's view in place of the editor). New
// callers default to TabKindNote so legacy code paths stay
// unchanged.
type TabKind uint8

const (
	TabKindNote TabKind = iota
	TabKindFeature
)

// FeatureID is the stable identifier for an editor-tab-hosted
// feature (TaskManager, DailyJot, Calendar, etc.). One ID per
// feature globally; instances are still singleton-per-feature
// at v1 (one TaskManager tab open at a time across the app).
type FeatureID string

const (
	FeatTaskManager  FeatureID = "task_manager"
	FeatDailyJot     FeatureID = "daily_jot"
	FeatCalendar     FeatureID = "calendar"
	FeatKanban       FeatureID = "kanban"
	FeatGoals        FeatureID = "goals"
	FeatProject      FeatureID = "project"
	FeatGraph        FeatureID = "graph"
	FeatHabits       FeatureID = "habits"
)

// featureTabPath builds the synthetic "Path" string used by
// feature tabs so they round-trip through findTab/HasTab/etc.
// without changing those lookups. The "feat:" prefix is reserved
// — vault notes never produce this path — so collisions are
// impossible in practice.
func featureTabPath(id FeatureID) string {
	return "feat:" + string(id)
}

// TabEntry represents a single open tab in the tab bar.
//
// Kind discriminates note vs feature; for note tabs Path is the
// vault-relative note path, for feature tabs it is
// featureTabPath(id) ("feat:<id>").
//
// Label, when non-empty, overrides the auto-generated tab name
// (basename without extension). Used by feature tabs to display
// "Tasks" instead of "feat:task_manager".
type TabEntry struct {
	Path     string
	Kind     TabKind
	Label    string
	Modified bool // has unsaved changes
	Pinned   bool
}

// TabBar is a visual tab bar component that shows open/recent notes as
// clickable tabs at the top of the editor area. It is a helper component
// (not a tea.Model) — callers invoke its methods directly.
type TabBar struct {
	tabs          []TabEntry
	maxTabs       int // default 8
	activeIdx     int // which tab is currently active
	closedHistory []string
	scrollOffset  int
	moveHighlight int       // index of tab being moved (-1 = none)
	moveHighTime  time.Time // when highlight was set
}

// NewTabBar creates a TabBar with sensible defaults.
func NewTabBar() *TabBar {
	return &TabBar{
		maxTabs:       8,
		activeIdx:     -1,
		moveHighlight: -1,
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
				tb.pushClosed(t.Path)
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
	tb.pushClosed(tb.tabs[idx].Path)
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

// TogglePin toggles the pinned state of the active tab.
func (tb *TabBar) TogglePin() {
	if tb.activeIdx >= 0 && tb.activeIdx < len(tb.tabs) {
		tb.tabs[tb.activeIdx].Pinned = !tb.tabs[tb.activeIdx].Pinned
	}
}

// IsActiveTabPinned reports whether the active tab is pinned.
func (tb *TabBar) IsActiveTabPinned() bool {
	if tb.activeIdx >= 0 && tb.activeIdx < len(tb.tabs) {
		return tb.tabs[tb.activeIdx].Pinned
	}
	return false
}

// GetActive returns the path of the currently active tab, or an empty string.
func (tb *TabBar) GetActive() string {
	if tb.activeIdx >= 0 && tb.activeIdx < len(tb.tabs) {
		return tb.tabs[tb.activeIdx].Path
	}
	return ""
}

// ActiveIndex returns the active tab index.
func (tb *TabBar) ActiveIndex() int {
	return tb.activeIdx
}

// Count returns the number of open tabs.
func (tb *TabBar) Count() int {
	return len(tb.tabs)
}

// SwitchToIndex switches to the tab at index i (0-based). Returns the path
// of the activated tab or "" if the index is invalid.
func (tb *TabBar) SwitchToIndex(i int) string {
	if i < 0 || i >= len(tb.tabs) {
		return ""
	}
	tb.activeIdx = i
	return tb.tabs[i].Path
}

// NextTab cycles to the next tab and returns its path. Wraps around.
func (tb *TabBar) NextTab() string {
	if len(tb.tabs) == 0 {
		return ""
	}
	if tb.activeIdx < 0 {
		tb.activeIdx = 0
	} else {
		tb.activeIdx = (tb.activeIdx + 1) % len(tb.tabs)
	}
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
// Feature-tab APIs (Phase 4 — Obsidian-style editor tabs for non-note surfaces)
// ---------------------------------------------------------------------------

// AddFeatureTab adds (or switches to) a tab for the given feature.
// Singleton-per-feature semantics: a second AddFeatureTab(FeatTaskManager, ...)
// just activates the existing tab instead of opening a duplicate.
//
// label is the human-facing tab title (e.g. "Tasks", "Jot",
// "Calendar"). Empty falls back to the feature ID — useful as a
// safety net but every caller should pass a real label.
//
// Eviction: feature tabs participate in the same maxTabs cap as
// note tabs. Pinned tabs are kept; the oldest unpinned tab is
// evicted to make room. The closed tab (whether note or feature)
// is pushed to closedHistory.
func (tb *TabBar) AddFeatureTab(id FeatureID, label string) {
	path := featureTabPath(id)
	if idx := tb.findTab(path); idx >= 0 {
		tb.activeIdx = idx
		// Refresh label in case the caller passed an updated
		// string — keeps the rendered title in sync if a profile
		// changes its branding.
		if label != "" {
			tb.tabs[idx].Label = label
		}
		return
	}
	// Eviction (mirrors AddTab's logic).
	if len(tb.tabs) >= tb.maxTabs {
		evicted := false
		for i, t := range tb.tabs {
			if !t.Pinned {
				tb.pushClosed(t.Path)
				tb.tabs = append(tb.tabs[:i], tb.tabs[i+1:]...)
				if tb.activeIdx >= i && tb.activeIdx > 0 {
					tb.activeIdx--
				}
				evicted = true
				break
			}
		}
		if !evicted {
			return
		}
	}
	tb.tabs = append(tb.tabs, TabEntry{
		Path:  path,
		Kind:  TabKindFeature,
		Label: label,
	})
	tb.activeIdx = len(tb.tabs) - 1
}

// HasFeatureTab reports whether a tab for the given feature is
// currently open (regardless of whether it's the active one).
func (tb *TabBar) HasFeatureTab(id FeatureID) bool {
	return tb.HasTab(featureTabPath(id))
}

// SetActiveFeature activates the tab for the given feature, if
// one is open. No-op when the feature has no tab — caller should
// AddFeatureTab first.
func (tb *TabBar) SetActiveFeature(id FeatureID) {
	tb.SetActive(featureTabPath(id))
}

// ActiveEntry returns the currently active tab entry along with
// whether one exists. Use this when you need both the path/Kind
// and the label/pinned state — GetActive only returns the path.
func (tb *TabBar) ActiveEntry() (TabEntry, bool) {
	if tb.activeIdx < 0 || tb.activeIdx >= len(tb.tabs) {
		return TabEntry{}, false
	}
	return tb.tabs[tb.activeIdx], true
}

// ActiveFeature returns the currently active feature ID and
// true if the active tab is a feature tab. Returns "", false
// when the active tab is a note (or there's no active tab).
//
// Render and Update routing branch on this: feature tab → render
// that feature's view in the editor pane, route keys to it; note
// tab → existing editor behavior.
func (tb *TabBar) ActiveFeature() (FeatureID, bool) {
	e, ok := tb.ActiveEntry()
	if !ok || e.Kind != TabKindFeature {
		return "", false
	}
	return FeatureID(strings.TrimPrefix(e.Path, "feat:")), true
}

// CloseFeatureTab closes the tab for the given feature, if any.
// Wraps RemoveTab + the synthetic feature path. Returns true if
// a tab was actually closed (false when the feature wasn't open
// or its tab was pinned).
func (tb *TabBar) CloseFeatureTab(id FeatureID) bool {
	path := featureTabPath(id)
	idx := tb.findTab(path)
	if idx < 0 || tb.tabs[idx].Pinned {
		return false
	}
	tb.RemoveTab(path)
	return true
}

// MoveLeft moves the active tab one position to the left. Returns true if moved.
func (tb *TabBar) MoveLeft() bool {
	if tb.activeIdx <= 0 || len(tb.tabs) < 2 {
		return false
	}
	tb.tabs[tb.activeIdx], tb.tabs[tb.activeIdx-1] = tb.tabs[tb.activeIdx-1], tb.tabs[tb.activeIdx]
	tb.activeIdx--
	tb.moveHighlight = tb.activeIdx
	tb.moveHighTime = time.Now()
	return true
}

// MoveRight moves the active tab one position to the right. Returns true if moved.
func (tb *TabBar) MoveRight() bool {
	if tb.activeIdx < 0 || tb.activeIdx >= len(tb.tabs)-1 || len(tb.tabs) < 2 {
		return false
	}
	tb.tabs[tb.activeIdx], tb.tabs[tb.activeIdx+1] = tb.tabs[tb.activeIdx+1], tb.tabs[tb.activeIdx]
	tb.activeIdx++
	tb.moveHighlight = tb.activeIdx
	tb.moveHighTime = time.Now()
	return true
}

// CloseActive closes the active tab (unless pinned) and returns the path of
// the new active tab, or "" if no tabs remain.
func (tb *TabBar) CloseActive() string {
	if tb.activeIdx < 0 || tb.activeIdx >= len(tb.tabs) {
		return ""
	}
	if tb.tabs[tb.activeIdx].Pinned {
		return tb.tabs[tb.activeIdx].Path
	}
	tb.pushClosed(tb.tabs[tb.activeIdx].Path)
	tb.tabs = append(tb.tabs[:tb.activeIdx], tb.tabs[tb.activeIdx+1:]...)
	if len(tb.tabs) == 0 {
		tb.activeIdx = -1
		return ""
	}
	if tb.activeIdx >= len(tb.tabs) {
		tb.activeIdx = len(tb.tabs) - 1
	}
	return tb.tabs[tb.activeIdx].Path
}

// ---------------------------------------------------------------------------
// Closed tab history
// ---------------------------------------------------------------------------

// pushClosed adds a path to the closed history stack (max 5, LIFO).
func (tb *TabBar) pushClosed(path string) {
	if path == "" {
		return
	}
	// Remove if already in history to avoid duplicates
	for i, p := range tb.closedHistory {
		if p == path {
			tb.closedHistory = append(tb.closedHistory[:i], tb.closedHistory[i+1:]...)
			break
		}
	}
	tb.closedHistory = append(tb.closedHistory, path)
	if len(tb.closedHistory) > 5 {
		tb.closedHistory = tb.closedHistory[len(tb.closedHistory)-5:]
	}
}

// ReopenLast pops the most recently closed tab path from history.
// Returns "" if there is nothing to reopen.
func (tb *TabBar) ReopenLast() string {
	if len(tb.closedHistory) == 0 {
		return ""
	}
	last := tb.closedHistory[len(tb.closedHistory)-1]
	tb.closedHistory = tb.closedHistory[:len(tb.closedHistory)-1]
	return last
}

// ---------------------------------------------------------------------------
// Close multiple tabs
// ---------------------------------------------------------------------------

// CloseOthers closes all tabs except the active one (pinned tabs are kept).
func (tb *TabBar) CloseOthers() {
	if tb.activeIdx < 0 || tb.activeIdx >= len(tb.tabs) {
		return
	}
	active := tb.tabs[tb.activeIdx]
	var kept []TabEntry
	for i, t := range tb.tabs {
		if i == tb.activeIdx {
			kept = append(kept, t)
		} else if t.Pinned {
			kept = append(kept, t)
		} else {
			tb.pushClosed(t.Path)
		}
	}
	tb.tabs = kept
	// Find the active tab's new index
	tb.activeIdx = 0
	for i, t := range tb.tabs {
		if t.Path == active.Path {
			tb.activeIdx = i
			break
		}
	}
}

// CloseToRight closes all unpinned tabs to the right of the active one.
func (tb *TabBar) CloseToRight() {
	if tb.activeIdx < 0 || tb.activeIdx >= len(tb.tabs)-1 {
		return
	}
	var kept []TabEntry
	kept = append(kept, tb.tabs[:tb.activeIdx+1]...)
	for _, t := range tb.tabs[tb.activeIdx+1:] {
		if t.Pinned {
			kept = append(kept, t)
		} else {
			tb.pushClosed(t.Path)
		}
	}
	tb.tabs = kept
	if tb.activeIdx >= len(tb.tabs) {
		tb.activeIdx = len(tb.tabs) - 1
	}
}

// CloseAll closes every tab (including pinned) and resets the tab bar.
func (tb *TabBar) CloseAll() {
	for _, t := range tb.tabs {
		tb.pushClosed(t.Path)
	}
	tb.tabs = nil
	tb.activeIdx = -1
}

// ---------------------------------------------------------------------------
// Tab scroll
// ---------------------------------------------------------------------------

// ScrollLeft scrolls the visible tab window one position left.
func (tb *TabBar) ScrollLeft() {
	if tb.scrollOffset > 0 {
		tb.scrollOffset--
	}
}

// ScrollRight scrolls the visible tab window one position right.
func (tb *TabBar) ScrollRight() {
	maxOffset := len(tb.tabs) - 1
	if maxOffset < 0 {
		maxOffset = 0
	}
	if tb.scrollOffset < maxOffset {
		tb.scrollOffset++
	}
}


// ---------------------------------------------------------------------------
// Persistence
// ---------------------------------------------------------------------------

// tabSessionData is the JSON schema for persisting open tabs.
//
// Schema is forward-additive: the original v1 fields (Tabs,
// Active, Pinned) remain authoritative. Kinds and Labels are
// parallel slices added in the tab-feature work; missing fields
// in older tabs.json files unmarshal as nil and every tab
// defaults to TabKindNote with no label override. New writes
// always include the new fields.
type tabSessionData struct {
	Tabs   []string `json:"tabs"`
	Active int      `json:"active"`
	Pinned []int    `json:"pinned"`
	Kinds  []int    `json:"kinds,omitempty"`  // parallel to Tabs; missing/short → TabKindNote
	Labels []string `json:"labels,omitempty"` // parallel to Tabs; missing/short → ""
}

// SaveTabs persists open tabs to <vaultPath>/.granit/tabs.json.
func (tb *TabBar) SaveTabs(vaultPath string) {
	if vaultPath == "" {
		return
	}
	dir := filepath.Join(vaultPath, ".granit")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}

	var paths []string
	var pinned []int
	kinds := make([]int, len(tb.tabs))
	labels := make([]string, len(tb.tabs))
	for i, t := range tb.tabs {
		paths = append(paths, t.Path)
		kinds[i] = int(t.Kind)
		labels[i] = t.Label
		if t.Pinned {
			pinned = append(pinned, i)
		}
	}

	data := tabSessionData{
		Tabs:   paths,
		Active: tb.activeIdx,
		Pinned: pinned,
		Kinds:  kinds,
		Labels: labels,
	}

	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return
	}
	_ = atomicWriteState(filepath.Join(dir, "tabs.json"), raw)
}

// LoadTabs restores tabs from <vaultPath>/.granit/tabs.json.
// It only adds tabs whose paths are present in validPaths (to skip deleted notes).
func (tb *TabBar) LoadTabs(vaultPath string, validPaths map[string]bool) {
	if vaultPath == "" {
		return
	}
	fp := filepath.Join(vaultPath, ".granit", "tabs.json")
	raw, err := os.ReadFile(fp)
	if err != nil {
		return
	}

	var data tabSessionData
	if err := json.Unmarshal(raw, &data); err != nil {
		// Corrupted JSON — delete the file and start with clean tabs.
		_ = os.Remove(fp)
		return
	}

	// Validate activeIdx is not negative.
	if data.Active < 0 {
		data.Active = 0
	}

	// Build pinned set, filtering out-of-bounds indices.
	pinnedSet := make(map[int]bool)
	for _, idx := range data.Pinned {
		if idx >= 0 && idx < len(data.Tabs) {
			pinnedSet[idx] = true
		}
	}

	// Clear existing tabs
	tb.tabs = nil
	tb.activeIdx = -1

	for i, p := range data.Tabs {
		// Determine kind first so we can decide whether to
		// validate against vault paths. Feature tabs aren't in
		// validPaths and shouldn't be filtered out.
		kind := TabKindNote
		if i < len(data.Kinds) {
			kind = TabKind(data.Kinds[i])
		}
		if kind == TabKindNote && validPaths != nil && !validPaths[p] {
			continue
		}
		label := ""
		if i < len(data.Labels) {
			label = data.Labels[i]
		}
		tb.tabs = append(tb.tabs, TabEntry{
			Path:   p,
			Kind:   kind,
			Label:  label,
			Pinned: pinnedSet[i],
		})
	}

	if len(tb.tabs) > 0 {
		tb.activeIdx = data.Active
		if tb.activeIdx < 0 || tb.activeIdx >= len(tb.tabs) {
			tb.activeIdx = 0
		}
	}
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

// tbTruncName truncates a name to maxLen characters, appending "..." if needed.
func tbTruncName(name string, maxLen int) string {
	runes := []rune(name)
	if len(runes) <= maxLen {
		return name
	}
	return string(runes[:maxLen-1]) + "\u2026"
}

// tbRenderTab renders a single tab label with the appropriate styling.
// Feature tabs use the explicit Label field so the user sees
// "Tasks" instead of the synthetic "feat:task_manager" path.
func tbRenderTab(entry TabEntry, isActive bool, isMoving bool) string {
	maxName := 14
	if entry.Pinned || entry.Modified {
		maxName -= 2
	}
	display := entry.Label
	if display == "" {
		display = tbBaseName(entry.Path)
	}
	name := tbTruncName(display, maxName)

	var parts []string

	// Pin indicator
	if entry.Pinned {
		pinIcon := lipgloss.NewStyle().Foreground(peach).Render("◆")
		parts = append(parts, pinIcon)
	}

	// Tab name with active/inactive/moving styling
	if isMoving {
		// Moving tab: highlight with peach background
		nameStyled := lipgloss.NewStyle().
			Foreground(crust).
			Background(peach).
			Bold(true).
			Render(name)
		parts = append(parts, nameStyled)
	} else if isActive {
		// Active tab: bright text with accent underline effect
		nameStyled := lipgloss.NewStyle().
			Foreground(mauve).
			Background(surface0).
			Bold(true).
			Render(name)
		parts = append(parts, nameStyled)
	} else {
		nameStyled := lipgloss.NewStyle().
			Foreground(overlay0).
			Render(name)
		parts = append(parts, nameStyled)
	}

	// Modified indicator (dot)
	if entry.Modified {
		dotStyle := lipgloss.NewStyle().Foreground(yellow).Bold(true)
		parts = append(parts, dotStyle.Render("●"))
	}

	// Close indicator for non-pinned tabs
	if !entry.Pinned {
		closeStyle := lipgloss.NewStyle().Foreground(surface2)
		parts = append(parts, closeStyle.Render("x"))
	}

	content := strings.Join(parts, " ")

	// Wrap in padding
	if isMoving {
		return lipgloss.NewStyle().
			Background(peach).
			Padding(0, 1).
			Render(content)
	}
	if isActive {
		return lipgloss.NewStyle().
			Background(surface0).
			Padding(0, 1).
			Render(content)
	}
	return lipgloss.NewStyle().
		Padding(0, 1).
		Render(content)
}

// Render renders the tab bar as a two-line styled string: tabs on top,
// accent underline on bottom. activeNote highlights the currently open note.
func (tb *TabBar) Render(width int, activeNote string) string {
	barBg := lipgloss.NewStyle().Background(crust).Foreground(overlay0)

	if len(tb.tabs) == 0 {
		emptyLine := barBg.Width(width).Render("")
		underline := lipgloss.NewStyle().Foreground(surface0).Width(width).Render(strings.Repeat("─", width))
		return emptyLine + "\n" + underline
	}

	// Ensure activeIdx is consistent with activeNote.
	if activeNote != "" {
		if idx := tb.findTab(activeNote); idx >= 0 {
			tb.activeIdx = idx
		}
	}

	// Clear move highlight after 500ms
	if tb.moveHighlight >= 0 && time.Since(tb.moveHighTime) >= 500*time.Millisecond {
		tb.moveHighlight = -1
	}
	isHighlightActive := tb.moveHighlight >= 0

	// Pre-render all tabs to measure widths for scroll calculation
	type tabInfo struct {
		rendered string
		width    int
		isActive bool
		index    int
	}

	var allRendered []tabInfo
	for i, entry := range tb.tabs {
		isActive := i == tb.activeIdx
		isMoving := isHighlightActive && i == tb.moveHighlight
		tabStr := tbRenderTab(entry, isActive, isMoving)
		tabWidth := lipgloss.Width(tabStr)
		allRendered = append(allRendered, tabInfo{rendered: tabStr, width: tabWidth, isActive: isActive, index: i})
	}

	// Determine how many tabs fit in the available width
	scrollArrowBudget := 0
	if tb.scrollOffset > 0 {
		scrollArrowBudget += 3 // "< " prefix
	}

	// Calculate visible tabs from scrollOffset
	var visible []tabInfo
	totalWidth := scrollArrowBudget
	overflowBudget := 8
	for i := tb.scrollOffset; i < len(allRendered); i++ {
		needed := allRendered[i].width
		if totalWidth+needed > width-overflowBudget && len(visible) > 0 && !allRendered[i].isActive {
			break
		}
		visible = append(visible, allRendered[i])
		totalWidth += needed
		if totalWidth >= width-overflowBudget {
			break
		}
	}

	// Clamp scroll so active tab is visible
	if tb.activeIdx >= 0 {
		activeVisible := false
		for _, v := range visible {
			if v.index == tb.activeIdx {
				activeVisible = true
				break
			}
		}
		if !activeVisible {
			// Adjust scrollOffset to make active visible
			if tb.activeIdx < tb.scrollOffset {
				tb.scrollOffset = tb.activeIdx
			} else {
				// Scroll right until active is visible
				tb.scrollOffset = tb.activeIdx
			}
			// Re-render visible set
			visible = nil
			scrollArrowBudget = 0
			if tb.scrollOffset > 0 {
				scrollArrowBudget = 3
			}
			totalWidth = scrollArrowBudget
			for i := tb.scrollOffset; i < len(allRendered); i++ {
				needed := allRendered[i].width
				if totalWidth+needed > width-overflowBudget && len(visible) > 0 && !allRendered[i].isActive {
					break
				}
				visible = append(visible, allRendered[i])
				totalWidth += needed
				if totalWidth >= width-overflowBudget {
					break
				}
			}
		}
	}

	hiddenAfter := 0
	if len(visible) > 0 {
		lastVisibleIdx := visible[len(visible)-1].index
		hiddenAfter = len(tb.tabs) - lastVisibleIdx - 1
	}

	// Build tab line
	var tabLine strings.Builder

	// Left scroll indicator
	if tb.scrollOffset > 0 {
		scrollStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
		tabLine.WriteString(scrollStyle.Render("< "))
	}

	for _, t := range visible {
		tabLine.WriteString(t.rendered)
	}

	// Right overflow/scroll indicator
	if hiddenAfter > 0 {
		scrollStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
		tabLine.WriteString(scrollStyle.Render(" +" + tbItoa(hiddenAfter) + " >"))
	}

	// Pad to fill width
	content := tabLine.String()
	contentWidth := lipgloss.Width(content)
	if contentWidth < width {
		gap := width - contentWidth
		content += barBg.Render(strings.Repeat(" ", gap))
	}

	// Build underline: accent color under active tab, dim elsewhere
	var underLine strings.Builder
	accentStyle := lipgloss.NewStyle().Foreground(mauve)
	moveStyle := lipgloss.NewStyle().Foreground(peach)
	dimLineStyle := lipgloss.NewStyle().Foreground(surface0)

	// Account for left scroll indicator in underline
	if tb.scrollOffset > 0 {
		underLine.WriteString(dimLineStyle.Render("──"))
	}

	for _, t := range visible {
		if t.isActive {
			underLine.WriteString(accentStyle.Render(strings.Repeat("━", t.width)))
		} else if isHighlightActive && t.index == tb.moveHighlight {
			underLine.WriteString(moveStyle.Render(strings.Repeat("━", t.width)))
		} else {
			underLine.WriteString(dimLineStyle.Render(strings.Repeat("─", t.width)))
		}
	}
	ulWidth := lipgloss.Width(underLine.String())
	if ulWidth < width {
		underLine.WriteString(dimLineStyle.Render(strings.Repeat("─", width-ulWidth)))
	}

	return barBg.Width(width).Render(content) + "\n" + underLine.String()
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
