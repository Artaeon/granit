package widgets

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/profiles"
)

// triageCountWidget is the simplest cell — one big number for
// "tasks waiting in the inbox" and a hint to open the triage
// view. The number's job is to nag (or reassure with a 0).
type triageCountWidget struct{}

func newTriageCountWidget() Widget { return &triageCountWidget{} }

func (triageCountWidget) ID() string                     { return profiles.WidgetTriageCount }
func (triageCountWidget) Title() string                  { return "Triage" }
func (triageCountWidget) MinSize() (int, int)            { return 12, 3 }
func (triageCountWidget) DataNeeds() []profiles.DataKind { return []profiles.DataKind{profiles.DataTriage} }

func (w triageCountWidget) Render(ctx WidgetCtx, width, height int) string {
	num := ctx.TriageInbox
	color := lipgloss.Color("10") // green when zero
	switch {
	case num >= 20:
		color = lipgloss.Color("9") // red — backlog warning
	case num >= 5:
		color = lipgloss.Color("11") // yellow — getting full
	}
	big := lipgloss.NewStyle().Foreground(color).Bold(true).Render(fmt.Sprintf("%d", num))
	hint := lipgloss.NewStyle().Faint(true).Render("enter to triage")
	return big + "\n" + hint
}

func (w triageCountWidget) HandleKey(ctx WidgetCtx, key string) (bool, tea.Cmd) {
	if key == "enter" && ctx.OpenTriage != nil {
		ctx.OpenTriage()
		return true, nil
	}
	return false, nil
}
