package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Colors
	primaryColor   = lipgloss.Color("#7C3AED")
	secondaryColor = lipgloss.Color("#A78BFA")
	accentColor    = lipgloss.Color("#F59E0B")
	bgColor        = lipgloss.Color("#1E1E2E")
	fgColor        = lipgloss.Color("#CDD6F4")
	dimColor       = lipgloss.Color("#6C7086")
	borderColor    = lipgloss.Color("#45475A")

	// Panel styles
	SidebarStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(borderColor).
			Padding(0, 1)

	EditorStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(borderColor).
			Padding(0, 1)

	BacklinksStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(borderColor).
			Padding(0, 1)

	StatusBarStyle = lipgloss.NewStyle().
			Background(primaryColor).
			Foreground(lipgloss.Color("#FFFFFF")).
			Padding(0, 1)

	// Text styles
	TitleStyle = lipgloss.NewStyle().
			Foreground(primaryColor).
			Bold(true)

	SelectedStyle = lipgloss.NewStyle().
			Foreground(accentColor).
			Bold(true)

	DimStyle = lipgloss.NewStyle().
			Foreground(dimColor)

	LinkStyle = lipgloss.NewStyle().
			Foreground(secondaryColor).
			Underline(true)

	HeaderStyle = lipgloss.NewStyle().
			Foreground(primaryColor).
			Bold(true).
			Underline(true).
			MarginBottom(1)
)
