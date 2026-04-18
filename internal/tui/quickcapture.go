package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	qcDateRe = regexp.MustCompile(`@(today|tomorrow|monday|tuesday|wednesday|thursday|friday|saturday|sunday|next week|next month|next \w+|end of week|end of month|in \d+ \w+|\d{4}-\d{2}-\d{2})`)
	qcPrioRe = regexp.MustCompile(`!(low|med|medium|high|highest)\b`)
)

// parseInlineTaskSyntax extracts @date and !priority from task text,
// returning cleaned text and markdown markers to append.
func parseInlineTaskSyntax(text string) (string, string) {
	var markers []string
	clean := text

	// Date: @today, @tomorrow, @monday, @YYYY-MM-DD
	if m := qcDateRe.FindStringSubmatch(clean); m != nil {
		dateStr := resolveRelativeDate(m[1])
		markers = append(markers, "\U0001F4C5 "+dateStr)
		clean = qcDateRe.ReplaceAllString(clean, "")
	}

	// Priority: !low, !med, !high, !highest
	if m := qcPrioRe.FindStringSubmatch(clean); m != nil {
		icons := map[string]string{
			"low": "\U0001F53D", "med": "\U0001F53C", "medium": "\U0001F53C",
			"high": "\u23EB", "highest": "\U0001F53A",
		}
		if icon, ok := icons[m[1]]; ok {
			markers = append(markers, icon)
		}
		clean = qcPrioRe.ReplaceAllString(clean, "")
	}

	clean = strings.TrimSpace(clean)
	return clean, strings.Join(markers, " ")
}

var qcNLDateRe = regexp.MustCompile(`in\s+(\d+)\s+(days?|weeks?|months?)`)

func resolveRelativeDate(ref string) string {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	lower := strings.ToLower(ref)
	switch lower {
	case "today":
		return today.Format("2006-01-02")
	case "tomorrow":
		return today.AddDate(0, 0, 1).Format("2006-01-02")
	case "next week":
		// Next Monday
		daysAhead := (int(time.Monday) - int(today.Weekday()) + 7) % 7
		if daysAhead == 0 {
			daysAhead = 7
		}
		return today.AddDate(0, 0, daysAhead).Format("2006-01-02")
	case "next month":
		return today.AddDate(0, 1, 0).Format("2006-01-02")
	case "end of week":
		daysAhead := (int(time.Friday) - int(today.Weekday()) + 7) % 7
		if daysAhead == 0 {
			daysAhead = 7
		}
		return today.AddDate(0, 0, daysAhead).Format("2006-01-02")
	case "end of month":
		firstOfNext := time.Date(today.Year(), today.Month()+1, 1, 0, 0, 0, 0, time.Local)
		return firstOfNext.AddDate(0, 0, -1).Format("2006-01-02")
	default:
		// "in N days/weeks/months"
		if m := qcNLDateRe.FindStringSubmatch(lower); m != nil {
			n := 0
			_, _ = fmt.Sscanf(m[1], "%d", &n)
			switch {
			case strings.HasPrefix(m[2], "day"):
				return today.AddDate(0, 0, n).Format("2006-01-02")
			case strings.HasPrefix(m[2], "week"):
				return today.AddDate(0, 0, n*7).Format("2006-01-02")
			case strings.HasPrefix(m[2], "month"):
				return today.AddDate(0, n, 0).Format("2006-01-02")
			}
		}
		// Weekday names
		weekdays := map[string]time.Weekday{
			"monday": time.Monday, "tuesday": time.Tuesday, "wednesday": time.Wednesday,
			"thursday": time.Thursday, "friday": time.Friday, "saturday": time.Saturday, "sunday": time.Sunday,
		}
		if wd, ok := weekdays[lower]; ok {
			daysAhead := (int(wd) - int(today.Weekday()) + 7) % 7
			if daysAhead == 0 {
				daysAhead = 7
			}
			return today.AddDate(0, 0, daysAhead).Format("2006-01-02")
		}
		// "next friday" etc.
		if strings.HasPrefix(lower, "next ") {
			dayName := strings.TrimPrefix(lower, "next ")
			if wd, ok := weekdays[dayName]; ok {
				daysAhead := (int(wd) - int(today.Weekday()) + 7) % 7
				if daysAhead == 0 {
					daysAhead = 7
				}
				return today.AddDate(0, 0, daysAhead).Format("2006-01-02")
			}
		}
		return ref // already YYYY-MM-DD
	}
}

type QuickCapture struct {
	OverlayBase
	vaultRoot string

	input     string
	mode      int    // 0=inbox, 1=daily, 2=task, 3=new note
	saved     bool
	statusMsg string
	resultPath string
}

func (qc *QuickCapture) Open(vaultRoot string) {
	qc.Activate()
	qc.vaultRoot = vaultRoot
	qc.input = ""
	qc.mode = 0
	qc.saved = false
	qc.statusMsg = ""
	qc.resultPath = ""
}

// GetResult returns the file path that was written to (consumed-once pattern).
func (qc *QuickCapture) GetResult() (filePath string, ok bool) {
	if qc.resultPath != "" {
		p := qc.resultPath
		qc.resultPath = ""
		return p, true
	}
	return "", false
}

func (qc QuickCapture) Update(msg tea.Msg) (QuickCapture, tea.Cmd) {
	if !qc.active {
		return qc, nil
	}

	// If saved, any key closes the overlay.
	if qc.saved {
		switch msg.(type) {
		case tea.KeyMsg:
			qc.active = false
		}
		return qc, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			qc.active = false
			return qc, nil
		case "ctrl+i":
			qc.mode = 0
			return qc, nil
		case "ctrl+d":
			qc.mode = 1
			return qc, nil
		case "ctrl+t":
			qc.mode = 2
			return qc, nil
		case "ctrl+n":
			qc.mode = 3
			return qc, nil
		case "enter":
			if strings.TrimSpace(qc.input) == "" {
				return qc, nil
			}
			qc.save()
			return qc, nil
		case "backspace":
			if len(qc.input) > 0 {
				qc.input = TrimLastRune(qc.input)
			}
			return qc, nil
		default:
			char := msg.String()
			if len(char) == 1 && char[0] >= 32 {
				qc.input += char
			}
			return qc, nil
		}
	}
	return qc, nil
}

func (qc *QuickCapture) save() {
	text := strings.TrimSpace(qc.input)
	if text == "" {
		return
	}

	now := time.Now()
	var filePath string
	var content string

	switch qc.mode {
	case 0: // Inbox
		filePath = filepath.Join(qc.vaultRoot, "Inbox.md")
		entry := fmt.Sprintf("\n- %s %s — %s", now.Format("2006-01-02"), now.Format("15:04"), text)
		content = entry

	case 1: // Daily note
		dir := filepath.Join(qc.vaultRoot, "Daily")
		_ = os.MkdirAll(dir, 0755)
		filePath = filepath.Join(dir, now.Format("2006-01-02")+".md")
		entry := fmt.Sprintf("\n- %s — %s", now.Format("15:04"), text)
		content = entry

	case 2: // Task
		filePath = filepath.Join(qc.vaultRoot, "Tasks.md")
		cleanText, markers := parseInlineTaskSyntax(text)
		entry := fmt.Sprintf("\n- [ ] %s", cleanText)
		if markers != "" {
			entry += " " + markers
		}
		content = entry

	case 3: // New note
		lines := strings.SplitN(text, "\n", 2)
		name := strings.TrimSpace(lines[0])
		// Sanitize filename
		name = strings.Map(func(r rune) rune {
			if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
				return '-'
			}
			return r
		}, name)
		if name == "" {
			name = "Untitled"
		}
		if !strings.HasSuffix(name, ".md") {
			name += ".md"
		}
		filePath = filepath.Join(qc.vaultRoot, name)
		content = text
	}

	if qc.mode == 3 {
		// New note: avoid overwriting existing files
		if _, statErr := os.Stat(filePath); statErr == nil {
			ext := filepath.Ext(filePath)
			base := strings.TrimSuffix(filePath, ext)
			found := false
			for n := 1; n < 100; n++ {
				candidate := fmt.Sprintf("%s-%d%s", base, n, ext)
				if _, err := os.Stat(candidate); os.IsNotExist(err) {
					filePath = candidate
					found = true
					break
				}
			}
			if !found {
				// Fallback: use timestamp to guarantee uniqueness.
				filePath = fmt.Sprintf("%s-%d%s", base, time.Now().UnixNano(), ext)
			}
		}
		if err := atomicWriteNote(filePath, content+"\n"); err != nil {
			qc.statusMsg = "Error: " + err.Error()
			return
		}
	} else {
		// Append to existing file atomically (read + append + write).
		existing, err := os.ReadFile(filePath)
		if err != nil && !os.IsNotExist(err) {
			qc.statusMsg = "Error: " + err.Error()
			return
		}
		merged := string(existing) + content
		if err := atomicWriteNote(filePath, merged); err != nil {
			qc.statusMsg = "Error: " + err.Error()
			return
		}
	}

	qc.saved = true
	qc.resultPath = filePath

	// Build status message
	relPath, err := filepath.Rel(qc.vaultRoot, filePath)
	if err != nil {
		relPath = filepath.Base(filePath)
	}
	qc.statusMsg = "Saved to " + relPath
}

func (qc QuickCapture) View() string {
	width := 56
	if qc.width > 0 && qc.width < width+6 {
		width = qc.width - 6
		if width < 30 {
			width = 30
		}
	}

	innerWidth := width - 6

	var b strings.Builder

	// Title
	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  " + IconEditChar + " Quick Capture")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", innerWidth)))
	b.WriteString("\n\n")

	if qc.saved {
		// Show success message
		successStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
		b.WriteString("  " + successStyle.Render(qc.statusMsg))
		b.WriteString("\n\n")
		b.WriteString(DimStyle.Render("  Press any key to close"))
	} else {
		// Mode labels
		modeNames := []struct {
			key   string
			label string
			color lipgloss.Color
		}{
			{"^I", "Inbox", blue},
			{"^D", "Daily", green},
			{"^T", "Task", yellow},
			{"^N", "Note", peach},
		}

		b.WriteString("  ")
		for i, m := range modeNames {
			if i > 0 {
				b.WriteString("  ")
			}
			if i == qc.mode {
				style := lipgloss.NewStyle().
					Foreground(crust).
					Background(m.color).
					Bold(true).
					Padding(0, 1)
				b.WriteString(style.Render(m.key + ":" + m.label))
			} else {
				keyStyle := lipgloss.NewStyle().Foreground(m.color)
				labelStyle := lipgloss.NewStyle().Foreground(overlay0)
				b.WriteString(keyStyle.Render(m.key) + labelStyle.Render(":"+m.label))
			}
		}
		b.WriteString("\n\n")

		// Input area
		promptStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
		b.WriteString("  " + promptStyle.Render("> "))

		inputWidth := innerWidth - 6
		if inputWidth < 10 {
			inputWidth = 10
		}

		displayInput := qc.input
		// Show only the tail of input if it overflows the line
		if len(displayInput) > inputWidth {
			displayInput = displayInput[len(displayInput)-inputWidth:]
		}

		// Render input text with cursor
		inputStyle := lipgloss.NewStyle().Foreground(text)
		cursorStyle := lipgloss.NewStyle().Background(text).Foreground(mantle)
		b.WriteString(inputStyle.Render(displayInput))
		b.WriteString(cursorStyle.Render(" "))

		// Pad the remaining space
		remaining := inputWidth - len(displayInput)
		if remaining > 1 {
			b.WriteString(strings.Repeat(" ", remaining-1))
		}

		b.WriteString("\n\n")

		// Help line
		ks := lipgloss.NewStyle().Foreground(lavender).Bold(true)
		ds := DimStyle
		b.WriteString("  " + ks.Render("Enter") + ds.Render(":save") + "  " + ks.Render("Esc") + ds.Render(":cancel") + "  " + ks.Render("Tab") + ds.Render(":mode"))
		if qc.mode == 2 { // Task mode
			b.WriteString("\n  " + ds.Render("Syntax: @tomorrow !high #tag ~1h"))
		}
	}

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}
