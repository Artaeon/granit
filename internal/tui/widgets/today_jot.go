package widgets

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/profiles"
)

// todayJotWidget is the front-door capture cell. Power users land
// here on Daily Hub open and start typing — Enter creates a task
// without opening any modal. Multi-line buffer; Shift+Enter
// inserts a newline within a single capture, Enter commits.
//
// Stateless across renders — the buffer lives in the controller's
// per-cell state, not on the widget itself, so the widget struct
// can be a singleton.
type todayJotWidget struct{}

func newTodayJotWidget() Widget { return &todayJotWidget{} }

func (todayJotWidget) ID() string                       { return profiles.WidgetTodayJot }
func (todayJotWidget) Title() string                    { return "Jot" }
func (todayJotWidget) MinSize() (int, int)              { return 30, 4 }
func (todayJotWidget) DataNeeds() []profiles.DataKind   { return nil }

func (w todayJotWidget) Render(ctx WidgetCtx, width, height int) string {
	buf, _ := ctx.Config["buffer"].(string)
	hint := lipgloss.NewStyle().Faint(true).Render("type · enter saves · esc clears")

	body := buf
	if body == "" {
		body = lipgloss.NewStyle().Faint(true).Render("brain dump…")
	}
	// Pad to fill the cell so the grid border draws cleanly.
	for strings.Count(body, "\n") < height-2 {
		body += "\n"
	}
	if width > 0 {
		body = lipgloss.NewStyle().Width(width).Render(body)
	}
	return body + "\n" + hint
}

func (w todayJotWidget) HandleKey(ctx WidgetCtx, key string) (bool, tea.Cmd) {
	buf, _ := ctx.Config["buffer"].(string)
	switch key {
	case "enter":
		text := strings.TrimSpace(buf)
		if text == "" {
			return true, nil
		}
		if ctx.CreateTask != nil {
			_ = ctx.CreateTask(text)
		}
		// Clear the buffer via the controller; widget can't
		// mutate ctx itself. Returning a custom message would be
		// the way; for now we return handled=true and let the
		// controller infer "this was an enter, clear buffer."
		return true, nil
	case "esc":
		return true, nil // controller clears buffer
	case "backspace":
		if len(buf) > 0 {
			ctx.Config["buffer"] = buf[:len(buf)-1]
		}
		return true, nil
	default:
		// Single printable char — append. Multi-char keys (alt+x,
		// ctrl+x) bubble.
		if len(key) == 1 {
			ctx.Config["buffer"] = buf + key
			return true, nil
		}
		return false, nil
	}
}
