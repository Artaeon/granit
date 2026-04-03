package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// LayoutPicker is an overlay that lets the user browse, preview, and select layouts.
type LayoutPicker struct {
	active  bool
	width   int
	height  int
	cursor  int
	current string // layout active before opening (for live preview)
	result  string // consumed-once selected layout
}

func NewLayoutPicker() LayoutPicker {
	return LayoutPicker{}
}

func (lp *LayoutPicker) SetSize(w, h int) {
	lp.width = w
	lp.height = h
}

func (lp *LayoutPicker) Open(currentLayout string) {
	lp.active = true
	lp.current = currentLayout
	lp.result = ""
	// Position cursor on current layout
	lp.cursor = 0
	for i, l := range AllLayouts() {
		if l == currentLayout {
			lp.cursor = i
			break
		}
	}
}

func (lp *LayoutPicker) Close() {
	lp.active = false
}

func (lp *LayoutPicker) IsActive() bool {
	return lp.active
}

// GetResult returns the selected layout (consumed once).
func (lp *LayoutPicker) GetResult() (string, bool) {
	if lp.result == "" {
		return "", false
	}
	r := lp.result
	lp.result = ""
	return r, true
}

func (lp LayoutPicker) Update(msg tea.Msg) (LayoutPicker, tea.Cmd) {
	if !lp.active {
		return lp, nil
	}
	layouts := AllLayouts()

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			lp.active = false
			return lp, nil
		case "enter":
			if lp.cursor >= 0 && lp.cursor < len(layouts) {
				lp.result = layouts[lp.cursor]
			}
			lp.active = false
			return lp, nil
		case "up", "k":
			if lp.cursor > 0 {
				lp.cursor--
			}
			return lp, nil
		case "down", "j":
			if lp.cursor < len(layouts)-1 {
				lp.cursor++
			}
			return lp, nil
		}
	}
	return lp, nil
}

func (lp LayoutPicker) View() string {
	layouts := AllLayouts()

	width := lp.width * 3 / 5
	if width < 70 {
		width = 70
	}
	if width > 110 {
		width = 110
	}
	innerW := width - 6

	var b strings.Builder

	// Title
	title := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("  Layout Picker")
	b.WriteString(title + "\n")
	b.WriteString(lipgloss.NewStyle().Foreground(surface0).Render(strings.Repeat("─", innerW)) + "\n")

	// Two-column: list on left, preview on right
	listWidth := 30
	previewWidth := innerW - listWidth - 3 // 3 for separator
	if previewWidth < 30 {
		previewWidth = 30
	}

	// Build layout list
	var listLines []string
	for i, layout := range layouts {
		name := layoutDisplayName(layout)
		panels := LayoutPanelCount(layout)
		panelStr := lipgloss.NewStyle().Foreground(surface1).Render(panelCountLabel(panels))

		indicator := "  "
		if layout == lp.current {
			indicator = lipgloss.NewStyle().Foreground(green).Render("● ")
		}

		if i == lp.cursor {
			line := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(ThemeAccentBar+" ") +
				indicator +
				lipgloss.NewStyle().Foreground(text).Bold(true).Render(name) + " " + panelStr
			listLines = append(listLines, lipgloss.NewStyle().Background(surface0).Width(listWidth).Render(line))
		} else {
			line := "   " + indicator +
				lipgloss.NewStyle().Foreground(subtext0).Render(name) + " " + panelStr
			listLines = append(listLines, lipgloss.NewStyle().Width(listWidth).Render(line))
		}
	}

	// Build preview for selected layout
	selectedLayout := ""
	if lp.cursor >= 0 && lp.cursor < len(layouts) {
		selectedLayout = layouts[lp.cursor]
	}
	previewLines := layoutPreview(selectedLayout, previewWidth)

	// Combine list and preview
	listContent := strings.Join(listLines, "\n")
	separator := lipgloss.NewStyle().Foreground(surface0).Render(
		strings.Repeat("│\n", len(listLines)))
	// Build separator column
	var sepLines []string
	for range listLines {
		sepLines = append(sepLines, lipgloss.NewStyle().Foreground(surface0).Render(" │ "))
	}
	separatorCol := strings.Join(sepLines, "\n")

	combined := lipgloss.JoinHorizontal(lipgloss.Top, listContent, separatorCol, previewLines)
	_ = separator // unused, we use sepLines instead
	b.WriteString(combined + "\n")

	// Description
	b.WriteString(lipgloss.NewStyle().Foreground(surface0).Render(strings.Repeat("─", innerW)) + "\n")
	if selectedLayout != "" {
		desc := LayoutDescription(selectedLayout)
		b.WriteString(lipgloss.NewStyle().Foreground(overlay0).Italic(true).Padding(0, 1).Render(desc) + "\n")
	}

	// Footer
	b.WriteString(lipgloss.NewStyle().Foreground(surface0).Render(strings.Repeat("─", innerW)) + "\n")
	helpText := lipgloss.NewStyle().Foreground(overlay0).Render(
		" ↑/↓ Browse • ↵ Apply • ● Current • Esc Cancel")
	b.WriteString(helpText)

	return lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(base).
		Render(b.String())
}

// layoutDisplayName returns a pretty name for a layout.
func layoutDisplayName(layout string) string {
	switch layout {
	case LayoutDefault:
		return "Default"
	case LayoutWriter:
		return "Writer"
	case LayoutReading:
		return "Reading"
	case LayoutDashboard:
		return "Dashboard"
	case LayoutZen:
		return "Zen"
	case LayoutCockpit:
		return "Cockpit"
	case LayoutStacked:
		return "Stacked"
	case LayoutCornell:
		return "Cornell"
	case LayoutFocus:
		return "Focus"
	default:
		return layout
	}
}

func panelCountLabel(count int) string {
	switch count {
	case 1:
		return "1 panel"
	case 2:
		return "2 panels"
	case 3:
		return "3 panels"
	case 4:
		return "4 panels"
	default:
		return ""
	}
}

// layoutPreview returns an ASCII-art preview diagram of a layout.
func layoutPreview(layout string, width int) string {
	h := 11 // preview height in lines

	// Box-drawing helpers
	tl, tr, bl, br := "┌", "┐", "└", "┘"
	hz, vt := "─", "│"

	box := func(label string, w, lines int) []string {
		if w < 3 {
			w = 3
		}
		inner := w - 2
		result := make([]string, 0, lines)
		result = append(result, tl+strings.Repeat(hz, inner)+tr)
		// Center the label
		labelLine := ""
		if len(label) > inner {
			label = label[:inner]
		}
		pad := inner - len(label)
		left := pad / 2
		right := pad - left
		labelLine = vt + strings.Repeat(" ", left) + label + strings.Repeat(" ", right) + vt
		emptyLine := vt + strings.Repeat(" ", inner) + vt
		mid := (lines - 2) / 2
		for i := 0; i < lines-2; i++ {
			if i == mid {
				result = append(result, labelLine)
			} else {
				result = append(result, emptyLine)
			}
		}
		result = append(result, bl+strings.Repeat(hz, inner)+br)
		return result
	}

	joinH := func(panels ...[]string) string {
		maxH := 0
		for _, p := range panels {
			if len(p) > maxH {
				maxH = len(p)
			}
		}
		var lines []string
		for i := 0; i < maxH; i++ {
			var parts []string
			for _, p := range panels {
				if i < len(p) {
					parts = append(parts, p[i])
				}
			}
			lines = append(lines, strings.Join(parts, ""))
		}
		return strings.Join(lines, "\n")
	}

	joinV := func(panels ...[]string) string {
		var lines []string
		for _, p := range panels {
			lines = append(lines, p...)
		}
		return strings.Join(lines, "\n")
	}

	sideW := width / 5
	if sideW < 8 {
		sideW = 8
	}

	style := lipgloss.NewStyle().Foreground(surface1)

	switch layout {
	case LayoutDefault:
		edW := width - sideW*2
		return style.Render(joinH(
			box("Files", sideW, h),
			box("Editor", edW, h),
			box("Links", sideW, h),
		))
	case LayoutWriter:
		edW := width - sideW
		return style.Render(joinH(
			box("Files", sideW, h),
			box("Editor", edW, h),
		))
	case LayoutReading:
		edW := width - sideW
		return style.Render(joinH(
			box("Editor", edW, h),
			box("Links", sideW, h),
		))
	case LayoutDashboard:
		smallW := width / 6
		if smallW < 7 {
			smallW = 7
		}
		edW := width - sideW - smallW*2
		return style.Render(joinH(
			box("Files", sideW, h),
			box("Editor", edW, h),
			box("Outline", smallW, h),
			box("Links", smallW, h),
		))
	case LayoutZen:
		padW := (width - width*3/5) / 2
		padStr := strings.Repeat(" ", padW)
		zenW := width - padW*2
		zenBox := box("Editor", zenW, h)
		var lines []string
		for _, l := range zenBox {
			lines = append(lines, padStr+l)
		}
		return style.Render(strings.Join(lines, "\n"))
	case LayoutFocus:
		edW := width - sideW
		padW := edW / 6
		if padW < 1 {
			padW = 1
		}
		centerW := edW - padW*2
		centerBox := box("Editor", centerW, h)
		padStr := strings.Repeat(" ", padW)
		var centerLines []string
		for _, l := range centerBox {
			centerLines = append(centerLines, padStr+l)
		}
		// Combine with sidebar
		sideBox := box("Files", sideW, h)
		return style.Render(joinH(sideBox, centerLines))
	case LayoutCockpit:
		rightW := width / 4
		if rightW < 10 {
			rightW = 10
		}
		edW := width - sideW - rightW
		topH := h / 2
		botH := h - topH
		sideBox := box("Files", sideW, h)
		edBox := box("Editor", edW, h)
		rightTop := box("Calendar", rightW, topH)
		rightBot := box("Tasks", rightW, botH)
		rightLines := append(rightTop, rightBot...)
		return style.Render(joinH(sideBox, edBox, rightLines))
	case LayoutStacked:
		edW := width - sideW
		topH := h * 2 / 3
		if topH < 4 {
			topH = 4
		}
		botH := h - topH
		outW := edW / 3
		blW := edW - outW
		topBox := box("Editor", edW, topH)
		botLeft := box("Outline", outW, botH)
		botRight := box("Links", blW, botH)
		var rightBot []string
		for i := 0; i < len(botLeft); i++ {
			left := ""
			right := ""
			if i < len(botLeft) {
				left = botLeft[i]
			}
			if i < len(botRight) {
				right = botRight[i]
			}
			rightBot = append(rightBot, left+right)
		}
		rightLines := append(topBox, rightBot...)
		return style.Render(joinH(box("Files", sideW, h), rightLines))
	case LayoutCornell:
		edW := width - sideW
		topH := h * 2 / 3
		if topH < 4 {
			topH = 4
		}
		botH := h - topH
		if botH < 3 {
			botH = 3
		}
		rightSide := joinV(
			box("Editor", edW, topH),
			box("Notes & Summary", edW, botH),
		)
		return style.Render(joinH(
			box("Files", sideW, topH+botH),
			strings.Split(rightSide, "\n"),
		))
	default:
		return style.Render(joinH(box("Editor", width, h)))
	}
}
