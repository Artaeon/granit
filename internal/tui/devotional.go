package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

type devotionalResultMsg struct {
	reflection string
	err        error
}

type devotionalTickMsg struct{}

func devotionalTickCmd() tea.Cmd {
	return tea.Tick(300*time.Millisecond, func(time.Time) tea.Msg {
		return devotionalTickMsg{}
	})
}

// ---------------------------------------------------------------------------
// Overlay
// ---------------------------------------------------------------------------

// Devotional generates a personal AI reflection connecting the daily scripture
// to the user's active goals and current focus.
type Devotional struct {
	active bool
	width  int
	height int

	// AI config
	ai AIConfig

	// Input data
	vaultRoot  string
	scripture  Scripture
	goals      []Goal
	todayFocus string

	// State
	loading     bool
	loadingTick int
	reflection  string
	lines       []string
	scroll      int
}

// NewDevotional creates a new inactive Devotional overlay.
func NewDevotional() Devotional {
	return Devotional{}
}

// IsActive reports whether the devotional overlay is visible.
func (d Devotional) IsActive() bool { return d.active }

// SetSize updates the available terminal dimensions.
func (d *Devotional) SetSize(w, h int) {
	d.width = w
	d.height = h
}

// Open loads the daily scripture, gathers goals, and starts the AI call.
func (d *Devotional) Open(vaultRoot string, ai AIConfig, goals []Goal) tea.Cmd {
	d.active = true
	d.vaultRoot = vaultRoot
	d.ai = ai
	d.scripture = DailyScripture(vaultRoot)
	d.goals = goals
	d.loading = true
	d.loadingTick = 0
	d.reflection = ""
	d.lines = nil
	d.scroll = 0

	return tea.Batch(d.generateCmd(), devotionalTickCmd())
}

// Update handles messages.
func (d Devotional) Update(msg tea.Msg) (Devotional, tea.Cmd) {
	if !d.active {
		return d, nil
	}

	switch msg := msg.(type) {
	case devotionalResultMsg:
		d.loading = false
		if msg.err != nil {
			d.reflection = "AI unavailable: " + msg.err.Error()
		} else {
			d.reflection = msg.reflection
		}
		d.lines = strings.Split(d.reflection, "\n")
		d.scroll = 0
		return d, nil

	case devotionalTickMsg:
		if d.loading {
			d.loadingTick++
			return d, devotionalTickCmd()
		}
		return d, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			d.active = false
		case "j", "down":
			if len(d.lines) > 0 && d.scroll < len(d.lines)-1 {
				d.scroll++
			}
		case "k", "up":
			if d.scroll > 0 {
				d.scroll--
			}
		}
	}
	return d, nil
}

// generateCmd sends the devotional prompt to the AI.
func (d *Devotional) generateCmd() tea.Cmd {
	ai := d.ai
	scripture := d.scripture
	goals := make([]Goal, len(d.goals))
	copy(goals, d.goals)

	return func() tea.Msg {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("TODAY: %s\n", time.Now().Format("Monday, January 2, 2006")))
		sb.WriteString(fmt.Sprintf("SCRIPTURE: \"%s\" -- %s\n", scripture.Text, scripture.Source))

		if len(goals) > 0 {
			sb.WriteString("\nACTIVE GOALS:\n")
			for _, g := range goals {
				if g.Status != GoalStatusActive {
					continue
				}
				overdue := ""
				if g.IsOverdue() {
					overdue = " (OVERDUE)"
				}
				sb.WriteString(fmt.Sprintf("- %s (%d%%, %s)%s\n", g.Title, g.Progress(), g.Category, overdue))
			}
		}

		systemPrompt := "You are DEEPCOVEN, a faith-informed personal advisor. " +
			"The user starts each day with a scripture verse. " +
			"Connect this verse to their current life context and goals.\n\n" +
			"Generate a brief personal devotional reflection:\n\n" +
			"1. VERSE INSIGHT: What is the core truth of this verse? (2 sentences)\n" +
			"2. TODAY'S APPLICATION: How does this verse speak to what the user is working on? " +
			"Connect it to their specific goals and challenges.\n" +
			"3. PRAYER FOCUS: A single sentence the user can carry through their day.\n" +
			"4. ACTION: One concrete thing to do today that lives out this verse.\n\n" +
			"Keep it personal, grounded, and under 12 lines. Reference specific goals by name."

		resp, err := ai.Chat(systemPrompt, sb.String())
		return devotionalResultMsg{reflection: strings.TrimSpace(resp), err: err}
	}
}

// View renders the devotional overlay.
func (d Devotional) View() string {
	if !d.active {
		return ""
	}

	width := d.width * 2 / 3
	if width < 60 {
		width = 60
	}
	if width > 90 {
		width = 90
	}
	innerW := width - 8

	var b strings.Builder

	// Header
	headerStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	b.WriteString("  " + headerStyle.Render(IconBotChar+" DEEPCOVEN -- Daily Devotional") + "\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", innerW-4)) + "\n\n")

	// Scripture
	verseStyle := lipgloss.NewStyle().Foreground(lavender).Italic(true)
	refStyle := lipgloss.NewStyle().Foreground(overlay1)
	b.WriteString("  " + verseStyle.Render(TruncateDisplay(d.scripture.Text, innerW-6)) + "\n")
	b.WriteString("  " + refStyle.Render("-- "+d.scripture.Source) + "\n\n")

	if d.loading {
		spinChars := []string{"\u25CB", "\u25D4", "\u25D1", "\u25D5", "\u25CF"}
		spin := spinChars[d.loadingTick%len(spinChars)]
		b.WriteString("  " + lipgloss.NewStyle().Foreground(mauve).Render(spin+" Reflecting on this verse...") + "\n")
	} else if d.reflection != "" {
		bodyStyle := lipgloss.NewStyle().Foreground(text)
		headStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)

		maxLines := d.height - 14
		if maxLines < 5 {
			maxLines = 5
		}

		visible := d.lines
		if d.scroll > 0 && d.scroll < len(d.lines) {
			visible = d.lines[d.scroll:]
		}
		if len(visible) > maxLines {
			visible = visible[:maxLines]
		}

		for _, line := range visible {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				b.WriteString("\n")
			} else if strings.HasPrefix(trimmed, "##") {
				b.WriteString("  " + headStyle.Render(strings.TrimLeft(trimmed, "# ")) + "\n")
			} else {
				b.WriteString("  " + bodyStyle.Render(TruncateDisplay(trimmed, innerW-6)) + "\n")
			}
		}

		if len(d.lines) > maxLines {
			b.WriteString("\n  " + DimStyle.Render(fmt.Sprintf("(%d/%d lines -- j/k to scroll)", d.scroll+1, len(d.lines))))
		}
	}

	// Help bar
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", innerW-4)) + "\n")
	b.WriteString(RenderHelpBar([]struct{ Key, Desc string }{
		{"j/k", "scroll"}, {"Esc", "close"},
	}))

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}
