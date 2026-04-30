package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/objects"
)

// ObjectBrowser is the Capacities-style typed-objects view: a
// two-pane editor tab with the registered Type list on the left
// and the gallery of objects of the focused type on the right.
//
// The data layer (registry + index) lives in internal/objects and
// is rebuilt by the host (Model) on vault refresh. ObjectBrowser is
// purely UI — it never reads from disk directly.
//
// Selecting an object and pressing Enter signals back to the host
// via GetJumpRequest(); the host turns that into "open this note
// in the editor pane" the same way Task Manager's `g` jump works.
//
// Keyboard surface (in tab mode):
//
//	Tab        — swap focus between type list and object grid
//	j/k or arrows — move cursor within the focused pane
//	Enter      — open selected object's note (or focus right pane)
//	/          — focus the in-grid filter input
//	Esc        — clear filter, then exit
type ObjectBrowser struct {
	OverlayBase

	// Source data. Set by the host immediately before the first
	// View(); refreshed when the vault re-scans.
	registry *objects.Registry
	index    *objects.Index

	// Focus + cursor state. Pane 0 = type list, 1 = object grid.
	focus       int
	typeCursor  int
	objCursor   int
	objScroll   int

	// Filter state — applied only to the right-pane grid. Empty
	// query returns the full per-type list.
	searching bool
	query     string

	// Consumed-once jump request: when the user presses Enter on
	// an object, we set jumpPath; the host reads it via
	// GetJumpRequest() on the next tick and clears it.
	jumpPath string
	jumpReq  bool

	// Inline create-object prompt — opened by 'n'. Holds the title
	// the user is typing for a new object of the focused type. The
	// host reads `createReq` to perform the file write.
	creating  bool
	createBuf string

	// Consumed-once create request: NotePath the host should write
	// to, plus the rendered file contents. Cleared after host reads.
	createPath    string
	createContent string
	createReq     bool

	// Delete confirmation — opened by 'D' on an object row. Two-step
	// confirmation prevents accidental deletion of the user's notes.
	// Stores the object's path so the host knows what to delete when
	// the consumed-once deleteReq fires.
	confirmingDelete bool
	deletePath       string
	deleteReq        bool
}

// NewObjectBrowser constructs a fresh, inactive ObjectBrowser. The
// host calls Open() (with vault data) and SetSize() before the first
// render.
func NewObjectBrowser() ObjectBrowser { return ObjectBrowser{} }

// Open activates the browser with the given registry and index.
// Refresh is the no-frontmatter-change variant — host calls Open
// once per session and Refresh on every vault scan thereafter.
func (b *ObjectBrowser) Open(reg *objects.Registry, idx *objects.Index) {
	b.Activate()
	b.registry = reg
	b.index = idx
	b.focus = 0
	b.typeCursor = 0
	b.objCursor = 0
	b.objScroll = 0
	b.query = ""
	b.searching = false
}

// Refresh swaps in a new registry/index without resetting the cursor
// state. Triggered when the host detects a vault change so the user
// keeps their place across rebuilds.
func (b *ObjectBrowser) Refresh(reg *objects.Registry, idx *objects.Index) {
	b.registry = reg
	b.index = idx
	// Clamp cursors against the new dataset so a deleted note
	// doesn't leave the cursor pointing past the end of the slice.
	types := b.typesWithObjects()
	if b.typeCursor >= len(types) {
		b.typeCursor = max0(len(types) - 1)
	}
	objs := b.currentObjects()
	if b.objCursor >= len(objs) {
		b.objCursor = max0(len(objs) - 1)
	}
}

// GetJumpRequest returns (path, ok) once after Enter on an object;
// subsequent calls return ("", false) until the next Enter. Mirrors
// the consumed-once pattern used by Task Manager + Habit Tracker.
func (b *ObjectBrowser) GetJumpRequest() (string, bool) {
	if !b.jumpReq {
		return "", false
	}
	p := b.jumpPath
	b.jumpReq = false
	b.jumpPath = ""
	return p, true
}

// GetCreateRequest returns (path, content, ok) once after the user
// commits the new-object prompt. Path is vault-relative; content is
// the full file body (frontmatter + placeholder heading). Cleared on
// read so the host can call this each tick safely.
func (b *ObjectBrowser) GetCreateRequest() (string, string, bool) {
	if !b.createReq {
		return "", "", false
	}
	p, c := b.createPath, b.createContent
	b.createReq = false
	b.createPath = ""
	b.createContent = ""
	return p, c, true
}

// GetDeleteRequest returns (path, ok) once after the user confirms
// deletion. The host removes the file from disk and refreshes state.
// Consumed-once.
func (b *ObjectBrowser) GetDeleteRequest() (string, bool) {
	if !b.deleteReq {
		return "", false
	}
	p := b.deletePath
	b.deleteReq = false
	b.deletePath = ""
	return p, true
}

// Update handles a single tea.KeyMsg. Returns the (possibly mutated)
// browser and any tea.Cmd to propagate. The host wires this through
// its own Update.
func (b ObjectBrowser) Update(msg tea.Msg) (ObjectBrowser, tea.Cmd) {
	if !b.active {
		return b, nil
	}
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return b, nil
	}
	key := km.String()

	// Create-object prompt swallows keys until Enter / Esc commits.
	if b.creating {
		return b.updateCreating(key), nil
	}

	// Delete confirmation: y commits, n/Esc cancels.
	if b.confirmingDelete {
		switch key {
		case "y", "Y":
			b.deleteReq = true
			b.confirmingDelete = false
			b.active = false
			return b, nil
		case "n", "N", "esc":
			b.confirmingDelete = false
			b.deletePath = ""
			return b, nil
		}
		return b, nil
	}

	// Search-mode swallows most keys until Enter / Esc commit.
	if b.searching {
		return b.updateSearching(key), nil
	}

	switch key {
	case "esc":
		// Clear the active filter on first Esc; close on second.
		if b.query != "" {
			b.query = ""
			b.objCursor = 0
			b.objScroll = 0
			return b, nil
		}
		b.active = false
		return b, nil
	case "tab", "shift+tab":
		b.focus = 1 - b.focus
		return b, nil
	case "/":
		b.searching = true
		b.query = ""
		b.focus = 1
		return b, nil
	case "n":
		// Open the create-object prompt for the focused type. Works
		// from either pane — we use whichever type the cursor is on
		// in the type list (typeCursor), regardless of pane focus.
		if _, ok := b.currentType(); ok {
			b.creating = true
			b.createBuf = ""
		}
		return b, nil
	case "D", "ctrl+d", "delete":
		// Delete current object — only meaningful when the grid pane
		// is focused (the type list has nothing to delete). Two-step
		// y/n confirmation in the footer; the host removes the file
		// and refreshes state on commit.
		if b.focus != 1 {
			return b, nil
		}
		objs := b.currentObjects()
		if b.objCursor < 0 || b.objCursor >= len(objs) {
			return b, nil
		}
		b.deletePath = objs[b.objCursor].NotePath
		b.confirmingDelete = true
		return b, nil
	case "j", "down":
		return b.moveCursor(+1), nil
	case "k", "up":
		return b.moveCursor(-1), nil
	case "enter":
		if b.focus == 0 {
			// Enter on a type focuses the grid for that type.
			b.focus = 1
			b.objCursor = 0
			b.objScroll = 0
			return b, nil
		}
		// Enter on an object — emit a jump request.
		objs := b.currentObjects()
		if b.objCursor >= 0 && b.objCursor < len(objs) {
			b.jumpPath = objs[b.objCursor].NotePath
			b.jumpReq = true
			b.active = false
		}
		return b, nil
	}
	return b, nil
}

// updateCreating handles keys while the create-object title prompt is
// active. Enter commits (emits createReq); Esc cancels.
func (b ObjectBrowser) updateCreating(key string) ObjectBrowser {
	switch key {
	case "esc":
		b.creating = false
		b.createBuf = ""
		return b
	case "enter":
		title := strings.TrimSpace(b.createBuf)
		b.creating = false
		b.createBuf = ""
		if title == "" {
			return b
		}
		t, ok := b.currentType()
		if !ok {
			return b
		}
		path := objects.PathFor(t, title)
		content := objects.BuildFrontmatter(t, title, nil) + "# " + title + "\n"
		b.createPath = path
		b.createContent = content
		b.createReq = true
		b.active = false
		return b
	case "backspace":
		if len(b.createBuf) > 0 {
			b.createBuf = TrimLastRune(b.createBuf)
		}
		return b
	default:
		if len(key) == 1 && key[0] >= 32 {
			b.createBuf += key
		}
		return b
	}
}

// updateSearching handles keys while the filter input is focused.
// Enter commits and exits search mode; Esc clears and exits.
func (b ObjectBrowser) updateSearching(key string) ObjectBrowser {
	switch key {
	case "esc":
		b.searching = false
		b.query = ""
		b.objCursor = 0
		return b
	case "enter":
		b.searching = false
		return b
	case "backspace":
		if len(b.query) > 0 {
			b.query = TrimLastRune(b.query)
			b.objCursor = 0
		}
		return b
	default:
		if len(key) == 1 {
			b.query += key
			b.objCursor = 0
		}
		return b
	}
}

// moveCursor moves the cursor in the focused pane, respecting bounds.
func (b ObjectBrowser) moveCursor(delta int) ObjectBrowser {
	switch b.focus {
	case 0:
		types := b.typesWithObjects()
		nextC := b.typeCursor + delta
		if nextC >= 0 && nextC < len(types) {
			b.typeCursor = nextC
			b.objCursor = 0
			b.objScroll = 0
		}
	case 1:
		objs := b.currentObjects()
		nextC := b.objCursor + delta
		if nextC >= 0 && nextC < len(objs) {
			b.objCursor = nextC
			// Keep cursor on screen — visible page height is
			// computed in renderGrid, mirror it here.
			vh := b.gridHeight()
			if b.objCursor >= b.objScroll+vh {
				b.objScroll = b.objCursor - vh + 1
			}
			if b.objCursor < b.objScroll {
				b.objScroll = b.objCursor
			}
		}
	}
	return b
}

// typesWithObjects returns every registered type, sorted by registry
// order. Both populated AND empty types are returned so the user can:
//   - see what types are available for creation (the original UX
//     hid empty types entirely, leaving users wondering "where are
//     the other 10 built-ins?")
//   - press 'n' on an empty type to create the first object of that
//     kind without having to pre-create a file by hand
//
// The renderer dims empty rows so populated types still stand out
// for browsing.
func (b ObjectBrowser) typesWithObjects() []objects.Type {
	if b.registry == nil {
		return nil
	}
	return b.registry.All()
}

// currentObjects returns the per-type object list, with the active
// search filter applied. Returns nil when no type is selected.
func (b ObjectBrowser) currentObjects() []*objects.Object {
	types := b.typesWithObjects()
	if b.typeCursor < 0 || b.typeCursor >= len(types) {
		return nil
	}
	return b.index.Search(types[b.typeCursor].ID, b.query)
}

// currentType returns the focused type or (zero, false) when nothing
// is selectable. Used by render code to fetch property columns.
func (b ObjectBrowser) currentType() (objects.Type, bool) {
	types := b.typesWithObjects()
	if b.typeCursor < 0 || b.typeCursor >= len(types) {
		return objects.Type{}, false
	}
	return types[b.typeCursor], true
}

// View renders the browser. Two or three panes depending on width:
//
//   - Narrow terminals (< 110 cols) → 2 panes: type list (left) +
//     gallery (right). Existing layout, gallery can use more
//     horizontal space.
//   - Wide terminals (≥ 110 cols) → 3 panes: type list (left, 26 cols)
//     + gallery (middle, fills remaining minus preview) + preview
//     (right, 40 cols) showing the focused object's properties +
//     body excerpt.
//
// The preview pane is the Capacities equivalent of "click a row to
// see details" — surfaces ALL frontmatter properties (including
// ones the gallery can't fit in its 3 columns) plus the first
// chunk of the body so users can decide whether to open the note.
func (b ObjectBrowser) View() string {
	if !b.active {
		return ""
	}
	w := b.width
	if w < 60 {
		w = 60
	}
	h := b.height
	if h < 12 {
		h = 12
	}

	// Threshold lowered from 110 → 95 so laptop users on the typical
	// 13"-15" terminal width (~95-120 cols) actually see the preview
	// pane. Preview width also shrinks proportionally on tighter
	// screens so the gallery doesn't get squeezed.
	const wideThreshold = 95
	previewW := 40
	if w < 110 {
		previewW = 32
	}

	leftW := 28
	if w < 90 {
		leftW = 22
	}

	header := b.renderHeader(w)
	footer := b.renderFooter(w)
	left := b.renderTypeList(leftW, h-2)

	var body string
	if w >= wideThreshold && b.focus == 1 {
		// 3-pane: type list + gallery + preview. Preview only
		// makes sense when an object row is focused — the focused
		// type-list row has no per-object detail to show.
		gridW := w - leftW - previewW - 6 // 6 for two separators
		if gridW < 30 {
			gridW = 30
		}
		grid := b.renderGrid(gridW, h-2)
		preview := b.renderPreview(previewW, h-2)
		body = lipgloss.JoinHorizontal(lipgloss.Top,
			left,
			lipgloss.NewStyle().Foreground(overlay0).Render(" │ "),
			grid,
			lipgloss.NewStyle().Foreground(overlay0).Render(" │ "),
			preview)
	} else {
		// 2-pane: type list + gallery (existing layout).
		gridW := w - leftW - 3
		if gridW < 30 {
			gridW = 30
		}
		grid := b.renderGrid(gridW, h-2)
		body = lipgloss.JoinHorizontal(lipgloss.Top,
			left,
			lipgloss.NewStyle().Foreground(overlay0).Render(" │ "),
			grid)
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

// renderPreview is the third-pane body (wide terminals only) —
// shows the focused object's full property bag and a body excerpt
// so the user can decide whether to open the underlying note. Pure
// read-only; no input handling. Falls back to a placeholder when
// there's no focused object (focus=0 on the type list).
func (b ObjectBrowser) renderPreview(w, h int) string {
	if w < 20 {
		w = 20
	}
	objs := b.currentObjects()
	if b.objCursor < 0 || b.objCursor >= len(objs) {
		return lipgloss.NewStyle().Width(w).Height(h).Render(
			DimStyle.Render("  Select an object on the left to preview."))
	}
	o := objs[b.objCursor]
	t, _ := b.currentType()

	var lines []string
	lines = append(lines, lipgloss.NewStyle().
		Foreground(subtext0).Bold(true).Render("  PREVIEW"))
	lines = append(lines, "")
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	lines = append(lines, "  "+titleStyle.Render(TruncateDisplay(o.Title, w-4)))
	lines = append(lines, "  "+DimStyle.Render(t.Name))
	lines = append(lines, "")

	// Property table — every property declared by the type, in
	// declaration order. Empty values render as a dim em dash so
	// the user sees the schema completeness at a glance.
	keyStyle := lipgloss.NewStyle().Foreground(subtext0).Width(maxInt(8, w/3-2))
	valStyle := lipgloss.NewStyle().Foreground(text)
	for _, p := range t.Properties {
		v := o.PropertyValue(p.Name)
		v = formatPropertyValue(v, p.Kind)
		// Truncate long values to fit the pane. Also truncate the
		// label — long property labels (e.g. user-defined fields with
		// verbose names) can otherwise wrap.
		labelAvail := keyStyle.GetWidth()
		label := TruncateDisplay(p.DisplayLabel(), labelAvail)
		maxV := w - labelAvail - 4
		if maxV < 5 {
			maxV = 5
		}
		v = TruncateDisplay(v, maxV)
		lines = append(lines, "  "+keyStyle.Render(label)+valStyle.Render(v))
	}

	// Footer: note path + truncated body excerpt. Path helps the
	// user verify they're looking at the right note; body excerpt
	// gives a preview of what Enter would open.
	lines = append(lines, "")
	lines = append(lines, DimStyle.Render("  "+TruncateDisplay(o.NotePath, w-4)))

	return lipgloss.NewStyle().Width(w).Height(h).Render(strings.Join(lines, "\n"))
}

// renderHeader is the title strip. Shows "Objects" + the global
// total + a hint about the untyped count when present.
//
// On narrow terminals the hint folds into a second line instead of
// overflowing into the rule below — the previous version concatenated
// every segment and let lipgloss wrap, producing 3-line headers on
// 80-col terminals.
func (b ObjectBrowser) renderHeader(w int) string {
	title := "  📦 Objects"
	titleStyled := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(title)

	count := "0"
	typeCount := len(b.typesWithObjects())
	if b.index != nil {
		count = fmt.Sprintf("%d", b.index.Total())
	}
	stat := fmt.Sprintf("   %s typed objects across %d types", count, typeCount)
	statStyled := lipgloss.NewStyle().Foreground(overlay0).Render(stat)

	hint := ""
	if b.index != nil {
		if u := b.index.UntypedCount(); u > 0 {
			hint = fmt.Sprintf("   ⚠  %d note(s) reference unknown types", u)
		}
	}

	// First line: title + stat. Truncate combined length to w so
	// nothing wraps into the rule.
	firstLine := title + stat
	if lipgloss.Width(firstLine) > w {
		// Drop the stat suffix when too tight; keep at least the title.
		firstLine = TruncateDisplay(firstLine, w)
		statStyled = ""
		titleStyled = lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(firstLine)
	}

	rule := DimStyle.Render(strings.Repeat("─", maxInt(0, w-2)))
	out := titleStyled + statStyled
	if hint != "" {
		// Hint goes on its own line so a wide warning label can't
		// push the rule onto a third visual row.
		hintLine := TruncateDisplay(hint, w)
		out += "\n" + lipgloss.NewStyle().Foreground(yellow).Render(hintLine)
	}
	return out + "\n" + rule
}

// renderTypeList draws the left pane: one row per type with icon,
// name, and object count. The active type is highlighted.
//
// Every row is pre-truncated to fit in `w` so lipgloss never wraps
// content into a second visual line — without this, long type names
// turned the sidebar into a multi-line mess.
func (b ObjectBrowser) renderTypeList(w, h int) string {
	types := b.typesWithObjects()
	var lines []string
	lines = append(lines, lipgloss.NewStyle().
		Foreground(subtext0).Bold(true).Render("  TYPES"))
	lines = append(lines, "")
	if len(types) == 0 {
		empty := DimStyle.Render(
			"  No typed notes yet.\n  Add `type: person`\n  to a note's frontmatter.")
		lines = append(lines, empty)
	}
	counts := map[string]int{}
	if b.index != nil {
		counts = b.index.CountByType()
	}
	for i, t := range types {
		focused := b.focus == 0 && i == b.typeCursor
		icon := emojiOrFallback(t.Icon, "•")
		badge := fmt.Sprintf(" %d", counts[t.ID])

		// Compute the title slot. Reserve: 2 leading spaces, the icon,
		// 2-space gap, then the badge at the right edge. Whatever's
		// left is for the type name — and we hard-truncate so lipgloss
		// never wraps inside a 28-col pane.
		nameAvail := w - 2 - lipgloss.Width(icon) - 2 - lipgloss.Width(badge)
		if nameAvail < 4 {
			nameAvail = 4
		}
		name := TruncateDisplay(t.Name, nameAvail)

		row := fmt.Sprintf("  %s  %s", icon, name)
		row = PadRight(row, w-lipgloss.Width(badge)) + badge
		row = TruncateDisplay(row, w)
		row = PadRight(row, w)

		isEmpty := counts[t.ID] == 0
		if focused {
			row = lipgloss.NewStyle().
				Background(surface0).Foreground(peach).Bold(true).
				Render(row)
		} else if isEmpty {
			// Empty types fade to overlay0 (same as the badge) so the
			// type list doubles as a "what types exist" reference
			// without populated types getting visually drowned.
			row = lipgloss.NewStyle().Foreground(overlay0).Render(row)
		} else {
			// Split the rendered line so the badge is dimmer than
			// the type name. Cosmetic only; matches the sidebar.
			headerLen := lipgloss.Width(row) - lipgloss.Width(badge)
			if headerLen < 0 || headerLen > len(row) {
				headerLen = len(row)
			}
			row = lipgloss.NewStyle().Foreground(text).Render(row[:headerLen]) +
				lipgloss.NewStyle().Foreground(overlay0).Render(row[headerLen:])
		}
		lines = append(lines, row)
	}
	out := strings.Join(lines, "\n")
	return lipgloss.NewStyle().Width(w).Height(h).Render(out)
}

// renderGrid draws the right pane: header columns from the focused
// type's properties, then one row per object. The active row is
// highlighted; non-active rows show truncated values.
func (b ObjectBrowser) renderGrid(w, h int) string {
	if w < 30 {
		w = 30
	}
	t, ok := b.currentType()
	if !ok {
		return lipgloss.NewStyle().Width(w).Height(h).Render(
			DimStyle.Render("  Select a type on the left."))
	}

	// Title row — pre-truncate so a long type name can't wrap into
	// a second line and push every other row down.
	titleRaw := fmt.Sprintf("  %s  %s", emojiOrFallback(t.Icon, "•"), t.Name)
	titleRaw = TruncateDisplay(titleRaw, w)
	titleLine := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(titleRaw)

	// Filter input row.
	filterLabel := DimStyle.Render("  Filter: ")
	filterValue := b.query
	if b.searching {
		filterValue += "█"
	}
	filterStyle := lipgloss.NewStyle().Foreground(text)
	if b.searching {
		filterStyle = filterStyle.Background(surface0)
	} else if b.query == "" {
		filterStyle = DimStyle
		filterValue = "(press / to search)"
	}
	// Truncate filter value to whatever's left after the label.
	filterAvail := w - lipgloss.Width(filterLabel) - 2
	if filterAvail < 4 {
		filterAvail = 4
	}
	filterValue = TruncateDisplay(filterValue, filterAvail)
	filterLine := filterLabel + filterStyle.Render(filterValue)

	// Column header.
	cols := buildColumnSpec(t, w-4)
	headerRow := formatRow("  ", cols, columnHeaders(t, cols))
	header := DimStyle.Render(TruncateDisplay(headerRow, w))

	// Body rows.
	objs := b.currentObjects()
	visH := h - 5 // title + filter + header + footer/spacing
	if visH < 3 {
		visH = 3
	}
	end := b.objScroll + visH
	if end > len(objs) {
		end = len(objs)
	}
	var bodyLines []string
	if len(objs) == 0 {
		bodyLines = append(bodyLines, DimStyle.Render(
			TruncateDisplay("  No objects match the current filter.", w)))
	}
	for i := b.objScroll; i < end; i++ {
		o := objs[i]
		row := formatRow("  ", cols, objectRowValues(o, t, cols))
		// Hard-truncate to pane width before any styling — without
		// this, formatRow's column padding can produce a string
		// wider than w (when w-4 doesn't divide cleanly across the
		// columns) and lipgloss.Width(w) wraps it.
		row = TruncateDisplay(row, w)
		row = PadRight(row, w)
		if b.focus == 1 && i == b.objCursor {
			row = lipgloss.NewStyle().
				Background(surface0).Foreground(peach).Bold(true).
				Render(row)
		}
		bodyLines = append(bodyLines, row)
	}
	scroll := ""
	if len(objs) > visH {
		scroll = DimStyle.Render(fmt.Sprintf(
			"  %d/%d", b.objCursor+1, len(objs)))
	}
	body := strings.Join(bodyLines, "\n")
	out := titleLine + "\n" + filterLine + "\n\n" + header + "\n" + body
	if scroll != "" {
		out += "\n\n" + scroll
	}
	return lipgloss.NewStyle().Width(w).Height(h).Render(out)
}

// renderFooter is the keybind hint strip at the bottom. Renders the
// active prompt (create / delete confirmation) inline when one is
// open so the user always sees the current input/decision context.
func (b ObjectBrowser) renderFooter(w int) string {
	rule := DimStyle.Render(strings.Repeat("─", maxInt(0, w-2)))
	if b.creating {
		t, _ := b.currentType()
		labelStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
		valStyle := lipgloss.NewStyle().Foreground(text)
		hintStyle := DimStyle
		label := labelStyle.Render(fmt.Sprintf("  New %s: ", t.Name))
		value := valStyle.Render(b.createBuf + "█")
		hint := hintStyle.Render("    Enter to create · Esc to cancel")
		line := TruncateDisplay(label+value+hint, w)
		return rule + "\n" + line
	}
	if b.confirmingDelete {
		warnStyle := lipgloss.NewStyle().Foreground(red).Bold(true)
		pathStyle := lipgloss.NewStyle().Foreground(text)
		hintStyle := DimStyle
		line := warnStyle.Render("  Delete ") +
			pathStyle.Render(b.deletePath) +
			hintStyle.Render(" ? (y/n) — irreversible, removes the underlying note file")
		line = TruncateDisplay(line, w)
		return rule + "\n" + line
	}
	return rule + "\n" + RenderHelpBar([]struct{ Key, Desc string }{
		{"j/k", "nav"}, {"Tab", "swap pane"}, {"Enter", "open"},
		{"n", "new"}, {"D", "delete"}, {"/", "filter"}, {"Esc", "close"},
	})
}

// gridHeight returns the visible body row count, mirroring the
// arithmetic inside renderGrid so cursor scrolling stays correct.
func (b ObjectBrowser) gridHeight() int {
	h := b.height - 7 // header + 2 lines of grid head + footer
	if h < 3 {
		h = 3
	}
	return h
}

// columnSpec captures one column of the gallery: a property and the
// width in characters allotted to it. Built per render so it adapts
// to the available pane width.
type columnSpec struct {
	prop  *objects.Property // nil → the special "title" column
	name  string            // header label
	width int
}

// buildColumnSpec returns the columns to render for a type, fitted
// into widthAvail characters. The first column is always Title; up
// to 3 additional property columns follow, weighted by importance
// (required > non-required, then declaration order).
func buildColumnSpec(t objects.Type, widthAvail int) []columnSpec {
	const titleW = 24
	if widthAvail < titleW+10 {
		// Very narrow pane — render title only.
		return []columnSpec{{name: "Title", width: widthAvail}}
	}
	out := []columnSpec{{name: "Title", width: titleW}}
	remaining := widthAvail - titleW - 2

	// Pick at most 3 properties by importance heuristic.
	picked := 0
	const maxCols = 3
	addProp := func(p *objects.Property) bool {
		if picked >= maxCols || remaining < 12 {
			return false
		}
		w := remaining / (maxCols - picked + 1)
		if w < 10 {
			w = 10
		}
		out = append(out, columnSpec{prop: p, name: p.DisplayLabel(), width: w})
		remaining -= w + 2
		picked++
		return true
	}
	for i := range t.Properties {
		if t.Properties[i].Required && t.Properties[i].Name != "title" {
			if !addProp(&t.Properties[i]) {
				break
			}
		}
	}
	for i := range t.Properties {
		if t.Properties[i].Required || t.Properties[i].Name == "title" {
			continue
		}
		if !addProp(&t.Properties[i]) {
			break
		}
	}
	return out
}

// columnHeaders extracts just the header strings from the column
// specs; used to render the table head row.
func columnHeaders(_ objects.Type, cols []columnSpec) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = c.name
	}
	return out
}

// objectRowValues maps an Object to the displayed values for each
// column in the spec, applying property-kind formatting (e.g.
// checkbox values become "✓"/"✗", missing values become "—").
func objectRowValues(o *objects.Object, _ objects.Type, cols []columnSpec) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		if c.prop == nil {
			out[i] = o.Title
			continue
		}
		v := o.PropertyValue(c.prop.Name)
		out[i] = formatPropertyValue(v, c.prop.Kind)
	}
	return out
}

// formatPropertyValue renders a raw frontmatter string per the
// declared property kind: checkboxes get ✓/✗ glyphs, dates pass
// through, empty values render as a dim em dash placeholder.
func formatPropertyValue(v string, kind objects.PropertyKind) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "—"
	}
	switch kind {
	case objects.KindCheckbox:
		switch strings.ToLower(v) {
		case "true", "yes", "y", "1", "on":
			return "✓"
		case "false", "no", "n", "0", "off":
			return "✗"
		}
		return v
	}
	return v
}

// formatRow joins the per-column values using each column's width
// (truncating with an ellipsis when overflow), separated by a
// two-space gutter. Returns a single line.
func formatRow(prefix string, cols []columnSpec, values []string) string {
	var b strings.Builder
	b.WriteString(prefix)
	for i, c := range cols {
		v := values[i]
		if displayWidth(v) > c.width {
			v = TruncateDisplay(v, c.width)
		}
		// Pad to width with spaces so subsequent columns align
		// even when the value is short.
		pad := c.width - displayWidth(v)
		b.WriteString(v)
		if pad > 0 {
			b.WriteString(strings.Repeat(" ", pad))
		}
		if i < len(cols)-1 {
			b.WriteString("  ")
		}
	}
	return b.String()
}

// emojiOrFallback returns the icon when non-empty, or fallback. Used
// so the type list never has a blank cell when an icon was omitted.
func emojiOrFallback(icon, fallback string) string {
	if strings.TrimSpace(icon) != "" {
		return icon
	}
	return fallback
}

// max0 returns x when positive, else zero. Used by Refresh's cursor
// clamp to avoid negative cursors when every object is deleted.
func max0(x int) int {
	if x < 0 {
		return 0
	}
	return x
}
