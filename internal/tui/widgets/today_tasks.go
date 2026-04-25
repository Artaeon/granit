package widgets

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/profiles"
	"github.com/artaeon/granit/internal/tasks"
)

// todayTasksWidget shows tasks due today plus tasks scheduled
// for today's time blocks. Single-line per task with a checkbox,
// priority chip, and due time when present. Space-key on the
// focused row completes the highlighted task.
type todayTasksWidget struct{}

func newTodayTasksWidget() Widget { return &todayTasksWidget{} }

func (todayTasksWidget) ID() string                     { return profiles.WidgetTodayTasks }
func (todayTasksWidget) Title() string                  { return "Today" }
func (todayTasksWidget) MinSize() (int, int)            { return 30, 4 }
func (todayTasksWidget) DataNeeds() []profiles.DataKind { return []profiles.DataKind{profiles.DataTasks} }

func (w todayTasksWidget) Render(ctx WidgetCtx, width, height int) string {
	today := time.Now().Format("2006-01-02")
	due := filterDueByDate(ctx.Tasks, today)
	if len(due) == 0 {
		return faintLine("nothing due today.", width)
	}
	cursor, _ := ctx.Config["cursor"].(int)

	// Render up to height-1 rows (leave one for footer count).
	visible := height - 1
	if visible < 1 {
		visible = 1
	}
	if len(due) > visible {
		due = due[:visible]
	}
	var b strings.Builder
	for i, t := range due {
		prefix := "  "
		if i == cursor {
			prefix = lipgloss.NewStyle().Bold(true).Render("▸ ")
		}
		check := "[ ]"
		if t.Done {
			check = "[x]"
		}
		prio := priorityChip(t.Priority)
		row := fmt.Sprintf("%s%s %s %s", prefix, check, prio, truncRight(t.Text, width-12))
		b.WriteString(row + "\n")
	}
	footer := lipgloss.NewStyle().Faint(true).Render(
		fmt.Sprintf("%d due · space=done · enter=open", len(due)))
	b.WriteString(footer)
	return b.String()
}

func (w todayTasksWidget) HandleKey(ctx WidgetCtx, key string) (bool, tea.Cmd) {
	cursor, _ := ctx.Config["cursor"].(int)
	today := time.Now().Format("2006-01-02")
	due := filterDueByDate(ctx.Tasks, today)
	switch key {
	case "up", "k":
		if cursor > 0 {
			ctx.Config["cursor"] = cursor - 1
		}
		return true, nil
	case "down", "j":
		if cursor < len(due)-1 {
			ctx.Config["cursor"] = cursor + 1
		}
		return true, nil
	case " ", "space":
		if cursor < len(due) && ctx.CompleteTask != nil {
			ctx.CompleteTask(due[cursor].ID)
		}
		return true, nil
	case "enter":
		if cursor < len(due) && ctx.OpenNote != nil {
			ctx.OpenNote(due[cursor].NotePath)
		}
		return true, nil
	}
	return false, nil
}

// filterDueByDate returns incomplete tasks that should appear in
// today's view. The criteria, in order:
//
//   1. DueDate matches today (filename-inferred or explicit 📅)
//   2. ScheduledStart falls on today's date (set by triage `s`
//      or the calendar planner)
//
// Snoozed tasks whose ScheduledStart is still in the future are
// excluded — that's the whole point of snooze, "don't bug me
// until then." Dropped tasks are excluded by triage state.
//
// Done tasks are always excluded.
func filterDueByDate(all []tasks.Task, ymd string) []tasks.Task {
	now := time.Now()
	var out []tasks.Task
	for _, t := range all {
		if t.Done {
			continue
		}
		if t.Triage == tasks.TriageDropped {
			continue
		}
		// Snoozed-into-the-future: respect the user's "not now."
		if t.Triage == tasks.TriageSnoozed && t.ScheduledStart != nil && t.ScheduledStart.After(now) {
			continue
		}
		if t.DueDate == ymd {
			out = append(out, t)
			continue
		}
		if t.ScheduledStart != nil && t.ScheduledStart.Format("2006-01-02") == ymd {
			out = append(out, t)
		}
	}
	return out
}

// priorityChip is a 1-char tag for the task's priority.
func priorityChip(p int) string {
	switch p {
	case 4:
		return "🔺"
	case 3:
		return "⏫"
	case 2:
		return "🔼"
	case 1:
		return "🔽"
	}
	return " "
}

func truncRight(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if len([]rune(s)) <= max {
		return s
	}
	r := []rune(s)
	if max < 2 {
		return string(r[:max])
	}
	return string(r[:max-1]) + "…"
}

func faintLine(msg string, width int) string {
	style := lipgloss.NewStyle().Faint(true)
	if width > 0 {
		style = style.Width(width)
	}
	return style.Render(msg)
}
