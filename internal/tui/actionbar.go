package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderActionBar gives the user a persistent, context-aware action strip. It
// is deliberately terse: power users need discoverability without a help modal.
func (m Model) renderActionBar(width int) string {
	items := []struct{ key, desc string }{
		{"Ctrl+X", "commands"},
		{"Alt+C", "center"},
		{"Ctrl+K", "tasks"},
		{"Alt+W", "profile"},
	}

	if m.tabBar != nil {
		if id, ok := m.tabBar.ActiveFeature(); ok {
			switch id {
			case FeatCommandCenter:
				items = []struct{ key, desc string }{
					{"Tab", "section"}, {"Enter", "act"}, {"Space", "focus"},
					{"Ctrl+K", "tasks"}, {"Ctrl+W", "close"},
				}
			case FeatTaskManager:
				items = []struct{ key, desc string }{
					{"a", "add"}, {"F", "filter"}, {"D", "dense"},
					{"v", "bulk"}, {"?", "help"}, {"Ctrl+W", "close"},
				}
				if task, ok := m.taskManager.selectedTask(); ok {
					items = []struct{ key, desc string }{
						{"x", "toggle"}, {"g", "source"}, {"i", "edit"},
						{"d", "due"}, {"E", "estimate"}, {"B", "block"},
						{"f", "focus"}, {"a", "add"},
					}
					if task.Done {
						items = []struct{ key, desc string }{
							{"x", "reopen"}, {"g", "source"}, {"n", "note"},
							{"a", "add"}, {"Ctrl+W", "close"},
						}
					}
				}
			case FeatCalendar:
				items = []struct{ key, desc string }{
					{"a", "event"}, {"b", "block"}, {"t", "today"},
					{"Tab", "view"}, {"Ctrl+W", "close"},
				}
			case FeatProject:
				items = []struct{ key, desc string }{
					{"n", "new"}, {"Enter", "open"}, {"Tab", "view"},
					{"Ctrl+W", "close"},
				}
			}
		}
	}

	if m.activeNote != "" {
		items = []struct{ key, desc string }{
			{"Ctrl+S", "save"}, {"Ctrl+E", "view"}, {"Ctrl+F", "find"},
			{"Ctrl+O", "outline"}, {"Ctrl+B", "star"}, {"F4", "rename"},
		}
		if m.editor.modified {
			items = append([]struct{ key, desc string }{{"modified", "unsaved"}}, items...)
		}
	}

	var parts []string
	keyStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(overlay1)
	for _, item := range items {
		parts = append(parts, keyStyle.Render(item.key)+" "+descStyle.Render(item.desc))
	}
	line := " " + strings.Join(parts, "  ")
	if lipgloss.Width(line) > width {
		for lipgloss.Width(line) > width && len(parts) > 1 {
			parts = parts[:len(parts)-1]
			line = " " + strings.Join(parts, "  ")
		}
	}
	return lipgloss.NewStyle().Background(crust).Width(width).Render(line)
}
