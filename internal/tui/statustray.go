package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// StatusTray is an overlay that shows expanded status bar information
// with the ability to jump to related overlays for details.
type StatusTray struct {
	active bool
	width  int
	height int
	cursor int
	scroll int

	// Snapshot of current statuses (populated on open)
	gitInitialized  bool
	gitStatus       string
	researchRunning bool
	researchStatus  string
	aiProvider      string
	aiModel         string
	clockInStatus   string
	pomodoroStatus  string
	noteCount       int
	vaultPath       string
	dueTodayCount   int
	overdueCount    int
	inboxCount      int
}

// StatusTrayAction is the action the user wants to take from the tray.
type StatusTrayAction int

const (
	trayActionNone StatusTrayAction = iota
	trayActionGit
	trayActionResearch
)

func NewStatusTray() StatusTray {
	return StatusTray{}
}

func (t StatusTray) IsActive() bool {
	return t.active
}

func (t *StatusTray) SetSize(w, h int) {
	t.width = w
	t.height = h
}

// Open populates the tray with current status snapshots and activates it.
func (t *StatusTray) Open(sb StatusBar, researchRunning bool, researchStatus string) {
	t.active = true
	t.cursor = 0
	t.scroll = 0
	t.gitInitialized = sb.gitInitialized
	t.gitStatus = sb.gitStatus
	t.researchRunning = researchRunning
	t.researchStatus = researchStatus
	t.aiProvider = sb.ai.Provider
	t.aiModel = sb.ai.Model
	t.clockInStatus = sb.clockInStatus
	t.pomodoroStatus = sb.pomodoroStatus
	t.noteCount = sb.noteCount
	t.vaultPath = sb.vaultPath
	t.dueTodayCount = sb.dueTodayCount
	t.overdueCount = sb.overdueCount
	t.inboxCount = sb.inboxCount
}

// Action returns the last selected action and resets it.
func (t *StatusTray) Action() StatusTrayAction {
	return trayActionNone
}

func (t StatusTray) items() []trayItem {
	var items []trayItem

	// Git
	gitIcon := "✓"
	gitColor := green
	gitDetail := "Repository synced"
	if !t.gitInitialized {
		gitIcon = "⚠"
		gitColor = yellow
		gitDetail = "No git repository detected"
	} else if t.gitStatus != "" && t.gitStatus != "synced" {
		gitIcon = "●"
		gitColor = peach
		gitDetail = t.gitStatus
	}
	items = append(items, trayItem{
		icon: gitIcon, label: "Git", detail: gitDetail,
		color: gitColor, action: trayActionGit,
	})

	// Research agent
	if t.researchRunning {
		items = append(items, trayItem{
			icon: IconBotChar, label: "Research Agent", detail: t.researchStatus,
			color: lavender, action: trayActionResearch,
		})
	} else {
		items = append(items, trayItem{
			icon: IconBotChar, label: "Research Agent", detail: "idle",
			color: overlay0, action: trayActionResearch,
		})
	}

	// AI provider
	if t.aiProvider != "" {
		aiColor := green
		if t.aiProvider == "openai" {
			aiColor = blue
		}
		items = append(items, trayItem{
			icon: IconBotChar, label: "AI Provider", detail: t.aiProvider + " / " + t.aiModel,
			color: aiColor,
		})
	}

	// Clock-in
	if t.clockInStatus != "" {
		items = append(items, trayItem{
			icon: "⏱", label: "Clock In", detail: t.clockInStatus,
			color: teal,
		})
	}

	// Pomodoro
	if t.pomodoroStatus != "" {
		items = append(items, trayItem{
			icon: "◆", label: "Pomodoro", detail: t.pomodoroStatus,
			color: peach,
		})
	}

	// Vault info
	items = append(items, trayItem{
		icon: "◇", label: "Vault", detail: fmt.Sprintf("%d notes — %s", t.noteCount, t.vaultPath),
		color: overlay0,
	})

	// Tasks
	if t.dueTodayCount > 0 || t.overdueCount > 0 {
		taskDetail := fmt.Sprintf("%d due today", t.dueTodayCount)
		taskColor := sapphire
		if t.overdueCount > 0 {
			taskDetail += fmt.Sprintf(", %d overdue", t.overdueCount)
			taskColor = red
		}
		items = append(items, trayItem{
			icon: "◆", label: "Tasks", detail: taskDetail,
			color: taskColor,
		})
	}

	// Inbox
	if t.inboxCount > 0 {
		items = append(items, trayItem{
			icon: "◆", label: "Inbox", detail: fmt.Sprintf("%d items", t.inboxCount),
			color: sapphire,
		})
	}

	return items
}

type trayItem struct {
	icon   string
	label  string
	detail string
	color  lipgloss.Color
	action StatusTrayAction
}

func (t StatusTray) Update(msg tea.KeyMsg) (StatusTray, tea.Cmd) {
	items := t.items()
	switch msg.String() {
	case "esc", "q":
		t.active = false
		return t, nil
	case "up", "k":
		if t.cursor > 0 {
			t.cursor--
		}
		return t, nil
	case "down", "j":
		if t.cursor < len(items)-1 {
			t.cursor++
		}
		return t, nil
	case "enter":
		if t.cursor < len(items) {
			item := items[t.cursor]
			if item.action != trayActionNone {
				t.active = false
				return t, func() tea.Msg {
					return statusTrayActionMsg{action: item.action}
				}
			}
		}
		return t, nil
	}
	return t, nil
}

// statusTrayActionMsg is sent when the user selects an actionable item.
type statusTrayActionMsg struct {
	action StatusTrayAction
}

func (t StatusTray) View() string {
	w := t.width * 2 / 3
	if w < 50 {
		w = 50
	}
	if w > 80 {
		w = 80
	}
	innerW := w - 6

	var b strings.Builder

	// Header
	title := lipgloss.NewStyle().Foreground(mauve).Bold(true).
		Render("  Status Tray")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", innerW)))
	b.WriteString("\n\n")

	items := t.items()
	for i, item := range items {
		iconStyle := lipgloss.NewStyle().Foreground(item.color).Bold(true)
		labelStyle := lipgloss.NewStyle().Foreground(text).Bold(true)
		detailStyle := lipgloss.NewStyle().Foreground(overlay0)

		prefix := "  "
		if i == t.cursor {
			prefix = lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("▸ ")
			labelStyle = labelStyle.Foreground(mauve)
		}

		line := prefix + iconStyle.Render(item.icon) + " " +
			labelStyle.Render(item.label) + "  " +
			detailStyle.Render(item.detail)

		// Indicate actionable items
		if item.action != trayActionNone && i == t.cursor {
			line += DimStyle.Render("  ↵")
		}

		b.WriteString(line)
		b.WriteString("\n")
	}

	// Footer
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", innerW)))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  j/k navigate  Enter open  Esc close"))

	border := lipgloss.NewStyle().
		Border(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Background(mantle).
		Padding(1, 2).
		Width(w)

	return border.Render(b.String())
}
