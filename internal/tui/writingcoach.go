package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Async messages
// ---------------------------------------------------------------------------

type writingCoachResultMsg struct {
	response string
	err      error
}

type writingCoachTickMsg struct{}

// ---------------------------------------------------------------------------
// coachFeedback – a single piece of writing feedback
// ---------------------------------------------------------------------------

type coachFeedback struct {
	Category   string // "clarity", "structure", "style", "grammar", "tone"
	Issue      string
	Suggestion string
	Severity   int // 1=minor, 2=moderate, 3=major
	LineRef    string
}

// ---------------------------------------------------------------------------
// WritingCoach overlay
// ---------------------------------------------------------------------------

type WritingCoach struct {
	active bool
	width  int
	height int

	vaultRoot   string
	noteContent string
	notePath    string

	// Phases: 0=setup, 1=analyzing, 2=results
	phase int

	// Soul note
	soulNote     string
	soulNotePath string
	hasSoulNote  bool

	// Feedback results
	feedback []coachFeedback
	cursor   int
	scroll   int

	// Spinner for loading phase
	spinner int

	// AI config
	aiProvider  string
	ollamaURL   string
	ollamaModel string
	openaiKey   string
	openaiModel string

	// Analysis mode: 0=clarity, 1=structure, 2=style, 3=full
	analysisMode int

	// Selected suggestion (consumed-once)
	selectedSuggestion string

	hasResult bool
}

// NewWritingCoach returns a WritingCoach in its default (inactive) state.
func NewWritingCoach() WritingCoach {
	return WritingCoach{}
}

// SetSize updates the available terminal dimensions.
func (wc *WritingCoach) SetSize(w, h int) {
	wc.width = w
	wc.height = h
}

// Open activates the writing coach overlay and loads the soul note if present.
func (wc *WritingCoach) Open(vaultRoot, noteContent, notePath string, aiProvider, ollamaURL, ollamaModel, openaiKey, openaiModel string) {
	wc.active = true
	wc.phase = 0
	wc.vaultRoot = vaultRoot
	wc.noteContent = noteContent
	wc.notePath = notePath
	wc.aiProvider = aiProvider
	wc.ollamaURL = ollamaURL
	wc.ollamaModel = ollamaModel
	wc.openaiKey = openaiKey
	wc.openaiModel = openaiModel
	wc.analysisMode = 3 // default to full review
	wc.cursor = 0
	wc.scroll = 0
	wc.spinner = 0
	wc.feedback = nil
	wc.selectedSuggestion = ""
	wc.hasResult = false

	// Load soul note
	wc.soulNotePath = filepath.Join(vaultRoot, ".granit", "soul-note.md")
	data, err := os.ReadFile(wc.soulNotePath)
	if err == nil && len(data) > 0 {
		wc.soulNote = string(data)
		wc.hasSoulNote = true
	} else {
		wc.soulNote = ""
		wc.hasSoulNote = false
	}
}

// IsActive reports whether the writing coach is currently visible.
func (wc WritingCoach) IsActive() bool {
	return wc.active
}

// GetSuggestion returns the selected suggestion text (consumed-once).
func (wc *WritingCoach) GetSuggestion() (string, bool) {
	if wc.selectedSuggestion == "" {
		return "", false
	}
	s := wc.selectedSuggestion
	wc.selectedSuggestion = ""
	return s, true
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (wc WritingCoach) overlayWidth() int {
	w := wc.width * 2 / 3
	if w < 60 {
		w = 60
	}
	if w > 100 {
		w = 100
	}
	return w
}


func (wc WritingCoach) modeLabel() string {
	switch wc.analysisMode {
	case 0:
		return "Clarity"
	case 1:
		return "Structure"
	case 2:
		return "Style"
	case 3:
		return "Full Review"
	}
	return "Full Review"
}

func (wc WritingCoach) noteTitle() string {
	base := filepath.Base(wc.notePath)
	return strings.TrimSuffix(base, ".md")
}

func (wc WritingCoach) wordCount() int {
	return len(strings.Fields(wc.noteContent))
}

func coachSeverityColor(severity int) lipgloss.Color {
	switch severity {
	case 1:
		return green
	case 2:
		return yellow
	case 3:
		return red
	}
	return yellow
}

func coachSeverityLabel(severity int) string {
	switch severity {
	case 1:
		return "minor"
	case 2:
		return "moderate"
	case 3:
		return "major"
	}
	return "moderate"
}

func coachCategoryIcon(cat string) string {
	switch cat {
	case "clarity":
		return IconBotChar
	case "structure":
		return IconEditChar
	case "style":
		return IconEditChar
	case "grammar":
		return IconEditChar
	case "tone":
		return IconBotChar
	}
	return IconBotChar
}

// ---------------------------------------------------------------------------
// Tick command
// ---------------------------------------------------------------------------

func writingCoachTickCmd() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
		return writingCoachTickMsg{}
	})
}

// ---------------------------------------------------------------------------
// AI request helpers
// ---------------------------------------------------------------------------

func (wc WritingCoach) buildPrompt() string {
	var sb strings.Builder

	// System context
	sb.WriteString("You are a writing coach. ")
	if wc.hasSoulNote && wc.soulNote != "" {
		sb.WriteString("Adopt this persona and style:\n")
		persona := wc.soulNote
		if len(persona) > 1500 {
			persona = persona[:1500]
		}
		sb.WriteString(persona)
		sb.WriteString("\n\n")
	}

	// Mode instruction
	var modeDesc string
	switch wc.analysisMode {
	case 0:
		modeDesc = "clarity (focus on unclear sentences, jargon, and ambiguity)"
	case 1:
		modeDesc = "structure (focus on heading hierarchy, paragraph length, and flow)"
	case 2:
		modeDesc = "style (focus on voice, tone consistency, and word choice)"
	case 3:
		modeDesc = "all aspects: clarity, structure, style, grammar, and tone"
	}

	sb.WriteString(fmt.Sprintf("Analyze this text for %s.\n", modeDesc))
	sb.WriteString("For each issue found, output exactly one line in this format:\n")
	sb.WriteString("CATEGORY | SEVERITY (1-3) | ISSUE | SUGGESTION | LINE_REF\n")
	sb.WriteString("Categories: clarity, structure, style, grammar, tone\n")
	sb.WriteString("Severity: 1=minor, 2=moderate, 3=major\n")
	sb.WriteString("LINE_REF: approximate line number or range, or \"general\" if not line-specific.\n")
	sb.WriteString("Do NOT include any other text, headers, or explanations. Only output the feedback lines.\n\n")

	content := wc.noteContent
	if len(content) > 4000 {
		content = content[:4000]
	}
	sb.WriteString("Text to analyze:\n---\n")
	sb.WriteString(content)
	sb.WriteString("\n---\n")

	return sb.String()
}

func callWritingCoachOllama(url, model, prompt string) tea.Cmd {
	return func() tea.Msg {
		reqBody := ollamaRequest{
			Model:  model,
			Prompt: prompt,
			Stream: false,
		}
		data, err := json.Marshal(reqBody)
		if err != nil {
			return writingCoachResultMsg{err: err}
		}

		client := &http.Client{Timeout: 120 * time.Second}
		resp, err := client.Post(url+"/api/generate", "application/json", bytes.NewReader(data))
		if err != nil {
			return writingCoachResultMsg{err: fmt.Errorf("cannot connect to Ollama at %s: %w", url, err)}
		}
		defer resp.Body.Close()

		var buf bytes.Buffer
		if _, err := buf.ReadFrom(resp.Body); err != nil {
			return writingCoachResultMsg{err: err}
		}

		if resp.StatusCode != 200 {
			return writingCoachResultMsg{err: fmt.Errorf("Ollama returned status %d: %s", resp.StatusCode, buf.String())}
		}

		var olResp ollamaResponse
		if err := json.Unmarshal(buf.Bytes(), &olResp); err != nil {
			return writingCoachResultMsg{err: err}
		}

		return writingCoachResultMsg{response: olResp.Response}
	}
}

func callWritingCoachOpenAI(apiKey, model, prompt string) tea.Cmd {
	return func() tea.Msg {
		reqBody := openaiRequest{
			Model: model,
			Messages: []openaiMessage{
				{Role: "system", Content: "You are a writing coach. Be specific and actionable in your feedback."},
				{Role: "user", Content: prompt},
			},
		}
		data, err := json.Marshal(reqBody)
		if err != nil {
			return writingCoachResultMsg{err: err}
		}

		client := &http.Client{Timeout: 60 * time.Second}
		req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(data))
		if err != nil {
			return writingCoachResultMsg{err: err}
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)

		resp, err := client.Do(req)
		if err != nil {
			return writingCoachResultMsg{err: fmt.Errorf("cannot connect to OpenAI: %w", err)}
		}
		defer resp.Body.Close()

		var buf bytes.Buffer
		if _, err := buf.ReadFrom(resp.Body); err != nil {
			return writingCoachResultMsg{err: err}
		}

		var oaiResp openaiResponse
		if err := json.Unmarshal(buf.Bytes(), &oaiResp); err != nil {
			return writingCoachResultMsg{err: err}
		}

		if oaiResp.Error != nil {
			return writingCoachResultMsg{err: fmt.Errorf("OpenAI error: %s", oaiResp.Error.Message)}
		}

		if len(oaiResp.Choices) == 0 {
			return writingCoachResultMsg{err: fmt.Errorf("OpenAI returned no choices")}
		}

		return writingCoachResultMsg{response: oaiResp.Choices[0].Message.Content}
	}
}

// ---------------------------------------------------------------------------
// Parse AI response into feedback items
// ---------------------------------------------------------------------------

func parseCoachResponse(response string) []coachFeedback {
	var items []coachFeedback
	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 5)
		if len(parts) < 4 {
			continue
		}

		cat := strings.TrimSpace(strings.ToLower(parts[0]))
		// Validate category
		switch cat {
		case "clarity", "structure", "style", "grammar", "tone":
			// ok
		default:
			cat = "style"
		}

		severity := 2
		sevStr := strings.TrimSpace(parts[1])
		if sevStr == "1" {
			severity = 1
		} else if sevStr == "3" {
			severity = 3
		}

		issue := strings.TrimSpace(parts[2])
		suggestion := strings.TrimSpace(parts[3])

		lineRef := ""
		if len(parts) >= 5 {
			lineRef = strings.TrimSpace(parts[4])
		}

		if issue == "" {
			continue
		}

		items = append(items, coachFeedback{
			Category:   cat,
			Issue:      issue,
			Suggestion: suggestion,
			Severity:   severity,
			LineRef:    lineRef,
		})
	}
	return items
}

// ---------------------------------------------------------------------------
// Local fallback analysis
// ---------------------------------------------------------------------------

func (wc *WritingCoach) runLocalAnalysis() {
	wc.feedback = nil
	lines := strings.Split(wc.noteContent, "\n")
	sentences := coachSplitSentences(wc.noteContent)

	mode := wc.analysisMode

	// --- Clarity checks (mode 0 or 3) ---
	if mode == 0 || mode == 3 {
		// Long sentences (>40 words)
		for i, sent := range sentences {
			words := strings.Fields(sent)
			if len(words) > 40 {
				wc.feedback = append(wc.feedback, coachFeedback{
					Category:   "clarity",
					Issue:      fmt.Sprintf("Sentence has %d words, which may be hard to follow.", len(words)),
					Suggestion: "Consider breaking this into two or more shorter sentences.",
					Severity:   2,
					LineRef:    fmt.Sprintf("sentence %d", i+1),
				})
			}
		}

		// Passive voice detection
		for i, sent := range sentences {
			lower := strings.ToLower(sent)
			words := strings.Fields(lower)
			for j := 0; j < len(words)-1; j++ {
				w := stripCoachPunctuation(words[j])
				next := stripCoachPunctuation(words[j+1])
				if (w == "was" || w == "were" || w == "been" || w == "being" || w == "is" || w == "are") && looksLikeCoachPastParticiple(next) {
					wc.feedback = append(wc.feedback, coachFeedback{
						Category:   "clarity",
						Issue:      fmt.Sprintf("Possible passive voice: \"%s %s\".", w, next),
						Suggestion: "Consider rewriting in active voice for clarity.",
						Severity:   1,
						LineRef:    fmt.Sprintf("sentence %d", i+1),
					})
					break // one per sentence
				}
			}
		}

		// Repeated words in consecutive sentences
		for i := 1; i < len(sentences); i++ {
			prevWords := coachSignificantWords(sentences[i-1])
			currWords := coachSignificantWords(sentences[i])
			for w := range currWords {
				if prevWords[w] && len(w) > 4 {
					wc.feedback = append(wc.feedback, coachFeedback{
						Category:   "clarity",
						Issue:      fmt.Sprintf("Word \"%s\" repeated in consecutive sentences.", w),
						Suggestion: "Consider using a synonym or restructuring to reduce repetition.",
						Severity:   1,
						LineRef:    fmt.Sprintf("sentences %d-%d", i, i+1),
					})
					break // one per pair
				}
			}
		}
	}

	// --- Structure checks (mode 1 or 3) ---
	if mode == 1 || mode == 3 {
		// Long paragraphs (>200 words)
		paragraphs := splitCoachParagraphs(wc.noteContent)
		for i, para := range paragraphs {
			paraWords := len(strings.Fields(para))
			if paraWords > 200 {
				wc.feedback = append(wc.feedback, coachFeedback{
					Category:   "structure",
					Issue:      fmt.Sprintf("Paragraph %d has %d words.", i+1, paraWords),
					Suggestion: "Break this into smaller paragraphs for better readability.",
					Severity:   2,
					LineRef:    fmt.Sprintf("paragraph %d", i+1),
				})
			}
		}

		// Missing headings: >500 words between headings
		wordsSinceHeading := 0
		foundHeading := false
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "#") {
				foundHeading = true
				wordsSinceHeading = 0
				continue
			}
			wordsSinceHeading += len(strings.Fields(trimmed))
			if wordsSinceHeading > 500 && foundHeading {
				wc.feedback = append(wc.feedback, coachFeedback{
					Category:   "structure",
					Issue:      fmt.Sprintf("Over %d words since the last heading.", wordsSinceHeading),
					Suggestion: "Add a heading to break up this section and improve navigation.",
					Severity:   2,
					LineRef:    "general",
				})
				wordsSinceHeading = 0
			}
		}
		if !foundHeading && len(strings.Fields(wc.noteContent)) > 500 {
			wc.feedback = append(wc.feedback, coachFeedback{
				Category:   "structure",
				Issue:      "No headings found in a note with over 500 words.",
				Suggestion: "Add headings to organize the content and improve scannability.",
				Severity:   3,
				LineRef:    "general",
			})
		}
	}

	// --- Style checks (mode 2 or 3) ---
	if mode == 2 || mode == 3 {
		// Excessive adverbs
		adverbCount := 0
		totalWords := 0
		for _, word := range strings.Fields(wc.noteContent) {
			clean := stripCoachPunctuation(strings.ToLower(word))
			totalWords++
			if strings.HasSuffix(clean, "ly") && len(clean) > 3 {
				// Exclude common non-adverb words ending in ly
				switch clean {
				case "only", "early", "family", "holy", "lily", "reply",
					"apply", "supply", "fly", "july", "italy", "rally",
					"belly", "ally", "bully", "jolly", "folly", "jelly":
					continue
				}
				adverbCount++
			}
		}
		if totalWords > 50 {
			ratio := float64(adverbCount) / float64(totalWords)
			if ratio > 0.04 {
				wc.feedback = append(wc.feedback, coachFeedback{
					Category:   "style",
					Issue:      fmt.Sprintf("High adverb density: %d adverbs in %d words (%.1f%%).", adverbCount, totalWords, ratio*100),
					Suggestion: "Consider replacing adverbs with stronger verbs for more impactful writing.",
					Severity:   1,
					LineRef:    "general",
				})
			}
		}

		// Very short sentences mixed with very long ones (inconsistent rhythm)
		if len(sentences) >= 6 {
			var lengths []int
			for _, s := range sentences {
				lengths = append(lengths, len(strings.Fields(s)))
			}
			shortCount := 0
			longCount := 0
			for _, l := range lengths {
				if l <= 5 {
					shortCount++
				} else if l >= 30 {
					longCount++
				}
			}
			if shortCount > len(sentences)/3 && longCount > len(sentences)/4 {
				wc.feedback = append(wc.feedback, coachFeedback{
					Category:   "style",
					Issue:      "Inconsistent sentence length: many very short and very long sentences.",
					Suggestion: "Vary sentence length more gradually for smoother reading rhythm.",
					Severity:   1,
					LineRef:    "general",
				})
			}
		}

		// Weak opening words
		weakStarts := 0
		weakWords := map[string]bool{
			"there": true, "it": true, "this": true,
		}
		for _, sent := range sentences {
			words := strings.Fields(sent)
			if len(words) > 0 {
				first := strings.ToLower(stripCoachPunctuation(words[0]))
				if weakWords[first] {
					weakStarts++
				}
			}
		}
		if len(sentences) > 5 && weakStarts > len(sentences)/3 {
			wc.feedback = append(wc.feedback, coachFeedback{
				Category:   "style",
				Issue:      fmt.Sprintf("%d of %d sentences start with weak openers (there/it/this).", weakStarts, len(sentences)),
				Suggestion: "Lead with the subject or action for stronger sentences.",
				Severity:   1,
				LineRef:    "general",
			})
		}
	}

	// If no issues found, add an encouraging message
	if len(wc.feedback) == 0 {
		wc.feedback = append(wc.feedback, coachFeedback{
			Category:   "style",
			Issue:      "No significant issues detected.",
			Suggestion: "Your writing looks solid! Consider reading it aloud for final polish.",
			Severity:   1,
			LineRef:    "general",
		})
	}
}

// coachSplitSentences splits text into sentences using simple heuristics,
// filtering out code blocks and headings for more accurate analysis.
func coachSplitSentences(text string) []string {
	// Remove code blocks
	cleaned := removeCodeBlocks(text)
	// Remove headings
	var nonHeadingLines []string
	for _, line := range strings.Split(cleaned, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		nonHeadingLines = append(nonHeadingLines, line)
	}
	joined := strings.Join(nonHeadingLines, " ")

	var sentences []string
	current := strings.Builder{}
	for _, ch := range joined {
		current.WriteRune(ch)
		if ch == '.' || ch == '!' || ch == '?' {
			s := strings.TrimSpace(current.String())
			if len(strings.Fields(s)) > 1 {
				sentences = append(sentences, s)
			}
			current.Reset()
		}
	}
	// trailing text
	s := strings.TrimSpace(current.String())
	if len(strings.Fields(s)) > 1 {
		sentences = append(sentences, s)
	}
	return sentences
}

func removeCodeBlocks(text string) string {
	var result strings.Builder
	inCode := false
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inCode = !inCode
			continue
		}
		if !inCode {
			result.WriteString(line)
			result.WriteString("\n")
		}
	}
	return result.String()
}

func coachSignificantWords(sentence string) map[string]bool {
	words := make(map[string]bool)
	for _, w := range strings.Fields(sentence) {
		clean := stripCoachPunctuation(strings.ToLower(w))
		if len(clean) > 4 && !stopwords[clean] {
			words[clean] = true
		}
	}
	return words
}

func splitCoachParagraphs(text string) []string {
	raw := strings.Split(text, "\n\n")
	var result []string
	for _, p := range raw {
		p = strings.TrimSpace(p)
		if p != "" && !strings.HasPrefix(p, "```") {
			result = append(result, p)
		}
	}
	return result
}

func stripCoachPunctuation(w string) string {
	return strings.Trim(w, ".,;:!?\"'`()[]{}#*_~<>@/\\|+-=")
}

func looksLikeCoachPastParticiple(w string) bool {
	if len(w) < 4 {
		return false
	}
	suffixes := []string{"ed", "en", "own", "ung", "ept"}
	for _, suf := range suffixes {
		if strings.HasSuffix(w, suf) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (wc WritingCoach) Update(msg tea.Msg) (WritingCoach, tea.Cmd) {
	if !wc.active {
		return wc, nil
	}

	switch msg := msg.(type) {
	case writingCoachTickMsg:
		if wc.phase == 1 {
			wc.spinner++
			return wc, writingCoachTickCmd()
		}

	case writingCoachResultMsg:
		if wc.phase != 1 {
			return wc, nil
		}
		if msg.err != nil {
			// Fallback to local analysis on error
			wc.runLocalAnalysis()
			wc.phase = 2
			wc.hasResult = true
			return wc, nil
		}
		wc.feedback = parseCoachResponse(msg.response)
		if len(wc.feedback) == 0 {
			// AI returned unparseable output — try local
			wc.runLocalAnalysis()
		}
		wc.phase = 2
		wc.hasResult = true
		return wc, nil

	case tea.KeyMsg:
		switch wc.phase {
		case 0:
			return wc.updateSetup(msg)
		case 1:
			return wc.updateAnalyzing(msg)
		case 2:
			return wc.updateResults(msg)
		}
	}

	return wc, nil
}

func (wc WritingCoach) updateSetup(msg tea.KeyMsg) (WritingCoach, tea.Cmd) {
	switch msg.String() {
	case "esc":
		wc.active = false
	case "up", "k":
		if wc.analysisMode > 0 {
			wc.analysisMode--
		}
	case "down", "j":
		if wc.analysisMode < 3 {
			wc.analysisMode++
		}
	case "s":
		// Toggle soul note (informational only — it's loaded from disk)
		wc.hasSoulNote = !wc.hasSoulNote
		if !wc.hasSoulNote {
			wc.soulNote = ""
		} else {
			data, err := os.ReadFile(wc.soulNotePath)
			if err == nil && len(data) > 0 {
				wc.soulNote = string(data)
			} else {
				wc.hasSoulNote = false
			}
		}
	case "enter":
		return wc.startAnalysis()
	}
	return wc, nil
}

func (wc WritingCoach) updateAnalyzing(msg tea.KeyMsg) (WritingCoach, tea.Cmd) {
	if msg.String() == "esc" {
		wc.phase = 0
		return wc, nil
	}
	return wc, nil
}

func (wc WritingCoach) updateResults(msg tea.KeyMsg) (WritingCoach, tea.Cmd) {
	switch msg.String() {
	case "esc":
		wc.active = false
	case "up", "k":
		if wc.cursor > 0 {
			wc.cursor--
			wc.ensureCursorVisible()
		}
	case "down", "j":
		if wc.cursor < len(wc.feedback)-1 {
			wc.cursor++
			wc.ensureCursorVisible()
		}
	case "enter":
		if wc.cursor >= 0 && wc.cursor < len(wc.feedback) {
			wc.selectedSuggestion = wc.feedback[wc.cursor].Suggestion
			wc.active = false
		}
	}
	return wc, nil
}

func (wc *WritingCoach) ensureCursorVisible() {
	visH := wc.visibleItems()
	if wc.cursor < wc.scroll {
		wc.scroll = wc.cursor
	}
	if wc.cursor >= wc.scroll+visH {
		wc.scroll = wc.cursor - visH + 1
	}
}

func (wc WritingCoach) visibleItems() int {
	// Each item takes ~3 lines (category+issue, suggestion, separator)
	h := (wc.height - 14) / 3
	if h < 3 {
		h = 3
	}
	return h
}

// ---------------------------------------------------------------------------
// Start analysis (AI or local)
// ---------------------------------------------------------------------------

func (wc WritingCoach) startAnalysis() (WritingCoach, tea.Cmd) {
	if wc.aiProvider == "ollama" || wc.aiProvider == "openai" {
		wc.phase = 1
		wc.spinner = 0
		wc.feedback = nil
		wc.cursor = 0
		wc.scroll = 0
		wc.hasResult = false

		prompt := wc.buildPrompt()

		if wc.aiProvider == "openai" && wc.openaiKey != "" {
			return wc, tea.Batch(
				callWritingCoachOpenAI(wc.openaiKey, wc.openaiModel, prompt),
				writingCoachTickCmd(),
			)
		}

		return wc, tea.Batch(
			callWritingCoachOllama(wc.ollamaURL, wc.ollamaModel, prompt),
			writingCoachTickCmd(),
		)
	}

	// Local analysis
	wc.phase = 2
	wc.cursor = 0
	wc.scroll = 0
	wc.hasResult = true
	wc.runLocalAnalysis()
	return wc, nil
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (wc WritingCoach) View() string {
	if !wc.active {
		return ""
	}

	width := wc.overlayWidth()

	switch wc.phase {
	case 0:
		return wc.viewSetup(width)
	case 1:
		return wc.viewAnalyzing(width)
	case 2:
		return wc.viewResults(width)
	}
	return ""
}

func (wc WritingCoach) viewSetup(width int) string {
	var buf strings.Builder

	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  " + IconBotChar + " Writing Coach")
	buf.WriteString(title)
	buf.WriteString("\n")
	buf.WriteString(DimStyle.Render(strings.Repeat("─", width-6)))
	buf.WriteString("\n\n")

	// Note info
	noteLine := lipgloss.NewStyle().Foreground(text).Render(
		fmt.Sprintf("  Note: %s (%d words)", wc.noteTitle(), wc.wordCount()))
	buf.WriteString(noteLine)
	buf.WriteString("\n\n")

	// AI provider
	providerLabel := "Local Analysis"
	providerColor := overlay0
	switch wc.aiProvider {
	case "ollama":
		providerLabel = "Ollama: " + wc.ollamaModel
		providerColor = green
	case "openai":
		providerLabel = "OpenAI: " + wc.openaiModel
		providerColor = blue
	}
	providerLine := lipgloss.NewStyle().Foreground(providerColor).Render(
		"  " + IconBotChar + " Provider: " + providerLabel)
	buf.WriteString(providerLine)
	buf.WriteString("\n")

	// Soul note status
	soulIcon := lipgloss.NewStyle().Foreground(red).Render("  " + IconEditChar + " Soul Note: not found")
	if wc.hasSoulNote {
		soulIcon = lipgloss.NewStyle().Foreground(green).Render("  " + IconEditChar + " Soul Note: loaded")
	}
	buf.WriteString(soulIcon)
	buf.WriteString("\n")
	buf.WriteString(DimStyle.Render(fmt.Sprintf("    %s", wc.soulNotePath)))
	buf.WriteString("\n\n")

	// Analysis mode selector
	buf.WriteString(DimStyle.Render("  Analysis mode:"))
	buf.WriteString("\n\n")

	modes := []struct {
		name string
		desc string
	}{
		{"Clarity", "Focus on unclear sentences, jargon, ambiguity"},
		{"Structure", "Focus on heading hierarchy, paragraph length, flow"},
		{"Style", "Focus on voice, tone consistency, word choice"},
		{"Full Review", "Comprehensive analysis of all aspects"},
	}

	for i, m := range modes {
		if i == wc.analysisMode {
			pointer := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("  > ")
			nameStyled := lipgloss.NewStyle().Foreground(peach).Bold(true).Render(m.name)
			row := lipgloss.NewStyle().
				Background(surface0).
				Width(width - 6).
				Render(pointer + nameStyled)
			buf.WriteString(row)
			buf.WriteString("\n")
			descStyled := lipgloss.NewStyle().
				Background(surface0).
				Foreground(overlay0).
				Width(width - 6).
				Render("      " + m.desc)
			buf.WriteString(descStyled)
		} else {
			nameStyled := lipgloss.NewStyle().Foreground(text).Render("    " + m.name)
			buf.WriteString(nameStyled)
			buf.WriteString("\n")
			buf.WriteString(DimStyle.Render("      " + m.desc))
		}
		if i < len(modes)-1 {
			buf.WriteString("\n\n")
		}
	}

	buf.WriteString("\n\n")
	buf.WriteString(DimStyle.Render("  " + strings.Repeat("─", width-10)))
	buf.WriteString("\n")
	buf.WriteString(DimStyle.Render("  j/k: mode  s: toggle soul note  Enter: analyze  Esc: close"))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(buf.String())
}

func (wc WritingCoach) viewAnalyzing(width int) string {
	var buf strings.Builder

	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  " + IconBotChar + " Writing Coach")
	buf.WriteString(title)
	buf.WriteString("\n")
	buf.WriteString(DimStyle.Render(strings.Repeat("─", width-6)))
	buf.WriteString("\n\n")

	// Spinner
	spinFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	frame := spinFrames[wc.spinner%len(spinFrames)]
	spinner := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(frame)

	var providerLabel string
	switch wc.aiProvider {
	case "ollama":
		providerLabel = "Ollama (" + wc.ollamaModel + ")"
	case "openai":
		providerLabel = "OpenAI (" + wc.openaiModel + ")"
	default:
		providerLabel = "local analysis"
	}

	buf.WriteString("  " + spinner + " " + lipgloss.NewStyle().Foreground(text).Render(
		"Analyzing with "+providerLabel+"..."))
	buf.WriteString("\n\n")

	buf.WriteString(DimStyle.Render(fmt.Sprintf("  Mode: %s", wc.modeLabel())))
	buf.WriteString("\n")
	if wc.hasSoulNote {
		buf.WriteString(DimStyle.Render("  Persona: soul note active"))
		buf.WriteString("\n")
	}
	buf.WriteString("\n")
	buf.WriteString(DimStyle.Render("  Esc: cancel"))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(buf.String())
}

func (wc WritingCoach) viewResults(width int) string {
	var buf strings.Builder
	innerWidth := width - 6

	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  " + IconBotChar + " Writing Coach — " + wc.modeLabel())
	buf.WriteString(title)
	buf.WriteString("\n")

	// Summary line
	summary := fmt.Sprintf("  %s — %d issues found", wc.noteTitle(), len(wc.feedback))
	buf.WriteString(DimStyle.Render(summary))
	buf.WriteString("\n")
	buf.WriteString(DimStyle.Render(strings.Repeat("─", innerWidth)))
	buf.WriteString("\n")

	// Severity counts
	var minorN, modN, majorN int
	for _, fb := range wc.feedback {
		switch fb.Severity {
		case 1:
			minorN++
		case 2:
			modN++
		case 3:
			majorN++
		}
	}
	countLine := fmt.Sprintf("  %s %d  %s %d  %s %d",
		lipgloss.NewStyle().Foreground(green).Render("minor:"), minorN,
		lipgloss.NewStyle().Foreground(yellow).Render("moderate:"), modN,
		lipgloss.NewStyle().Foreground(red).Render("major:"), majorN,
	)
	buf.WriteString(countLine)
	buf.WriteString("\n")
	buf.WriteString(DimStyle.Render(strings.Repeat("─", innerWidth)))
	buf.WriteString("\n")

	// Feedback items
	visH := wc.visibleItems()
	if visH > len(wc.feedback) {
		visH = len(wc.feedback)
	}

	scroll := wc.scroll
	maxScroll := len(wc.feedback) - visH
	if maxScroll < 0 {
		maxScroll = 0
	}
	if scroll > maxScroll {
		scroll = maxScroll
	}

	end := scroll + visH
	if end > len(wc.feedback) {
		end = len(wc.feedback)
	}

	for idx := scroll; idx < end; idx++ {
		fb := wc.feedback[idx]
		isSelected := idx == wc.cursor

		sevColor := coachSeverityColor(fb.Severity)
		sevLabel := coachSeverityLabel(fb.Severity)
		catIcon := coachCategoryIcon(fb.Category)

		// Category tag + severity badge
		catTag := lipgloss.NewStyle().
			Foreground(lavender).
			Bold(true).
			Render(catIcon + " " + fb.Category)
		sevBadge := lipgloss.NewStyle().
			Foreground(sevColor).
			Render("[" + sevLabel + "]")
		lineTag := ""
		if fb.LineRef != "" && fb.LineRef != "general" {
			lineTag = DimStyle.Render(" @ " + fb.LineRef)
		}

		if isSelected {
			pointer := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("  > ")
			headerLine := pointer + catTag + " " + sevBadge + lineTag
			buf.WriteString(lipgloss.NewStyle().
				Background(surface0).
				Width(innerWidth).
				Render(headerLine))
			buf.WriteString("\n")

			// Issue
			issueLine := lipgloss.NewStyle().
				Foreground(text).
				Background(surface0).
				Width(innerWidth).
				Render("      " + truncateCoach(fb.Issue, innerWidth-8))
			buf.WriteString(issueLine)
			buf.WriteString("\n")

			// Suggestion (highlighted for selected)
			suggLine := lipgloss.NewStyle().
				Foreground(teal).
				Background(surface0).
				Italic(true).
				Width(innerWidth).
				Render("      " + truncateCoach(fb.Suggestion, innerWidth-8))
			buf.WriteString(suggLine)
		} else {
			headerLine := "    " + catTag + " " + sevBadge + lineTag
			buf.WriteString(headerLine)
			buf.WriteString("\n")

			issueLine := lipgloss.NewStyle().Foreground(text).
				Render("      " + truncateCoach(fb.Issue, innerWidth-8))
			buf.WriteString(issueLine)
			buf.WriteString("\n")

			suggLine := DimStyle.Render("      " + truncateCoach(fb.Suggestion, innerWidth-8))
			buf.WriteString(suggLine)
		}

		if idx < end-1 {
			buf.WriteString("\n")
			buf.WriteString(DimStyle.Render("  " + strings.Repeat("·", innerWidth-4)))
			buf.WriteString("\n")
		}
	}

	// Scroll indicator
	if len(wc.feedback) > visH {
		buf.WriteString("\n")
		scrollPct := 0
		if maxScroll > 0 {
			scrollPct = scroll * 100 / maxScroll
		}
		scrollInfo := fmt.Sprintf("  [%d/%d] %d%%", wc.cursor+1, len(wc.feedback), scrollPct)
		buf.WriteString(DimStyle.Render(scrollInfo))
	}

	buf.WriteString("\n\n")
	buf.WriteString(DimStyle.Render("  " + strings.Repeat("─", innerWidth-4)))
	buf.WriteString("\n")
	buf.WriteString(DimStyle.Render("  j/k: navigate  Enter: use suggestion  Esc: close"))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(buf.String())
}

// truncateCoach truncates a string to maxLen, appending "..." if needed.
func truncateCoach(s string, maxLen int) string {
	if maxLen <= 0 {
		return s
	}
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
