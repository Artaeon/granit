package widgets

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/profiles"
)

// habitStreakWidget shows today's habit checklist with streak
// numbers. v1 is read-only; toggling habits stays in the
// HabitTracker overlay (Alt+B).
type habitStreakWidget struct{}

func newHabitStreakWidget() Widget { return &habitStreakWidget{} }

func (habitStreakWidget) ID() string                     { return profiles.WidgetHabitStreak }
func (habitStreakWidget) Title() string                  { return "Habits" }
func (habitStreakWidget) MinSize() (int, int)            { return 22, 3 }
func (habitStreakWidget) DataNeeds() []profiles.DataKind { return []profiles.DataKind{profiles.DataHabits} }

func (w habitStreakWidget) Render(ctx WidgetCtx, width, height int) string {
	if len(ctx.Habits) == 0 {
		return faintLine("no habits — add some in Habit Tracker (Alt+B).", width)
	}
	visible := height
	if visible < 1 {
		visible = 1
	}
	habits := ctx.Habits
	if len(habits) > visible {
		habits = habits[:visible]
	}
	var b strings.Builder
	for _, h := range habits {
		mark := "·"
		if h.DoneToday {
			mark = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render("✓")
		}
		streak := lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf("%dd", h.Streak))
		row := fmt.Sprintf("%s %s  %s",
			mark,
			truncRight(h.Name, width-len(streak)-5),
			streak)
		b.WriteString(row + "\n")
	}
	return b.String()
}

func (w habitStreakWidget) HandleKey(WidgetCtx, string) (bool, tea.Cmd) {
	return false, nil
}
