package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type QuickCapture struct {
	active    bool
	width     int
	height    int
	vaultRoot string

	input     string
	mode      int    // 0=inbox, 1=daily, 2=task, 3=new note
	saved     bool
	statusMsg string
	resultPath string
}

func NewQuickCapture() QuickCapture {
	return QuickCapture{}
}

func (qc *QuickCapture) SetSize(width, height int) {
	qc.width = width
	qc.height = height
}

func (qc *QuickCapture) Open(vaultRoot string) {
	qc.active = true
	qc.vaultRoot = vaultRoot
	qc.input = ""
	qc.mode = 0
	qc.saved = false
	qc.statusMsg = ""
	qc.resultPath = ""
}

func (qc *QuickCapture) Close() {
	qc.active = false
}

func (qc QuickCapture) IsActive() bool {
	return qc.active
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
				qc.input = qc.input[:len(qc.input)-1]
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
		entry := fmt.Sprintf("\n- [ ] %s", text)
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
		// New note: write full content
		err := os.WriteFile(filePath, []byte(content+"\n"), 0644)
		if err != nil {
			qc.statusMsg = "Error: " + err.Error()
			return
		}
	} else {
		// Append to existing file
		f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			qc.statusMsg = "Error: " + err.Error()
			return
		}
		_, err = f.WriteString(content)
		_ = f.Close()
		if err != nil {
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
		b.WriteString(DimStyle.Render("  Enter: save  Esc: cancel"))
	}

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(mauve).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}
