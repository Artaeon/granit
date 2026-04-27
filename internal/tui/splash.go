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

type SplashModel struct {
	width     int
	height    int
	tick      int
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
	return tea.Tick(50*time.Millisecond, func(t time.Time) tea.Msg {
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
		s.tick++
		if s.tick >= 25 {
			s.done = true
			return s, nil
		}
		return s, tea.Tick(50*time.Millisecond, func(t time.Time) tea.Msg {
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

// splashGradientColors returns a gradient slice cycling through theme accent colors.
func splashGradientColors() []lipgloss.Color {
	return []lipgloss.Color{sapphire, blue, teal, green, yellow, peach, red, mauve, pink, lavender}
}

// splashColorAt picks a gradient color based on position and animation offset.
func splashColorAt(pos, total, offset int) lipgloss.Color {
	grad := splashGradientColors()
	idx := ((pos + offset) * len(grad)) / (total + 1)
	if idx < 0 {
		idx = -idx
	}
	return grad[idx%len(grad)]
}

func (s SplashModel) View() string {
	if s.width == 0 || s.height == 0 {
		return ""
	}

	var content strings.Builder
	logoW := 50

	// ── Logo: sweep reveal then hold ─────────────────────────────────
	if s.tick <= 9 {
		s.renderLogoReveal(&content)
	} else {
		s.renderLogoStatic(&content)
	}

	// ── Expanding rule (grows from center) ───────────────────────────
	if s.tick >= 8 {
		content.WriteString("\n")
		ruleLen := (s.tick - 8) * 8
		if ruleLen > logoW {
			ruleLen = logoW
		}
		pad := (logoW - ruleLen) / 2
		ruleStyle := lipgloss.NewStyle().Foreground(surface1)
		content.WriteString("  " + strings.Repeat(" ", pad) + ruleStyle.Render(strings.Repeat("─", ruleLen)))
		content.WriteString("\n")
	}

	// ── Tagline (typewriter) ─────────────────────────────────────────
	if s.tick >= 10 {
		tagline := "Terminal Operator Workspace"
		runes := []rune(tagline)
		shown := (s.tick - 10) * 6
		if shown > len(runes) {
			shown = len(runes)
		}
		tagStyle := lipgloss.NewStyle().Foreground(overlay0).Italic(true)
		content.WriteString("    " + tagStyle.Render(string(runes[:shown])))
		if shown < len(runes) {
			cursorStyle := lipgloss.NewStyle().Foreground(mauve)
			content.WriteString(cursorStyle.Render("▎"))
		}
		content.WriteString("\n")
	}

	// ── Vault info (fade in one by one) ──────────────────────────────
	if s.tick >= 15 {
		content.WriteString("\n")
		dimStyle := lipgloss.NewStyle().Foreground(surface2)
		valStyle := lipgloss.NewStyle().Foreground(overlay0)
		content.WriteString(dimStyle.Render("    vault  ") + valStyle.Render(s.vaultPath) + "\n")
		if s.tick >= 17 {
			content.WriteString(dimStyle.Render("    notes  ") + valStyle.Render(itoa(s.noteCount)) + "\n")
		}
	}

	if s.tick >= 18 {
		content.WriteString("\n")
		keyStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
		descStyle := lipgloss.NewStyle().Foreground(overlay0)
		content.WriteString("    " + keyStyle.Render("Ctrl+K") + descStyle.Render(" tasks  ") +
			keyStyle.Render("Alt+C") + descStyle.Render(" command center  ") +
			keyStyle.Render("Ctrl+X") + descStyle.Render(" actions") + "\n")
	}

	// ── Ready with check ─────────────────────────────────────────────
	if s.tick >= 21 {
		content.WriteString("\n")
		checkStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
		content.WriteString(checkStyle.Render("    ✓ Ready for power UI"))
		content.WriteString("\n")
	}

	// ── Skip hint ────────────────────────────────────────────────────
	if s.tick >= 12 {
		content.WriteString("\n")
		hintStyle := lipgloss.NewStyle().Foreground(surface1)
		content.WriteString(hintStyle.Render("    press any key"))
	}

	contentStr := content.String()

	centered := lipgloss.Place(
		s.width,
		s.height,
		lipgloss.Center,
		lipgloss.Center,
		contentStr,
		lipgloss.WithWhitespaceBackground(crust),
	)

	return centered
}

// renderLogoReveal draws the logo with a sweeping rainbow reveal effect.
func (s SplashModel) renderLogoReveal(b *strings.Builder) {
	dimStyle := lipgloss.NewStyle().Foreground(surface1)

	for i, line := range splashLogo {
		runes := []rune(line)
		lineLen := len(runes)

		// Stagger each line by 1 tick
		lineStart := i
		ticksIn := s.tick - lineStart
		if ticksIn < 0 {
			b.WriteString(dimStyle.Render(string(runes)))
			b.WriteString("\n")
			continue
		}

		// Reveal speed: full line in ~4 ticks
		charsRevealed := ticksIn * lineLen / 4
		if charsRevealed > lineLen {
			charsRevealed = lineLen
		}

		// Build the line char by char with gradient coloring
		var lineBuilder strings.Builder
		for j, r := range runes {
			if j < charsRevealed {
				col := splashColorAt(j, lineLen, s.tick)
				st := lipgloss.NewStyle().Foreground(col).Bold(true)
				lineBuilder.WriteString(st.Render(string(r)))
			} else if j == charsRevealed && charsRevealed < lineLen {
				// Cursor position — bright white
				cursorStyle := lipgloss.NewStyle().Foreground(text).Bold(true)
				lineBuilder.WriteString(cursorStyle.Render(string(r)))
			} else {
				lineBuilder.WriteString(dimStyle.Render(string(r)))
			}
		}
		b.WriteString(lineBuilder.String())
		b.WriteString("\n")
	}
}

// renderLogoStatic renders the logo in the primary accent color.
func (s SplashModel) renderLogoStatic(b *strings.Builder) {
	logoStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	for _, line := range splashLogo {
		b.WriteString(logoStyle.Render(line))
		b.WriteString("\n")
	}
}

// ─── Exit Splash ─────────────────────────────────────────────────────────────

type exitTickMsg struct{}

// ExitSplash renders an animated goodbye screen when quitting Granit.
type ExitSplash struct {
	width     int
	height    int
	tick      int
	done      bool
	noteCount int
	uptime    time.Duration
	tabCount  int
	gitStatus string
	lastNote  string
}

func NewExitSplash(noteCount int, uptime time.Duration) ExitSplash {
	return ExitSplash{
		noteCount: noteCount,
		uptime:    uptime,
	}
}

func (e *ExitSplash) SetContext(tabCount int, gitStatus, lastNote string) {
	e.tabCount = tabCount
	e.gitStatus = gitStatus
	e.lastNote = lastNote
}

func (e ExitSplash) Init() tea.Cmd {
	return tea.Tick(40*time.Millisecond, func(t time.Time) tea.Msg {
		return exitTickMsg{}
	})
}

func (e ExitSplash) Update(msg tea.Msg) (ExitSplash, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		e.width = msg.Width
		e.height = msg.Height
		return e, nil

	case exitTickMsg:
		e.tick++
		if e.tick >= 35 {
			e.done = true
			return e, tea.Quit
		}
		return e, tea.Tick(40*time.Millisecond, func(t time.Time) tea.Msg {
			return exitTickMsg{}
		})

	case tea.KeyMsg:
		// Any key skips exit animation
		if msg.String() != "" {
			e.done = true
			return e, tea.Quit
		}
	}
	return e, nil
}

func (e ExitSplash) IsDone() bool {
	return e.done
}

func (e ExitSplash) View() string {
	if e.width == 0 || e.height == 0 {
		return ""
	}

	var content strings.Builder

	// ── Logo fade ────────────────────────────────────────────────────
	logoStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	if e.tick >= 10 {
		logoStyle = lipgloss.NewStyle().Foreground(surface2)
	}
	if e.tick >= 18 {
		logoStyle = lipgloss.NewStyle().Foreground(surface1)
	}
	for _, line := range splashLogo {
		content.WriteString(logoStyle.Render(line))
		content.WriteString("\n")
	}

	// ── Session stats ────────────────────────────────────────────────
	if e.tick >= 5 {
		content.WriteString("\n")
		uptimeStr := exitFormatDuration(e.uptime + time.Duration(e.tick)*40*time.Millisecond)
		dimStyle := lipgloss.NewStyle().Foreground(surface2)
		valStyle := lipgloss.NewStyle().Foreground(overlay0)
		content.WriteString(dimStyle.Render("    session  ") + valStyle.Render(uptimeStr) + "\n")
		content.WriteString(dimStyle.Render("    notes    ") + valStyle.Render(itoa(e.noteCount)) + "\n")
		if e.tabCount > 0 {
			content.WriteString(dimStyle.Render("    tabs     ") + valStyle.Render(itoa(e.tabCount)+" restored next time") + "\n")
		}
		if e.gitStatus != "" {
			gitColor := green
			if e.gitStatus != "synced" {
				gitColor = peach
			}
			content.WriteString(dimStyle.Render("    sync     ") + lipgloss.NewStyle().Foreground(gitColor).Render(e.gitStatus) + "\n")
		}
		if e.lastNote != "" && e.tick >= 8 {
			content.WriteString(dimStyle.Render("    last     ") + valStyle.Render(TruncateDisplay(e.lastNote, 42)) + "\n")
		}
	}

	// ── Goodbye ──────────────────────────────────────────────────────
	if e.tick >= 12 {
		content.WriteString("\n")
		ruleStyle := lipgloss.NewStyle().Foreground(surface1)
		content.WriteString(ruleStyle.Render("    " + strings.Repeat("─", 40)))
		content.WriteString("\n\n")
		byeStyle := lipgloss.NewStyle().Foreground(mauve).Italic(true)
		content.WriteString(byeStyle.Render("    Workspace saved. Until next time."))
		content.WriteString("\n")
	}

	contentStr := content.String()

	centered := lipgloss.Place(
		e.width,
		e.height,
		lipgloss.Center,
		lipgloss.Center,
		contentStr,
		lipgloss.WithWhitespaceBackground(crust),
	)

	return centered
}

func exitFormatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return itoa(h) + "h " + itoa(m) + "m " + itoa(s) + "s"
	}
	if m > 0 {
		return itoa(m) + "m " + itoa(s) + "s"
	}
	return itoa(s) + "s"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	if neg {
		s = "-" + s
	}
	return s
}
