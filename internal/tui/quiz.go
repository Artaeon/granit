package tui

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// QuizQuestion represents a single quiz question generated from note content.
type QuizQuestion struct {
	Type     string   // "fill_blank", "true_false", "definition", "multiple_choice"
	Question string
	Answer   string
	Options  []string // for multiple choice
	Source   string   // source note path
}

// QuizAnswer records the user's response to a single question.
type QuizAnswer struct {
	Selected  string
	Correct   bool
	TimeTaken time.Duration
}

// QuizSession holds the state for an in-progress quiz.
type QuizSession struct {
	Questions []QuizQuestion
	Current   int
	Score     int
	Total     int
	Answers   []QuizAnswer
	StartTime time.Time
}

// ---------------------------------------------------------------------------
// Quiz states
// ---------------------------------------------------------------------------

const (
	quizStateSetup    = 0
	quizStateQuestion = 1
	quizStateFeedback = 2
	quizStateResults  = 3
)

// ---------------------------------------------------------------------------
// QuizMode overlay
// ---------------------------------------------------------------------------

// QuizMode implements the overlay pattern used throughout the TUI.
type QuizMode struct {
	active bool
	width  int
	height int

	state int // one of quizState* constants

	// Setup screen
	sourceOptions []string
	sourceCursor  int

	// Note contents provided by the host
	noteContents map[string]string
	currentNote  string
	currentBody  string

	// Active session
	session QuizSession

	// Question interaction
	mcCursor   int    // multiple-choice cursor
	inputBuf   string // typed answer for fill_blank / definition
	questionTS time.Time

	// Feedback
	lastCorrect bool
	lastAnswer  string

	// Results scroll
	scroll int
}

// NewQuizMode returns a zero-value QuizMode ready for use.
func NewQuizMode() QuizMode {
	return QuizMode{
		sourceOptions: []string{"Current Note", "All Notes", "By Tag"},
	}
}

// ---------------------------------------------------------------------------
// Overlay interface
// ---------------------------------------------------------------------------

func (q *QuizMode) IsActive() bool          { return q.active }
func (q *QuizMode) SetSize(w, h int)        { q.width = w; q.height = h }
func (q *QuizMode) Close()                  { q.active = false }

// Open resets the overlay to the setup screen.
func (q *QuizMode) Open() {
	q.active = true
	q.state = quizStateSetup
	q.sourceCursor = 0
	q.scroll = 0
	q.inputBuf = ""
	q.mcCursor = 0
}

// SetNoteContents provides vault content for quiz generation.
func (q *QuizMode) SetNoteContents(notes map[string]string) {
	q.noteContents = notes
}

// GetScore returns the final score after a quiz session.
func (q *QuizMode) GetScore() (correct, total int) {
	return q.session.Score, q.session.Total
}

// ---------------------------------------------------------------------------
// Quiz generation
// ---------------------------------------------------------------------------

// GenerateQuiz creates questions from a single note's content.
func GenerateQuiz(notePath string, content string) []QuizQuestion {
	var questions []QuizQuestion

	lines := strings.Split(content, "\n")

	questions = append(questions, generateDefinitionQuestions(notePath, lines)...)
	questions = append(questions, generateFillBlankQuestions(notePath, lines)...)
	questions = append(questions, generateTrueFalseQuestions(notePath, lines)...)
	questions = append(questions, generateMultipleChoiceQuestions(notePath, lines)...)

	return questions
}

// generateDefinitionQuestions looks for "term :: definition" patterns and
// heading + content pairs.
func generateDefinitionQuestions(source string, lines []string) []QuizQuestion {
	var qs []QuizQuestion

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if parts := strings.SplitN(line, "::", 2); len(parts) == 2 {
			term := strings.TrimSpace(parts[0])
			def := strings.TrimSpace(parts[1])
			if term != "" && def != "" {
				qs = append(qs, QuizQuestion{
					Type:     "definition",
					Question: fmt.Sprintf("Define: %s", term),
					Answer:   def,
					Source:   source,
				})
			}
		}
	}

	// Heading + first non-empty body line as definition match
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") || strings.HasPrefix(trimmed, "### ") {
			heading := strings.TrimLeft(trimmed, "# ")
			heading = strings.TrimSpace(heading)
			if heading == "" {
				continue
			}
			// Find first non-empty body line after heading
			for j := i + 1; j < len(lines) && j <= i+3; j++ {
				body := strings.TrimSpace(lines[j])
				if body == "" || strings.HasPrefix(body, "#") {
					continue
				}
				if len(body) > 15 {
					qs = append(qs, QuizQuestion{
						Type:     "definition",
						Question: fmt.Sprintf("What is described by: %s", heading),
						Answer:   body,
						Source:   source,
					})
				}
				break
			}
		}
	}

	return qs
}

// generateFillBlankQuestions picks sentences that contain bold, italic, or
// definition-list key terms and blanks them out.
func generateFillBlankQuestions(source string, lines []string) []QuizQuestion {
	var qs []QuizQuestion

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Try **bold** terms first.
		if startBold := strings.Index(trimmed, "**"); startBold >= 0 {
			after := trimmed[startBold+2:]
			if endBold := strings.Index(after, "**"); endBold >= 0 {
				term := after[:endBold]
				if len(term) >= 2 && len(term) <= 60 {
					blanked := trimmed[:startBold] + "______" + trimmed[startBold+2+endBold+2:]
					blanked = strings.ReplaceAll(blanked, "**", "")
					blanked = strings.ReplaceAll(blanked, "*", "")
					qs = append(qs, QuizQuestion{
						Type:     "fill_blank",
						Question: blanked,
						Answer:   term,
						Source:   source,
					})
					continue
				}
			}
		}

		// Try *italic* terms (single asterisk, not bold).
		if !strings.Contains(trimmed, "**") {
			if startItalic := strings.Index(trimmed, "*"); startItalic >= 0 {
				after := trimmed[startItalic+1:]
				if endItalic := strings.Index(after, "*"); endItalic > 0 {
					term := after[:endItalic]
					if len(term) >= 2 && len(term) <= 60 {
						blanked := trimmed[:startItalic] + "______" + trimmed[startItalic+1+endItalic+1:]
						blanked = strings.ReplaceAll(blanked, "*", "")
						qs = append(qs, QuizQuestion{
							Type:     "fill_blank",
							Question: blanked,
							Answer:   term,
							Source:   source,
						})
						continue
					}
				}
			}
		}

		// Try definition list terms: "- term :: definition" — blank the term.
		if strings.HasPrefix(trimmed, "- ") && strings.Contains(trimmed, " :: ") {
			parts := strings.SplitN(trimmed[2:], " :: ", 2)
			if len(parts) == 2 {
				term := strings.TrimSpace(parts[0])
				def := strings.TrimSpace(parts[1])
				if len(term) >= 2 && len(term) <= 60 && def != "" {
					blanked := "- ______ :: " + def
					qs = append(qs, QuizQuestion{
						Type:     "fill_blank",
						Question: blanked,
						Answer:   term,
						Source:   source,
					})
				}
			}
		}
	}

	return qs
}

// generateTrueFalseQuestions creates true/false statements from note content.
// Roughly half are true (verbatim) and half are false (negated).
func generateTrueFalseQuestions(source string, lines []string) []QuizQuestion {
	var qs []QuizQuestion

	var sentences []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		// Strip markdown formatting
		clean := strings.ReplaceAll(trimmed, "**", "")
		clean = strings.ReplaceAll(clean, "*", "")
		clean = strings.ReplaceAll(clean, "`", "")
		clean = strings.TrimPrefix(clean, "- ")
		clean = strings.TrimSpace(clean)

		if len(clean) > 20 && len(clean) < 200 {
			sentences = append(sentences, clean)
		}
	}

	for i, s := range sentences {
		if len(qs) >= 5 {
			break
		}
		// Alternate true and false questions for diversity.
		if i%2 == 1 {
			if negated, ok := negateSentence(s); ok {
				qs = append(qs, QuizQuestion{
					Type:     "true_false",
					Question: fmt.Sprintf("True or False: %s", negated),
					Answer:   "false",
					Source:   source,
				})
				continue
			}
		}
		qs = append(qs, QuizQuestion{
			Type:     "true_false",
			Question: fmt.Sprintf("True or False: %s", s),
			Answer:   "true",
			Source:   source,
		})
	}

	return qs
}

// negateSentence attempts to negate a sentence by inserting "not" after common
// verbs or replacing key positive words. Returns the negated string and true
// if a negation was applied.
func negateSentence(s string) (string, bool) {
	lower := strings.ToLower(s)

	// Try inserting "not" after common verbs/auxiliaries.
	verbs := []string{" is ", " are ", " was ", " were ", " can ", " will ", " has ", " have ", " does ", " do ", " should ", " must ", " could ", " would "}
	for _, v := range verbs {
		idx := strings.Index(lower, v)
		if idx >= 0 {
			insertAt := idx + len(v)
			return s[:insertAt] + "not " + s[insertAt:], true
		}
	}

	// Try replacing "always" with "never" and vice-versa.
	replacements := [][2]string{
		{"always", "never"},
		{"all", "no"},
		{"every", "no"},
	}
	for _, r := range replacements {
		if strings.Contains(lower, r[0]) {
			// Perform case-insensitive replacement of the first occurrence.
			idx := strings.Index(lower, r[0])
			return s[:idx] + r[1] + s[idx+len(r[0]):], true
		}
	}

	return s, false
}

// generateMultipleChoiceQuestions builds questions from lists in the note.
func generateMultipleChoiceQuestions(source string, lines []string) []QuizQuestion {
	var qs []QuizQuestion

	// Collect list items grouped by the heading above them
	type listGroup struct {
		heading string
		items   []string
	}
	var groups []listGroup
	currentHeading := ""

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") || strings.HasPrefix(trimmed, "## ") || strings.HasPrefix(trimmed, "### ") {
			currentHeading = strings.TrimLeft(trimmed, "# ")
			currentHeading = strings.TrimSpace(currentHeading)
			continue
		}
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			item := strings.TrimSpace(trimmed[2:])
			item = strings.ReplaceAll(item, "**", "")
			item = strings.ReplaceAll(item, "*", "")
			if item == "" {
				continue
			}
			if len(groups) == 0 || groups[len(groups)-1].heading != currentHeading {
				groups = append(groups, listGroup{heading: currentHeading, items: []string{item}})
			} else {
				groups[len(groups)-1].items = append(groups[len(groups)-1].items, item)
			}
		}
	}

	for _, g := range groups {
		if len(g.items) < 4 {
			continue
		}
		// Pick a correct answer and build distractors
		correctIdx := rand.Intn(len(g.items))
		correct := g.items[correctIdx]

		// Collect distractors
		var distractors []string
		for i, it := range g.items {
			if i != correctIdx {
				distractors = append(distractors, it)
			}
		}
		// Shuffle and take up to 3
		rand.Shuffle(len(distractors), func(i, j int) { distractors[i], distractors[j] = distractors[j], distractors[i] })
		if len(distractors) > 3 {
			distractors = distractors[:3]
		}

		options := append([]string{correct}, distractors...)
		rand.Shuffle(len(options), func(i, j int) { options[i], options[j] = options[j], options[i] })

		prompt := "Which of the following is listed"
		if g.heading != "" {
			prompt = fmt.Sprintf("Which of the following is listed under \"%s\"", g.heading)
		}

		qs = append(qs, QuizQuestion{
			Type:     "multiple_choice",
			Question: prompt + "?",
			Answer:   correct,
			Options:  options,
			Source:   source,
		})
	}

	return qs
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (q QuizMode) Update(msg tea.Msg) (QuizMode, tea.Cmd) {
	if !q.active {
		return q, nil
	}

	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return q, nil
	}
	key := km.String()

	switch q.state {
	case quizStateSetup:
		return q.updateSetup(key)
	case quizStateQuestion:
		return q.updateQuestion(key, km)
	case quizStateFeedback:
		return q.updateFeedback(key)
	case quizStateResults:
		return q.updateResults(key)
	}

	return q, nil
}

func (q QuizMode) updateSetup(key string) (QuizMode, tea.Cmd) {
	switch key {
	case "esc", "q":
		q.active = false
	case "j", "down":
		if q.sourceCursor < len(q.sourceOptions)-1 {
			q.sourceCursor++
		}
	case "k", "up":
		if q.sourceCursor > 0 {
			q.sourceCursor--
		}
	case "enter":
		q.startQuiz()
	}
	return q, nil
}

func (q *QuizMode) startQuiz() {
	var allQuestions []QuizQuestion

	switch q.sourceCursor {
	case 0: // Current Note
		if q.currentNote != "" && q.currentBody != "" {
			allQuestions = GenerateQuiz(q.currentNote, q.currentBody)
		}
	default: // All Notes / By Tag — use all available notes
		for path, body := range q.noteContents {
			allQuestions = append(allQuestions, GenerateQuiz(path, body)...)
		}
	}

	if len(allQuestions) == 0 {
		return
	}

	rand.Shuffle(len(allQuestions), func(i, j int) {
		allQuestions[i], allQuestions[j] = allQuestions[j], allQuestions[i]
	})

	// Cap at 20 questions
	if len(allQuestions) > 20 {
		allQuestions = allQuestions[:20]
	}

	q.session = QuizSession{
		Questions: allQuestions,
		Current:   0,
		Score:     0,
		Total:     len(allQuestions),
		Answers:   make([]QuizAnswer, 0, len(allQuestions)),
		StartTime: time.Now(),
	}
	q.state = quizStateQuestion
	q.mcCursor = 0
	q.inputBuf = ""
	q.questionTS = time.Now()
}

func (q QuizMode) updateQuestion(key string, km tea.KeyMsg) (QuizMode, tea.Cmd) {
	if key == "esc" {
		q.active = false
		return q, nil
	}

	cur := q.session.Questions[q.session.Current]

	switch cur.Type {
	case "multiple_choice":
		switch key {
		case "j", "down":
			if q.mcCursor < len(cur.Options)-1 {
				q.mcCursor++
			}
		case "k", "up":
			if q.mcCursor > 0 {
				q.mcCursor--
			}
		case "enter":
			selected := cur.Options[q.mcCursor]
			q.submitAnswer(selected)
		}

	case "true_false":
		switch key {
		case "t":
			q.submitAnswer("true")
		case "f":
			q.submitAnswer("false")
		}

	case "fill_blank", "definition":
		switch key {
		case "enter":
			q.submitAnswer(q.inputBuf)
		case "backspace":
			if len(q.inputBuf) > 0 {
				q.inputBuf = q.inputBuf[:len(q.inputBuf)-1]
			}
		default:
			// Accept printable characters
			if len(key) == 1 && key[0] >= 32 && key[0] <= 126 {
				q.inputBuf += key
			} else if km.Type == tea.KeySpace {
				q.inputBuf += " "
			}
		}
	}

	return q, nil
}

func (q *QuizMode) submitAnswer(selected string) {
	cur := q.session.Questions[q.session.Current]
	elapsed := time.Since(q.questionTS)

	correct := strings.EqualFold(strings.TrimSpace(selected), strings.TrimSpace(cur.Answer))

	q.session.Answers = append(q.session.Answers, QuizAnswer{
		Selected:  selected,
		Correct:   correct,
		TimeTaken: elapsed,
	})

	if correct {
		q.session.Score++
	}

	q.lastCorrect = correct
	q.lastAnswer = cur.Answer
	q.state = quizStateFeedback
}

func (q QuizMode) updateFeedback(key string) (QuizMode, tea.Cmd) {
	switch key {
	case "esc":
		q.active = false
	case " ", "space", "enter":
		q.session.Current++
		if q.session.Current >= q.session.Total {
			q.state = quizStateResults
			q.scroll = 0
		} else {
			q.state = quizStateQuestion
			q.mcCursor = 0
			q.inputBuf = ""
			q.questionTS = time.Now()
		}
	}
	return q, nil
}

func (q QuizMode) updateResults(key string) (QuizMode, tea.Cmd) {
	switch key {
	case "esc", "q":
		q.active = false
	case "j", "down":
		q.scroll++
	case "k", "up":
		if q.scroll > 0 {
			q.scroll--
		}
	case "r":
		// Retry wrong questions
		var wrong []QuizQuestion
		for i, a := range q.session.Answers {
			if !a.Correct {
				wrong = append(wrong, q.session.Questions[i])
			}
		}
		if len(wrong) > 0 {
			q.session = QuizSession{
				Questions: wrong,
				Current:   0,
				Score:     0,
				Total:     len(wrong),
				Answers:   make([]QuizAnswer, 0, len(wrong)),
				StartTime: time.Now(),
			}
			q.state = quizStateQuestion
			q.mcCursor = 0
			q.inputBuf = ""
			q.questionTS = time.Now()
		}
	case "n":
		q.state = quizStateSetup
		q.sourceCursor = 0
	}
	return q, nil
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (q QuizMode) View() string {
	width := q.width * 2 / 3
	if width < 55 {
		width = 55
	}
	if width > 90 {
		width = 90
	}

	innerWidth := width - 6

	var body string
	switch q.state {
	case quizStateSetup:
		body = q.viewSetup(innerWidth)
	case quizStateQuestion:
		body = q.viewQuestion(innerWidth)
	case quizStateFeedback:
		body = q.viewFeedback(innerWidth)
	case quizStateResults:
		body = q.viewResults(innerWidth)
	}

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(body)
}

func (q QuizMode) viewSetup(w int) string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	b.WriteString(titleStyle.Render("  Quiz Mode"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", 30)))
	b.WriteString("\n\n")

	labelStyle := lipgloss.NewStyle().Foreground(text)
	b.WriteString(labelStyle.Render("  Choose quiz source:"))
	b.WriteString("\n\n")

	for i, opt := range q.sourceOptions {
		if i == q.sourceCursor {
			cursor := lipgloss.NewStyle().Foreground(peach).Bold(true)
			b.WriteString(cursor.Render("  > " + opt))
		} else {
			b.WriteString(NormalItemStyle.Render("    " + opt))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")

	// Show total available questions
	total := q.countAvailable()
	numStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
	b.WriteString(DimStyle.Render("  Available questions: ") + numStyle.Render(fmt.Sprintf("%d", total)))
	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render("  j/k: navigate  Enter: start  Esc: close"))

	return b.String()
}

func (q QuizMode) countAvailable() int {
	switch q.sourceCursor {
	case 0:
		if q.currentNote != "" && q.currentBody != "" {
			return len(GenerateQuiz(q.currentNote, q.currentBody))
		}
		return 0
	default:
		count := 0
		for path, body := range q.noteContents {
			count += len(GenerateQuiz(path, body))
		}
		return count
	}
}

func (q QuizMode) viewQuestion(w int) string {
	var b strings.Builder

	cur := q.session.Questions[q.session.Current]

	// Progress bar
	b.WriteString(q.renderProgressBar(w))
	b.WriteString("\n\n")

	// Type indicator
	typeStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	typeLabel := ""
	switch cur.Type {
	case "fill_blank":
		typeLabel = "[Fill in the Blank]"
	case "true_false":
		typeLabel = "[True / False]"
	case "definition":
		typeLabel = "[Definition]"
	case "multiple_choice":
		typeLabel = "[Multiple Choice]"
	}
	b.WriteString("  " + typeStyle.Render(typeLabel))
	b.WriteString("\n\n")

	// Question text
	qStyle := lipgloss.NewStyle().Foreground(text).Bold(true)
	wrapped := quizWordWrap(cur.Question, w-4)
	b.WriteString("  " + qStyle.Render(wrapped))
	b.WriteString("\n\n")

	// Answer area
	switch cur.Type {
	case "multiple_choice":
		labels := []string{"A", "B", "C", "D"}
		for i, opt := range cur.Options {
			label := ""
			if i < len(labels) {
				label = labels[i]
			} else {
				label = fmt.Sprintf("%d", i+1)
			}
			if i == q.mcCursor {
				sel := lipgloss.NewStyle().Foreground(peach).Bold(true)
				b.WriteString(sel.Render(fmt.Sprintf("  > %s) %s", label, opt)))
			} else {
				b.WriteString(NormalItemStyle.Render(fmt.Sprintf("    %s) %s", label, opt)))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  j/k: navigate  Enter: select"))

	case "true_false":
		tfStyle := lipgloss.NewStyle().Foreground(yellow)
		b.WriteString(tfStyle.Render("  Press t for True, f for False"))

	case "fill_blank", "definition":
		promptStyle := SearchPromptStyle
		b.WriteString(promptStyle.Render("  > ") + NormalItemStyle.Render(q.inputBuf))
		cursorBlink := lipgloss.NewStyle().Foreground(text).Render("\u2588")
		b.WriteString(cursorBlink)
		b.WriteString("\n\n")
		b.WriteString(DimStyle.Render("  Type your answer, Enter to submit"))
	}

	return b.String()
}

func (q QuizMode) viewFeedback(w int) string {
	var b strings.Builder

	// Progress bar
	b.WriteString(q.renderProgressBar(w))
	b.WriteString("\n\n")

	if q.lastCorrect {
		checkStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
		b.WriteString(checkStyle.Render("  \u2714 Correct!"))
	} else {
		crossStyle := lipgloss.NewStyle().Foreground(red).Bold(true)
		b.WriteString(crossStyle.Render("  \u2718 Incorrect"))
		b.WriteString("\n\n")
		ansLabel := lipgloss.NewStyle().Foreground(yellow).Bold(true)
		ansVal := lipgloss.NewStyle().Foreground(text)
		b.WriteString("  " + ansLabel.Render("Correct answer: ") + ansVal.Render(q.lastAnswer))
	}

	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render("  Space to continue"))

	return b.String()
}

func (q QuizMode) viewResults(w int) string {
	var lines []string

	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	lines = append(lines, titleStyle.Render("  Quiz Results"))
	lines = append(lines, DimStyle.Render("  "+strings.Repeat("\u2500", 30)))
	lines = append(lines, "")

	numStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(text)

	// Score
	pct := 0
	if q.session.Total > 0 {
		pct = q.session.Score * 100 / q.session.Total
	}
	scoreColor := green
	if pct < 50 {
		scoreColor = red
	} else if pct < 75 {
		scoreColor = yellow
	}
	scoreStyle := lipgloss.NewStyle().Foreground(scoreColor).Bold(true)
	lines = append(lines, "  "+labelStyle.Render("Score: ")+scoreStyle.Render(fmt.Sprintf("%d / %d  (%d%%)", q.session.Score, q.session.Total, pct)))

	// Time taken
	elapsed := time.Since(q.session.StartTime)
	lines = append(lines, "  "+labelStyle.Render("Time:  ")+numStyle.Render(formatDuration(elapsed)))
	lines = append(lines, "")

	// Breakdown by type
	sectionStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	lines = append(lines, sectionStyle.Render("  Breakdown by Type"))
	lines = append(lines, DimStyle.Render("  "+strings.Repeat("\u2500", 30)))

	typeCorrect := make(map[string]int)
	typeTotal := make(map[string]int)
	for i, a := range q.session.Answers {
		t := q.session.Questions[i].Type
		typeTotal[t]++
		if a.Correct {
			typeCorrect[t]++
		}
	}

	typeNames := map[string]string{
		"fill_blank":      "Fill in the Blank",
		"true_false":      "True / False",
		"definition":      "Definition",
		"multiple_choice": "Multiple Choice",
	}

	for _, t := range []string{"fill_blank", "true_false", "definition", "multiple_choice"} {
		tot := typeTotal[t]
		if tot == 0 {
			continue
		}
		cor := typeCorrect[t]
		name := typeNames[t]
		lines = append(lines, "  "+labelStyle.Render(fmt.Sprintf("%-20s", name))+numStyle.Render(fmt.Sprintf("%d / %d", cor, tot)))
	}

	lines = append(lines, "")

	// Question-by-question
	lines = append(lines, sectionStyle.Render("  Answers"))
	lines = append(lines, DimStyle.Render("  "+strings.Repeat("\u2500", 30)))
	for i, a := range q.session.Answers {
		mark := lipgloss.NewStyle().Foreground(green).Render("\u2714")
		if !a.Correct {
			mark = lipgloss.NewStyle().Foreground(red).Render("\u2718")
		}
		qText := q.session.Questions[i].Question
		if len(qText) > w-10 {
			qText = qText[:w-13] + "..."
		}
		lines = append(lines, fmt.Sprintf("  %s %s", mark, DimStyle.Render(qText)))
	}

	lines = append(lines, "")
	lines = append(lines, DimStyle.Render("  r: retry wrong  n: new quiz  Esc: close"))

	// Scrollable view
	var b strings.Builder
	visH := q.height - 8
	if visH < 10 {
		visH = 10
	}
	maxScroll := len(lines) - visH
	if maxScroll < 0 {
		maxScroll = 0
	}
	scroll := q.scroll
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

	return b.String()
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (q QuizMode) renderProgressBar(w int) string {
	cur := q.session.Current + 1
	total := q.session.Total

	label := fmt.Sprintf("  %d / %d", cur, total)
	barWidth := w - len(label) - 6
	if barWidth < 10 {
		barWidth = 10
	}

	filled := 0
	if total > 0 {
		filled = cur * barWidth / total
	}
	if filled > barWidth {
		filled = barWidth
	}

	filledStyle := lipgloss.NewStyle().Foreground(mauve)
	emptyStyle := lipgloss.NewStyle().Foreground(surface0)
	labelStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)

	bar := filledStyle.Render(strings.Repeat("\u2588", filled)) + emptyStyle.Render(strings.Repeat("\u2591", barWidth-filled))

	return "  " + bar + labelStyle.Render(label)
}

func quizWordWrap(s string, width int) string {
	if width <= 0 || len(s) <= width {
		return s
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return s
	}

	var lines []string
	current := words[0]
	for _, w := range words[1:] {
		if len(current)+1+len(w) > width {
			lines = append(lines, current)
			current = "  " + w
		} else {
			current += " " + w
		}
	}
	lines = append(lines, current)
	return strings.Join(lines, "\n")
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
