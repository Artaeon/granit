package tui

import (
	"math"
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

// Particle characters for the sparkle effect around the logo
var sparkleChars = []string{"·", "✦", "✧", "◆", "◇", "⟡", "∗", "⊹", "⋆"}

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
		if s.tick >= 40 {
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
	return []lipgloss.Color{mauve, blue, sapphire, teal, green, yellow, peach, red, pink, mauve}
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

	// ── Phase 1 (ticks 0-9): Logo reveal with rainbow sweep ─────────
	if s.tick <= 9 {
		s.renderLogoReveal(&content)
	} else if s.tick <= 13 {
		// Phase 2: Logo glow/pulse with color cycling
		s.renderLogoPulse(&content)
	} else {
		// After phase 2: static logo with accent color
		s.renderLogoStatic(&content)
	}

	// ── Phase 3 (ticks 10+): Animated horizontal rule ────────────────
	if s.tick >= 10 {
		content.WriteString("\n")
		s.renderAnimatedRule(&content)
	}

	// ── Phase 4 (ticks 12+): Tagline with typewriter + cursor ────────
	if s.tick >= 12 {
		s.renderTagline(&content)
	}

	// ── Phase 5 (ticks 15+): Animated system checks ─────────────────
	if s.tick >= 15 {
		content.WriteString("\n")
		s.renderSystemChecks(&content)
	}

	// ── Phase 6 (ticks 21+): Gradient progress bar ──────────────────
	if s.tick >= 21 {
		content.WriteString("\n")
		s.renderProgressBar(&content)
	}

	// ── Phase 7 (ticks 26+): Vault info with slide-in ───────────────
	if s.tick >= 26 {
		content.WriteString("\n")
		s.renderVaultInfo(&content)
	}

	// ── Phase 8 (ticks 31+): Ready message with glow ────────────────
	if s.tick >= 31 {
		content.WriteString("\n")
		s.renderReady(&content)
	}

	// ── Skip hint: shown immediately ─────────────────────────────────
	{
		hintStyle := lipgloss.NewStyle().Foreground(surface2)
		if s.tick%8 < 4 {
			hintStyle = lipgloss.NewStyle().Foreground(overlay0)
		}
		content.WriteString("\n")
		content.WriteString(hintStyle.Render("  Press any key to continue"))
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

// renderLogoPulse renders the fully revealed logo with a color wave pulsing through it.
func (s SplashModel) renderLogoPulse(b *strings.Builder) {
	for _, line := range splashLogo {
		runes := []rune(line)
		lineLen := len(runes)
		var lineBuilder strings.Builder
		for j, r := range runes {
			col := splashColorAt(j, lineLen, s.tick*2)
			st := lipgloss.NewStyle().Foreground(col).Bold(true)
			lineBuilder.WriteString(st.Render(string(r)))
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

// renderAnimatedRule draws a horizontal rule with a shimmer effect.
func (s SplashModel) renderAnimatedRule(b *strings.Builder) {
	maxLen := 52
	progress := s.tick - 10
	currentLen := progress * maxLen / 6
	if currentLen > maxLen {
		currentLen = maxLen
	}

	var ruleBuilder strings.Builder
	ruleBuilder.WriteString("  ")
	for i := 0; i < currentLen; i++ {
		// Shimmer: cycle colors along the rule
		col := splashColorAt(i, maxLen, s.tick)
		st := lipgloss.NewStyle().Foreground(col)
		if i == currentLen-1 && currentLen < maxLen {
			// Leading edge sparkle
			sparkle := sparkleChars[s.tick%len(sparkleChars)]
			st = lipgloss.NewStyle().Foreground(text).Bold(true)
			ruleBuilder.WriteString(st.Render(sparkle))
		} else {
			ruleBuilder.WriteString(st.Render("━"))
		}
	}
	b.WriteString(ruleBuilder.String())
	b.WriteString("\n")
}

// renderTagline renders the tagline with typewriter effect and blinking cursor.
func (s SplashModel) renderTagline(b *strings.Builder) {
	tagline := "Terminal Knowledge Manager — Obsidian Compatible"
	runes := []rune(tagline)
	total := len(runes)

	ticksIn := s.tick - 12
	charsShown := ticksIn * 8 // 8 chars per tick for faster typing
	if charsShown > total {
		charsShown = total
	}

	var line strings.Builder
	line.WriteString("  ")

	tagStyle := lipgloss.NewStyle().Foreground(overlay0).Italic(true)
	if charsShown > 0 {
		line.WriteString(tagStyle.Render(string(runes[:charsShown])))
	}

	// Blinking cursor while typing or shortly after
	if charsShown < total || (s.tick-12) < 10 {
		if s.tick%4 < 2 {
			cursorStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
			line.WriteString(cursorStyle.Render("▌"))
		} else {
			line.WriteString(" ")
		}
	}

	b.WriteString(line.String())
	b.WriteString("\n")
}

// renderSystemChecks renders animated system initialization lines.
func (s SplashModel) renderSystemChecks(b *strings.Builder) {
	type checkItem struct {
		label string
		icon  string
		color lipgloss.Color
	}
	checks := []checkItem{
		{"Loading vault index", "◈", sapphire},
		{"Scanning notes", "◈", blue},
		{"Building link graph", "◈", teal},
		{"Initializing plugins", "◈", green},
	}

	ticksIn := s.tick - 15
	linesRevealed := ticksIn/1 + 1
	if linesRevealed > len(checks) {
		linesRevealed = len(checks)
	}

	for i := 0; i < linesRevealed; i++ {
		check := checks[i]
		lineAge := ticksIn - i*1

		var prefix string
		var prefixColor lipgloss.Color

		if lineAge < 3 {
			// Spinning animation
			spinners := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
			prefix = spinners[s.tick%len(spinners)]
			prefixColor = check.color
		} else {
			// Completed
			prefix = "✓"
			prefixColor = green
		}

		prefixStyle := lipgloss.NewStyle().Foreground(prefixColor).Bold(true)
		labelStyle := lipgloss.NewStyle().Foreground(overlay0)

		b.WriteString("  ")
		b.WriteString(prefixStyle.Render(prefix))
		b.WriteString(labelStyle.Render(" " + check.label))

		if lineAge >= 3 {
			doneStyle := lipgloss.NewStyle().Foreground(surface2)
			b.WriteString(doneStyle.Render(" done"))
		}
		b.WriteString("\n")
	}
}

// renderProgressBar draws a gradient progress bar with percentage.
func (s SplashModel) renderProgressBar(b *strings.Builder) {
	barWidth := 44
	ticksIn := s.tick - 21
	filled := ticksIn * barWidth / 5
	if filled > barWidth {
		filled = barWidth
	}

	var bar strings.Builder
	bar.WriteString("  ")

	grad := splashGradientColors()
	for j := 0; j < barWidth; j++ {
		if j < filled {
			colorIdx := j * len(grad) / barWidth
			blockStyle := lipgloss.NewStyle().Foreground(grad[colorIdx%len(grad)])
			bar.WriteString(blockStyle.Render("█"))
		} else if j == filled && filled < barWidth {
			// Leading edge
			edgeStyle := lipgloss.NewStyle().Foreground(surface2)
			bar.WriteString(edgeStyle.Render("▓"))
		} else {
			dimBlock := lipgloss.NewStyle().Foreground(surface1)
			bar.WriteString(dimBlock.Render("░"))
		}
	}

	pct := filled * 100 / barWidth
	pctStyle := lipgloss.NewStyle().Foreground(text).Bold(true)
	bar.WriteString(pctStyle.Render(" " + itoa(pct) + "%"))

	b.WriteString(bar.String())
	b.WriteString("\n")
}

// renderVaultInfo renders vault information with slide-in effect.
func (s SplashModel) renderVaultInfo(b *strings.Builder) {
	type infoLine struct {
		icon  string
		label string
		value string
		color lipgloss.Color
	}
	lines := []infoLine{
		{"◆", "Vault", s.vaultPath, blue},
		{"◆", "Notes", itoa(s.noteCount), teal},
		{"◆", "Version", "v0.1.0", mauve},
	}

	ticksIn := s.tick - 26
	linesRevealed := ticksIn/1 + 1
	if linesRevealed > len(lines) {
		linesRevealed = len(lines)
	}

	for i := 0; i < linesRevealed; i++ {
		line := lines[i]
		lineAge := ticksIn - i*1

		// Slide-in: indent decreases over time
		indent := 10 - lineAge*3
		if indent < 0 {
			indent = 0
		}

		iconStyle := lipgloss.NewStyle().Foreground(line.color).Bold(true)
		labelStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
		valueStyle := lipgloss.NewStyle().Foreground(overlay0)

		b.WriteString(strings.Repeat(" ", indent+2))
		b.WriteString(iconStyle.Render(line.icon))
		b.WriteString(" ")
		b.WriteString(labelStyle.Render(line.label + ": "))
		b.WriteString(valueStyle.Render(line.value))
		b.WriteString("\n")
	}
}

// renderReady renders the "Ready!" message with pulsing glow.
func (s SplashModel) renderReady(b *strings.Builder) {
	ticksIn := s.tick - 31

	if ticksIn >= 0 {
		// Pulsing checkmark - alternate between bright and normal
		var checkStyle lipgloss.Style
		if s.tick%6 < 3 {
			checkStyle = lipgloss.NewStyle().Foreground(green).Bold(true)
		} else {
			checkStyle = lipgloss.NewStyle().Foreground(teal).Bold(true)
		}
		b.WriteString(checkStyle.Render("  ✦ Ready!"))
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
}

func NewExitSplash(noteCount int, uptime time.Duration) ExitSplash {
	return ExitSplash{
		noteCount: noteCount,
		uptime:    uptime,
	}
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
		if e.tick >= 50 {
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

	// ── Phase 1 (ticks 0-12): Saving animation ──────────────────────
	if e.tick >= 0 {
		if e.tick < 12 {
			spinners := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
			spinStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
			labelStyle := lipgloss.NewStyle().Foreground(overlay0)
			content.WriteString("  ")
			content.WriteString(spinStyle.Render(spinners[e.tick%len(spinners)]))
			content.WriteString(labelStyle.Render(" Saving workspace state..."))
			content.WriteString("\n\n")
		} else {
			checkStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
			labelStyle := lipgloss.NewStyle().Foreground(overlay0)
			content.WriteString("  ")
			content.WriteString(checkStyle.Render("✓"))
			content.WriteString(labelStyle.Render(" Workspace saved"))
			content.WriteString("\n\n")
		}
	}

	// ── Phase 2 (ticks 10-25): Logo dissolve effect ─────────────────
	if e.tick >= 10 {
		dissolveProgress := e.tick - 10
		for i, line := range splashLogo {
			runes := []rune(line)
			var lineBuilder strings.Builder
			for j, r := range runes {
				// Dissolve from edges inward with randomish pattern
				distFromEdge := j
				if j > len(runes)/2 {
					distFromEdge = len(runes) - j
				}
				// Wave function for organic dissolve
				wave := int(math.Sin(float64(j+i*7)*0.3) * 3)
				threshold := dissolveProgress*3 - distFromEdge/2 + wave

				if r == ' ' {
					lineBuilder.WriteString(" ")
				} else if threshold > 8 {
					// Fully dissolved
					lineBuilder.WriteString(" ")
				} else if threshold > 5 {
					// Fading
					st := lipgloss.NewStyle().Foreground(surface1)
					lineBuilder.WriteString(st.Render("·"))
				} else if threshold > 2 {
					// Dimming
					st := lipgloss.NewStyle().Foreground(surface2)
					lineBuilder.WriteString(st.Render(string(r)))
				} else {
					// Still visible
					col := splashColorAt(j, len(runes), e.tick)
					st := lipgloss.NewStyle().Foreground(col).Bold(true)
					lineBuilder.WriteString(st.Render(string(r)))
				}
			}
			content.WriteString(lineBuilder.String())
			content.WriteString("\n")
		}
	}

	// ── Phase 3 (ticks 20-35): Session stats ────────────────────────
	if e.tick >= 20 {
		content.WriteString("\n")

		// Format uptime
		uptimeStr := exitFormatDuration(e.uptime + time.Duration(e.tick)*40*time.Millisecond)

		type statLine struct {
			icon  string
			label string
			value string
			color lipgloss.Color
		}
		stats := []statLine{
			{"◇", "Session", uptimeStr, blue},
			{"◇", "Notes", itoa(e.noteCount), teal},
		}

		ticksIn := e.tick - 20
		linesShown := ticksIn/3 + 1
		if linesShown > len(stats) {
			linesShown = len(stats)
		}

		for i := 0; i < linesShown; i++ {
			s := stats[i]
			iconStyle := lipgloss.NewStyle().Foreground(s.color)
			labelStyle := lipgloss.NewStyle().Foreground(surface2)
			valueStyle := lipgloss.NewStyle().Foreground(overlay0)

			content.WriteString("  ")
			content.WriteString(iconStyle.Render(s.icon))
			content.WriteString(labelStyle.Render(" " + s.label + ": "))
			content.WriteString(valueStyle.Render(s.value))
			content.WriteString("\n")
		}
	}

	// ── Phase 4 (ticks 28-40): Goodbye wave rule ────────────────────
	if e.tick >= 28 {
		content.WriteString("\n")
		ruleLen := 40
		var rule strings.Builder
		rule.WriteString("  ")

		// Rule collapses from edges
		ticksIn := e.tick - 28
		visibleLen := ruleLen - ticksIn*3
		if visibleLen < 0 {
			visibleLen = 0
		}
		padLeft := (ruleLen - visibleLen) / 2

		rule.WriteString(strings.Repeat(" ", padLeft))
		for i := 0; i < visibleLen; i++ {
			col := splashColorAt(i, visibleLen, e.tick*2)
			st := lipgloss.NewStyle().Foreground(col)
			rule.WriteString(st.Render("━"))
		}
		content.WriteString(rule.String())
		content.WriteString("\n")
	}

	// ── Phase 5 (ticks 32+): Goodbye message ────────────────────────
	if e.tick >= 32 {
		content.WriteString("\n")

		goodbye := "Until next time."
		runes := []rune(goodbye)
		ticksIn := e.tick - 32
		charsShown := ticksIn * 2
		if charsShown > len(runes) {
			charsShown = len(runes)
		}

		byeStyle := lipgloss.NewStyle().Foreground(mauve).Italic(true)
		content.WriteString("  ")
		content.WriteString(byeStyle.Render(string(runes[:charsShown])))

		// Blinking cursor
		if charsShown < len(runes) || ticksIn < 12 {
			if e.tick%4 < 2 {
				cursorStyle := lipgloss.NewStyle().Foreground(mauve)
				content.WriteString(cursorStyle.Render("▌"))
			}
		}
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
