package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// FocusMode provides a distraction-free writing experience by centering the
// editor in a narrow column and hiding the sidebar and backlinks panel.
type FocusMode struct {
	active      bool
	width       int
	height      int
	targetWords int // optional word count goal (0 = no goal)
	startWords  int // word count when focus mode started

	// Goal-setting prompt state
	settingGoal bool   // true while the goal input prompt is displayed
	goalInput   string // text being typed into the goal prompt

	// Congratulations state
	goalReached bool // set once when the target is first met
}

// NewFocusMode returns a FocusMode in its default (inactive) state.
func NewFocusMode() FocusMode {
	return FocusMode{}
}

// SetSize updates the available terminal dimensions.
func (f *FocusMode) SetSize(width, height int) {
	f.width = width
	f.height = height
}

// Open activates focus mode, recording the current word count so that
// progress toward a target can be measured.
func (f *FocusMode) Open(currentWordCount int) {
	f.active = true
	f.startWords = currentWordCount
	f.settingGoal = false
	f.goalInput = ""
	f.goalReached = false
}

// Close deactivates focus mode.
func (f *FocusMode) Close() {
	f.active = false
	f.settingGoal = false
	f.goalInput = ""
}

// IsActive reports whether focus mode is currently engaged.
func (f *FocusMode) IsActive() bool {
	return f.active
}

// IsSettingGoal reports whether the goal-input prompt is open.
func (f *FocusMode) IsSettingGoal() bool {
	return f.settingGoal
}

// OpenGoalPrompt shows the word-count goal input prompt.
func (f *FocusMode) OpenGoalPrompt() {
	f.settingGoal = true
	f.goalInput = ""
	if f.targetWords > 0 {
		f.goalInput = strconv.Itoa(f.targetWords)
	}
}

// SetTargetWords sets an optional word-count goal. Pass 0 to clear the goal.
func (f *FocusMode) SetTargetWords(n int) {
	f.targetWords = n
}

// Update handles key events while the goal-setting prompt is active.
// It returns an updated FocusMode and an optional command.
func (f FocusMode) Update(msg tea.Msg) (FocusMode, tea.Cmd) {
	if !f.settingGoal {
		return f, nil
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return f, nil
	}

	switch keyMsg.String() {
	case "esc":
		// Cancel goal setting without changing the target
		f.settingGoal = false
		f.goalInput = ""
		return f, nil

	case "enter":
		// Confirm the goal
		f.settingGoal = false
		n, err := strconv.Atoi(strings.TrimSpace(f.goalInput))
		if err != nil || n <= 0 {
			// Invalid or zero clears the target
			f.targetWords = 0
		} else {
			f.targetWords = n
			f.goalReached = false // reset in case user sets a new target
		}
		f.goalInput = ""
		return f, nil

	case "backspace":
		if len(f.goalInput) > 0 {
			f.goalInput = f.goalInput[:len(f.goalInput)-1]
		}
		return f, nil

	default:
		// Accept only digit characters
		for _, r := range keyMsg.Runes {
			if r >= '0' && r <= '9' {
				f.goalInput += string(r)
			}
		}
		return f, nil
	}
}

// checkGoalReached updates the goalReached flag when the word-count target is
// first met. Call this from RenderEditor so it stays in sync.
func (f *FocusMode) checkGoalReached(wordCount int) {
	if f.targetWords <= 0 || f.goalReached {
		return
	}
	progress := wordCount - f.startWords
	if progress >= f.targetWords {
		f.goalReached = true
	}
}

// RenderEditor wraps the editor view in a centered, minimal layout suitable
// for distraction-free writing. It strips surrounding chrome and shows only
// the editor content inside a narrow column with a thin status line at the
// bottom.
func (f *FocusMode) RenderEditor(editorView string, wordCount int) string {
	f.checkGoalReached(wordCount)

	// --- column geometry ---
	const maxColumn = 80
	w := f.width
	h := f.height
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}

	colWidth := w - 4 // leave room for border + padding
	if colWidth > maxColumn {
		colWidth = maxColumn
	}
	if colWidth < 20 {
		colWidth = 20
	}

	// Available height: total minus border (2) minus status line (1) minus
	// a small top/bottom breathing margin supplied by padding.
	innerHeight := h - 4
	if innerHeight < 4 {
		innerHeight = 4
	}

	// --- status line ---
	statusLine := f.buildStatus(wordCount, colWidth)

	// --- goal prompt overlay or congratulations banner ---
	var overlay string
	if f.settingGoal {
		overlay = f.renderGoalPrompt(colWidth)
	} else if f.goalReached {
		overlay = f.renderCongrats(colWidth)
	}

	// --- truncate / pad editor content to fit ---
	lines := strings.Split(editorView, "\n")
	// Reserve lines for status (3 lines: ruler + info + hint) + optional overlay
	statusLines := strings.Count(statusLine, "\n") + 1
	overlayLines := 0
	if overlay != "" {
		overlayLines = strings.Count(overlay, "\n") + 1
	}
	editorHeight := innerHeight - statusLines - overlayLines
	if editorHeight < 1 {
		editorHeight = 1
	}
	if len(lines) > editorHeight {
		lines = lines[:editorHeight]
	}
	editorContent := strings.Join(lines, "\n")

	body := editorContent + "\n"
	if overlay != "" {
		body += overlay + "\n"
	}
	body += statusLine

	// --- focused panel style ---
	panel := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(mauve).
		Foreground(text).
		Width(colWidth).
		Height(innerHeight).
		Padding(0, 1)

	rendered := panel.Render(body)

	// --- center the panel horizontally and vertically ---
	centered := lipgloss.Place(
		w,
		h,
		lipgloss.Center,
		lipgloss.Center,
		rendered,
	)

	return centered
}

// buildStatus produces the minimal status string shown at the bottom of the
// focus-mode panel: word count and, when a target is set, a small progress
// bar with "X/Y words".
func (f *FocusMode) buildStatus(wordCount int, availWidth int) string {
	if availWidth < 4 {
		availWidth = 4
	}

	dimStyle := lipgloss.NewStyle().Foreground(overlay0)
	countStyle := lipgloss.NewStyle().Foreground(subtext0)

	sep := dimStyle.Render("─")
	rulerLen := availWidth - 2
	if rulerLen < 0 {
		rulerLen = 0
	}
	ruler := strings.Repeat(sep, rulerLen)

	var info string
	if f.targetWords > 0 {
		progress := wordCount - f.startWords
		if progress < 0 {
			progress = 0
		}
		pct := 0
		if f.targetWords > 0 {
			pct = (progress * 100) / f.targetWords
			if pct > 100 {
				pct = 100
			}
		}
		barMaxW := availWidth - 2
		if barMaxW < 0 {
			barMaxW = 0
		}
		info = f.renderProgressBar(progress, f.targetWords, barMaxW) +
			"  " + countStyle.Render(fmt.Sprintf("%d/%d words  %d%%", progress, f.targetWords, pct))
	} else {
		info = countStyle.Render(fmt.Sprintf("%d words", wordCount))
	}

	// Hint line: show Alt+G shortcut
	hintStyle := lipgloss.NewStyle().Foreground(overlay0)
	hint := hintStyle.Render("Alt+G: set word goal")

	// Compose: ruler on one conceptual line, info right-aligned below it.
	// We put the info on the same line as the ruler, right-aligned.
	infoWidth := lipgloss.Width(info)
	rulerWidth := lipgloss.Width(ruler)
	padding := rulerWidth - infoWidth
	if padding < 0 {
		padding = 0
	}

	hintWidth := lipgloss.Width(hint)
	hintPad := rulerWidth - hintWidth
	if hintPad < 0 {
		hintPad = 0
	}

	return ruler + "\n" + strings.Repeat(" ", padding) + info +
		"\n" + strings.Repeat(" ", hintPad) + hint
}

// renderGoalPrompt returns a small inline prompt for typing a word-count goal.
func (f *FocusMode) renderGoalPrompt(availWidth int) string {
	promptStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
	inputStyle := lipgloss.NewStyle().Foreground(text).Background(surface0).Padding(0, 1)
	hintStyle := lipgloss.NewStyle().Foreground(overlay0)

	label := promptStyle.Render("Word count goal: ")

	display := f.goalInput
	cursor := lipgloss.NewStyle().Background(text).Foreground(mantle).Render(" ")
	display += cursor

	inputWidth := availWidth - lipgloss.Width(label) - 6
	if inputWidth < 8 {
		inputWidth = 8
	}
	inputBox := inputStyle.Width(inputWidth).Render(display)

	line := "  " + label + inputBox
	hints := "  " + hintStyle.Render("Enter: confirm  Esc: cancel  (0 = clear goal)")

	return line + "\n" + hints
}

// renderCongrats returns a small congratulations banner to display after the
// word-count target has been reached.
func (f *FocusMode) renderCongrats(availWidth int) string {
	style := lipgloss.NewStyle().Foreground(green).Bold(true)
	msg := "Goal reached! Well done!"
	rendered := style.Render(msg)
	w := lipgloss.Width(rendered)
	pad := (availWidth - 2 - w) / 2
	if pad < 0 {
		pad = 0
	}
	return strings.Repeat(" ", pad) + rendered
}

// renderProgressBar draws a small horizontal bar using block characters,
// coloured with the theme's primary (mauve) and surface colours.
func (f *FocusMode) renderProgressBar(current, target, maxWidth int) string {
	barWidth := maxWidth / 3
	if barWidth < 8 {
		barWidth = 8
	}
	if barWidth > 20 {
		barWidth = 20
	}

	filled := 0
	if target > 0 {
		filled = (current * barWidth) / target
		if filled > barWidth {
			filled = barWidth
		}
	}
	empty := barWidth - filled

	filledStyle := lipgloss.NewStyle().Foreground(mauve)
	emptyStyle := lipgloss.NewStyle().Foreground(surface1)

	bar := filledStyle.Render(strings.Repeat("█", filled)) +
		emptyStyle.Render(strings.Repeat("░", empty))

	return bar
}
