package tui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/vault"
)

// ---------------------------------------------------------------------------
// Result types
// ---------------------------------------------------------------------------

type usResultType int

const (
	usResultNote usResultType = iota
	usResultTask
	usResultGoal
	usResultHabit
)

type usResult struct {
	Type     usResultType
	Title    string
	Context  string // snippet / detail
	Icon     string
	NotePath string // navigation: note path
	LineNum  int    // navigation: line number
	GoalID   string // navigation: goal ID
}

// ---------------------------------------------------------------------------
// UniversalSearch overlay
// ---------------------------------------------------------------------------

type UniversalSearch struct {
	active bool
	width  int
	height int

	query   string
	results []usResult
	cursor  int
	scroll  int

	// Source data
	notes  map[string]*vault.Note
	tasks  []Task
	goals  []Goal
	habits []habitEntry

	// Consumed-once navigation
	navResult *usResult
}

func NewUniversalSearch() UniversalSearch {
	return UniversalSearch{}
}

func (us *UniversalSearch) IsActive() bool {
	return us.active
}

func (us *UniversalSearch) SetSize(w, h int) {
	us.width = w
	us.height = h
}

func (us *UniversalSearch) Open(notes map[string]*vault.Note, tasks []Task, goals []Goal, habits []habitEntry) {
	us.active = true
	us.query = ""
	us.results = nil
	us.cursor = 0
	us.scroll = 0
	us.navResult = nil
	us.notes = notes
	us.tasks = tasks
	us.goals = goals
	us.habits = habits
}

func (us *UniversalSearch) Close() {
	us.active = false
}

func (us *UniversalSearch) NavResult() *usResult {
	r := us.navResult
	us.navResult = nil
	return r
}

// ---------------------------------------------------------------------------
// Search logic
// ---------------------------------------------------------------------------

func usFuzzyMatch(str, pattern string) bool {
	str = strings.ToLower(str)
	pattern = strings.ToLower(pattern)
	pi := 0
	for i := 0; i < len(str) && pi < len(pattern); i++ {
		if str[i] == pattern[pi] {
			pi++
		}
	}
	return pi == len(pattern)
}

func usFuzzyScore(str, pattern string) int {
	str = strings.ToLower(str)
	pattern = strings.ToLower(pattern)
	score := 0
	pi := 0
	lastMatch := -1
	for i := 0; i < len(str) && pi < len(pattern); i++ {
		if str[i] == pattern[pi] {
			score += 10
			if i == 0 {
				score += 20 // start of string bonus
			}
			if lastMatch == i-1 {
				score += 5 // consecutive bonus
			}
			lastMatch = i
			pi++
		}
	}
	if pi < len(pattern) {
		return 0
	}
	return score
}

func (us *UniversalSearch) search() {
	us.results = nil
	q := strings.TrimSpace(us.query)
	if q == "" {
		return
	}

	type scored struct {
		result usResult
		score  int
	}
	var all []scored

	// Search notes (title match)
	for path, note := range us.notes {
		name := strings.TrimSuffix(filepath.Base(path), ".md")
		if s := usFuzzyScore(name, q); s > 0 {
			all = append(all, scored{
				result: usResult{
					Type:     usResultNote,
					Title:    name,
					Context:  path,
					Icon:     "\u25C6", // ◆
					NotePath: path,
				},
				score: s + 10, // boost notes
			})
		} else if note != nil && strings.Contains(strings.ToLower(note.Content), strings.ToLower(q)) {
			// Content match — find the matching line
			ctx := ""
			for _, line := range strings.Split(note.Content, "\n") {
				if strings.Contains(strings.ToLower(line), strings.ToLower(q)) {
					ctx = strings.TrimSpace(line)
					if len(ctx) > 60 {
						ctx = ctx[:60] + "..."
					}
					break
				}
			}
			all = append(all, scored{
				result: usResult{
					Type:     usResultNote,
					Title:    name,
					Context:  ctx,
					Icon:     "\u25C6",
					NotePath: path,
				},
				score: 5,
			})
		}
	}

	// Search tasks
	for _, t := range us.tasks {
		clean := tmCleanText(t.Text)
		if s := usFuzzyScore(clean, q); s > 0 {
			status := "[ ]"
			if t.Done {
				status = "[x]"
			}
			all = append(all, scored{
				result: usResult{
					Type:     usResultTask,
					Title:    clean,
					Context:  status + " " + t.NotePath,
					Icon:     "\u2610", // ☐
					NotePath: t.NotePath,
					LineNum:  t.LineNum,
				},
				score: s,
			})
		}
	}

	// Search goals
	for _, g := range us.goals {
		if s := usFuzzyScore(g.Title, q); s > 0 {
			ctx := string(g.Status)
			if g.TargetDate != "" {
				ctx += " — " + g.TimeframeLabel()
			}
			all = append(all, scored{
				result: usResult{
					Type:    usResultGoal,
					Title:   g.Title,
					Context: ctx,
					Icon:    "\u2691", // ⚑
					GoalID:  g.ID,
				},
				score: s,
			})
		}
		// Also search milestones
		for _, ms := range g.Milestones {
			if s := usFuzzyScore(ms.Text, q); s > 0 {
				all = append(all, scored{
					result: usResult{
						Type:    usResultGoal,
						Title:   ms.Text,
						Context: "milestone of " + g.Title,
						Icon:    "\u2691",
						GoalID:  g.ID,
					},
					score: s - 5,
				})
			}
		}
	}

	// Search habits
	for _, h := range us.habits {
		if s := usFuzzyScore(h.Name, q); s > 0 {
			ctx := fmt.Sprintf("%d day streak", h.Streak)
			all = append(all, scored{
				result: usResult{
					Type:    usResultHabit,
					Title:   h.Name,
					Context: ctx,
					Icon:    "\u2605", // ★
				},
				score: s,
			})
		}
	}

	// Sort by score descending, cap at 40
	sort.SliceStable(all, func(i, j int) bool {
		return all[i].score > all[j].score
	})
	if len(all) > 40 {
		all = all[:40]
	}
	for _, s := range all {
		us.results = append(us.results, s.result)
	}
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (us UniversalSearch) Update(msg tea.Msg) (UniversalSearch, tea.Cmd) {
	if !us.active {
		return us, nil
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		key := msg.String()
		switch key {
		case "esc":
			us.Close()
		case "enter":
			if us.cursor >= 0 && us.cursor < len(us.results) {
				r := us.results[us.cursor]
				us.navResult = &r
				us.Close()
			}
		case "up", "ctrl+k":
			if us.cursor > 0 {
				us.cursor--
				if us.cursor < us.scroll {
					us.scroll = us.cursor
				}
			}
		case "down", "ctrl+j":
			if us.cursor < len(us.results)-1 {
				us.cursor++
				maxVis := us.height - 8
				if maxVis < 3 {
					maxVis = 3
				}
				if us.cursor >= us.scroll+maxVis {
					us.scroll = us.cursor - maxVis + 1
				}
			}
		case "backspace":
			if len(us.query) > 0 {
				us.query = us.query[:len(us.query)-1]
				us.cursor = 0
				us.scroll = 0
				us.search()
			}
		default:
			if len(key) == 1 || key == " " {
				us.query += key
				us.cursor = 0
				us.scroll = 0
				us.search()
			}
		}
	}
	return us, nil
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (us *UniversalSearch) View() string {
	if !us.active {
		return ""
	}

	width := us.width * 2 / 3
	if width < 60 {
		width = 60
	}
	if width > 100 {
		width = 100
	}
	innerW := width - 8

	var b strings.Builder

	// Title
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	b.WriteString(titleStyle.Render(IconSearchChar+" Search Everything") + "\n\n")

	// Search input
	promptStyle := lipgloss.NewStyle().Foreground(lavender).Bold(true)
	inputStyle := lipgloss.NewStyle().Foreground(text)
	b.WriteString(promptStyle.Render("  > ") + inputStyle.Render(us.query+"\u2588") + "\n\n")

	// Results
	if us.query == "" {
		b.WriteString("  " + DimStyle.Render("Type to search notes, tasks, goals, and habits") + "\n")
	} else if len(us.results) == 0 {
		b.WriteString("  " + DimStyle.Render("No results found") + "\n")
	} else {
		maxVis := us.height - 8
		if maxVis < 3 {
			maxVis = 3
		}

		typeNames := map[usResultType]string{
			usResultNote:  "NOTES",
			usResultTask:  "TASKS",
			usResultGoal:  "GOALS",
			usResultHabit: "HABITS",
		}
		typeColors := map[usResultType]lipgloss.Color{
			usResultNote:  blue,
			usResultTask:  green,
			usResultGoal:  mauve,
			usResultHabit: yellow,
		}

		lastType := usResultType(-1)
		shown := 0

		for i := us.scroll; i < len(us.results) && shown < maxVis; i++ {
			r := us.results[i]

			// Type header
			if r.Type != lastType {
				b.WriteString("  " + lipgloss.NewStyle().Foreground(typeColors[r.Type]).Bold(true).Render(typeNames[r.Type]) + "\n")
				lastType = r.Type
				shown++
				if shown >= maxVis {
					break
				}
			}

			// Result row
			prefix := "    "
			titleSt := lipgloss.NewStyle().Foreground(text)
			if i == us.cursor {
				prefix = "  " + lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("> ")
				titleSt = titleSt.Bold(true)
			}

			icon := lipgloss.NewStyle().Foreground(typeColors[r.Type]).Render(r.Icon + " ")
			title := titleSt.Render(TruncateDisplay(r.Title, innerW-15))
			ctx := ""
			if r.Context != "" {
				ctx = " " + DimStyle.Render(TruncateDisplay(r.Context, innerW-len(r.Title)-10))
			}

			b.WriteString(prefix + icon + title + ctx + "\n")
			shown++
		}

		// Count
		b.WriteString("\n  " + DimStyle.Render(fmt.Sprintf("%d results", len(us.results))))
	}

	// Help
	b.WriteString("\n  " + DimStyle.Render("Enter:open  Esc:close  ↑/↓:navigate"))

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}
