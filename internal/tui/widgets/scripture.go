package widgets

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/profiles"
)

// scriptureWidget shows the day's verse — salvaged from the
// existing dashboard.go scripture display so users who relied on
// the daily-centering moment don't lose it in the relaunch. The
// data comes from the existing scripture.go subsystem (21
// built-in verses + .granit/scriptures.md user overrides), wired
// into ctx by the Daily Hub controller.
type scriptureWidget struct{}

func newScriptureWidget() Widget { return &scriptureWidget{} }

func (scriptureWidget) ID() string                     { return profiles.WidgetScripture }
func (scriptureWidget) Title() string                  { return "Verse" }
func (scriptureWidget) MinSize() (int, int)            { return 36, 3 }
func (scriptureWidget) DataNeeds() []profiles.DataKind { return []profiles.DataKind{profiles.DataScripture} }

func (w scriptureWidget) Render(ctx WidgetCtx, width, height int) string {
	if ctx.Scripture.Text == "" {
		return faintLine("no scripture configured.", width)
	}
	textStyle := lipgloss.NewStyle().Italic(true)
	refStyle := lipgloss.NewStyle().Faint(true).Bold(true)

	body := textStyle.Render(wrap(ctx.Scripture.Text, width))
	ref := refStyle.Render("— " + ctx.Scripture.Reference)
	return body + "\n" + ref
}

func (w scriptureWidget) HandleKey(WidgetCtx, string) (bool, tea.Cmd) {
	return false, nil
}

// wrap is a minimal word-wrap. Doesn't try to be smart about
// lipgloss-rendered escape sequences — the input here is plain
// ASCII verses so it's safe.
func wrap(s string, width int) string {
	if width <= 0 {
		return s
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return ""
	}
	var b strings.Builder
	col := 0
	for i, w := range words {
		if i > 0 {
			if col+1+len(w) > width {
				b.WriteByte('\n')
				col = 0
			} else {
				b.WriteByte(' ')
				col++
			}
		}
		b.WriteString(w)
		col += len(w)
	}
	return b.String()
}
