package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type FindReplace struct {
	active      bool
	findQuery   string
	replaceText string
	matches     []FindMatch
	matchIdx    int
	width       int
	height      int
	mode        int // 0=find, 1=replace
	focusField  int // 0=find field, 1=replace field
	resultLine  int // line to jump to
	doReplace   bool
	doReplaceAll bool

	// Search history for find queries (Up/Down to recall)
	history    []string
	historyIdx int    // -1 means new query mode
	savedQuery string // stash current typed query when browsing history
	vaultRoot  string
}

type FindMatch struct {
	line int
	col  int
	text string // the surrounding line text
}

func NewFindReplace() FindReplace {
	return FindReplace{resultLine: -1, historyIdx: -1}
}

func (fr *FindReplace) SetSize(width, height int) {
	fr.width = width
	fr.height = height
}

func (fr *FindReplace) OpenFind(vaultRoot string) {
	fr.active = true
	fr.mode = 0
	fr.findQuery = ""
	fr.replaceText = ""
	fr.matches = nil
	fr.matchIdx = 0
	fr.focusField = 0
	fr.resultLine = -1
	fr.doReplace = false
	fr.doReplaceAll = false
	fr.vaultRoot = vaultRoot
	fr.historyIdx = -1
	fr.savedQuery = ""
	h := loadSearchHistory(vaultRoot)
	fr.history = h.FindReplace
}

func (fr *FindReplace) OpenReplace(vaultRoot string) {
	fr.active = true
	fr.mode = 1
	fr.findQuery = ""
	fr.replaceText = ""
	fr.matches = nil
	fr.matchIdx = 0
	fr.focusField = 0
	fr.resultLine = -1
	fr.doReplace = false
	fr.doReplaceAll = false
	fr.vaultRoot = vaultRoot
	fr.historyIdx = -1
	fr.savedQuery = ""
	h := loadSearchHistory(vaultRoot)
	fr.history = h.FindReplace
}

func (fr *FindReplace) Close() {
	fr.active = false
}

func (fr *FindReplace) IsActive() bool {
	return fr.active
}

func (fr *FindReplace) GetJumpLine() int {
	r := fr.resultLine
	fr.resultLine = -1
	return r
}

func (fr *FindReplace) ShouldReplace() bool {
	r := fr.doReplace
	fr.doReplace = false
	return r
}

func (fr *FindReplace) ShouldReplaceAll() bool {
	r := fr.doReplaceAll
	fr.doReplaceAll = false
	return r
}

func (fr *FindReplace) GetFindQuery() string {
	return fr.findQuery
}

func (fr *FindReplace) GetReplaceText() string {
	return fr.replaceText
}

func (fr *FindReplace) UpdateMatches(content []string) {
	fr.matches = nil
	if fr.findQuery == "" {
		return
	}
	query := strings.ToLower(fr.findQuery)
	for i, line := range content {
		lower := strings.ToLower(line)
		col := strings.Index(lower, query)
		for col != -1 {
			fr.matches = append(fr.matches, FindMatch{
				line: i,
				col:  col,
				text: line,
			})
			next := strings.Index(lower[col+len(query):], query)
			if next == -1 {
				break
			}
			col = col + len(query) + next
		}
	}
	if fr.matchIdx >= len(fr.matches) {
		fr.matchIdx = 0
	}
}

func (fr FindReplace) Update(msg tea.Msg) (FindReplace, tea.Cmd) {
	if !fr.active {
		return fr, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			fr.active = false
			return fr, nil
		case "tab":
			if fr.mode == 1 {
				fr.focusField = (fr.focusField + 1) % 2
			}
			return fr, nil
		case "enter":
			if fr.focusField == 0 && len(fr.matches) > 0 {
				// Jump to current match
				fr.resultLine = fr.matches[fr.matchIdx].line
				// Save find query to history
				if fr.findQuery != "" {
					fr.history = appendToHistory(fr.history, fr.findQuery)
					h := loadSearchHistory(fr.vaultRoot)
					h.FindReplace = fr.history
					saveSearchHistory(fr.vaultRoot, h)
				}
			} else if fr.focusField == 1 && fr.mode == 1 {
				// Replace current
				fr.doReplace = true
			}
			return fr, nil
		case "ctrl+a":
			if fr.mode == 1 {
				fr.doReplaceAll = true
			}
			return fr, nil
		case "ctrl+n", "down":
			if len(fr.matches) > 0 {
				fr.matchIdx = (fr.matchIdx + 1) % len(fr.matches)
				fr.resultLine = fr.matches[fr.matchIdx].line
			} else if fr.focusField == 0 && len(fr.history) > 0 {
				// Browse history forward in find field
				if fr.historyIdx >= 0 {
					if fr.historyIdx < len(fr.history)-1 {
						fr.historyIdx++
						fr.findQuery = fr.history[fr.historyIdx]
					} else {
						fr.historyIdx = -1
						fr.findQuery = fr.savedQuery
					}
				}
			}
			return fr, nil
		case "ctrl+p", "up":
			if len(fr.matches) > 0 {
				fr.matchIdx = (fr.matchIdx - 1 + len(fr.matches)) % len(fr.matches)
				fr.resultLine = fr.matches[fr.matchIdx].line
			} else if fr.focusField == 0 && len(fr.history) > 0 {
				// Browse history (most recent first) in find field
				if fr.historyIdx == -1 {
					fr.savedQuery = fr.findQuery
					fr.historyIdx = len(fr.history) - 1
				} else if fr.historyIdx > 0 {
					fr.historyIdx--
				}
				fr.findQuery = fr.history[fr.historyIdx]
			}
			return fr, nil
		case "backspace":
			if fr.focusField == 0 && len(fr.findQuery) > 0 {
				fr.findQuery = fr.findQuery[:len(fr.findQuery)-1]
				fr.historyIdx = -1
			} else if fr.focusField == 1 && len(fr.replaceText) > 0 {
				fr.replaceText = fr.replaceText[:len(fr.replaceText)-1]
			}
			return fr, nil
		default:
			char := msg.String()
			if len(char) == 1 && char[0] >= 32 {
				if fr.focusField == 0 {
					fr.findQuery += char
					fr.historyIdx = -1
				} else {
					fr.replaceText += char
				}
			}
			return fr, nil
		}
	}
	return fr, nil
}

func (fr FindReplace) View() string {
	width := fr.width / 2
	if width < 50 {
		width = 50
	}
	if width > 80 {
		width = 80
	}

	var b strings.Builder

	titleText := "  Find"
	if fr.mode == 1 {
		titleText = "  Find & Replace"
	}
	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render(titleText)
	b.WriteString(title)
	b.WriteString("\n\n")

	// Find field
	findLabel := "  Find:    "
	if fr.focusField == 0 {
		findLabel = SearchPromptStyle.Render("  Find:    ")
	} else {
		findLabel = DimStyle.Render("  Find:    ")
	}
	findInput := fr.findQuery
	if fr.focusField == 0 {
		findInput += DimStyle.Render("_")
	}
	b.WriteString(findLabel + findInput)

	// Match count
	if fr.findQuery != "" {
		matchInfo := DimStyle.Render("  " + smallNum(len(fr.matches)) + " matches")
		if len(fr.matches) > 0 {
			matchInfo += DimStyle.Render(" [" + smallNum(fr.matchIdx+1) + "/" + smallNum(len(fr.matches)) + "]")
		}
		b.WriteString(matchInfo)
	}
	b.WriteString("\n")

	// Replace field
	if fr.mode == 1 {
		replaceLabel := "  Replace: "
		if fr.focusField == 1 {
			replaceLabel = lipgloss.NewStyle().Foreground(green).Bold(true).Render("  Replace: ")
		} else {
			replaceLabel = DimStyle.Render("  Replace: ")
		}
		replaceInput := fr.replaceText
		if fr.focusField == 1 {
			replaceInput += DimStyle.Render("_")
		}
		b.WriteString(replaceLabel + replaceInput)
		b.WriteString("\n")
	}

	// Preview matches
	if len(fr.matches) > 0 {
		b.WriteString("\n")
		b.WriteString(DimStyle.Render(strings.Repeat("─", width-6)))
		b.WriteString("\n")

		maxPreview := 5
		for i := 0; i < len(fr.matches) && i < maxPreview; i++ {
			m := fr.matches[i]
			lineStr := DimStyle.Render("  L" + smallNum(m.line+1) + ": ")
			preview := m.text
			if len(preview) > width-20 {
				preview = preview[:width-23] + "..."
			}
			// Highlight the match in the preview
			lowerPreview := strings.ToLower(preview)
			lowerQuery := strings.ToLower(fr.findQuery)
			idx := strings.Index(lowerPreview, lowerQuery)
			if idx >= 0 {
				before := preview[:idx]
				match := preview[idx : idx+len(fr.findQuery)]
				after := preview[idx+len(fr.findQuery):]
				highlighted := MatchHighlightStyle.Render(match)
				preview = before + highlighted + after
			}

			if i == fr.matchIdx {
				b.WriteString(lipgloss.NewStyle().
					Background(surface0).
					Width(width - 6).
					Render(lineStr + preview))
			} else {
				b.WriteString(lineStr + preview)
			}
			if i < maxPreview-1 && i < len(fr.matches)-1 {
				b.WriteString("\n")
			}
		}
	}

	b.WriteString("\n\n")
	hints := "  ↑↓: navigate/history  Enter: jump"
	if fr.mode == 1 {
		hints += "  Tab: switch field  Ctrl+A: replace all"
	}
	b.WriteString(DimStyle.Render(hints))

	borderColor := mauve
	if fr.mode == 1 {
		borderColor = green
	}

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}
