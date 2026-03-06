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
	return tea.Tick(60*time.Millisecond, func(t time.Time) tea.Msg {
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
		if s.tick >= 60 {
			s.done = true
			return s, nil
		}
		return s, tea.Tick(60*time.Millisecond, func(t time.Time) tea.Msg {
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

	// ── Phase 1 (ticks 0-15): Logo typing effect ──────────────────────
	logoCompleteStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	logoCursorStyle := lipgloss.NewStyle().Foreground(lavender).Bold(true)
	logoDimStyle := lipgloss.NewStyle().Foreground(surface1)

	if s.tick <= 15 {
		// Each logo line gets ~2.5 ticks to type out.
		// We distribute 16 ticks across 6 lines.
		for i, line := range splashLogo {
			runes := []rune(line)
			lineLen := len(runes)

			// Determine how many chars of this line are revealed.
			// Line i starts revealing at tick = i*2.6 (approx).
			lineStartTick := i * 16 / len(splashLogo)
			ticksIntoLine := s.tick - lineStartTick
			if ticksIntoLine < 0 {
				// Line not started yet: render blank placeholder
				content.WriteString(logoDimStyle.Render(strings.Repeat(" ", lineLen)))
				content.WriteString("\n")
				continue
			}

			// How far through the line are we?
			ticksPerLine := 16/len(splashLogo) + 1
			charsRevealed := ticksIntoLine * lineLen / ticksPerLine
			if charsRevealed > lineLen {
				charsRevealed = lineLen
			}

			if charsRevealed >= lineLen {
				// Line fully typed
				content.WriteString(logoCompleteStyle.Render(line))
			} else if charsRevealed > 0 {
				// Partially typed: completed portion + cursor char + dim remainder
				completed := string(runes[:charsRevealed-1])
				cursor := string(runes[charsRevealed-1 : charsRevealed])
				rest := string(runes[charsRevealed:])
				content.WriteString(logoCompleteStyle.Render(completed))
				content.WriteString(logoCursorStyle.Render(cursor))
				content.WriteString(logoDimStyle.Render(rest))
			} else {
				content.WriteString(logoDimStyle.Render(string(runes)))
			}
			content.WriteString("\n")
		}
	} else {
		// After phase 1 the full logo is always shown
		for _, line := range splashLogo {
			content.WriteString(logoCompleteStyle.Render(line))
			content.WriteString("\n")
		}
	}

	// ── Phase 2 (ticks 16-25): Horizontal rule + tagline ─────────────
	if s.tick >= 16 {
		content.WriteString("\n")

		// Horizontal rule drawing itself
		maxRuleLen := 50
		ruleProgress := s.tick - 16
		ruleLen := ruleProgress * maxRuleLen / 5
		if ruleLen > maxRuleLen {
			ruleLen = maxRuleLen
		}
		rule := strings.Repeat("\u2500", ruleLen)
		ruleStyle := lipgloss.NewStyle().Foreground(surface2)
		content.WriteString(ruleStyle.Render(rule))
		content.WriteString("\n")

		// Tagline fading in word by word
		if s.tick >= 19 {
			taglineWords := []string{"Terminal", "Knowledge", "Manager", "\u2014", "Obsidian", "Compatible"}
			wordsToShow := (s.tick - 19) + 1
			if wordsToShow > len(taglineWords) {
				wordsToShow = len(taglineWords)
			}
			visibleWords := strings.Join(taglineWords[:wordsToShow], " ")

			taglineStyle := lipgloss.NewStyle().Foreground(overlay0).Italic(true)
			content.WriteString(taglineStyle.Render("  " + visibleWords))
			content.WriteString("\n")
		}
	}

	// ── Phase 3 (ticks 26-35): Progress bar ──────────────────────────
	if s.tick >= 26 {
		content.WriteString("\n")

		initStyle := lipgloss.NewStyle().Foreground(yellow)
		content.WriteString(initStyle.Render("  Initializing vault..."))
		content.WriteString("\n")

		barWidth := 40
		progress := s.tick - 26
		filled := progress * barWidth / 10
		if filled > barWidth {
			filled = barWidth
		}

		// Cycle through colors for the filled portion
		barColors := []lipgloss.Color{mauve, blue, green, mauve, blue, green}
		var bar strings.Builder
		bar.WriteString("  ")
		for j := 0; j < barWidth; j++ {
			if j < filled {
				colorIdx := j * len(barColors) / barWidth
				blockStyle := lipgloss.NewStyle().Foreground(barColors[colorIdx])
				bar.WriteString(blockStyle.Render("\u2588"))
			} else {
				dimBlock := lipgloss.NewStyle().Foreground(surface1)
				bar.WriteString(dimBlock.Render("\u2591"))
			}
		}

		// Percentage
		pct := filled * 100 / barWidth
		pctStr := " " + itoa(pct) + "%"
		pctStyle := lipgloss.NewStyle().Foreground(surface2)
		bar.WriteString(pctStyle.Render(pctStr))

		content.WriteString(bar.String())
		content.WriteString("\n")
	}

	// ── Phase 4 (ticks 36-45): Vault info ────────────────────────────
	if s.tick >= 36 {
		content.WriteString("\n")

		labelStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
		valueStyle := lipgloss.NewStyle().Foreground(overlay0)
		prefixStyle := lipgloss.NewStyle().Foreground(green)

		type infoLine struct {
			label string
			value string
		}
		lines := []infoLine{
			{"Vault", s.vaultPath},
			{"Notes", itoa(s.noteCount)},
			{"Version", "v0.1.0"},
		}

		linesRevealed := s.tick - 36 + 1
		if linesRevealed > len(lines) {
			linesRevealed = len(lines)
		}

		for i := 0; i < linesRevealed; i++ {
			content.WriteString(prefixStyle.Render("  > "))
			content.WriteString(labelStyle.Render(lines[i].label + ": "))
			content.WriteString(valueStyle.Render(lines[i].value))
			content.WriteString("\n")
		}
	}

	// ── Phase 5 (ticks 46-55): Ready + press any key ─────────────────
	if s.tick >= 46 {
		content.WriteString("\n")
		readyStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
		content.WriteString(readyStyle.Render("  \u2714 Ready!"))
		content.WriteString("\n")

		if s.tick >= 50 {
			hintStyle := lipgloss.NewStyle().Foreground(surface2)
			content.WriteString("\n")
			content.WriteString(hintStyle.Render("  Press any key to continue"))
		}
	}

	// ── Phase 6 (tick 55+): Auto-dismiss ─────────────────────────────
	// Handled in Update; any keypress also dismisses at any time.

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
