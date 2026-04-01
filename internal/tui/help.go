package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type HelpOverlay struct {
	active    bool
	width     int
	height    int
	scroll    int
	searching bool
	query     string
}

func NewHelpOverlay() HelpOverlay {
	return HelpOverlay{}
}

func (h *HelpOverlay) SetSize(width, height int) {
	h.width = width
	h.height = height
}

func (h *HelpOverlay) Toggle() {
	h.active = !h.active
	h.scroll = 0
	h.searching = false
	h.query = ""
}

func (h *HelpOverlay) IsActive() bool {
	return h.active
}

type helpSection struct {
	title    string
	bindings []helpBinding
}

type helpBinding struct {
	key  string
	desc string
}

var helpSections = []helpSection{
	{
		title: "Navigation",
		bindings: []helpBinding{
			{"Tab / Shift+Tab", "Cycle between panels (sidebar, editor, backlinks)"},
			{"F1 / Alt+1", "Focus file sidebar"},
			{"F2 / Alt+2", "Focus editor"},
			{"F3 / Alt+3", "Focus backlinks panel"},
			{"Esc", "Close current overlay / return to sidebar"},
			{"j / k / ↑ / ↓", "Navigate up/down in lists"},
			{"Enter", "Open selected file or link"},
		},
	},
	{
		title: "File Operations",
		bindings: []helpBinding{
			{"Ctrl+P", "Quick open — fuzzy search all files"},
			{"Ctrl+N", "Create a new note"},
			{"Ctrl+S", "Save current note"},
			{"F4", "Rename current note"},
			{"Ctrl+X", "Command palette — search all commands"},
		},
	},
	{
		title: "Editor",
		bindings: []helpBinding{
			{"Ctrl+E", "Toggle between view and edit mode"},
			{"Ctrl+U", "Undo last edit"},
			{"Ctrl+Y", "Redo / accept ghost writer suggestion"},
			{"Ctrl+F", "Find in current file"},
			{"Ctrl+H", "Find & replace in file"},
			{"Ctrl+K", "Delete to end of line"},
			{"Ctrl+D", "Delete character forward / multi-select word"},
			{"Tab", "Insert indent (4 spaces)"},
			{"Home / End", "Jump to line start / end"},
			{"PgUp / PgDn", "Scroll one page up / down"},
		},
	},
	{
		title: "Daily Workflow",
		bindings: []helpBinding{
			{"Alt+D", "Open today's daily note"},
			{"Alt+H", "Dashboard — overview with scripture, tasks, projects, goals"},
			{"Alt+J", "Daily Jot — quick time-stamped bullets, scrollable history"},
			{"Alt+M", "Morning Routine — start your day with briefing & plan"},
			{"Alt+E", "Evening Review — reflect, audit overdue, plan tomorrow"},
			{"Alt+P", "Plan My Day — AI generates your daily schedule"},
			{"Alt+W", "Open this week's note"},
			{"Alt+[", "Navigate to previous daily note"},
			{"Alt+]", "Navigate to next daily note"},
		},
	},
	{
		title: "Views & Tools",
		bindings: []helpBinding{
			{"Ctrl+G", "Note graph — visual connections between notes"},
			{"Ctrl+T", "Tag browser — browse notes by tags"},
			{"Ctrl+O", "Note outline — heading structure"},
			{"Ctrl+B", "Bookmarks & recently opened notes"},
			{"Ctrl+J", "Quick switch between recent files"},
			{"Ctrl+W", "Visual canvas / whiteboard"},
			{"Ctrl+L", "Calendar — month, week, and agenda views"},
			{"Ctrl+R", "AI research agent on current note"},
			{"Ctrl+Z", "Focus / zen mode — distraction-free writing"},
			{"Ctrl+,", "Settings panel"},
			{"Ctrl+/", "Universal search — notes, tasks, goals, habits"},
			{"Alt+C", "Command Center"},
			{"Alt+G", "Git overlay"},
		},
	},
	{
		title: "Task Manager (Ctrl+K)",
		bindings: []helpBinding{
			{"j / k", "Navigate tasks"},
			{"x", "Toggle task done/undone"},
			{"a", "Add new task"},
			{"S", "AI breakdown — split task into subtasks"},
			{"A", "Auto-suggest priority (heuristic)"},
			{"p", "Cycle priority (none → low → med → high → highest)"},
			{"d", "Set due date"},
			{"r", "Reschedule task"},
			{"R", "Batch reschedule all overdue tasks"},
			{"e", "Expand/collapse subtasks"},
			{"f", "Start focus session on this task"},
			{"g", "Jump to task in source note"},
			{"n", "Add/edit task note"},
			{"E", "Set time estimate"},
			{"s", "Cycle sort mode (priority, date, A-Z, source, tag)"},
			{"#", "Filter by tag"},
			{"P", "Filter by priority"},
			{"c", "Clear all filters"},
			{"/", "Search tasks"},
			{"u", "Undo last action"},
			{"v", "Bulk select mode"},
			{"W", "Pin/unpin task"},
			{"T", "Save task as template"},
			{"t", "Create from template"},
			{"X", "Archive completed tasks (>30 days)"},
			{"1-6", "Switch view (Today/Upcoming/All/Done/Calendar/Kanban)"},
		},
	},
	{
		title: "Goals (via Command Palette)",
		bindings: []helpBinding{
			{"a", "Add new goal"},
			{"G", "AI generate milestones for selected goal"},
			{"x / space", "Toggle milestone completion"},
			{"m", "Add milestone manually"},
			{"d", "Set target date"},
			{"e", "Expand/collapse milestones"},
			{"C", "Set goal color"},
			{"n", "Edit goal notes"},
			{"s", "Change status (active/paused/completed/archived)"},
			{"1-4", "Switch view (All/Active/Completed/Review)"},
		},
	},
	{
		title: "Projects (via Command Palette)",
		bindings: []helpBinding{
			{"a", "Add new project"},
			{"e", "Edit project details"},
			{"s", "Change status"},
			{"c", "Set category"},
			{"d", "Set due date"},
			{"p", "Set priority"},
			{"n", "Open project notes"},
			{"g", "Manage project goals/phases"},
		},
	},
	{
		title: "AI Features",
		bindings: []helpBinding{
			{"Ctrl+R", "Research agent — deep dive on current note"},
			{"Alt+P", "Plan My Day — AI daily schedule"},
			{"Alt+M", "Morning Routine — briefing + planning"},
			{"S (tasks)", "AI task breakdown into subtasks"},
			{"G (goals)", "AI milestone generation"},
			{"", ""},
			{"", "Providers: Ollama (local), OpenAI, Nous, Nerve"},
			{"", "Configure in Settings (Ctrl+,) → AI section"},
			{"", "Ghost Writer: inline AI completions (toggle in settings)"},
			{"", "Auto-tagger: suggests tags on note save (toggle in settings)"},
		},
	},
	{
		title: "Sidebar",
		bindings: []helpBinding{
			{"Type", "Fuzzy search files by name"},
			{"Backspace", "Clear search character"},
			{"Esc", "Clear search filter"},
			{"n", "New folder"},
		},
	},
	{
		title: "Quick Reference",
		bindings: []helpBinding{
			{"", "Task format:  - [ ] task text #tag 📅 2026-04-01 🔼"},
			{"", "Priority:     🔺 highest  ⏫ high  🔼 medium  🔽 low"},
			{"", "Due date:     📅 YYYY-MM-DD"},
			{"", "Estimate:     ~30m or ~2h"},
			{"", "Recurrence:   🔁 daily/weekly/monthly"},
			{"", "Link:         [[note name]] or [[note|display text]]"},
			{"", "Tag:          #tag-name (lowercase, hyphens OK)"},
			{"", "Scripture:    .granit/scriptures.md (one per line, — source)"},
			{"", "Business:     tag tasks #revenue #client #business"},
		},
	},
	{
		title: "Application",
		bindings: []helpBinding{
			{"Ctrl+Q / Ctrl+C", "Quit Granit"},
			{"F5 / Alt+?", "Show this help"},
		},
	},
}

func (h HelpOverlay) Update(msg tea.Msg) (HelpOverlay, tea.Cmd) {
	if !h.active {
		return h, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		key := msg.String()

		if h.searching {
			switch key {
			case "esc":
				if h.query != "" {
					h.query = ""
					h.scroll = 0
				} else {
					h.searching = false
				}
			case "backspace":
				if len(h.query) > 0 {
					h.query = h.query[:len(h.query)-1]
					h.scroll = 0
				}
			case "enter":
				h.searching = false
			default:
				if len(key) == 1 && key[0] >= 32 {
					h.query += key
					h.scroll = 0
				}
			}
			return h, nil
		}

		switch key {
		case "esc", "f5", "q":
			h.active = false
		case "/":
			h.searching = true
			h.query = ""
		case "up", "k":
			if h.scroll > 0 {
				h.scroll--
			}
		case "down", "j":
			allLines := h.buildLines()
			visH := h.visibleHeight()
			maxScroll := len(allLines) - visH
			if maxScroll < 0 {
				maxScroll = 0
			}
			if h.scroll < maxScroll {
				h.scroll++
			}
		}
	}
	return h, nil
}

func (h HelpOverlay) visibleHeight() int {
	visH := h.height - 10
	if visH < 10 {
		visH = 10
	}
	return visH
}

// buildLines generates all help content, optionally filtered by query.
func (h HelpOverlay) buildLines() []string {
	query := strings.ToLower(h.query)

	var allLines []string
	for _, section := range helpSections {
		var sectionLines []string
		hasMatch := false

		for _, binding := range section.bindings {
			if query != "" {
				combined := strings.ToLower(binding.key + " " + binding.desc + " " + section.title)
				if !strings.Contains(combined, query) {
					continue
				}
			}
			hasMatch = true

			if binding.key == "" {
				// Info line (no key)
				sectionLines = append(sectionLines, DimStyle.Render("    "+binding.desc))
			} else {
				keyStyle := lipgloss.NewStyle().
					Foreground(lavender).
					Bold(true).
					Width(22).
					Render("    " + binding.key)
				descStyle := lipgloss.NewStyle().
					Foreground(text).
					Render(binding.desc)
				sectionLines = append(sectionLines, keyStyle+descStyle)
			}
		}

		if !hasMatch {
			continue
		}

		sectionTitle := lipgloss.NewStyle().
			Foreground(blue).
			Bold(true).
			Render("  " + section.title)
		allLines = append(allLines, "")
		allLines = append(allLines, sectionTitle)
		allLines = append(allLines, DimStyle.Render("  "+strings.Repeat("─", 30)))
		allLines = append(allLines, sectionLines...)
	}

	if len(allLines) == 0 && query != "" {
		allLines = append(allLines, "")
		allLines = append(allLines, DimStyle.Render("  No matches for \""+h.query+"\""))
	}

	return allLines
}

func (h HelpOverlay) View() string {
	width := h.width * 2 / 3
	if width < 60 {
		width = 60
	}
	if width > 90 {
		width = 90
	}

	var b strings.Builder

	// Header
	logo := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  " + IconHelpChar + " Granit — Help & Shortcuts")
	b.WriteString(logo)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", width-6)))
	b.WriteString("\n")

	// Search bar
	if h.searching || h.query != "" {
		prompt := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("  / ")
		qText := h.query
		if h.searching {
			qText += lipgloss.NewStyle().Foreground(overlay0).Render("_")
		}
		b.WriteString(prompt + lipgloss.NewStyle().Foreground(text).Render(qText))
		b.WriteString("\n")
		b.WriteString(DimStyle.Render(strings.Repeat("─", width-6)))
		b.WriteString("\n")
	}

	// Build filtered lines
	allLines := h.buildLines()

	// Apply scroll
	visH := h.visibleHeight()
	if h.searching || h.query != "" {
		visH -= 2 // search bar takes 2 lines
	}

	maxScroll := len(allLines) - visH
	if maxScroll < 0 {
		maxScroll = 0
	}
	scroll := h.scroll
	if scroll > maxScroll {
		scroll = maxScroll
	}

	end := scroll + visH
	if end > len(allLines) {
		end = len(allLines)
	}

	for i := scroll; i < end; i++ {
		b.WriteString(allLines[i])
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	// Footer
	b.WriteString("\n\n")
	if h.searching {
		b.WriteString(DimStyle.Render("  type to search  Enter: done  Esc: clear"))
	} else {
		matchInfo := ""
		if h.query != "" {
			matchInfo = DimStyle.Render("  filter: \"" + h.query + "\"  ")
		}
		b.WriteString(matchInfo + DimStyle.Render("  /: search  j/k: scroll  Esc: close"))
	}

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}
