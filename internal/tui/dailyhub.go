package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/profiles"
	"github.com/artaeon/granit/internal/tasks"
	"github.com/artaeon/granit/internal/tui/widgets"
)

// DailyHub is the Phase 3 dashboard overlay — replaces
// dashboard.go's Alt+H view when cfg.UseProfiles is on. Renders
// the active profile's DashboardSpec onto a 12-col grid, hands
// keyboard input to the focused widget.
//
// State held here:
//   - which profile is active (snapshot taken on Open so it
//     doesn't change mid-session if the user switches profiles
//     elsewhere — they have to reopen the hub)
//   - per-cell scratch state (cursor positions, jot buffer, etc.)
//     keyed by widget ID
//   - the focused cell index
//
// Snapshot data (tasks, calendar, goals, etc.) lives entirely on
// WidgetCtx — built by the caller (Model) on each render so the
// hub never reads stale data.
type DailyHub struct {
	OverlayBase
	profile   *profiles.Profile
	registry  *widgets.Registry
	focused   int
	cellState map[string]map[string]any
}

// NewDailyHub builds an empty hub. The widget registry is
// injected at construction time; the active profile is set per
// Open call so the hub picks up live profile switches.
func NewDailyHub(reg *widgets.Registry) DailyHub {
	return DailyHub{
		registry:  reg,
		cellState: make(map[string]map[string]any),
	}
}

// Open snapshots the active profile and activates the overlay.
// Cells get fresh scratch state so reopening the hub doesn't
// inherit stale cursor positions from a previous session.
func (d *DailyHub) Open(p *profiles.Profile) {
	d.Activate()
	d.profile = p
	d.focused = 0
	d.cellState = make(map[string]map[string]any)
	if p != nil {
		for _, cell := range p.Dashboard.Cells {
			d.cellState[cell.WidgetID] = map[string]any{}
		}
	}
}

// Render lays out the grid and renders each cell. Caller passes
// the WidgetCtx prebuilt from the Model. Per-cell context is
// derived by overlaying the cell's static Config + the dynamic
// scratch state for that widget.
func (d *DailyHub) Render(width, height int, baseCtx widgets.WidgetCtx) string {
	if d.profile == nil || len(d.profile.Dashboard.Cells) == 0 {
		return d.renderEmpty(width, height)
	}
	cells := d.profile.Dashboard.Cells
	rowsCount := 0
	for _, c := range cells {
		end := c.Row + maxInt(c.RowSpan, 1)
		if end > rowsCount {
			rowsCount = end
		}
	}
	if rowsCount == 0 {
		rowsCount = 1
	}

	// Reserve top line for the profile-name header so users always
	// know which dashboard they're looking at.
	header := d.renderHeader(width)
	innerHeight := height - lipgloss.Height(header) - 1
	if innerHeight < rowsCount {
		innerHeight = rowsCount // best effort; widgets clip below MinSize
	}

	cellHeight := innerHeight / rowsCount
	if cellHeight < 3 {
		cellHeight = 3
	}
	colWidth := width / 12
	if colWidth < 1 {
		colWidth = 1
	}

	// Build a 2D byte buffer per row by horizontally concatenating
	// rendered cells in column order. Cells that span multiple
	// rows render once for the whole height; cells that span
	// multiple columns get the proportional width.
	rendered := make([][]string, rowsCount)
	for r := range rendered {
		rendered[r] = []string{}
	}

	// Sort cells by (Row, Col) so concatenation per row is
	// stable and predictable.
	sorted := append([]profiles.DashboardCell(nil), cells...)
	sortCells(sorted)

	// Track per-row "next free col" so we can leave gap-fillers.
	nextCol := make([]int, rowsCount)

	for i, c := range sorted {
		colSpan := maxInt(c.ColSpan, 1)
		rowSpan := maxInt(c.RowSpan, 1)
		w := colWidth*colSpan - 1
		if w < 1 {
			w = 1
		}
		h := cellHeight*rowSpan - 1
		if h < 1 {
			h = 1
		}

		// Fill any horizontal gap before this cell (if a previous
		// cell on this row didn't span all the way).
		if c.Col > nextCol[c.Row] {
			gap := strings.Repeat(" ", colWidth*(c.Col-nextCol[c.Row]))
			rendered[c.Row] = append(rendered[c.Row], gap)
		}

		body := d.renderCell(c, w, h, baseCtx, i == d.focused)
		rendered[c.Row] = append(rendered[c.Row], body)
		nextCol[c.Row] = c.Col + colSpan

		// Mark spanned rows as occupied so we don't try to render
		// into them.
		for r := c.Row + 1; r < c.Row+rowSpan; r++ {
			nextCol[r] = 12 // fully occupied
		}
	}

	// Concatenate rows. lipgloss.JoinHorizontal handles uneven
	// heights cleanly (it pads short cells to match the tallest).
	var rowsOut []string
	for r := 0; r < rowsCount; r++ {
		if len(rendered[r]) == 0 {
			continue
		}
		rowsOut = append(rowsOut, lipgloss.JoinHorizontal(lipgloss.Top, rendered[r]...))
	}
	body := strings.Join(rowsOut, "\n")
	return header + "\n" + body
}

// renderHeader is a one-line breadcrumb showing profile name +
// focus hint. Power-user UX: identify the profile, list the chord
// to switch.
func (d *DailyHub) renderHeader(width int) string {
	name := "Daily Hub"
	if d.profile != nil {
		name = d.profile.Name
	}
	left := lipgloss.NewStyle().Bold(true).Render(name)
	right := lipgloss.NewStyle().Faint(true).Render(
		fmt.Sprintf("%s · tab=focus · alt+w=switch · esc=close",
			time.Now().Format("Mon Jan 2  15:04")))
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

// renderEmpty handles the "profile has no dashboard cells" case
// (Classic with empty Dashboard, or a freshly-forked profile).
func (d *DailyHub) renderEmpty(width, height int) string {
	hint := lipgloss.NewStyle().Faint(true).Render(
		"This profile has no dashboard. Press alt+w to switch profiles\n" +
			"or hand-edit .granit/profiles/<id>.json to add cells.")
	pad := strings.Repeat("\n", maxInt((height-3)/2, 0))
	return pad + lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(hint)
}

// renderCell renders a single grid cell with border + title +
// widget body. Focused cells get a brighter border so the user
// always knows where keys land.
func (d *DailyHub) renderCell(c profiles.DashboardCell, w, h int, base widgets.WidgetCtx, focused bool) string {
	wdg, err := d.registry.Get(c.WidgetID)
	if err != nil {
		return errStub(c.WidgetID, w, h)
	}

	// Per-cell ctx: shallow-copy the base, replace Config with
	// the merged static+scratch map for this widget.
	ctx := base
	state := d.cellState[c.WidgetID]
	if state == nil {
		state = map[string]any{}
		d.cellState[c.WidgetID] = state
	}
	merged := map[string]any{}
	for k, v := range c.Config {
		merged[k] = v
	}
	for k, v := range state {
		merged[k] = v
	}
	ctx.Config = merged

	innerW := w - 2
	innerH := h - 2
	minW, minH := wdg.MinSize()
	if innerW < minW || innerH < minH {
		return tooSmallStub(c.WidgetID, w, h)
	}

	body := wdg.Render(ctx, innerW, innerH)

	// Carry scratch back so widget mutations to ctx.Config persist
	// across renders (jot buffer, cursors, etc.).
	for k, v := range merged {
		state[k] = v
	}

	border := lipgloss.RoundedBorder()
	style := lipgloss.NewStyle().Border(border).Width(innerW).Height(innerH)
	if focused {
		style = style.BorderForeground(lipgloss.Color("12"))
	} else {
		style = style.BorderForeground(lipgloss.Color("8"))
	}
	titled := lipgloss.NewStyle().Bold(true).Render(wdg.Title()) + "\n" + body
	return style.Render(titled)
}

// HandleKey routes keyboard events. Tab/Shift+Tab cycles focus;
// Alt+1..9 jumps directly; Esc closes; everything else goes to
// the focused widget. If the focused widget returns handled=false,
// the key bubbles to the global dispatcher.
func (d *DailyHub) HandleKey(key string, base widgets.WidgetCtx) (bubbled bool, cmd tea.Cmd) {
	if d.profile == nil || len(d.profile.Dashboard.Cells) == 0 {
		// Empty hub — only consume Esc.
		if key == "esc" {
			d.Close()
			return false, nil
		}
		return true, nil
	}
	switch key {
	case "esc":
		d.Close()
		return false, nil
	case "tab":
		d.focused = (d.focused + 1) % len(d.profile.Dashboard.Cells)
		return false, nil
	case "shift+tab":
		d.focused = (d.focused - 1 + len(d.profile.Dashboard.Cells)) % len(d.profile.Dashboard.Cells)
		return false, nil
	}
	// Alt+1..9 jumps to the Nth cell.
	if len(key) == 5 && strings.HasPrefix(key, "alt+") {
		ch := key[4]
		if ch >= '1' && ch <= '9' {
			idx := int(ch - '1')
			if idx < len(d.profile.Dashboard.Cells) {
				d.focused = idx
			}
			return false, nil
		}
	}

	// Route to focused widget.
	c := d.profile.Dashboard.Cells[d.focused]
	wdg, err := d.registry.Get(c.WidgetID)
	if err != nil {
		return true, nil // unknown widget → bubble
	}
	state := d.cellState[c.WidgetID]
	if state == nil {
		state = map[string]any{}
		d.cellState[c.WidgetID] = state
	}
	ctx := base
	ctx.Config = state // pass scratch directly; widget mutations persist

	handled, cmd := wdg.HandleKey(ctx, key)
	return !handled, cmd
}

// Close hides the hub. Scratch state is intentionally retained so
// reopening the hub during the same session keeps the jot buffer
// (etc.) intact — power users hate losing in-flight captures.
func (d *DailyHub) Close() {
	d.OverlayBase.Close()
}

// errStub renders a small fallback when a profile references a
// widget ID that isn't registered. Most likely cause: a Lua
// plugin that defined the widget hasn't loaded yet.
func errStub(id string, w, h int) string {
	style := lipgloss.NewStyle().Width(w-2).Height(h-2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("9")).
		Foreground(lipgloss.Color("9"))
	return style.Render("missing widget: " + id)
}

func tooSmallStub(id string, w, h int) string {
	style := lipgloss.NewStyle().Width(w-2).Height(h-2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("8")).
		Faint(true)
	return style.Render(id + ": cell too small")
}

// sortCells sorts in place by (Row asc, Col asc) — stable-output
// helper for the row-by-row concatenation pass.
func sortCells(cells []profiles.DashboardCell) {
	for i := 1; i < len(cells); i++ {
		for j := i; j > 0 && cellLess(cells[j], cells[j-1]); j-- {
			cells[j-1], cells[j] = cells[j], cells[j-1]
		}
	}
}

func cellLess(a, b profiles.DashboardCell) bool {
	if a.Row != b.Row {
		return a.Row < b.Row
	}
	return a.Col < b.Col
}

// buildWidgetCtx populates the data slices a widget might need.
// Called by the renderer + key handler on every Daily Hub frame
// so external state changes (new task, new note) show up
// immediately. Pulls from m.taskStore (when wired), the vault,
// and existing helpers for scripture / business pulse.
//
// Lives on Model (not on DailyHub) because it needs to reach
// into Model state — kept separate so the hub stays Model-free
// and unit-testable.
func (m *Model) buildWidgetCtx() widgets.WidgetCtx {
	taskList := m.cachedTasks
	if m.taskStore != nil {
		taskList = m.taskStore.All()
	}
	ctx := widgets.WidgetCtx{
		Tasks:        taskList,
		Scripture:    m.widgetScripture(),
		RecentNotes:  m.widgetRecentNotes(8),
		TriageInbox:  countTriageInbox(taskList),
		// TodayEvents / Goals / Habits / BusinessPulse — empty for
		// now; data integration ships in follow-up commits as the
		// existing data sources get factored into widget-shaped
		// helpers. Widgets render reasonable empty-states until
		// then.
	}
	ctx.OpenNote = func(path string) { m.loadNote(path) }
	ctx.CompleteTask = func(id string) {
		if m.taskStore != nil {
			_ = m.taskStore.Complete(id)
		}
	}
	ctx.OpenTriage = func() { /* wired in commit 6 alongside picker */ }
	ctx.CreateTask = func(text string) error {
		if m.taskStore == nil {
			return nil
		}
		_, err := m.taskStore.Create(text, tasks.CreateOpts{Origin: tasks.OriginJot})
		return err
	}
	return ctx
}

// widgetScripture pulls the day's verse from the existing
// scripture subsystem and adapts to the widget shape.
func (m *Model) widgetScripture() widgets.ScriptureVerse {
	s := DailyScripture(m.vault.Root)
	return widgets.ScriptureVerse{Reference: s.Source, Text: s.Text}
}

// widgetRecentNotes returns the N most-recently-modified notes,
// pre-formatted for the cell. The vault.Note has ModTime; we
// sort by it descending.
func (m *Model) widgetRecentNotes(n int) []widgets.NoteRef {
	type ranked struct {
		path string
		mod  time.Time
	}
	all := make([]ranked, 0, len(m.vault.Notes))
	for path, note := range m.vault.Notes {
		all = append(all, ranked{path: path, mod: note.ModTime})
	}
	// Top-N selection via partial sort — for small n (default 8)
	// this is faster than full sort, but at ~1000 notes the
	// constant factor doesn't matter; just full-sort for clarity.
	for i := 1; i < len(all); i++ {
		for j := i; j > 0 && all[j].mod.After(all[j-1].mod); j-- {
			all[j-1], all[j] = all[j], all[j-1]
		}
	}
	if len(all) > n {
		all = all[:n]
	}
	out := make([]widgets.NoteRef, 0, len(all))
	now := time.Now()
	for _, r := range all {
		title := strings.TrimSuffix(r.path, ".md")
		if idx := strings.LastIndex(title, "/"); idx >= 0 {
			title = title[idx+1:]
		}
		out = append(out, widgets.NoteRef{
			Path:     r.path,
			Title:    title,
			Modified: humanAgo(now.Sub(r.mod)),
		})
	}
	return out
}

func humanAgo(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	default:
		return fmt.Sprintf("%dmo", int(d.Hours()/(24*30)))
	}
}

func countTriageInbox(all []tasks.Task) int {
	count := 0
	for _, t := range all {
		if t.Triage == tasks.TriageInbox {
			count++
		}
	}
	return count
}
