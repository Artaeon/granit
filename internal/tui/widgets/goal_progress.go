package widgets

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/profiles"
)

// goalProgressWidget shows up to N active goals with progress
// bars. Read-only — the goals overlay (palette: GoalsMode) is for
// editing.
type goalProgressWidget struct{}

func newGoalProgressWidget() Widget { return &goalProgressWidget{} }

func (goalProgressWidget) ID() string                     { return profiles.WidgetGoalProgress }
func (goalProgressWidget) Title() string                  { return "Goals" }
func (goalProgressWidget) MinSize() (int, int)            { return 30, 3 }
func (goalProgressWidget) DataNeeds() []profiles.DataKind { return []profiles.DataKind{profiles.DataGoals} }

func (w goalProgressWidget) Render(ctx WidgetCtx, width, height int) string {
	if len(ctx.Goals) == 0 {
		return faintLine("no active goals — set some in Goals mode.", width)
	}
	visible := height
	if visible < 1 {
		visible = 1
	}
	goals := ctx.Goals
	if len(goals) > visible {
		goals = goals[:visible]
	}
	var b strings.Builder
	for _, g := range goals {
		bar := progressBar(g.Progress, width-len(g.Name)-12)
		row := fmt.Sprintf("%s  %s  %s",
			truncRight(g.Name, 18),
			bar,
			lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf("%2.0f%%", g.Progress*100)))
		b.WriteString(row + "\n")
	}
	return b.String()
}

func (w goalProgressWidget) HandleKey(WidgetCtx, string) (bool, tea.Cmd) {
	return false, nil
}

// progressBar renders a fixed-width unicode bar. Block + light
// shade for filled/empty so it reads at a glance.
func progressBar(pct float64, width int) string {
	if width < 4 {
		width = 4
	}
	if pct < 0 {
		pct = 0
	}
	if pct > 1 {
		pct = 1
	}
	filled := int(pct * float64(width))
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}
