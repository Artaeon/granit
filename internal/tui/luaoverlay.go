package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// luaRunResultMsg carries the result of a Lua script execution.
type luaRunResultMsg struct {
	result LuaResult
}

// LuaOverlay shows available Lua scripts and lets the user run them.
type LuaOverlay struct {
	active  bool
	width   int
	height  int
	scripts []LuaScript
	cursor  int
	message string
	engine  *LuaEngine

	// Context for script execution
	notePath    string
	noteContent string
	noteMeta    map[string]string
}

func NewLuaOverlay() LuaOverlay {
	return LuaOverlay{}
}

func (lo *LuaOverlay) IsActive() bool {
	return lo.active
}

func (lo *LuaOverlay) SetEngine(engine *LuaEngine) {
	lo.engine = engine
}

func (lo *LuaOverlay) Open(notePath, noteContent string, meta map[string]string) {
	lo.active = true
	lo.cursor = 0
	lo.message = ""
	lo.notePath = notePath
	lo.noteContent = noteContent
	lo.noteMeta = meta
	if lo.engine != nil {
		lo.engine.LoadScripts()
		lo.scripts = lo.engine.GetScripts()
	}
}

func (lo *LuaOverlay) Close() {
	lo.active = false
}

func (lo *LuaOverlay) SetSize(w, h int) {
	lo.width = w
	lo.height = h
}

// GetResult returns the last script result (for app.go to process).
func (lo *LuaOverlay) GetResult() *LuaResult {
	return nil // results come through luaRunResultMsg
}

func (lo LuaOverlay) Update(msg tea.Msg) (LuaOverlay, tea.Cmd) {
	if !lo.active {
		return lo, nil
	}

	switch msg := msg.(type) {
	case luaRunResultMsg:
		r := msg.result
		if r.Error != nil {
			lo.message = "Error: " + r.Error.Error()
		} else if r.Message != "" {
			lo.message = r.Message
		} else {
			lo.message = "Script completed"
		}
		return lo, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			lo.active = false
			return lo, nil
		case "up", "k":
			if lo.cursor > 0 {
				lo.cursor--
			}
		case "down", "j":
			if lo.cursor < len(lo.scripts)-1 {
				lo.cursor++
			}
		case "enter":
			if lo.engine != nil && lo.cursor < len(lo.scripts) {
				script := lo.scripts[lo.cursor]
				engine := lo.engine
				notePath := lo.notePath
				noteContent := lo.noteContent
				noteMeta := lo.noteMeta
				return lo, func() tea.Msg {
					result := engine.RunScript(script, notePath, noteContent, noteMeta)
					return luaRunResultMsg{result: result}
				}
			}
		}
	}
	return lo, nil
}

func (lo LuaOverlay) View() string {
	width := lo.width * 2 / 3
	if width < 50 {
		width = 50
	}
	if width > 80 {
		width = 80
	}

	var b strings.Builder

	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  " + IconBotChar + " Lua Scripts")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n")

	if len(lo.scripts) == 0 {
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  No Lua scripts found"))
		b.WriteString("\n\n")
		b.WriteString(DimStyle.Render("  Add .lua files to:"))
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("    <vault>/.granit/lua/"))
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("    ~/.config/granit/lua/"))
	} else {
		b.WriteString("\n")
		for i, s := range lo.scripts {
			icon := lipgloss.NewStyle().Foreground(blue).Render(" ")
			line := "  " + icon + " " + s.Name

			if i == lo.cursor {
				b.WriteString(lipgloss.NewStyle().
					Background(surface0).
					Foreground(peach).
					Bold(true).
					Width(width - 6).
					Render(line))
			} else {
				b.WriteString(NormalItemStyle.Render(line))
			}
			b.WriteString("\n")

			// Show path dimmed
			shortPath := s.Path
			if len(shortPath) > width-12 {
				shortPath = "..." + shortPath[len(shortPath)-width+15:]
			}
			if i == lo.cursor {
				b.WriteString(lipgloss.NewStyle().
					Background(surface0).
					Foreground(overlay0).
					Width(width - 6).
					Render("    " + shortPath))
			} else {
				b.WriteString(DimStyle.Render("    " + shortPath))
			}
			b.WriteString("\n")

			if i < len(lo.scripts)-1 {
				b.WriteString("\n")
			}
		}
	}

	if lo.message != "" {
		b.WriteString("\n")
		color := yellow
		if strings.HasPrefix(lo.message, "Error") {
			color = red
		}
		b.WriteString(lipgloss.NewStyle().Foreground(color).Render("  " + lo.message))
	}

	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Enter: run  Esc: close"))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}
