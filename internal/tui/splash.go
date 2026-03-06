package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var splashLogo = []string{
	"   ██████╗ ██████╗  █████╗ ███╗   ██╗██╗████████╗",
	"  ██╔════╝ ██╔══██╗██╔══██╗████╗  ██║██║╚══██╔══╝",
	"  ██║  ███╗██████╔╝███████║██╔██╗ ██║██║   ██║   ",
	"  ██║   ██║██╔══██╗██╔══██║██║╚██╗██║██║   ██║   ",
	"  ╚██████╔╝██║  ██║██║  ██║██║ ╚████║██║   ██║   ",
	"   ╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═╝╚═╝  ╚═══╝╚═╝   ╚═╝",
}

type splashTickMsg struct{}
type splashDoneMsg struct{}

type SplashModel struct {
	width     int
	height    int
	progress  int // animation progress (0 to len(splashLogo)+extra)
	done      bool
	vaultPath string
	noteCount int
	startTime time.Time
}

func NewSplashModel(vaultPath string, noteCount int) SplashModel {
	return SplashModel{
		vaultPath: vaultPath,
		noteCount: noteCount,
		startTime: time.Now(),
	}
}

func (s SplashModel) Init() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(t time.Time) tea.Msg {
		return splashTickMsg{}
	})
}

func (s SplashModel) Update(msg tea.Msg) (SplashModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.width = msg.Width
		s.height = msg.Height
		return s, nil

	case splashTickMsg:
		s.progress++
		maxProgress := len(splashLogo) + 5 // logo lines + extra for subtitle
		if s.progress >= maxProgress {
			s.done = true
			return s, nil
		}
		return s, tea.Tick(150*time.Millisecond, func(t time.Time) tea.Msg {
			return splashTickMsg{}
		})

	case tea.KeyMsg:
		if msg.String() != "" {
			s.done = true
			return s, nil
		}
	}
	return s, nil
}

func (s SplashModel) IsDone() bool {
	return s.done
}

func (s SplashModel) View() string {
	if s.width == 0 || s.height == 0 {
		return ""
	}

	var content strings.Builder

	// Animated logo
	logoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#CBA6F7")).Bold(true)
	glowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#B4BEFE"))

	for i, line := range splashLogo {
		if i < s.progress {
			if i == s.progress-1 {
				content.WriteString(glowStyle.Render(line))
			} else {
				content.WriteString(logoStyle.Render(line))
			}
			content.WriteString("\n")
		}
	}

	// Tagline (appears after logo)
	if s.progress > len(splashLogo) {
		content.WriteString("\n")
		tagline := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6C7086")).
			Italic(true).
			Render("  Terminal Knowledge Manager — Obsidian Compatible")
		content.WriteString(tagline)
		content.WriteString("\n")
	}

	// Version + vault info (appears after tagline)
	if s.progress > len(splashLogo)+1 {
		content.WriteString("\n")
		version := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#89B4FA")).
			Render("  v0.1.0")
		content.WriteString(version)
		content.WriteString("\n")
	}

	if s.progress > len(splashLogo)+2 {
		vaultInfo := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6C7086")).
			Render("  Vault: " + s.vaultPath)
		content.WriteString(vaultInfo)
		content.WriteString("\n")

		noteInfo := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6C7086")).
			Render("  Notes: " + itoa(s.noteCount))
		content.WriteString(noteInfo)
		content.WriteString("\n")
	}

	// Loading / ready message
	if s.progress > len(splashLogo)+3 {
		content.WriteString("\n")
		if s.done {
			ready := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#A6E3A1")).
				Bold(true).
				Render("  Ready!")
			content.WriteString(ready)
		} else {
			loading := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#F9E2AF")).
				Render("  Loading...")
			content.WriteString(loading)
		}
		content.WriteString("\n")
	}

	// Press any key hint
	if s.progress > len(splashLogo)+4 {
		content.WriteString("\n")
		hint := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#585B70")).
			Render("  Press any key to continue")
		content.WriteString(hint)
	}

	// Center the entire block vertically
	contentStr := content.String()
	contentLines := strings.Split(contentStr, "\n")
	totalLines := len(contentLines)
	topPadding := (s.height - totalLines) / 3
	if topPadding < 1 {
		topPadding = 1
	}

	// Center horizontally by using lipgloss Place
	centered := lipgloss.Place(
		s.width,
		s.height,
		lipgloss.Center,
		lipgloss.Center,
		contentStr,
		lipgloss.WithWhitespaceBackground(lipgloss.Color("#11111B")),
	)

	return centered
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
