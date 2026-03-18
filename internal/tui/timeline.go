package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TimelineEntry represents a single note on the timeline.
type TimelineEntry struct {
	Path    string
	Title   string
	Date    time.Time
	Tags    []string
	Preview string // first 80 chars of content
}

// TimelineGroup is a cluster of entries under a shared label (day, week, or month).
type TimelineGroup struct {
	Label   string // "March 2024", "Week 10, 2024", "2024-03-05"
	Entries []TimelineEntry
}

type timelineView int

const (
	timelineByDay   timelineView = iota
	timelineByWeek
	timelineByMonth
)

// Timeline is a chronological overlay showing all notes grouped by day/week/month.
type Timeline struct {
	active bool
	width  int
	height int

	// Data
	groups  []TimelineGroup
	entries []TimelineEntry // flat list of all entries, sorted newest-first

	// Navigation
	cursor    int // index into flat entry list
	scroll    int
	groupView timelineView

	// Result
	selectedNote string
	selected     bool
}

// NewTimeline creates an inactive Timeline ready to be opened.
func NewTimeline() Timeline {
	return Timeline{}
}

// IsActive reports whether the timeline overlay is visible.
func (t Timeline) IsActive() bool {
	return t.active
}

// SetSize updates the available terminal dimensions.
func (t *Timeline) SetSize(w, h int) {
	t.width = w
	t.height = h
}

// Close hides the overlay.
func (t *Timeline) Close() {
	t.active = false
}

// Open receives note data, sorts it, groups it, and activates the overlay.
func (t *Timeline) Open(notes map[string]TimelineEntry) {
	t.active = true
	t.cursor = 0
	t.scroll = 0
	t.selectedNote = ""
	t.selected = false

	t.entries = make([]TimelineEntry, 0, len(notes))
	for _, entry := range notes {
		t.entries = append(t.entries, entry)
	}

	// Sort descending (newest first)
	sort.Slice(t.entries, func(i, j int) bool {
		return t.entries[i].Date.After(t.entries[j].Date)
	})

	t.buildGroups()
}

// GetSelectedNote returns the path of the note the user chose and resets the
// flag. The second return value is false when nothing was selected.
func (t *Timeline) GetSelectedNote() (string, bool) {
	if !t.selected {
		return "", false
	}
	path := t.selectedNote
	t.selectedNote = ""
	t.selected = false
	return path, true
}

// buildGroups partitions the sorted entries into groups based on the current
// groupView setting.
func (t *Timeline) buildGroups() {
	t.groups = nil
	if len(t.entries) == 0 {
		return
	}

	var current *TimelineGroup
	prevLabel := ""

	for _, entry := range t.entries {
		label := t.groupLabel(entry.Date)
		if label != prevLabel {
			if current != nil {
				t.groups = append(t.groups, *current)
			}
			current = &TimelineGroup{Label: label}
			prevLabel = label
		}
		current.Entries = append(current.Entries, entry)
	}
	if current != nil {
		t.groups = append(t.groups, *current)
	}
}

// groupLabel returns the display label for a date under the active view mode.
func (t *Timeline) groupLabel(d time.Time) string {
	switch t.groupView {
	case timelineByWeek:
		year, week := d.ISOWeek()
		return fmt.Sprintf("Week %d, %d", week, year)
	case timelineByMonth:
		return d.Format("January 2006")
	default: // timelineByDay
		return d.Format("2006-01-02")
	}
}

// cycleView advances the group mode day -> week -> month -> day and rebuilds.
func (t *Timeline) cycleView() {
	t.groupView = (t.groupView + 1) % 3
	t.buildGroups()
}

// viewModeName returns a human-readable name for a timelineView constant.
func viewModeName(v timelineView) string {
	switch v {
	case timelineByDay:
		return "Day"
	case timelineByWeek:
		return "Week"
	case timelineByMonth:
		return "Month"
	}
	return ""
}

// Update handles keyboard input.  Value receiver to match the Bubble Tea
// convention used by the other overlays in this project.
func (t Timeline) Update(msg tea.Msg) (Timeline, tea.Cmd) {
	if !t.active {
		return t, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			t.active = false
			return t, nil

		case "up", "k":
			if t.cursor > 0 {
				t.cursor--
				if t.cursor < t.scroll {
					t.scroll = t.cursor
				}
			}

		case "down", "j":
			if t.cursor < len(t.entries)-1 {
				t.cursor++
				visH := t.visibleRows()
				if t.cursor >= t.scroll+visH {
					t.scroll = t.cursor - visH + 1
				}
			}

		case "enter":
			if len(t.entries) > 0 && t.cursor < len(t.entries) {
				t.selectedNote = t.entries[t.cursor].Path
				t.selected = true
				t.active = false
			}
			return t, nil

		case "tab":
			t.cycleView()
		}
	}
	return t, nil
}

// visibleRows returns how many entry rows fit in the viewport, accounting for
// group headers and chrome.
func (t Timeline) visibleRows() int {
	// Reserve lines for: title, separator, view selector, blank, bottom
	// separator, footer, padding.  Each group header takes 2-3 extra lines
	// but we keep the estimate conservative so scroll-into-view works.
	v := t.height - 12
	if v < 1 {
		v = 1
	}
	return v
}

// View renders the timeline overlay.  Value receiver as per project convention.
func (t Timeline) View() string {
	width := t.width * 2 / 3
	if width < 60 {
		width = 60
	}
	if width > 100 {
		width = 100
	}

	innerWidth := width - 6

	var b strings.Builder

	// ── Header ──────────────────────────────────────────────────────────
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	countStyle := lipgloss.NewStyle().Foreground(overlay0)

	b.WriteString(titleStyle.Render("  Timeline"))
	b.WriteString(countStyle.Render(" — " + smallNum(len(t.entries)) + " notes"))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).
		Render("  " + strings.Repeat("─", innerWidth-4)))
	b.WriteString("\n")

	// ── View mode selector ──────────────────────────────────────────────
	activeMode := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	inactiveMode := lipgloss.NewStyle().Foreground(overlay0)

	modes := []timelineView{timelineByDay, timelineByWeek, timelineByMonth}
	b.WriteString("  View: ")
	for i, m := range modes {
		name := viewModeName(m)
		if m == t.groupView {
			b.WriteString(activeMode.Render("[" + name + "]"))
		} else {
			b.WriteString(inactiveMode.Render(name))
		}
		if i < len(modes)-1 {
			b.WriteString(" ")
		}
	}
	b.WriteString("\n\n")

	// ── Body ────────────────────────────────────────────────────────────
	if len(t.entries) == 0 {
		b.WriteString(DimStyle.Render("  No notes found"))
		b.WriteString("\n")
	} else {
		t.renderBody(&b, innerWidth)
	}

	// ── Footer ──────────────────────────────────────────────────────────
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).
		Render("  " + strings.Repeat("─", innerWidth-4)))
	b.WriteString("\n")

	b.WriteString(RenderHelpBar([]struct{ Key, Desc string }{
		{"Enter", "open"}, {"Tab", "view"}, {"Esc", "close"},
	}))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(mauve).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

// renderBody writes the grouped entries into b, honouring scroll and cursor.
func (t Timeline) renderBody(b *strings.Builder, innerWidth int) {
	dotStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	connectorDim := lipgloss.NewStyle().Foreground(surface2)
	pathStyle := lipgloss.NewStyle().Foreground(lavender)
	previewStyle := lipgloss.NewStyle().Foreground(text)
	tagStyle := lipgloss.NewStyle().Foreground(blue)
	groupLabelStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	groupSep := lipgloss.NewStyle().Foreground(surface2)

	selectedFg := lipgloss.NewStyle().Foreground(peach).Bold(true)
	selectedBg := lipgloss.NewStyle().Background(surface0)

	// Build a list of renderable lines so we can apply vertical scrolling.
	type lineItem struct {
		text       string
		entryIndex int // -1 for non-entry lines (headers, blanks)
	}

	var lines []lineItem

	entryIdx := 0 // running counter through flat t.entries
	for gi, group := range t.groups {
		// Blank line between groups (not before the first)
		if gi > 0 {
			lines = append(lines, lineItem{text: "", entryIndex: -1})
		}

		// Group label (month or date header)
		lines = append(lines, lineItem{
			text:       "  " + groupLabelStyle.Render(group.Label),
			entryIndex: -1,
		})
		lines = append(lines, lineItem{
			text:       "  " + groupSep.Render(strings.Repeat("─", innerWidth-6)),
			entryIndex: -1,
		})

		// Day sub-headers when in month/week view
		buckets := t.buildDayBuckets(group, entryIdx)

		for _, bucket := range buckets {
			// Date dot line
			if t.groupView != timelineByDay {
				lines = append(lines, lineItem{
					text:       "  " + dotStyle.Render("● ") + groupLabelStyle.Render(bucket.dateStr),
					entryIndex: -1,
				})
			} else {
				lines = append(lines, lineItem{
					text:       "  " + dotStyle.Render("● ") + groupLabelStyle.Render(bucket.dateStr),
					entryIndex: -1,
				})
			}

			for ei, eIdx := range bucket.indices {
				entry := t.entries[eIdx]
				isLast := ei == len(bucket.indices)-1
				connector := "├─"
				if isLast {
					connector = "└─"
				}

				// Build note line
				title := entry.Title
				if title == "" {
					title = entry.Path
				}
				maxTitleLen := innerWidth/2 - 10
				if maxTitleLen < 12 {
					maxTitleLen = 12
				}
				if len(title) > maxTitleLen {
					title = title[:maxTitleLen-3] + "..."
				}

				preview := entry.Preview
				maxPreviewLen := innerWidth - maxTitleLen - 20
				if maxPreviewLen < 10 {
					maxPreviewLen = 10
				}
				if len(preview) > maxPreviewLen {
					preview = preview[:maxPreviewLen-3] + "..."
				}

				tagStr := ""
				if len(entry.Tags) > 0 {
					rendered := make([]string, 0, len(entry.Tags))
					for _, tg := range entry.Tags {
						if len(rendered) >= 3 {
							break // show at most 3 tags per line
						}
						rendered = append(rendered, tagStyle.Render("#"+tg))
					}
					tagStr = "  " + strings.Join(rendered, " ")
				}

				isSel := eIdx == t.cursor

				if isSel {
					line := "    " + connectorDim.Render(connector+" ") +
						selectedFg.Render(title)
					if preview != "" {
						line += DimStyle.Render(" ─── ") +
							selectedFg.Render("\""+preview+"\"")
					}
					line += tagStr

					b2 := selectedBg.Width(innerWidth).Render(line)
					lines = append(lines, lineItem{
						text:       ThemeAccentBar + b2[1:], // accent bar replaces first char
						entryIndex: eIdx,
					})
				} else {
					line := "    " + connectorDim.Render(connector+" ") +
						pathStyle.Render(title)
					if preview != "" {
						line += DimStyle.Render(" ─── ") +
							previewStyle.Render("\""+preview+"\"")
					}
					line += tagStr
					lines = append(lines, lineItem{text: line, entryIndex: eIdx})
				}
			}
		}
		entryIdx += len(group.Entries)
	}

	// Scrolling: find which output line corresponds to the cursor entry so we
	// can keep it visible.
	cursorLine := 0
	for i, li := range lines {
		if li.entryIndex == t.cursor {
			cursorLine = i
			break
		}
	}

	visH := t.height - 14
	if visH < 5 {
		visH = 5
	}

	scroll := t.scroll
	// Adjust scroll so cursorLine stays visible.
	if cursorLine < scroll {
		scroll = cursorLine
	}
	if cursorLine >= scroll+visH {
		scroll = cursorLine - visH + 1
	}
	if scroll < 0 {
		scroll = 0
	}
	maxScroll := len(lines) - visH
	if maxScroll < 0 {
		maxScroll = 0
	}
	if scroll > maxScroll {
		scroll = maxScroll
	}

	end := scroll + visH
	if end > len(lines) {
		end = len(lines)
	}

	for i := scroll; i < end; i++ {
		b.WriteString(lines[i].text)
		if i < end-1 {
			b.WriteString("\n")
		}
	}
}

// dayBucket groups entries inside a TimelineGroup by their calendar day so
// that month/week views still show per-day dots.
type dayBucket struct {
	dateStr string
	indices []int // indices into t.entries
}

// buildDayBuckets splits a group's entries into per-day buckets, returning the
// date string and the flat-index into t.entries for each entry.
func (t Timeline) buildDayBuckets(group TimelineGroup, startIdx int) []dayBucket {
	var buckets []dayBucket
	prevDay := ""

	for i, entry := range group.Entries {
		day := entry.Date.Format("2006-01-02")
		if day != prevDay {
			buckets = append(buckets, dayBucket{dateStr: day})
			prevDay = day
		}
		buckets[len(buckets)-1].indices = append(
			buckets[len(buckets)-1].indices, startIdx+i,
		)
	}
	return buckets
}
