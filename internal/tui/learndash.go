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

// ---------------------------------------------------------------------------
// Data types
// ---------------------------------------------------------------------------

// QuizScore records a single quiz attempt.
type QuizScore struct {
	Date    string `json:"date"`
	Correct int    `json:"correct"`
	Total   int    `json:"total"`
	Source  string `json:"source"`
}

// LearnStats holds persistent learning-progress data.
type LearnStats struct {
	TotalReviews   int            `json:"total_reviews"`
	StreakDays      int            `json:"streak_days"`
	LastReviewDate  string         `json:"last_review_date"`
	DailyReviews   map[string]int `json:"daily_reviews"`
	QuizScores     []QuizScore    `json:"quiz_scores"`
}

// ---------------------------------------------------------------------------
// Card-level statistics (set at runtime, not persisted)
// ---------------------------------------------------------------------------

type cardStats struct {
	total    int
	due      int
	newCards int
	mastered int
}

// ---------------------------------------------------------------------------
// Topic stats entry
// ---------------------------------------------------------------------------

type topicEntry struct {
	name      string
	count     int
	masteryPc int
}

// ---------------------------------------------------------------------------
// LearnDashboard overlay
// ---------------------------------------------------------------------------

// LearnDashboard is an overlay panel that tracks study progress with
// flashcard-review statistics, quiz history, and weekly activity.
type LearnDashboard struct {
	OverlayBase
	scroll int

	stats LearnStats
	cards cardStats

	// Mastery breakdown buckets (set externally via SetMasteryBreakdown)
	masteryNew      int
	masteryLearning int
	masteryReview   int
	masteryMastered int

	// Topic stats (set externally via SetTopicStats)
	topics []topicEntry

	vaultPath string
}

// NewLearnDashboard creates a new dashboard bound to the given vault.
func NewLearnDashboard(vaultPath string) LearnDashboard {
	ld := LearnDashboard{
		vaultPath: vaultPath,
	}
	ld.stats = LoadStats(vaultPath)
	return ld
}

// ---------------------------------------------------------------------------
// Overlay interface
// ---------------------------------------------------------------------------

// Open activates the overlay and reloads stats from disk.
func (ld *LearnDashboard) Open() {
	ld.Activate()
	ld.scroll = 0
	ld.stats = LoadStats(ld.vaultPath)
}

// ---------------------------------------------------------------------------
// Data mutation helpers
// ---------------------------------------------------------------------------

// RecordReview increments the daily review count for date and updates the
// streak.  date should be formatted as "2006-01-02".
func (ld *LearnDashboard) RecordReview(date string) {
	if ld.stats.DailyReviews == nil {
		ld.stats.DailyReviews = make(map[string]int)
	}
	ld.stats.DailyReviews[date]++
	ld.stats.TotalReviews++

	// Update streak
	if ld.stats.LastReviewDate == "" {
		ld.stats.StreakDays = 1
	} else {
		last, err1 := time.Parse("2006-01-02", ld.stats.LastReviewDate)
		cur, err2 := time.Parse("2006-01-02", date)
		if err1 == nil && err2 == nil {
			diff := int(cur.Sub(last).Hours() / 24)
			switch {
			case diff == 1:
				ld.stats.StreakDays++
			case diff > 1:
				ld.stats.StreakDays = 1
			}
			// diff == 0 → same day, streak unchanged
		}
	}
	ld.stats.LastReviewDate = date
	SaveStats(ld.vaultPath, ld.stats)
}

// RecordQuiz appends a quiz score to history (kept at most 20 entries).
func (ld *LearnDashboard) RecordQuiz(score QuizScore) {
	ld.stats.QuizScores = append(ld.stats.QuizScores, score)
	if len(ld.stats.QuizScores) > 20 {
		ld.stats.QuizScores = ld.stats.QuizScores[len(ld.stats.QuizScores)-20:]
	}
	SaveStats(ld.vaultPath, ld.stats)
}

// SetCardStats updates the card overview numbers.
func (ld *LearnDashboard) SetCardStats(total, due, newCards, mastered int) {
	ld.cards = cardStats{
		total:    total,
		due:      due,
		newCards: newCards,
		mastered: mastered,
	}
}

// SetMasteryBreakdown sets the mastery bucket counts.
func (ld *LearnDashboard) SetMasteryBreakdown(newC, learning, review, mastered int) {
	ld.masteryNew = newC
	ld.masteryLearning = learning
	ld.masteryReview = review
	ld.masteryMastered = mastered
}

// SetTopicStats sets per-topic card counts and mastery percentages.
func (ld *LearnDashboard) SetTopicStats(topics []topicEntry) {
	ld.topics = topics
}

// ---------------------------------------------------------------------------
// Persistence
// ---------------------------------------------------------------------------

func statsPath(vaultPath string) string {
	return filepath.Join(vaultPath, ".granit", "learnstats.json")
}

// SaveStats writes the learn stats to disk as JSON.
func SaveStats(vaultPath string, stats LearnStats) {
	p := statsPath(vaultPath)
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	data, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return
	}
	_ = atomicWriteState(p, data)
}

// LoadStats reads learn stats from disk, returning zero-value on error.
func LoadStats(vaultPath string) LearnStats {
	data, err := os.ReadFile(statsPath(vaultPath))
	if err != nil {
		return LearnStats{DailyReviews: make(map[string]int)}
	}
	var s LearnStats
	if err := json.Unmarshal(data, &s); err != nil {
		return LearnStats{DailyReviews: make(map[string]int)}
	}
	if s.DailyReviews == nil {
		s.DailyReviews = make(map[string]int)
	}
	return s
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

// Update handles keyboard input for the overlay.
func (ld LearnDashboard) Update(msg tea.Msg) (LearnDashboard, tea.Cmd) {
	if !ld.active {
		return ld, nil
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			ld.active = false
		case "up", "k":
			if ld.scroll > 0 {
				ld.scroll--
			}
		case "down", "j":
			ld.scroll++
		}
	}
	return ld, nil
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

// View renders the learning dashboard overlay.
func (ld LearnDashboard) View() string {
	width := ld.width * 2 / 3
	if width < 60 {
		width = 60
	}
	if width > 90 {
		width = 90
	}

	sectionStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(text)
	numStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
	barFgStyle := lipgloss.NewStyle().Foreground(mauve)
	flameStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
	greenStyle := lipgloss.NewStyle().Foreground(green)
	yellowStyle := lipgloss.NewStyle().Foreground(yellow)
	redStyle := lipgloss.NewStyle().Foreground(red)

	var lines []string

	// ── 1. Study Streak ────────────────────────────────────
	lines = append(lines, sectionStyle.Render("  Study Streak"))
	lines = append(lines, DimStyle.Render("  "+strings.Repeat("─", 30)))
	streakText := fmt.Sprintf("%d day", ld.stats.StreakDays)
	if ld.stats.StreakDays != 1 {
		streakText += "s"
	}
	lines = append(lines, "  "+flameStyle.Render("🔥 ")+numStyle.Render(streakText))
	if ld.stats.LastReviewDate != "" {
		lines = append(lines, "  "+DimStyle.Render("Last review: "+ld.stats.LastReviewDate))
	}
	lines = append(lines, "  "+labelStyle.Render("Total reviews: ")+numStyle.Render(fmt.Sprintf("%d", ld.stats.TotalReviews)))
	lines = append(lines, "")

	// ── 2. Cards Overview ──────────────────────────────────
	lines = append(lines, sectionStyle.Render("  Cards Overview"))
	lines = append(lines, DimStyle.Render("  "+strings.Repeat("─", 30)))
	learning := ld.cards.total - ld.cards.newCards - ld.cards.mastered
	if learning < 0 {
		learning = 0
	}
	lines = append(lines, labelStyle.Render("  Total:     ")+numStyle.Render(fmt.Sprintf("%d", ld.cards.total)))
	lines = append(lines, labelStyle.Render("  Due today: ")+yellowStyle.Render(fmt.Sprintf("%d", ld.cards.due)))
	lines = append(lines, labelStyle.Render("  New:       ")+greenStyle.Render(fmt.Sprintf("%d", ld.cards.newCards)))
	lines = append(lines, labelStyle.Render("  Learning:  ")+numStyle.Render(fmt.Sprintf("%d", learning)))
	lines = append(lines, labelStyle.Render("  Mastered:  ")+greenStyle.Render(fmt.Sprintf("%d", ld.cards.mastered)))
	lines = append(lines, "")

	// ── 3. Weekly Activity ─────────────────────────────────
	lines = append(lines, sectionStyle.Render("  Weekly Activity"))
	lines = append(lines, DimStyle.Render("  "+strings.Repeat("─", 30)))
	lines = append(lines, ld.renderWeeklyActivity(barFgStyle, numStyle, labelStyle)...)
	lines = append(lines, "")

	// ── 4. Mastery Breakdown ───────────────────────────────
	lines = append(lines, sectionStyle.Render("  Mastery Breakdown"))
	lines = append(lines, DimStyle.Render("  "+strings.Repeat("─", 30)))
	lines = append(lines, ld.renderMasteryBreakdown(width)...)
	lines = append(lines, "")

	// ── 5. Topic Stats ─────────────────────────────────────
	if len(ld.topics) > 0 {
		lines = append(lines, sectionStyle.Render("  Topic Stats"))
		lines = append(lines, DimStyle.Render("  "+strings.Repeat("─", 30)))
		for _, t := range ld.topics {
			name := t.name
			if len(name) > 20 {
				name = name[:17] + "..."
			}
			pctBar := ld.percentBar(t.masteryPc, 15)
			lines = append(lines,
				"  "+labelStyle.Render(padRight(name, 22))+
					pctBar+" "+
					numStyle.Render(fmt.Sprintf("%d cards  %d%%", t.count, t.masteryPc)))
		}
		lines = append(lines, "")
	}

	// ── 6. Quiz History ────────────────────────────────────
	quizScores := ld.stats.QuizScores
	if len(quizScores) > 5 {
		quizScores = quizScores[len(quizScores)-5:]
	}
	if len(quizScores) > 0 {
		lines = append(lines, sectionStyle.Render("  Quiz History"))
		lines = append(lines, DimStyle.Render("  "+strings.Repeat("─", 30)))
		for _, qs := range quizScores {
			pct := 0
			if qs.Total > 0 {
				pct = qs.Correct * 100 / qs.Total
			}
			pctStr := fmt.Sprintf("%d/%d (%d%%)", qs.Correct, qs.Total, pct)
			var coloredPct string
			switch {
			case pct >= 80:
				coloredPct = greenStyle.Render(pctStr)
			case pct >= 50:
				coloredPct = yellowStyle.Render(pctStr)
			default:
				coloredPct = redStyle.Render(pctStr)
			}
			src := qs.Source
			if len(src) > 18 {
				src = src[:15] + "..."
			}
			lines = append(lines, "  "+DimStyle.Render(qs.Date)+" "+labelStyle.Render(padRight(src, 20))+" "+coloredPct)
		}
		lines = append(lines, "")
	}

	// ── Assemble with scroll ───────────────────────────────
	var b strings.Builder

	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  Learning Dashboard")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", width-6)))
	b.WriteString("\n")

	visH := ld.height - 8
	if visH < 10 {
		visH = 10
	}
	maxScroll := len(lines) - visH
	if maxScroll < 0 {
		maxScroll = 0
	}
	if ld.scroll > maxScroll {
		ld.scroll = maxScroll
	}
	end := ld.scroll + visH
	if end > len(lines) {
		end = len(lines)
	}
	for i := ld.scroll; i < end; i++ {
		b.WriteString(lines[i])
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render("  j/k: scroll  Esc: close"))

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

// ---------------------------------------------------------------------------
// Internal rendering helpers
// ---------------------------------------------------------------------------

// renderWeeklyActivity builds a 7-day bar chart using Unicode block chars.
func (ld LearnDashboard) renderWeeklyActivity(barStyle, numStyle, labelStyle lipgloss.Style) []string {
	blocks := []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

	today := time.Now()
	days := make([]string, 7)
	counts := make([]int, 7)
	maxCount := 0
	for i := 6; i >= 0; i-- {
		d := today.AddDate(0, 0, -(6 - i))
		key := d.Format("2006-01-02")
		days[i] = d.Format("Mon")
		counts[i] = ld.stats.DailyReviews[key]
		if counts[i] > maxCount {
			maxCount = counts[i]
		}
	}

	var out []string
	// Bar row
	barRow := "  "
	for _, c := range counts {
		idx := 0
		if maxCount > 0 && c > 0 {
			idx = c * (len(blocks) - 1) / maxCount
		}
		barRow += barStyle.Render(string(blocks[idx])) + " "
	}
	out = append(out, barRow)

	// Labels row
	labelRow := "  "
	for _, d := range days {
		labelRow += DimStyle.Render(padRight(d[:2], 2)) + " "
	}
	out = append(out, labelRow)

	// Counts row
	countRow := "  "
	for _, c := range counts {
		countRow += numStyle.Render(padRight(fmt.Sprintf("%d", c), 2)) + " "
	}
	out = append(out, countRow)

	return out
}

// renderMasteryBreakdown renders percentage bars for mastery buckets.
func (ld LearnDashboard) renderMasteryBreakdown(width int) []string {
	total := ld.masteryNew + ld.masteryLearning + ld.masteryReview + ld.masteryMastered
	if total == 0 {
		return []string{DimStyle.Render("  No cards to analyze")}
	}

	pctNew := ld.masteryNew * 100 / total
	pctLearning := ld.masteryLearning * 100 / total
	pctReview := ld.masteryReview * 100 / total
	pctMastered := ld.masteryMastered * 100 / total

	barWidth := 20
	type bucket struct {
		label string
		pct   int
		count int
		color lipgloss.Color
	}
	buckets := []bucket{
		{"New (0 reps)   ", pctNew, ld.masteryNew, blue},
		{"Learning (1-3) ", pctLearning, ld.masteryLearning, yellow},
		{"Review (4-10)  ", pctReview, ld.masteryReview, peach},
		{"Mastered (11+) ", pctMastered, ld.masteryMastered, green},
	}
	_ = width

	var out []string
	for _, bk := range buckets {
		filled := bk.pct * barWidth / 100
		if filled < 0 {
			filled = 0
		}
		if filled > barWidth {
			filled = barWidth
		}
		empty := barWidth - filled
		bar := lipgloss.NewStyle().Foreground(bk.color).Render(strings.Repeat("█", filled)) +
			DimStyle.Render(strings.Repeat("░", empty))
		label := NormalItemStyle.Render(bk.label)
		pctStr := lipgloss.NewStyle().Foreground(bk.color).Bold(true).Render(fmt.Sprintf("%3d%% (%d)", bk.pct, bk.count))
		out = append(out, "  "+label+bar+" "+pctStr)
	}
	return out
}

// percentBar renders a small percentage bar of the given width.
func (ld LearnDashboard) percentBar(pct, width int) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := pct * width / 100
	empty := width - filled
	return lipgloss.NewStyle().Foreground(green).Render(strings.Repeat("█", filled)) +
		DimStyle.Render(strings.Repeat("░", empty))
}

// ---------------------------------------------------------------------------
// Sorting helper for topics (exported for callers that build topic lists)
// ---------------------------------------------------------------------------

// SortTopicsByMastery sorts topic entries descending by mastery percentage.
func SortTopicsByMastery(topics []topicEntry) {
	sort.Slice(topics, func(i, j int) bool {
		return topics[i].masteryPc > topics[j].masteryPc
	})
}
