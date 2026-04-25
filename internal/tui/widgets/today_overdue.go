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

// todayOverdueWidget surfaces incomplete tasks whose DueDate has
// passed. Pulled out from dashboard.go's overdue view as a
// dedicated cell so each profile can position it independently.
// Same Space=complete / Enter=open key handling as today.tasks.
type todayOverdueWidget struct{}

func newTodayOverdueWidget() Widget { return &todayOverdueWidget{} }

func (todayOverdueWidget) ID() string                     { return profiles.WidgetTodayOverdue }
func (todayOverdueWidget) Title() string                  { return "Overdue" }
func (todayOverdueWidget) MinSize() (int, int)            { return 30, 4 }
func (todayOverdueWidget) DataNeeds() []profiles.DataKind { return []profiles.DataKind{profiles.DataTasks} }

func (w todayOverdueWidget) Render(ctx WidgetCtx, width, height int) string {
	now := time.Now()
	overdue := filterOverdue(ctx.Tasks, now)
	if len(overdue) == 0 {
		return faintLine("clean — no overdue tasks.", width)
	}
	cursor, _ := ctx.Config["cursor"].(int)

	visible := height - 1
	if visible < 1 {
		visible = 1
	}
	if len(overdue) > visible {
		overdue = overdue[:visible]
	}
	var b strings.Builder
	for i, t := range overdue {
		prefix := "  "
		if i == cursor {
			prefix = lipgloss.NewStyle().Bold(true).Render("▸ ")
		}
		// Days overdue for context — power users want the number.
		days := daysOverdue(t.DueDate, now)
		ago := ""
		if days > 0 {
			ago = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(fmt.Sprintf("%dd", days))
		}
		row := fmt.Sprintf("%s%s [ ] %s %s",
			prefix, ago, priorityChip(t.Priority),
			truncRight(t.Text, width-15))
		b.WriteString(row + "\n")
	}
	footer := lipgloss.NewStyle().Faint(true).Render(
		fmt.Sprintf("%d overdue · space=done · enter=open", len(overdue)))
	b.WriteString(footer)
	return b.String()
}

func (w todayOverdueWidget) HandleKey(ctx WidgetCtx, key string) (bool, tea.Cmd) {
	cursor, _ := ctx.Config["cursor"].(int)
	overdue := filterOverdue(ctx.Tasks, time.Now())
	switch key {
	case "up", "k":
		if cursor > 0 {
			ctx.Config["cursor"] = cursor - 1
		}
		return true, nil
	case "down", "j":
		if cursor < len(overdue)-1 {
			ctx.Config["cursor"] = cursor + 1
		}
		return true, nil
	case " ", "space":
		if cursor < len(overdue) && ctx.CompleteTask != nil {
			ctx.CompleteTask(overdue[cursor].ID)
		}
		return true, nil
	case "enter":
		if cursor < len(overdue) && ctx.OpenNote != nil {
			ctx.OpenNote(overdue[cursor].NotePath)
		}
		return true, nil
	}
	return false, nil
}

// filterOverdue returns incomplete tasks whose DueDate is before
// today (using local time, midnight boundary).
func filterOverdue(all []tasks.Task, now time.Time) []tasks.Task {
	cutoff := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var out []tasks.Task
	for _, t := range all {
		if t.Done || t.DueDate == "" {
			continue
		}
		due, err := time.ParseInLocation("2006-01-02", t.DueDate, now.Location())
		if err != nil {
			continue
		}
		if due.Before(cutoff) {
			out = append(out, t)
		}
	}
	return out
}

func daysOverdue(dueDate string, now time.Time) int {
	due, err := time.ParseInLocation("2006-01-02", dueDate, now.Location())
	if err != nil {
		return 0
	}
	cutoff := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return int(cutoff.Sub(due).Hours() / 24)
}
