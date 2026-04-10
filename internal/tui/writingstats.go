package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// dailyStats records vault-wide word counts for a single day.
type dailyStats struct {
	Date        string `json:"date"`        // YYYY-MM-DD
	WordCount   int    `json:"wordCount"`   // total vault words that day
	NotesEdited int    `json:"notesEdited"` // files modified that day
}

// writingStatsData is the on-disk JSON format stored in .granit/writing-stats.json.
type writingStatsData struct {
	Days []dailyStats `json:"days"`
}

// noteWordCount holds per-note statistics for the Notes tab.
type noteWordCount struct {
	Path      string
	Name      string
	WordCount int
	ModTime   time.Time
}

// WritingStats is a TUI overlay that tracks and visualises writing
// productivity across the vault.
type WritingStats struct {
	active    bool
	width     int
	height    int
	vaultRoot string

	tab int // 0=overview, 1=activity, 2=notes

	// Computed stats
	totalWords   int
	totalNotes   int
	avgWordsNote int
	longestNote  string
	longestWords int
	todayWords   int
	streak       int

	// Daily data
	dailyWords []dailyStats

	// Top notes
	topByLength []noteWordCount
	topByRecent []noteWordCount

	scroll int
}

// IsActive returns whether the overlay is visible.
func (ws WritingStats) IsActive() bool {
	return ws.active
}

// SetSize updates the available terminal dimensions.
func (ws *WritingStats) SetSize(w, h int) {
	ws.width = w
	ws.height = h
}

// Open scans the vault, computes stats, saves the daily log and activates the
// overlay.
func (ws *WritingStats) Open(vaultRoot string) {
	ws.active = true
	ws.vaultRoot = vaultRoot
	ws.tab = 0
	ws.scroll = 0
	ws.scan()
}

// scan walks the vault, counts words, and persists the daily log.
func (ws *WritingStats) scan() {
	ws.totalWords = 0
	ws.totalNotes = 0
	ws.longestNote = ""
	ws.longestWords = 0
	ws.todayWords = 0
	ws.topByLength = nil
	ws.topByRecent = nil

	now := time.Now()
	today := now.Format("2006-01-02")
	weekAgo := now.AddDate(0, 0, -7)

	type fileInfo struct {
		relPath   string
		name      string
		wordCount int
		modTime   time.Time
	}

	var files []fileInfo

	// Per-day aggregation: date -> notes edited that day
	dayEdited := make(map[string]int)

	_ = filepath.Walk(ws.vaultRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		name := info.Name()

		// Skip hidden files/dirs and common non-vault dirs.
		if strings.HasPrefix(name, ".") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() && name == "node_modules" {
			return filepath.SkipDir
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(name, ".md") {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}

		content := string(data)
		words := len(strings.Fields(content))

		rel, _ := filepath.Rel(ws.vaultRoot, path)
		if rel == "" {
			rel = name
		}

		fi := fileInfo{
			relPath:   rel,
			name:      strings.TrimSuffix(name, ".md"),
			wordCount: words,
			modTime:   info.ModTime(),
		}
		files = append(files, fi)

		ws.totalWords += words
		ws.totalNotes++

		if words > ws.longestWords {
			ws.longestWords = words
			ws.longestNote = fi.name
		}

		// Track daily edits
		modDay := info.ModTime().Format("2006-01-02")
		dayEdited[modDay]++

		// Today's words: count words from files modified today.
		if modDay == today {
			ws.todayWords += words
		}

		// Week detection (used later if needed, kept simple)
		_ = weekAgo

		return nil
	})

	// Average words per note
	if ws.totalNotes > 0 {
		ws.avgWordsNote = ws.totalWords / ws.totalNotes
	} else {
		ws.avgWordsNote = 0
	}

	// --- Top 10 longest notes ---
	sort.Slice(files, func(i, j int) bool {
		return files[i].wordCount > files[j].wordCount
	})
	limit := 10
	if len(files) < limit {
		limit = len(files)
	}
	for _, f := range files[:limit] {
		ws.topByLength = append(ws.topByLength, noteWordCount{
			Path:      f.relPath,
			Name:      f.name,
			WordCount: f.wordCount,
			ModTime:   f.modTime,
		})
	}

	// --- Top 10 recently modified ---
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.After(files[j].modTime)
	})
	limit = 10
	if len(files) < limit {
		limit = len(files)
	}
	for _, f := range files[:limit] {
		ws.topByRecent = append(ws.topByRecent, noteWordCount{
			Path:      f.relPath,
			Name:      f.name,
			WordCount: f.wordCount,
			ModTime:   f.modTime,
		})
	}

	// --- Daily log persistence ---
	ws.loadDailyLog()

	// Upsert today's entry.
	found := false
	for i := range ws.dailyWords {
		if ws.dailyWords[i].Date == today {
			ws.dailyWords[i].WordCount = ws.totalWords
			ws.dailyWords[i].NotesEdited = dayEdited[today]
			found = true
			break
		}
	}
	if !found {
		ws.dailyWords = append(ws.dailyWords, dailyStats{
			Date:        today,
			WordCount:   ws.totalWords,
			NotesEdited: dayEdited[today],
		})
	}

	// Sort daily log chronologically.
	sort.Slice(ws.dailyWords, func(i, j int) bool {
		return ws.dailyWords[i].Date < ws.dailyWords[j].Date
	})

	// Keep at most 90 days of history.
	if len(ws.dailyWords) > 90 {
		ws.dailyWords = ws.dailyWords[len(ws.dailyWords)-90:]
	}

	ws.saveDailyLog()

	// --- Writing streak ---
	ws.streak = ws.computeStreak(dayEdited, now)
}

// computeStreak returns the number of consecutive days (ending today or
// yesterday) that have at least one edited note.
func (ws *WritingStats) computeStreak(dayEdited map[string]int, now time.Time) int {
	streak := 0
	day := now

	// Allow streak to start from today or yesterday (in case no edits yet today).
	if dayEdited[day.Format("2006-01-02")] == 0 {
		day = day.AddDate(0, 0, -1)
	}

	for {
		key := day.Format("2006-01-02")
		if dayEdited[key] > 0 {
			streak++
			day = day.AddDate(0, 0, -1)
		} else {
			break
		}
	}
	return streak
}

// statsFilePath returns the path to the JSON log file.
func (ws *WritingStats) statsFilePath() string {
	return filepath.Join(ws.vaultRoot, ".granit", "writing-stats.json")
}

// loadDailyLog reads the persisted daily log from disk.
func (ws *WritingStats) loadDailyLog() {
	ws.dailyWords = nil
	data, err := os.ReadFile(ws.statsFilePath())
	if err != nil {
		return
	}
	var sd writingStatsData
	if err := json.Unmarshal(data, &sd); err != nil {
		return
	}
	ws.dailyWords = sd.Days
}

// saveDailyLog writes the daily log to disk, creating the .granit directory
// if necessary.
func (ws *WritingStats) saveDailyLog() {
	dir := filepath.Join(ws.vaultRoot, ".granit")
	_ = os.MkdirAll(dir, 0755)

	sd := writingStatsData{Days: ws.dailyWords}
	data, err := json.MarshalIndent(sd, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(ws.statsFilePath(), data, 0o600)
}

// Update handles key messages while the overlay is active.
func (ws WritingStats) Update(msg tea.Msg) (WritingStats, tea.Cmd) {
	if !ws.active {
		return ws, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			ws.active = false
		case "1":
			ws.tab = 0
			ws.scroll = 0
		case "2":
			ws.tab = 1
			ws.scroll = 0
		case "3":
			ws.tab = 2
			ws.scroll = 0
		case "tab":
			ws.tab = (ws.tab + 1) % 3
			ws.scroll = 0
		case "j", "down":
			ws.scroll++
		case "k", "up":
			if ws.scroll > 0 {
				ws.scroll--
			}
		}
	}
	return ws, nil
}

// View renders the overlay.
func (ws WritingStats) View() string {
	width := ws.width * 2 / 3
	if width < 60 {
		width = 60
	}
	if width > 90 {
		width = 90
	}

	var b strings.Builder

	// --- Tab bar ---
	activeTabStyle := lipgloss.NewStyle().
		Foreground(crust).
		Background(mauve).
		Bold(true).
		Padding(0, 1)

	inactiveTabStyle := lipgloss.NewStyle().
		Foreground(overlay0).
		Background(surface0).
		Padding(0, 1)

	tabs := []string{"Overview", "Activity", "Notes"}
	for i, label := range tabs {
		if i == ws.tab {
			b.WriteString(activeTabStyle.Render(" " + label + " "))
		} else {
			b.WriteString(inactiveTabStyle.Render(" " + label + " "))
		}
		if i < len(tabs)-1 {
			b.WriteString(" ")
		}
	}
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", width-6)))
	b.WriteString("\n\n")

	// Build content lines depending on active tab.
	var lines []string
	switch ws.tab {
	case 0:
		lines = ws.viewOverview(width)
	case 1:
		lines = ws.viewActivity(width)
	case 2:
		lines = ws.viewNotes(width)
	}

	// Scrollable region.
	visH := ws.height - 10
	if visH < 8 {
		visH = 8
	}
	maxScroll := len(lines) - visH
	if maxScroll < 0 {
		maxScroll = 0
	}
	if ws.scroll > maxScroll {
		ws.scroll = maxScroll
	}
	end := ws.scroll + visH
	if end > len(lines) {
		end = len(lines)
	}

	for i := ws.scroll; i < end; i++ {
		b.WriteString(lines[i])
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	// Footer
	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", width-6)))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  1/2/3: tabs  Tab: next  j/k: scroll  Esc: close"))

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

// ---------------------------------------------------------------------------
// Tab renderers
// ---------------------------------------------------------------------------

func (ws WritingStats) viewOverview(width int) []string {
	sectionStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(text)
	numStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)

	var lines []string

	lines = append(lines, sectionStyle.Render("  Overview"))
	lines = append(lines, DimStyle.Render("  "+strings.Repeat("─", 30)))
	lines = append(lines, "")

	lines = append(lines, labelStyle.Render("  Total Notes:       ")+numStyle.Render(formatNum(ws.totalNotes)))
	lines = append(lines, labelStyle.Render("  Total Words:       ")+numStyle.Render(formatNum(ws.totalWords)))
	lines = append(lines, labelStyle.Render("  Avg Words/Note:    ")+numStyle.Render(formatNum(ws.avgWordsNote)))
	lines = append(lines, "")

	longestDisplay := TruncateDisplay(ws.longestNote, 30)
	lines = append(lines, labelStyle.Render("  Longest Note:      ")+lipgloss.NewStyle().Foreground(blue).Bold(true).Render(longestDisplay))
	lines = append(lines, labelStyle.Render("                     ")+DimStyle.Render(formatNum(ws.longestWords)+" words"))
	lines = append(lines, "")

	lines = append(lines, sectionStyle.Render("  Today"))
	lines = append(lines, DimStyle.Render("  "+strings.Repeat("─", 30)))
	lines = append(lines, "")
	lines = append(lines, labelStyle.Render("  Words Today:       ")+numStyle.Render(formatNum(ws.todayWords)))

	streakColor := green
	if ws.streak == 0 {
		streakColor = red
	}
	streakStyle := lipgloss.NewStyle().Foreground(streakColor).Bold(true)
	lines = append(lines, labelStyle.Render("  Writing Streak:    ")+streakStyle.Render(fmt.Sprintf("%d day", ws.streak))+streakStyle.Render(wsPlural(ws.streak)))

	lines = append(lines, "")

	// Mini sparkline from last 7 days of daily log
	if len(ws.dailyWords) > 1 {
		lines = append(lines, sectionStyle.Render("  Last 7 Days"))
		lines = append(lines, DimStyle.Render("  "+strings.Repeat("─", 30)))
		lines = append(lines, "")

		last7 := ws.lastNDays(7)
		// Compute daily deltas (word count change day to day).
		for _, d := range last7 {
			dayLabel := d.Date[5:] // MM-DD
			edited := d.NotesEdited
			marker := lipgloss.NewStyle().Foreground(green).Render("  " + dayLabel + "  ")
			detail := DimStyle.Render(fmt.Sprintf("%d notes edited", edited))
			lines = append(lines, marker+detail)
		}
	}

	return lines
}

func (ws WritingStats) viewActivity(width int) []string {
	sectionStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(text)
	numStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
	barFillStyle := lipgloss.NewStyle().Foreground(mauve)

	var lines []string

	lines = append(lines, sectionStyle.Render("  14-Day Activity"))
	lines = append(lines, DimStyle.Render("  "+strings.Repeat("─", 30)))
	lines = append(lines, "")

	last14 := ws.lastNDays(14)

	if len(last14) == 0 {
		lines = append(lines, DimStyle.Render("  No activity data yet."))
		lines = append(lines, DimStyle.Render("  Open this overlay again after editing notes."))
		return lines
	}

	// Compute per-day "words written" as notes edited (since we track total
	// words, the delta between consecutive days is the net change — but that
	// can be negative on deletions). For the chart we use NotesEdited as the
	// activity metric.
	maxEdited := 0
	for _, d := range last14 {
		if d.NotesEdited > maxEdited {
			maxEdited = d.NotesEdited
		}
	}

	barMax := width - 30
	if barMax < 10 {
		barMax = 10
	}
	if barMax > 40 {
		barMax = 40
	}

	for _, d := range last14 {
		dayLabel := d.Date[5:] // MM-DD
		barLen := 0
		if maxEdited > 0 {
			barLen = d.NotesEdited * barMax / maxEdited
		}
		if barLen < 1 && d.NotesEdited > 0 {
			barLen = 1
		}
		empty := barMax - barLen
		bar := barFillStyle.Render(strings.Repeat("█", barLen)) + DimStyle.Render(strings.Repeat("░", empty))
		count := numStyle.Render(fmt.Sprintf(" %d", d.NotesEdited))
		lines = append(lines, "  "+labelStyle.Render(dayLabel)+"  "+bar+count)
	}

	lines = append(lines, "")

	// Most productive day
	lines = append(lines, sectionStyle.Render("  Highlights"))
	lines = append(lines, DimStyle.Render("  "+strings.Repeat("─", 30)))
	lines = append(lines, "")

	bestDay := ws.mostProductiveDay()
	if bestDay.Date != "" {
		lines = append(lines, labelStyle.Render("  Most Productive Day:  ")+numStyle.Render(bestDay.Date))
		lines = append(lines, labelStyle.Render("                        ")+DimStyle.Render(fmt.Sprintf("%d notes edited", bestDay.NotesEdited)))
	}

	lines = append(lines, "")

	// Totals from the 14-day window
	totalEdited := 0
	activeDays := 0
	for _, d := range last14 {
		totalEdited += d.NotesEdited
		if d.NotesEdited > 0 {
			activeDays++
		}
	}
	lines = append(lines, labelStyle.Render("  Active Days (14d):    ")+numStyle.Render(fmt.Sprintf("%d / %d", activeDays, len(last14))))
	lines = append(lines, labelStyle.Render("  Notes Edited (14d):   ")+numStyle.Render(formatNum(totalEdited)))

	return lines
}

func (ws WritingStats) viewNotes(width int) []string {
	sectionStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(text)
	numStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
	barFillStyle := lipgloss.NewStyle().Foreground(teal)

	var lines []string

	// --- Top 10 longest ---
	lines = append(lines, sectionStyle.Render("  Top 10 Longest Notes"))
	lines = append(lines, DimStyle.Render("  "+strings.Repeat("─", 30)))
	lines = append(lines, "")

	if len(ws.topByLength) == 0 {
		lines = append(lines, DimStyle.Render("  No notes found."))
	} else {
		maxWords := ws.topByLength[0].WordCount
		barMax := 20

		for i, n := range ws.topByLength {
			name := n.Name
			if len(name) > 22 {
				name = name[:19] + "..."
			}

			barLen := 0
			if maxWords > 0 {
				barLen = n.WordCount * barMax / maxWords
			}
			if barLen < 1 && n.WordCount > 0 {
				barLen = 1
			}
			empty := barMax - barLen
			bar := barFillStyle.Render(strings.Repeat("█", barLen)) + DimStyle.Render(strings.Repeat("░", empty))
			rank := DimStyle.Render(fmt.Sprintf("%2d. ", i+1))
			lines = append(lines, "  "+rank+labelStyle.Render(wsPadRight(name, 23))+bar+" "+numStyle.Render(formatNum(n.WordCount)+" w"))
		}
	}

	lines = append(lines, "")

	// --- Top 10 recently modified ---
	lines = append(lines, sectionStyle.Render("  Top 10 Recently Modified"))
	lines = append(lines, DimStyle.Render("  "+strings.Repeat("─", 30)))
	lines = append(lines, "")

	if len(ws.topByRecent) == 0 {
		lines = append(lines, DimStyle.Render("  No notes found."))
	} else {
		for i, n := range ws.topByRecent {
			name := n.Name
			if len(name) > 25 {
				name = name[:22] + "..."
			}

			age := wsTimeAgo(n.ModTime)
			rank := DimStyle.Render(fmt.Sprintf("%2d. ", i+1))
			nameStr := labelStyle.Render(wsPadRight(name, 27))
			wordStr := numStyle.Render(formatNum(n.WordCount) + " w")
			ageStr := DimStyle.Render("  " + age)
			lines = append(lines, "  "+rank+nameStr+wordStr+ageStr)
		}
	}

	return lines
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// lastNDays returns the last n daily entries, filling gaps with zero values
// so the chart always shows a continuous range.
func (ws WritingStats) lastNDays(n int) []dailyStats {
	now := time.Now()

	// Build a map from existing data for fast lookup.
	dayMap := make(map[string]dailyStats, len(ws.dailyWords))
	for _, d := range ws.dailyWords {
		dayMap[d.Date] = d
	}

	result := make([]dailyStats, 0, n)
	for i := n - 1; i >= 0; i-- {
		day := now.AddDate(0, 0, -i)
		key := day.Format("2006-01-02")
		if d, ok := dayMap[key]; ok {
			result = append(result, d)
		} else {
			result = append(result, dailyStats{Date: key})
		}
	}
	return result
}

// mostProductiveDay returns the daily entry with the highest NotesEdited
// across the entire log.
func (ws WritingStats) mostProductiveDay() dailyStats {
	var best dailyStats
	for _, d := range ws.dailyWords {
		if d.NotesEdited > best.NotesEdited {
			best = d
		}
	}
	return best
}

// wsPlural returns "s" when count != 1.
func wsPlural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// wsPadRight pads a string with spaces to the given width.
func wsPadRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

// wsTimeAgo returns a human-readable relative time string.
func wsTimeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		return fmt.Sprintf("%dm ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		return fmt.Sprintf("%dh ago", h)
	case d < 48*time.Hour:
		return "yesterday"
	default:
		days := int(d.Hours() / 24)
		return fmt.Sprintf("%dd ago", days)
	}
}
