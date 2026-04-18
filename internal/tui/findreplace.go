package tui

import (
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type FindReplace struct {
	OverlayBase
	findQuery   string
	replaceText string
	matches     []FindMatch
	matchIdx    int
	mode        int // 0=find, 1=replace
	focusField  int // 0=find field, 1=replace field
	resultLine  int // line to jump to
	doReplace   bool
	doReplaceAll bool

	// Regex search mode
	regexMode bool
	regexErr  string // non-empty when the regex pattern is invalid

	// Search history for find queries (Up/Down to recall)
	history    []string
	historyIdx int    // -1 means new query mode
	savedQuery string // stash current typed query when browsing history
	vaultRoot  string

	// Preview scroll state
	previewScroll int // first visible match index in preview list
}

type FindMatch struct {
	line int
	col  int
	text string // the surrounding line text
}

func NewFindReplace() FindReplace {
	return FindReplace{resultLine: -1, historyIdx: -1}
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
	fr.regexErr = ""
	fr.previewScroll = 0
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
	fr.regexErr = ""
	fr.previewScroll = 0
	h := loadSearchHistory(vaultRoot)
	fr.history = h.FindReplace
}

// IsRegexMode reports whether regex search is enabled.
func (fr *FindReplace) IsRegexMode() bool {
	return fr.regexMode
}

// ToggleRegex flips the regex mode flag.
func (fr *FindReplace) ToggleRegex() {
	fr.regexMode = !fr.regexMode
	fr.regexErr = ""
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
	fr.regexErr = ""
	fr.previewScroll = 0
	if fr.findQuery == "" {
		fr.matchIdx = 0
		return
	}

	if fr.regexMode {
		fr.updateMatchesRegex(content)
	} else {
		fr.updateMatchesPlain(content)
	}

	if fr.matchIdx >= len(fr.matches) {
		fr.matchIdx = 0
	}
}

// previewPageSize is the maximum number of matches shown in the preview list.
const previewPageSize = 5

// ensureMatchVisible adjusts previewScroll so that matchIdx is visible.
func (fr *FindReplace) ensureMatchVisible() {
	if fr.matchIdx < fr.previewScroll {
		fr.previewScroll = fr.matchIdx
	} else if fr.matchIdx >= fr.previewScroll+previewPageSize {
		fr.previewScroll = fr.matchIdx - previewPageSize + 1
	}
}

func (fr *FindReplace) updateMatchesPlain(content []string) {
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

func (fr *FindReplace) updateMatchesRegex(content []string) {
	re, err := regexp.Compile("(?i)" + fr.findQuery)
	if err != nil {
		fr.regexErr = err.Error()
		return
	}

	for i, line := range content {
		locs := re.FindAllStringIndex(line, -1)
		for _, loc := range locs {
			fr.matches = append(fr.matches, FindMatch{
				line: i,
				col:  loc[0],
				text: line,
			})
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
		case "alt+r":
			fr.regexMode = !fr.regexMode
			fr.regexErr = ""
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
				fr.ensureMatchVisible()
			} else if fr.focusField == 0 && len(fr.history) > 0 {
				// Browse history forward in find field
				if fr.historyIdx >= 0 {
					if fr.historyIdx < len(fr.history)-1 {
						fr.historyIdx++
						fr.findQuery = fr.history[fr.historyIdx]
					} else {
						fr.historyIdx = -1
						fr.findQuery = fr.savedQuery
						fr.savedQuery = ""
					}
				}
			}
			return fr, nil
		case "ctrl+p", "up":
			if len(fr.matches) > 0 {
				fr.matchIdx = (fr.matchIdx - 1 + len(fr.matches)) % len(fr.matches)
				fr.resultLine = fr.matches[fr.matchIdx].line
				fr.ensureMatchVisible()
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
				fr.findQuery = TrimLastRune(fr.findQuery)
				fr.historyIdx = -1
				fr.savedQuery = ""
			} else if fr.focusField == 1 && len(fr.replaceText) > 0 {
				fr.replaceText = TrimLastRune(fr.replaceText)
			}
			return fr, nil
		default:
			char := msg.String()
			if len(char) == 1 && char[0] >= 32 {
				if fr.focusField == 0 {
					fr.findQuery += char
					fr.historyIdx = -1
					fr.savedQuery = ""
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

	// Regex mode indicator next to title
	modeIndicator := DimStyle.Render(" [Aa]")
	if fr.regexMode {
		modeIndicator = lipgloss.NewStyle().Foreground(yellow).Bold(true).Render(" [.*]")
	}
	b.WriteString(modeIndicator)
	b.WriteString("\n\n")

	// Find field
	var findLabel string
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
		if fr.regexErr != "" {
			errStyle := lipgloss.NewStyle().Foreground(red).Bold(true)
			b.WriteString("  " + errStyle.Render("Invalid regex"))
		} else {
			matchInfo := DimStyle.Render("  " + smallNum(len(fr.matches)) + " matches")
			if len(fr.matches) > 0 {
				matchInfo += DimStyle.Render(" [" + smallNum(fr.matchIdx+1) + "/" + smallNum(len(fr.matches)) + "]")
			}
			b.WriteString(matchInfo)
		}
	}
	b.WriteString("\n")

	// Regex error detail
	if fr.regexErr != "" {
		errStyle := lipgloss.NewStyle().Foreground(red)
		b.WriteString(errStyle.Render("  " + fr.regexErr))
		b.WriteString("\n")
	}

	// Replace field
	if fr.mode == 1 {
		var replaceLabel string
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
		if fr.regexMode {
			b.WriteString(DimStyle.Render("  ($1, $2 for groups)"))
		}
		b.WriteString("\n")
	}

	// Preview matches
	if len(fr.matches) > 0 {
		b.WriteString("\n")
		b.WriteString(DimStyle.Render(strings.Repeat("─", width-6)))
		b.WriteString("\n")

		// Show scroll range indicator when there are more matches than visible
		total := len(fr.matches)
		start := fr.previewScroll
		end := start + previewPageSize
		if end > total {
			end = total
		}
		if total > previewPageSize {
			rangeInfo := "  (showing " + smallNum(start+1) + "-" + smallNum(end) + " of " + smallNum(total) + " matches)"
			b.WriteString(DimStyle.Render(rangeInfo))
			b.WriteString("\n")
		}

		for i := start; i < end; i++ {
			m := fr.matches[i]
			lineStr := DimStyle.Render("  L" + smallNum(m.line+1) + ": ")
			preview := TruncateDisplay(m.text, width-20)

			if fr.regexMode {
				// Regex highlighting
				preview = fr.highlightRegex(preview)
			} else {
				// Plain text highlighting
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
			}

			if i == fr.matchIdx {
				b.WriteString(lipgloss.NewStyle().
					Background(surface0).
					Width(width - 6).
					Render(lineStr + preview))
			} else {
				b.WriteString(lineStr + preview)
			}
			if i < end-1 {
				b.WriteString("\n")
			}
		}
	}

	b.WriteString("\n\n")
	hints := "  ↑↓: navigate/history  Enter: jump  Alt+R: regex"
	if fr.mode == 1 {
		hints += "  Tab: switch  Ctrl+A: replace all"
	}
	b.WriteString(DimStyle.Render(hints))

	borderColor := mauve
	if fr.mode == 1 {
		borderColor = green
	}

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(borderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

// highlightRegex highlights all regex matches in the preview string.
func (fr FindReplace) highlightRegex(preview string) string {
	re, err := regexp.Compile("(?i)" + fr.findQuery)
	if err != nil {
		return preview
	}

	locs := re.FindAllStringIndex(preview, -1)
	if len(locs) == 0 {
		return preview
	}

	var result strings.Builder
	prev := 0
	for _, loc := range locs {
		if loc[0] > prev {
			result.WriteString(preview[prev:loc[0]])
		}
		result.WriteString(MatchHighlightStyle.Render(preview[loc[0]:loc[1]]))
		prev = loc[1]
	}
	if prev < len(preview) {
		result.WriteString(preview[prev:])
	}
	return result.String()
}
