package widgets

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/profiles"
)

// recentNotesWidget lists the N most-recently-modified notes.
// Up/down moves the cursor; Enter opens the note via the
// controller's OpenNote callback.
type recentNotesWidget struct{}

func newRecentNotesWidget() Widget { return &recentNotesWidget{} }

func (recentNotesWidget) ID() string                     { return profiles.WidgetRecentNotes }
func (recentNotesWidget) Title() string                  { return "Recent Notes" }
func (recentNotesWidget) MinSize() (int, int)            { return 24, 3 }
func (recentNotesWidget) DataNeeds() []profiles.DataKind { return []profiles.DataKind{profiles.DataNotes} }

func (w recentNotesWidget) Render(ctx WidgetCtx, width, height int) string {
	if len(ctx.RecentNotes) == 0 {
		return faintLine("vault is empty.", width)
	}
	cursor, _ := ctx.Config["cursor"].(int)
	visible := height
	if visible < 1 {
		visible = 1
	}
	notes := ctx.RecentNotes
	if len(notes) > visible {
		notes = notes[:visible]
	}
	var b strings.Builder
	for i, n := range notes {
		prefix := "  "
		if i == cursor {
			prefix = lipgloss.NewStyle().Bold(true).Render("▸ ")
		}
		ago := lipgloss.NewStyle().Faint(true).Render(n.Modified)
		row := fmt.Sprintf("%s%s  %s",
			prefix,
			truncRight(n.Title, width-len(n.Modified)-6),
			ago)
		b.WriteString(row + "\n")
	}
	return b.String()
}

func (w recentNotesWidget) HandleKey(ctx WidgetCtx, key string) (bool, tea.Cmd) {
	cursor, _ := ctx.Config["cursor"].(int)
	switch key {
	case "up", "k":
		if cursor > 0 {
			ctx.Config["cursor"] = cursor - 1
		}
		return true, nil
	case "down", "j":
		if cursor < len(ctx.RecentNotes)-1 {
			ctx.Config["cursor"] = cursor + 1
		}
		return true, nil
	case "enter":
		if cursor < len(ctx.RecentNotes) && ctx.OpenNote != nil {
			ctx.OpenNote(ctx.RecentNotes[cursor].Path)
		}
		return true, nil
	}
	return false, nil
}
