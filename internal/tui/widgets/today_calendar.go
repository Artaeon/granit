package widgets

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/profiles"
)

// todayCalendarWidget lists today's events and time blocks. Read-
// only at v1 — the calendar overlay (Alt+C) is where you create
// blocks. Click-equivalent (enter on focused row) opens the
// calendar at that block.
type todayCalendarWidget struct{}

func newTodayCalendarWidget() Widget { return &todayCalendarWidget{} }

func (todayCalendarWidget) ID() string                     { return profiles.WidgetTodayCalendar }
func (todayCalendarWidget) Title() string                  { return "Today" }
func (todayCalendarWidget) MinSize() (int, int)            { return 18, 3 }
func (todayCalendarWidget) DataNeeds() []profiles.DataKind { return []profiles.DataKind{profiles.DataCalendar, profiles.DataPlanner} }

func (w todayCalendarWidget) Render(ctx WidgetCtx, width, height int) string {
	header := lipgloss.NewStyle().Bold(true).Render(time.Now().Format("Mon Jan 2"))
	if len(ctx.TodayEvents) == 0 {
		return header + "\n" + faintLine("no events scheduled.", width)
	}
	visible := height - 2
	if visible < 1 {
		visible = 1
	}
	events := ctx.TodayEvents
	if len(events) > visible {
		events = events[:visible]
	}
	var b strings.Builder
	b.WriteString(header + "\n")
	for _, ev := range events {
		kindGlyph := "·"
		switch ev.Kind {
		case "block":
			kindGlyph = "▣"
		case "deadline":
			kindGlyph = "!"
		}
		row := fmt.Sprintf("%s %s %s",
			lipgloss.NewStyle().Faint(true).Render(ev.Time),
			kindGlyph,
			truncRight(ev.Title, width-10))
		b.WriteString(row + "\n")
	}
	return b.String()
}

func (w todayCalendarWidget) HandleKey(ctx WidgetCtx, key string) (bool, tea.Cmd) {
	return false, nil // read-only at v1
}
