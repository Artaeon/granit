package tui

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Flashcard data model
// ---------------------------------------------------------------------------

type Flashcard struct {
	ID         string    `json:"id"`
	Question   string    `json:"question"`
	Answer     string    `json:"answer"`
	Source     string    `json:"source"`
	Due        time.Time `json:"due"`
	Interval   float64   `json:"interval"`
	EaseFactor float64   `json:"ease_factor"`
	Reps       int       `json:"reps"`
	Lapses     int       `json:"lapses"`
}

type flashcardProgress struct {
	Cards       map[string]*Flashcard `json:"cards"`
	TotalReviews int                  `json:"total_reviews"`
	StreakDays   int                  `json:"streak_days"`
	LastReview   time.Time            `json:"last_review"`
}

// ---------------------------------------------------------------------------
// SM-2 spaced repetition algorithm
// ---------------------------------------------------------------------------

func ReviewCard(card *Flashcard, quality int) {
	if quality < 0 {
		quality = 0
	}
	if quality > 5 {
		quality = 5
	}

	// Update ease factor: EF = EF + (0.1 - (5-q) * (0.08 + (5-q) * 0.02))
	q := float64(quality)
	card.EaseFactor += 0.1 - (5-q)*(0.08+(5-q)*0.02)
	if card.EaseFactor < 1.3 {
		card.EaseFactor = 1.3
	}

	switch {
	case quality <= 2:
		// Failed — reset
		card.Interval = 1
		card.Lapses++
		card.Reps = 0
	case quality == 3:
		// Hard
		card.Reps++
		if card.Interval < 1 {
			card.Interval = 1
		}
		card.Interval *= 1.2
	case quality == 4:
		// Good
		card.Reps++
		if card.Interval < 1 {
			card.Interval = 1
		}
		card.Interval *= card.EaseFactor
	case quality == 5:
		// Easy
		card.Reps++
		if card.Interval < 1 {
			card.Interval = 1
		}
		card.Interval *= card.EaseFactor * 1.3
	}

	card.Due = time.Now().Add(time.Duration(card.Interval*24) * time.Hour)
}

// ---------------------------------------------------------------------------
// Card extraction from note content
// ---------------------------------------------------------------------------

func cardID(question, source string) string {
	h := sha256.Sum256([]byte(question + "|" + source))
	return hex.EncodeToString(h[:16])
}

func ExtractCards(notePath string, content string) []Flashcard {
	var cards []Flashcard
	lines := strings.Split(content, "\n")

	// Pass 1: explicit Q: / A: pairs
	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "Q:") || strings.HasPrefix(trimmed, "q:") {
			question := strings.TrimSpace(trimmed[2:])
			answer := ""
			if i+1 < len(lines) {
				next := strings.TrimSpace(lines[i+1])
				if strings.HasPrefix(next, "A:") || strings.HasPrefix(next, "a:") {
					answer = strings.TrimSpace(next[2:])
					i++ // skip the answer line
				}
			}
			if question != "" && answer != "" {
				cards = append(cards, Flashcard{
					ID:         cardID(question, notePath),
					Question:   question,
					Answer:     answer,
					Source:     notePath,
					Due:        time.Now(),
					Interval:   0,
					EaseFactor: 2.5,
				})
			}
		}
	}

	// Pass 2: ## heading with content below (heading = question, content = answer)
	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "## ") {
			heading := strings.TrimSpace(trimmed[3:])
			if heading == "" {
				continue
			}
			var bodyLines []string
			for j := i + 1; j < len(lines); j++ {
				jTrimmed := strings.TrimSpace(lines[j])
				// Stop at next heading of same or higher level
				if strings.HasPrefix(jTrimmed, "## ") || strings.HasPrefix(jTrimmed, "# ") {
					break
				}
				bodyLines = append(bodyLines, lines[j])
			}
			body := strings.TrimSpace(strings.Join(bodyLines, "\n"))
			if body != "" {
				cards = append(cards, Flashcard{
					ID:         cardID(heading, notePath),
					Question:   heading,
					Answer:     body,
					Source:     notePath,
					Due:        time.Now(),
					Interval:   0,
					EaseFactor: 2.5,
				})
			}
		}
	}

	// Pass 3: definition lists — `- term :: definition`
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") && strings.Contains(trimmed, " :: ") {
			parts := strings.SplitN(trimmed[2:], " :: ", 2)
			if len(parts) == 2 {
				term := strings.TrimSpace(parts[0])
				def := strings.TrimSpace(parts[1])
				if term != "" && def != "" {
					cards = append(cards, Flashcard{
						ID:         cardID(term, notePath),
						Question:   term,
						Answer:     def,
						Source:     notePath,
						Due:        time.Now(),
						Interval:   0,
						EaseFactor: 2.5,
					})
				}
			}
		}
	}

	// Pass 4: cloze deletions — {{c1::answer}}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "{{c") && strings.Contains(trimmed, "::") && strings.Contains(trimmed, "}}") {
			remaining := trimmed
			for strings.Contains(remaining, "{{c") {
				start := strings.Index(remaining, "{{c")
				if start == -1 {
					break
				}
				// Find the matching }}
				end := strings.Index(remaining[start:], "}}")
				if end == -1 {
					break
				}
				end += start

				clozeContent := remaining[start : end+2]
				// Parse out the answer: {{c1::answer}} or {{c1::answer::hint}}
				inner := clozeContent[2 : len(clozeContent)-2] // strip {{ }}
				colonIdx := strings.Index(inner, "::")
				if colonIdx == -1 {
					remaining = remaining[end+2:]
					continue
				}
				answer := inner[colonIdx+2:]
				// If there's a hint part, strip it
				if hintIdx := strings.Index(answer, "::"); hintIdx != -1 {
					answer = answer[:hintIdx]
				}

				// Build question: replace cloze with blank
				question := strings.Replace(trimmed, clozeContent, "[...]", 1)

				if answer != "" {
					cards = append(cards, Flashcard{
						ID:         cardID(question, notePath),
						Question:   question,
						Answer:     answer,
						Source:     notePath,
						Due:        time.Now(),
						Interval:   0,
						EaseFactor: 2.5,
					})
				}

				remaining = remaining[end+2:]
			}
		}
	}

	return cards
}

func ExtractAllCards(notes map[string]string) []Flashcard {
	var all []Flashcard
	for path, content := range notes {
		all = append(all, ExtractCards(path, content)...)
	}
	return all
}

// ---------------------------------------------------------------------------
// Flashcards overlay — Bubble Tea component
// ---------------------------------------------------------------------------

const (
	fcModeDeck   = 0
	fcModeReview = 1
	fcModeStats  = 2
)

type Flashcards struct {
	active    bool
	width     int
	height    int
	mode      int
	vaultPath string

	// All known cards (extracted + persisted)
	cards    []*Flashcard
	progress flashcardProgress

	// Review session
	reviewQueue  []*Flashcard
	reviewIdx    int
	showAnswer   bool
	sessionDone  int
	sessionTotal int

	// Stats
	scroll int

	// Status feedback
	statusMsg string
}

func NewFlashcards(vaultPath string) Flashcards {
	fc := Flashcards{
		vaultPath: vaultPath,
		progress: flashcardProgress{
			Cards: make(map[string]*Flashcard),
		},
	}
	fc.LoadProgress(vaultPath)
	return fc
}

func (fc *Flashcards) IsActive() bool {
	return fc.active
}

func (fc *Flashcards) Open() {
	fc.active = true
	fc.mode = fcModeDeck
	fc.showAnswer = false
	fc.scroll = 0
}

func (fc *Flashcards) Close() {
	fc.active = false
}

func (fc *Flashcards) SetSize(width, height int) {
	fc.width = width
	fc.height = height
}

// GetStats returns total, due, new, and mastered card counts.
func (fc *Flashcards) GetStats() (total, due, newCards, mastered int) {
	now := time.Now()
	total = len(fc.cards)
	for _, c := range fc.cards {
		if c.Reps == 0 {
			newCards++
		} else if c.Interval > 21 && c.EaseFactor >= 2.5 {
			mastered++
		}
		if c.Due.IsZero() || c.Due.Before(now) {
			due++
		}
	}
	return
}

// LoadCards merges extracted cards with persisted progress.
func (fc *Flashcards) LoadCards(notes map[string]string) {
	extracted := ExtractAllCards(notes)

	// Merge: keep persisted scheduling data, add new cards
	for _, card := range extracted {
		if existing, ok := fc.progress.Cards[card.ID]; ok {
			// Update content but keep scheduling
			existing.Question = card.Question
			existing.Answer = card.Answer
			existing.Source = card.Source
		} else {
			c := card // copy
			fc.progress.Cards[card.ID] = &c
		}
	}

	// Rebuild the slice from the progress map
	fc.cards = make([]*Flashcard, 0, len(fc.progress.Cards))
	for _, card := range fc.progress.Cards {
		fc.cards = append(fc.cards, card)
	}
}

// ---------------------------------------------------------------------------
// Persistence
// ---------------------------------------------------------------------------

func (fc *Flashcards) progressPath(vaultPath string) string {
	return filepath.Join(vaultPath, ".granit", "flashcards.json")
}

func (fc *Flashcards) SaveProgress(vaultPath string) {
	p := fc.progressPath(vaultPath)
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fc.statusMsg = "Failed to save progress: " + err.Error()
		return
	}
	data, err := json.MarshalIndent(fc.progress, "", "  ")
	if err != nil {
		fc.statusMsg = "Failed to save progress: " + err.Error()
		return
	}
	if err := os.WriteFile(p, data, 0o600); err != nil {
		fc.statusMsg = "Failed to save progress: " + err.Error()
	}
}

func (fc *Flashcards) LoadProgress(vaultPath string) {
	p := fc.progressPath(vaultPath)
	data, err := os.ReadFile(p)
	if err != nil {
		// File doesn't exist yet — not an error.
		return
	}
	var prog flashcardProgress
	if err := json.Unmarshal(data, &prog); err != nil {
		// Corrupted progress file — warn user and continue with empty progress.
		fc.statusMsg = fmt.Sprintf("Warning: flashcard progress file is corrupted (%s) — starting fresh", err)
		fc.progress = flashcardProgress{Cards: make(map[string]*Flashcard)}
		return
	}
	if prog.Cards == nil {
		prog.Cards = make(map[string]*Flashcard)
	}
	fc.progress = prog

	fc.cards = make([]*Flashcard, 0, len(fc.progress.Cards))
	for _, card := range fc.progress.Cards {
		fc.cards = append(fc.cards, card)
	}
}

// ---------------------------------------------------------------------------
// Review helpers
// ---------------------------------------------------------------------------

func (fc *Flashcards) dueCards() []*Flashcard {
	now := time.Now()
	var due []*Flashcard
	for _, c := range fc.cards {
		if !c.Due.After(now) {
			due = append(due, c)
		}
	}
	return due
}

func (fc *Flashcards) newCards() int {
	count := 0
	for _, c := range fc.cards {
		if c.Reps == 0 && c.Lapses == 0 {
			count++
		}
	}
	return count
}

func (fc *Flashcards) masteredCards() int {
	count := 0
	for _, c := range fc.cards {
		if c.Interval >= 21 { // 3+ weeks interval = mastered
			count++
		}
	}
	return count
}

func (fc *Flashcards) startReview() {
	fc.reviewQueue = fc.dueCards()
	fc.reviewIdx = 0
	fc.showAnswer = false
	fc.sessionDone = 0
	fc.sessionTotal = len(fc.reviewQueue)
	if fc.sessionTotal > 0 {
		fc.mode = fcModeReview
	}
}

func (fc *Flashcards) currentCard() *Flashcard {
	if fc.reviewIdx < len(fc.reviewQueue) {
		return fc.reviewQueue[fc.reviewIdx]
	}
	return nil
}

func (fc *Flashcards) rateCard(quality int) {
	card := fc.currentCard()
	if card == nil {
		return
	}
	ReviewCard(card, quality)
	fc.sessionDone++
	fc.progress.TotalReviews++

	// Update streak
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	lastDay := time.Date(fc.progress.LastReview.Year(), fc.progress.LastReview.Month(), fc.progress.LastReview.Day(), 0, 0, 0, 0, now.Location())
	diff := today.Sub(lastDay).Hours() / 24
	if diff <= 1 && diff >= 0 {
		// Same day or next day — maintain streak
		if diff >= 1 {
			fc.progress.StreakDays++
		}
	} else if diff > 1 {
		fc.progress.StreakDays = 1
	}
	fc.progress.LastReview = now

	fc.reviewIdx++
	fc.showAnswer = false

	if fc.reviewIdx >= len(fc.reviewQueue) {
		fc.mode = fcModeDeck
	}

	fc.SaveProgress(fc.vaultPath)
}

func (fc *Flashcards) retentionRate() float64 {
	if fc.progress.TotalReviews == 0 {
		return 0
	}
	totalLapses := 0
	for _, c := range fc.cards {
		totalLapses += c.Lapses
	}
	if fc.progress.TotalReviews <= totalLapses {
		return 0
	}
	return float64(fc.progress.TotalReviews-totalLapses) / float64(fc.progress.TotalReviews) * 100
}

// cardsByDifficulty returns counts: easy, good, hard, new
func (fc *Flashcards) cardsByDifficulty() (easy, good, hard, newCards int) {
	for _, c := range fc.cards {
		switch {
		case c.Reps == 0 && c.Lapses == 0:
			newCards++
		case c.EaseFactor >= 2.5:
			easy++
		case c.EaseFactor >= 1.8:
			good++
		default:
			hard++
		}
	}
	return
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (fc Flashcards) Update(msg tea.Msg) (Flashcards, tea.Cmd) {
	if !fc.active {
		return fc, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		key := msg.String()

		switch fc.mode {
		case fcModeDeck:
			fc.statusMsg = "" // clear on any key press
			switch key {
			case "esc", "q":
				fc.active = false
			case "enter":
				fc.startReview()
			case "s":
				fc.mode = fcModeStats
				fc.scroll = 0
			}

		case fcModeReview:
			switch {
			case key == "esc":
				fc.mode = fcModeDeck
			case key == " " && !fc.showAnswer:
				fc.showAnswer = true
			case fc.showAnswer && key == "1":
				fc.rateCard(1)
			case fc.showAnswer && key == "2":
				fc.rateCard(2)
			case fc.showAnswer && key == "3":
				fc.rateCard(3)
			case fc.showAnswer && key == "4":
				fc.rateCard(4)
			case fc.showAnswer && key == "5":
				fc.rateCard(5)
			}

		case fcModeStats:
			switch key {
			case "esc", "q", "s":
				fc.mode = fcModeDeck
			case "up", "k":
				if fc.scroll > 0 {
					fc.scroll--
				}
			case "down", "j":
				fc.scroll++
			}
		}
	}

	return fc, nil
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (fc Flashcards) View() string {
	switch fc.mode {
	case fcModeReview:
		return fc.viewReview()
	case fcModeStats:
		return fc.viewStats()
	default:
		return fc.viewDeck()
	}
}

func (fc Flashcards) overlayWidth() int {
	w := fc.width * 2 / 3
	if w < 50 {
		w = 50
	}
	if w > 80 {
		w = 80
	}
	return w
}

// --- Deck view ---

func (fc Flashcards) viewDeck() string {
	w := fc.overlayWidth()
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	sectionStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(text)
	numStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)

	b.WriteString(titleStyle.Render("  Flashcards"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", w-8)))
	b.WriteString("\n\n")

	dueCount := len(fc.dueCards())
	totalCount := len(fc.cards)
	newCount := fc.newCards()
	masteredCount := fc.masteredCards()

	b.WriteString(sectionStyle.Render("  Deck Overview"))
	b.WriteString("\n\n")

	b.WriteString(labelStyle.Render("  Total Cards:    "))
	b.WriteString(numStyle.Render(fmt.Sprintf("%d", totalCount)))
	b.WriteString("\n")

	dueStyle := lipgloss.NewStyle().Foreground(yellow).Bold(true)
	b.WriteString(labelStyle.Render("  Due Today:      "))
	b.WriteString(dueStyle.Render(fmt.Sprintf("%d", dueCount)))
	b.WriteString("\n")

	newStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	b.WriteString(labelStyle.Render("  New Cards:      "))
	b.WriteString(newStyle.Render(fmt.Sprintf("%d", newCount)))
	b.WriteString("\n")

	masteredStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
	b.WriteString(labelStyle.Render("  Mastered:       "))
	b.WriteString(masteredStyle.Render(fmt.Sprintf("%d", masteredCount)))
	b.WriteString("\n\n")

	// Progress bar
	if totalCount > 0 {
		barWidth := w - 12
		if barWidth < 10 {
			barWidth = 10
		}
		filled := masteredCount * barWidth / totalCount
		if filled > barWidth {
			filled = barWidth
		}
		barFill := lipgloss.NewStyle().Foreground(green).Render(strings.Repeat("\u2588", filled))
		barEmpty := DimStyle.Render(strings.Repeat("\u2591", barWidth-filled))
		pct := masteredCount * 100 / totalCount
		b.WriteString(fmt.Sprintf("  %s%s %d%%\n", barFill, barEmpty, pct))
	}

	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", w-8)))
	b.WriteString("\n")

	if fc.statusMsg != "" {
		warnStyle := lipgloss.NewStyle().Foreground(yellow)
		b.WriteString(warnStyle.Render("  " + fc.statusMsg))
		b.WriteString("\n")
	}

	if dueCount > 0 {
		b.WriteString(DimStyle.Render("  Enter: start review  s: stats  Esc: close"))
	} else {
		b.WriteString(DimStyle.Render("  No cards due! s: stats  Esc: close"))
	}

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(w).
		Background(mantle)

	return border.Render(b.String())
}

// --- Review view ---

func (fc Flashcards) viewReview() string {
	w := fc.overlayWidth()
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	questionStyle := lipgloss.NewStyle().Foreground(text).Bold(true)
	answerStyle := lipgloss.NewStyle().Foreground(green)
	sourceStyle := lipgloss.NewStyle().Foreground(overlay0).Italic(true)

	b.WriteString(titleStyle.Render("  Review"))
	b.WriteString("  ")

	// Progress indicator
	progress := fmt.Sprintf("%d/%d", fc.sessionDone+1, fc.sessionTotal)
	b.WriteString(DimStyle.Render(progress))
	b.WriteString("\n")

	// Progress bar
	barWidth := w - 8
	if barWidth < 10 {
		barWidth = 10
	}
	filled := 0
	if fc.sessionTotal > 0 {
		filled = fc.sessionDone * barWidth / fc.sessionTotal
	}
	barFill := lipgloss.NewStyle().Foreground(mauve).Render(strings.Repeat("\u2588", filled))
	barEmpty := DimStyle.Render(strings.Repeat("\u2591", barWidth-filled))
	b.WriteString("  " + barFill + barEmpty)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", w-8)))
	b.WriteString("\n\n")

	card := fc.currentCard()
	if card == nil {
		b.WriteString(lipgloss.NewStyle().Foreground(green).Bold(true).Render("  All done!"))
		b.WriteString("\n\n")
		b.WriteString(DimStyle.Render("  Esc: back to deck"))
	} else {
		// Source
		src := card.Source
		if len(src) > 40 {
			src = "..." + src[len(src)-37:]
		}
		b.WriteString(sourceStyle.Render("  " + src))
		b.WriteString("\n\n")

		// Question
		b.WriteString(lipgloss.NewStyle().Foreground(blue).Bold(true).Render("  Q: "))
		// Wrap question text
		qLines := wrapText(card.Question, w-12)
		for i, line := range qLines {
			if i == 0 {
				b.WriteString(questionStyle.Render(line))
			} else {
				b.WriteString("     " + questionStyle.Render(line))
			}
			b.WriteString("\n")
		}

		b.WriteString("\n")

		if fc.showAnswer {
			b.WriteString(lipgloss.NewStyle().Foreground(green).Bold(true).Render("  A: "))
			aLines := wrapText(card.Answer, w-12)
			for i, line := range aLines {
				if i == 0 {
					b.WriteString(answerStyle.Render(line))
				} else {
					b.WriteString("     " + answerStyle.Render(line))
				}
				b.WriteString("\n")
			}

			b.WriteString("\n")
			b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", w-8)))
			b.WriteString("\n")

			// Rating buttons
			r1 := lipgloss.NewStyle().Foreground(red).Bold(true).Render("1:Again")
			r2 := lipgloss.NewStyle().Foreground(red).Render("2:Fail")
			r3 := lipgloss.NewStyle().Foreground(yellow).Render("3:Hard")
			r4 := lipgloss.NewStyle().Foreground(green).Render("4:Good")
			r5 := lipgloss.NewStyle().Foreground(blue).Bold(true).Render("5:Easy")
			b.WriteString(fmt.Sprintf("  %s  %s  %s  %s  %s", r1, r2, r3, r4, r5))
		} else {
			b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", w-8)))
			b.WriteString("\n")
			b.WriteString(DimStyle.Render("  Space: reveal answer  Esc: quit review"))
		}
	}

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(w).
		Background(mantle)

	return border.Render(b.String())
}

// --- Stats view ---

func (fc Flashcards) viewStats() string {
	w := fc.overlayWidth()

	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	sectionStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(text)
	numStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
	barStyle := lipgloss.NewStyle().Foreground(mauve)

	var lines []string

	// General stats
	lines = append(lines, sectionStyle.Render("  Review Statistics"))
	lines = append(lines, DimStyle.Render("  "+strings.Repeat("\u2500", 30)))
	lines = append(lines, labelStyle.Render("  Total Reviews:   ")+numStyle.Render(fmt.Sprintf("%d", fc.progress.TotalReviews)))
	lines = append(lines, labelStyle.Render("  Streak:          ")+numStyle.Render(fmt.Sprintf("%d days", fc.progress.StreakDays)))
	lines = append(lines, labelStyle.Render("  Retention Rate:  ")+numStyle.Render(fmt.Sprintf("%.1f%%", fc.retentionRate())))
	lines = append(lines, "")

	// Cards by difficulty
	easy, good, hard, newCards := fc.cardsByDifficulty()
	total := len(fc.cards)

	lines = append(lines, sectionStyle.Render("  Cards by Difficulty"))
	lines = append(lines, DimStyle.Render("  "+strings.Repeat("\u2500", 30)))

	diffEntries := []struct {
		label string
		count int
		color lipgloss.Color
	}{
		{"New", newCards, blue},
		{"Easy", easy, green},
		{"Good", good, yellow},
		{"Hard", hard, red},
	}

	barWidth := 25
	for _, entry := range diffEntries {
		filled := 0
		if total > 0 {
			filled = entry.count * barWidth / total
		}
		if filled < 1 && entry.count > 0 {
			filled = 1
		}
		bar := lipgloss.NewStyle().Foreground(entry.color).Render(strings.Repeat("\u2588", filled))
		bar += DimStyle.Render(strings.Repeat("\u2591", barWidth-filled))
		lines = append(lines, fmt.Sprintf("  %s %s %s",
			labelStyle.Render(fcPadRight(entry.label, 6)),
			bar,
			numStyle.Render(fmt.Sprintf("%d", entry.count)),
		))
	}

	lines = append(lines, "")

	// Mastery breakdown
	masteredCount := fc.masteredCards()
	learningCount := total - masteredCount - newCards
	if learningCount < 0 {
		learningCount = 0
	}

	lines = append(lines, sectionStyle.Render("  Mastery"))
	lines = append(lines, DimStyle.Render("  "+strings.Repeat("\u2500", 30)))

	if total > 0 {
		mPct := masteredCount * 100 / total
		lPct := learningCount * 100 / total
		nPct := newCards * 100 / total

		mBar := lipgloss.NewStyle().Foreground(green).Render(strings.Repeat("\u2588", masteredCount*barWidth/total))
		lBar := barStyle.Render(strings.Repeat("\u2588", learningCount*barWidth/total))
		nBar := lipgloss.NewStyle().Foreground(blue).Render(strings.Repeat("\u2588", newCards*barWidth/total))
		rest := barWidth - (masteredCount*barWidth/total + learningCount*barWidth/total + newCards*barWidth/total)
		if rest < 0 {
			rest = 0
		}
		lines = append(lines, "  "+mBar+lBar+nBar+DimStyle.Render(strings.Repeat("\u2591", rest)))
		lines = append(lines, fmt.Sprintf("  %s %d%%  %s %d%%  %s %d%%",
			lipgloss.NewStyle().Foreground(green).Render("\u2588 Mastered"),
			mPct,
			barStyle.Render("\u2588 Learning"),
			lPct,
			lipgloss.NewStyle().Foreground(blue).Render("\u2588 New"),
			nPct,
		))
	} else {
		lines = append(lines, DimStyle.Render("  No cards yet"))
	}

	// Build scrollable output
	var b strings.Builder

	b.WriteString(titleStyle.Render("  Flashcard Stats"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", w-6)))
	b.WriteString("\n")

	visH := fc.height - 8
	if visH < 10 {
		visH = 10
	}
	maxScroll := len(lines) - visH
	if maxScroll < 0 {
		maxScroll = 0
	}
	scroll := fc.scroll
	if scroll > maxScroll {
		scroll = maxScroll
	}
	end := scroll + visH
	if end > len(lines) {
		end = len(lines)
	}

	for i := scroll; i < end; i++ {
		b.WriteString(lines[i])
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render("  j/k: scroll  s/Esc: back"))

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(w).
		Background(mantle)

	return border.Render(b.String())
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func wrapText(s string, maxWidth int) []string {
	if maxWidth <= 0 {
		maxWidth = 40
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{""}
	}

	var lines []string
	current := words[0]
	for _, word := range words[1:] {
		if len(current)+1+len(word) > maxWidth {
			lines = append(lines, current)
			current = word
		} else {
			current += " " + word
		}
	}
	lines = append(lines, current)
	return lines
}

func fcPadRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}
