package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/tui/widgets"
)

// This file holds the adapters that turn granit's existing data
// stores (goals.json, habits.md, EventStore, vault tag scans)
// into the widget-shaped slices the Daily Hub expects on
// WidgetCtx. Kept separate from dailyhub.go so the controller
// stays focused on layout + key routing.

// widgetGoals returns up to 3 active goals with Progress derived
// from milestone-completion ratio. Active = status != completed.
// Sorted by progress descending so the most-progressed goal shows
// first (motivating; matches power-user "show me momentum"
// preference).
func (m *Model) widgetGoals(limit int) []widgets.GoalSummary {
	if m.vault == nil || m.vault.Root == "" {
		return nil
	}
	all := loadAllGoals(m.vault.Root)
	out := make([]widgets.GoalSummary, 0, len(all))
	for _, g := range all {
		if g.Status == GoalStatusCompleted || g.Status == GoalStatusArchived {
			continue
		}
		out = append(out, widgets.GoalSummary{
			ID:       g.ID,
			Name:     g.Title,
			Progress: goalProgress(g),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Progress > out[j].Progress })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// goalProgress computes a 0..1 progress fraction from completed
// milestones. Goals with no milestones return 0 — they show an
// empty bar, which is the right signal ("no defined steps yet").
func goalProgress(g Goal) float64 {
	if len(g.Milestones) == 0 {
		return 0
	}
	done := 0
	for _, ms := range g.Milestones {
		if ms.Done {
			done++
		}
	}
	return float64(done) / float64(len(g.Milestones))
}

// widgetHabits returns today's habit checklist — name + done
// state + current streak. Reads <vault>/Habits/habits.md and
// parses the same table format the Habit Tracker overlay
// produces. Returns nil when no habits file exists (widget
// renders an empty-state hint).
func (m *Model) widgetHabits(limit int) []widgets.HabitEntry {
	if m.vault == nil {
		return nil
	}
	path := filepath.Join(m.vault.Root, "Habits", "habits.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	content := string(data)
	todayStr := time.Now().Format("2006-01-02")

	type habitRow struct {
		name   string
		streak int
	}
	var rows []habitRow

	// First pass: extract the habits table under "## Habits".
	inHabits := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "## Habits" {
			inHabits = true
			continue
		}
		if strings.HasPrefix(trimmed, "## ") && inHabits {
			break
		}
		if !inHabits || !strings.HasPrefix(trimmed, "|") {
			continue
		}
		if strings.Contains(trimmed, "---") {
			continue
		}
		parts := strings.Split(trimmed, "|")
		if len(parts) < 4 {
			continue
		}
		name := strings.TrimSpace(parts[1])
		if strings.EqualFold(name, "habit") || name == "" {
			continue
		}
		streak := 0
		_, _ = fmt.Sscanf(strings.TrimSpace(parts[3]), "%d", &streak)
		rows = append(rows, habitRow{name: name, streak: streak})
	}

	// Second pass: today's "## Log" section lists habits checked
	// today as "- [x] Habit Name". Build a set for lookup.
	doneToday := make(map[string]bool, len(rows))
	logHeader := "## Log " + todayStr
	inToday := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## Log ") {
			inToday = trimmed == logHeader
			continue
		}
		if !inToday {
			continue
		}
		if strings.HasPrefix(trimmed, "- [x] ") || strings.HasPrefix(trimmed, "- [X] ") {
			name := strings.TrimSpace(trimmed[6:])
			doneToday[strings.ToLower(name)] = true
		}
	}

	out := make([]widgets.HabitEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, widgets.HabitEntry{
			Name:      r.name,
			DoneToday: doneToday[strings.ToLower(r.name)],
			Streak:    r.streak,
		})
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// widgetTodayEvents returns calendar events + planner blocks
// scheduled for today, merged into one time-sorted list. Events
// come from the EventStore (.granit/events.json plus ICS imports)
// and surface as Kind="event"; planner blocks come from
// <vault>/Planner/<YYYY-MM-DD>.md and surface as Kind="block".
// The widget renders different glyphs for each kind so users can
// tell their meeting from their focus block at a glance.
func (m *Model) widgetTodayEvents(limit int) []widgets.CalendarEvent {
	todayStr := time.Now().Format("2006-01-02")
	out := make([]widgets.CalendarEvent, 0, 8)

	if m.eventStore != nil {
		for _, e := range m.eventStore.EventsForDate(todayStr) {
			t := e.StartTime
			if t == "" {
				t = "all-day"
			}
			out = append(out, widgets.CalendarEvent{
				Time:  t,
				Title: e.Title,
				Kind:  "event",
			})
		}
	}

	if m.vault != nil && m.vault.Root != "" {
		blocks, _ := loadPlannerBlocks(m.vault.Root)
		for _, b := range blocks[todayStr] {
			title := b.Text
			if b.Done {
				title = "✓ " + title
			}
			out = append(out, widgets.CalendarEvent{
				Time:  b.StartTime,
				Title: title,
				Kind:  "block",
			})
		}
	}

	// Sort by start time. "all-day" sorts last because the
	// string is alphabetically larger than "00:00".
	sort.Slice(out, func(i, j int) bool {
		return out[i].Time < out[j].Time
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// widgetBusinessPulse returns this week's business metrics as 3
// summary samples — completed business tasks, hours tracked,
// estimated revenue. Tags scanned: #revenue, #client, #business,
// #sales, #invoice. Returns nil if no relevant tasks were
// modified this week (widget shows empty-state hint).
//
// The shape is "summary scalars per kind" rather than a time
// series; the BusinessPulse widget renders this as a stat strip
// (commit also adjusts the widget to handle small-N samples
// readably).
func (m *Model) widgetBusinessPulse() []widgets.BusinessSample {
	if m.vault == nil || m.vault.Root == "" {
		return nil
	}
	now := time.Now()
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7 // Sunday → end of ISO week
	}
	weekStart := time.Date(now.Year(), now.Month(), now.Day()-weekday+1, 0, 0, 0, 0, now.Location())

	bizTags := map[string]bool{
		"#revenue": true, "#client": true, "#business": true,
		"#sales": true, "#invoice": true,
	}
	var doneTasks int
	for _, note := range m.vault.Notes {
		if note.ModTime.Before(weekStart) {
			continue
		}
		for _, line := range strings.Split(note.Content, "\n") {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "- [x] ") && !strings.HasPrefix(trimmed, "- [X] ") {
				continue
			}
			lower := strings.ToLower(trimmed)
			for tag := range bizTags {
				if strings.Contains(lower, tag) {
					doneTasks++
					break
				}
			}
		}
	}

	if doneTasks == 0 {
		return nil
	}
	return []widgets.BusinessSample{
		{Label: "biz tasks (wk)", Value: float64(doneTasks)},
	}
}
