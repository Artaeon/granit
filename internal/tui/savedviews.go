package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/objects"
)

// SavedViews is the editor-tab surface for a single saved view. The same
// component instance hosts whichever view the user picked — opening a
// different view re-targets it (singleton-per-tab semantics, mirroring the
// FeatSheetView pattern).
//
// Lifecycle:
//
//   - NewSavedViews() builds the zero value.
//   - Open(catalog, idx, viewID) loads a specific view and re-evaluates the
//     candidate set against the current objects index.
//   - Refresh(idx) re-evaluates against a fresh index without changing the
//     view selection (used when the vault re-scans).
//   - View() renders the list. Update() handles keys.
//
// Keyboard surface (in tab mode):
//
//	j/k or arrows — move cursor
//	Enter         — open the selected object's note
//	g / G         — jump to top / bottom
//	r             — refresh (re-evaluate against current index)
//	Esc           — request close (host clears the tab)
//
// The component never reads from disk: registry/index/catalog are injected.
type SavedViews struct {
	OverlayBase

	catalog  *objects.ViewCatalog
	index    *objects.Index
	registry *objects.Registry // optional; needed for in-tab object creation

	currentID string
	currentV  objects.View
	matches   []*objects.Object

	cursor int
	scroll int

	// Picker mode: when true, the tab renders the catalog list instead
	// of a resolved view. Pressing Enter on a catalog row loads that
	// view; pressing `p` from inside a loaded view returns to the picker.
	pickerMode   bool
	pickerCursor int

	// Consumed-once jump request, mirrors ObjectBrowser pattern.
	jumpPath string
	jumpReq  bool

	// Inline create-object prompt, mirrors ObjectBrowser. When the
	// view targets a specific Type, pressing 'n' opens this so the
	// user can capture a new instance without leaving the view.
	creating  bool
	createBuf string

	// Consumed-once create request: NotePath for the host to write
	// + content body. Cleared after the host reads.
	createPath    string
	createContent string
	createReq     bool

	// Inline result filter: '/' enters filter mode, characters narrow
	// the visible match list by title contains-substring (case
	// insensitive). Composes with the view's where clause — the where
	// clause runs first, the filter then narrows the result. Esc clears.
	filterMode bool
	filterBuf  string

	// Delete confirmation, mirrors ObjectBrowser. y commits, n/Esc
	// cancels. Two-step gate so a stray keystroke doesn't nuke a note.
	confirmingDelete bool
	deletePath       string
	deleteReq        bool

	// Empty until a view has been opened. Empty also when a view is open
	// but yields zero matches — the renderer distinguishes "no view loaded"
	// from "view loaded, no results" via currentID.
	loaded bool
}

// NewSavedViews constructs a fresh, inactive SavedViews surface.
func NewSavedViews() SavedViews { return SavedViews{} }

// SetRegistry feeds the typed-objects registry so the in-tab create
// flow (the 'n' key) can resolve the active view's Type into a Folder
// + FilenamePattern + property schema. Optional — without a registry,
// 'n' is a no-op.
func (s *SavedViews) SetRegistry(r *objects.Registry) {
	s.registry = r
}

// OpenPicker activates the saved-views tab in catalog-picker mode. The user
// then chooses a view from the list; Enter loads it. Used when the host
// has no specific view ID to jump to (the common Alt+v entry path).
func (s *SavedViews) OpenPicker(catalog *objects.ViewCatalog, idx *objects.Index) {
	s.Activate()
	s.catalog = catalog
	s.index = idx
	s.pickerMode = true
	s.pickerCursor = 0
	s.loaded = false
	s.currentID = ""
	s.matches = nil
	s.cursor = 0
	s.scroll = 0
}

// Open loads the named view from the catalog and re-evaluates it against the
// given index. Returns an error when the view ID isn't in the catalog —
// callers should fall back to the picker rather than silently no-op.
func (s *SavedViews) Open(catalog *objects.ViewCatalog, idx *objects.Index, viewID string) error {
	if catalog == nil {
		return fmt.Errorf("saved views: nil catalog")
	}
	v, ok := catalog.ByID(viewID)
	if !ok {
		return fmt.Errorf("saved views: view %q not found", viewID)
	}
	s.Activate()
	s.catalog = catalog
	s.index = idx
	s.currentID = viewID
	s.currentV = v
	s.matches = objects.Evaluate(idx, v)
	s.cursor = 0
	s.scroll = 0
	s.loaded = true
	s.pickerMode = false
	return nil
}

// Refresh re-evaluates the current view against a (potentially new) index.
// No-op when no view is loaded.
func (s *SavedViews) Refresh(idx *objects.Index) {
	if !s.loaded {
		return
	}
	s.index = idx
	s.matches = objects.Evaluate(idx, s.currentV)
	if s.cursor >= len(s.matches) {
		s.cursor = 0
		s.scroll = 0
	}
}

// CurrentViewID returns the ID of the loaded view ("" when none).
func (s *SavedViews) CurrentViewID() string { return s.currentID }

// CurrentLabel returns the user-facing label for the active tab.
func (s *SavedViews) CurrentLabel() string {
	if !s.loaded {
		return "Saved View"
	}
	return s.currentV.Name
}

// GetJumpRequest returns the path the user pressed Enter on, then clears it
// so the host only acts once. Mirrors ObjectBrowser.GetJumpRequest.
func (s *SavedViews) GetJumpRequest() (string, bool) {
	if !s.jumpReq {
		return "", false
	}
	p := s.jumpPath
	s.jumpReq = false
	s.jumpPath = ""
	return p, true
}

// GetCreateRequest returns (path, content, ok) once after the user
// commits the inline new-object prompt. Mirrors the ObjectBrowser API
// so the host's create handler is the same.
func (s *SavedViews) GetCreateRequest() (string, string, bool) {
	if !s.createReq {
		return "", "", false
	}
	p, c := s.createPath, s.createContent
	s.createReq = false
	s.createPath = ""
	s.createContent = ""
	return p, c, true
}

// GetDeleteRequest returns (path, ok) once after the user confirms a
// delete. Consumed-once.
func (s *SavedViews) GetDeleteRequest() (string, bool) {
	if !s.deleteReq {
		return "", false
	}
	p := s.deletePath
	s.deleteReq = false
	s.deletePath = ""
	return p, true
}

// visibleMatches returns the matches narrowed by the active filter.
// When the filter is empty, returns the full match list.
func (s *SavedViews) visibleMatches() []*objects.Object {
	if strings.TrimSpace(s.filterBuf) == "" {
		return s.matches
	}
	q := strings.ToLower(s.filterBuf)
	out := make([]*objects.Object, 0, len(s.matches))
	for _, obj := range s.matches {
		if strings.Contains(strings.ToLower(obj.Title), q) {
			out = append(out, obj)
		}
	}
	return out
}

// Update handles a key while the SavedViews tab is in focus. Two key
// surfaces depending on mode:
//
//	picker mode → j/k navigate catalog, Enter loads the cursored view
//	view mode   → j/k navigate matches, Enter opens note, p re-opens picker
func (s *SavedViews) Update(msg tea.Msg) (SavedViews, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return *s, nil
	}
	key := keyMsg.String()
	// Create-object prompt swallows keys until Enter / Esc commits.
	if s.creating {
		return s.updateCreating(key), nil
	}
	// Delete confirmation: y commits, n/Esc cancels.
	if s.confirmingDelete {
		switch key {
		case "y", "Y":
			s.deleteReq = true
			s.confirmingDelete = false
			s.active = false
			return *s, nil
		case "n", "N", "esc":
			s.confirmingDelete = false
			s.deletePath = ""
		}
		return *s, nil
	}
	// Filter mode: characters narrow the result list; Enter/Esc exits.
	if s.filterMode {
		switch key {
		case "esc":
			s.filterMode = false
			s.filterBuf = ""
			s.cursor = 0
			s.scroll = 0
		case "enter":
			s.filterMode = false
		case "backspace":
			if len(s.filterBuf) > 0 {
				s.filterBuf = TrimLastRune(s.filterBuf)
				if s.cursor >= len(s.visibleMatches()) {
					s.cursor = 0
				}
			}
		default:
			if len(key) == 1 && key[0] >= 32 {
				s.filterBuf += key
				if s.cursor >= len(s.visibleMatches()) {
					s.cursor = 0
				}
			}
		}
		return *s, nil
	}
	if s.pickerMode {
		return s.updatePicker(keyMsg), nil
	}
	if !s.loaded {
		return *s, nil
	}
	visible := s.visibleMatches()
	switch key {
	case "j", "down":
		if s.cursor < len(visible)-1 {
			s.cursor++
		}
	case "k", "up":
		if s.cursor > 0 {
			s.cursor--
		}
	case "g":
		s.cursor = 0
		s.scroll = 0
	case "G":
		s.cursor = len(visible) - 1
		if s.cursor < 0 {
			s.cursor = 0
		}
	case "r":
		s.Refresh(s.index)
	case "p":
		// Return to picker. The user can then load a different view.
		s.pickerMode = true
		s.pickerCursor = 0
	case "/":
		s.filterMode = true
		s.filterBuf = ""
	case "n":
		// Quick-create a new object of this view's type. Only meaningful
		// when the view targets a specific type (Type != ""); a "match
		// any type" view has no schema to instantiate from.
		if strings.TrimSpace(s.currentV.Type) != "" {
			s.creating = true
			s.createBuf = ""
		}
	case "D", "ctrl+d", "delete":
		// Delete focused object — same UX as Object Browser.
		if s.cursor >= 0 && s.cursor < len(visible) {
			s.deletePath = visible[s.cursor].NotePath
			s.confirmingDelete = true
		}
	case "esc":
		// Esc clears the result filter first; if no filter is active,
		// returns to the picker (less drastic than closing the tab).
		if strings.TrimSpace(s.filterBuf) != "" {
			s.filterBuf = ""
			s.cursor = 0
			s.scroll = 0
		} else {
			s.pickerMode = true
			s.pickerCursor = 0
		}
	case "enter":
		if s.cursor >= 0 && s.cursor < len(visible) {
			s.jumpPath = visible[s.cursor].NotePath
			s.jumpReq = true
		}
	}
	return *s, nil
}

// updateCreating handles keys while the create-object prompt is active.
// Mirrors ObjectBrowser.updateCreating so the muscle memory transfers.
func (s *SavedViews) updateCreating(key string) SavedViews {
	switch key {
	case "esc":
		s.creating = false
		s.createBuf = ""
		return *s
	case "enter":
		title := strings.TrimSpace(s.createBuf)
		s.creating = false
		s.createBuf = ""
		if title == "" || s.registry == nil {
			return *s
		}
		t, ok := s.registry.ByID(s.currentV.Type)
		if !ok {
			return *s
		}
		s.createPath = objects.PathFor(t, title)
		s.createContent = objects.BuildFrontmatter(t, title, nil) + "# " + title + "\n"
		s.createReq = true
		s.active = false
		return *s
	case "backspace":
		if len(s.createBuf) > 0 {
			s.createBuf = TrimLastRune(s.createBuf)
		}
		return *s
	default:
		if len(key) == 1 && key[0] >= 32 {
			s.createBuf += key
		}
		return *s
	}
}

// updatePicker handles keys while in catalog-picker mode.
func (s *SavedViews) updatePicker(keyMsg tea.KeyMsg) SavedViews {
	all := s.catalog.All()
	switch keyMsg.String() {
	case "j", "down":
		if s.pickerCursor < len(all)-1 {
			s.pickerCursor++
		}
	case "k", "up":
		if s.pickerCursor > 0 {
			s.pickerCursor--
		}
	case "g":
		s.pickerCursor = 0
	case "G":
		s.pickerCursor = len(all) - 1
		if s.pickerCursor < 0 {
			s.pickerCursor = 0
		}
	case "enter":
		if s.pickerCursor >= 0 && s.pickerCursor < len(all) {
			_ = s.Open(s.catalog, s.index, all[s.pickerCursor].ID)
		}
	}
	return *s
}

// View renders the saved-view tab. Three render branches:
//
//   - picker mode → catalog list with descriptions
//   - loaded view → result list with property columns
//   - neither    → empty hint
func (s *SavedViews) View() string {
	if s.pickerMode && s.catalog != nil {
		return s.viewPicker()
	}
	if !s.loaded {
		emptyStyle := lipgloss.NewStyle().
			Foreground(overlay0).Italic(true).
			Padding(2, 4)
		return emptyStyle.Render("No saved view loaded — open one with Alt+V")
	}

	width := s.Width()
	if width <= 0 {
		width = 80
	}
	height := s.Height()
	if height <= 0 {
		height = 24
	}

	var b strings.Builder

	// Header.
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(overlay0)
	countStyle := lipgloss.NewStyle().Foreground(peach)

	b.WriteString(" ")
	b.WriteString(titleStyle.Render(s.currentV.Name))
	b.WriteString("  ")
	visible := s.visibleMatches()
	if strings.TrimSpace(s.filterBuf) != "" {
		// Show filtered/total when a filter is narrowing the result set.
		b.WriteString(countStyle.Render(fmt.Sprintf("(%d/%d)", len(visible), len(s.matches))))
	} else {
		b.WriteString(countStyle.Render(fmt.Sprintf("(%d)", len(s.matches))))
	}
	b.WriteString("\n")
	if s.currentV.Description != "" {
		b.WriteString(" ")
		b.WriteString(descStyle.Render(s.currentV.Description))
		b.WriteString("\n")
	}
	b.WriteString(" ")
	b.WriteString(descStyle.Render(s.viewSummary()))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", width-2))
	b.WriteString("\n")

	// Inline filter prompt — replaces the column header when filter mode
	// is active so the user always sees what they're typing.
	if s.filterMode {
		filterLabel := lipgloss.NewStyle().Foreground(yellow).Bold(true)
		filterValue := lipgloss.NewStyle().Foreground(text)
		hintStyle := lipgloss.NewStyle().Foreground(overlay0).Italic(true)
		line := filterLabel.Render(" / ") + filterValue.Render(s.filterBuf+"█") +
			"  " + hintStyle.Render("Enter to commit · Esc to clear")
		b.WriteString(TruncateDisplay(line, width))
		b.WriteString("\n")
	} else if strings.TrimSpace(s.filterBuf) != "" {
		// Filter committed but not actively editing — render a quiet
		// "filter: foo (Esc to clear)" line so the user knows results
		// are narrowed.
		dim := lipgloss.NewStyle().Foreground(overlay0).Italic(true)
		b.WriteString(dim.Render(fmt.Sprintf(" filter: %q  (Esc to clear)", s.filterBuf)))
		b.WriteString("\n")
	}

	// Empty result-state — distinguish "view has nothing" from
	// "filter excluded everything", since the fix is different.
	if len(visible) == 0 {
		emptyStyle := lipgloss.NewStyle().Foreground(overlay0).Italic(true)
		b.WriteString("\n  ")
		if strings.TrimSpace(s.filterBuf) != "" {
			b.WriteString(emptyStyle.Render(fmt.Sprintf("No matches for filter %q.", s.filterBuf)))
			b.WriteString("\n  ")
			b.WriteString(emptyStyle.Render("Esc to clear, or refine the search."))
		} else {
			b.WriteString(emptyStyle.Render("No objects match this view."))
			b.WriteString("\n  ")
			b.WriteString(emptyStyle.Render("Adjust the where-clause or capture more objects of this type."))
		}
		return b.String()
	}

	// Determine which property columns to render — pull from the type
	// schema. Fall back to title-only when the type isn't registered.
	cols := s.columnHeaders()
	headerStyle := lipgloss.NewStyle().Foreground(overlay1).Bold(true)
	b.WriteString(" ")
	b.WriteString(headerStyle.Render(s.formatRow("TITLE", cols, nil)))
	b.WriteString("\n")

	listHeight := height - 8 // 1 extra row for the filter prompt
	if listHeight < 5 {
		listHeight = 5
	}
	if s.cursor >= s.scroll+listHeight {
		s.scroll = s.cursor - listHeight + 1
	}
	if s.cursor < s.scroll {
		s.scroll = s.cursor
	}
	end := s.scroll + listHeight
	if end > len(visible) {
		end = len(visible)
	}

	rowStyle := lipgloss.NewStyle().Foreground(text)
	selStyle := lipgloss.NewStyle().Background(surface0).Foreground(text).Bold(true)
	for i := s.scroll; i < end; i++ {
		obj := visible[i]
		row := s.formatRow(obj.Title, cols, obj)
		if i == s.cursor {
			b.WriteString(selStyle.Render(" " + row))
		} else {
			b.WriteString(rowStyle.Render(" " + row))
		}
		b.WriteString("\n")
	}

	// Inline footer: delete confirm > create prompt > standard hints.
	switch {
	case s.confirmingDelete:
		warnStyle := lipgloss.NewStyle().Foreground(red).Bold(true)
		pathStyle := lipgloss.NewStyle().Foreground(text)
		hintStyle := lipgloss.NewStyle().Foreground(overlay0).Italic(true)
		b.WriteString(strings.Repeat("─", width-2))
		b.WriteString("\n")
		line := warnStyle.Render(" Delete ") +
			pathStyle.Render(s.deletePath) +
			hintStyle.Render(" ? (y/n) — irreversible, removes the underlying note file")
		b.WriteString(TruncateDisplay(line, width))
	case s.creating && s.registry != nil:
		t, _ := s.registry.ByID(s.currentV.Type)
		labelStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
		valStyle := lipgloss.NewStyle().Foreground(text)
		hintStyle := lipgloss.NewStyle().Foreground(overlay0).Italic(true)
		b.WriteString(strings.Repeat("─", width-2))
		b.WriteString("\n")
		label := labelStyle.Render(fmt.Sprintf(" New %s: ", t.Name))
		value := valStyle.Render(s.createBuf + "█")
		hint := hintStyle.Render("    Enter to create · Esc to cancel")
		b.WriteString(TruncateDisplay(label+value+hint, width))
	default:
		hint := lipgloss.NewStyle().Foreground(overlay0).Italic(true)
		hints := "j/k nav · Enter open · / filter · n new · D delete · r refresh · p picker · Esc back"
		if strings.TrimSpace(s.currentV.Type) == "" {
			hints = "j/k nav · Enter open · / filter · D delete · r refresh · p picker · Esc back"
		}
		b.WriteString("\n")
		b.WriteString(hint.Render(" " + hints))
	}

	return b.String()
}

// viewPicker renders the catalog list. Selecting a row loads its view.
func (s *SavedViews) viewPicker() string {
	width := s.Width()
	if width <= 0 {
		width = 80
	}

	all := s.catalog.All()
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	hintStyle := lipgloss.NewStyle().Foreground(overlay0).Italic(true)
	rowStyle := lipgloss.NewStyle().Foreground(text)
	descStyle := lipgloss.NewStyle().Foreground(overlay0)
	selStyle := lipgloss.NewStyle().Background(surface0).Foreground(text).Bold(true)

	var b strings.Builder
	b.WriteString(" ")
	b.WriteString(titleStyle.Render(fmt.Sprintf("Saved Views (%d)", len(all))))
	b.WriteString("\n ")
	b.WriteString(hintStyle.Render("j/k navigate · Enter load · Esc close · views live in .granit/views/"))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", width-2))
	b.WriteString("\n")

	if len(all) == 0 {
		b.WriteString("\n  ")
		b.WriteString(hintStyle.Render("No views in catalog. Drop a JSON file in .granit/views/ to add one."))
		return b.String()
	}

	pad := func(s string, w int) string {
		if len(s) > w {
			if w > 1 {
				return s[:w-1] + "…"
			}
			return s[:w]
		}
		return s + strings.Repeat(" ", w-len(s))
	}
	nameW := 28
	descW := width - nameW - 4
	if descW < 20 {
		descW = 20
	}
	for i, v := range all {
		row := " " + pad(v.Name, nameW) + "  " + descStyle.Render(pad(v.Description, descW))
		if i == s.pickerCursor {
			b.WriteString(selStyle.Render(row))
		} else {
			b.WriteString(rowStyle.Render(row))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// viewSummary builds a one-line "type=X • where=…" string for the header.
func (s *SavedViews) viewSummary() string {
	parts := []string{}
	if s.currentV.Type != "" {
		parts = append(parts, "type="+s.currentV.Type)
	} else {
		parts = append(parts, "type=any")
	}
	for _, c := range s.currentV.Where {
		switch c.Op {
		case objects.ViewOpExists:
			parts = append(parts, c.Property+" exists")
		case objects.ViewOpMissing:
			parts = append(parts, c.Property+" missing")
		default:
			parts = append(parts, fmt.Sprintf("%s %s %q", c.Property, c.Op, c.Value))
		}
	}
	if s.currentV.Sort != nil {
		dir := s.currentV.Sort.Direction
		if dir == "" {
			dir = "asc"
		}
		parts = append(parts, "sort="+s.currentV.Sort.Property+" "+dir)
	}
	if s.currentV.Limit > 0 {
		parts = append(parts, fmt.Sprintf("limit=%d", s.currentV.Limit))
	}
	return strings.Join(parts, " • ")
}

// columnHeaders returns the property names to surface as columns. Pulls from
// the type schema when available; otherwise empty (title-only fallback).
func (s *SavedViews) columnHeaders() []string {
	if s.catalog == nil || s.currentV.Type == "" {
		return nil
	}
	// We don't carry the registry here, but the Object's Properties map
	// keys give us a reasonable fallback: union of property names across
	// the matched objects, capped to the first 3 for layout.
	seen := map[string]bool{}
	var cols []string
	for _, obj := range s.matches {
		for k := range obj.Properties {
			if k == "tags" || k == "type" || k == "title" {
				continue
			}
			if !seen[k] {
				seen[k] = true
				cols = append(cols, k)
				if len(cols) >= 3 {
					return cols
				}
			}
		}
	}
	return cols
}

// formatRow renders a single line: title + 3 property columns. obj is nil
// when rendering the header row (uppercase column names).
func (s *SavedViews) formatRow(title string, cols []string, obj *objects.Object) string {
	width := s.Width()
	if width <= 0 {
		width = 80
	}
	titleW := 32
	colW := (width - titleW - 6) / max(len(cols), 1)
	if colW < 8 {
		colW = 8
	}

	pad := func(text string, w int) string {
		if len(text) > w {
			if w > 1 {
				return text[:w-1] + "…"
			}
			return text[:w]
		}
		return text + strings.Repeat(" ", w-len(text))
	}

	var b strings.Builder
	b.WriteString(pad(title, titleW))
	for _, c := range cols {
		b.WriteString("  ")
		var v string
		if obj != nil {
			v = obj.PropertyValue(c)
			if strings.TrimSpace(v) == "" {
				v = "—"
			}
		} else {
			v = strings.ToUpper(c)
		}
		b.WriteString(pad(v, colW))
	}
	return b.String()
}
