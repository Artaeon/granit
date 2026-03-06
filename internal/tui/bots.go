package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// BotResult – returned to the caller after a bot finishes
// ---------------------------------------------------------------------------

// BotResult carries the output of a bot run back to the application.
type BotResult struct {
	Action   string   // "tag", "link", "summary", "none"
	Tags     []string // suggested tags
	Links    []string // suggested links (note paths)
	Summary  string   // generated summary
	NotePath string   // which note this applies to
}

// ---------------------------------------------------------------------------
// Bot descriptors
// ---------------------------------------------------------------------------

type botKind int

const (
	botAutoTagger botKind = iota
	botLinkSuggester
	botSummarizer
	botQuestionBot
	botWritingAssistant
	botDailyDigest
)

type botDescriptor struct {
	kind botKind
	icon string
	name string
	desc string
}

var botList = []botDescriptor{
	{botAutoTagger, "\U0001F3F7", "Auto-Tagger", "Suggest tags for current note"},
	{botLinkSuggester, "\U0001F517", "Link Suggester", "Find related notes"},
	{botSummarizer, "\U0001F4DD", "Summarizer", "Create a brief summary"},
	{botQuestionBot, "\u2753", "Question Bot", "Ask questions about your notes"},
	{botWritingAssistant, "\u270D", "Writing Assistant", "Suggest improvements"},
	{botDailyDigest, "\U0001F4CA", "Daily Digest", "Summarize vault activity"},
}

// ---------------------------------------------------------------------------
// Stopwords
// ---------------------------------------------------------------------------

var stopwords = map[string]bool{
	"the": true, "a": true, "an": true, "is": true, "are": true,
	"was": true, "were": true, "be": true, "been": true, "being": true,
	"have": true, "has": true, "had": true, "do": true, "does": true,
	"did": true, "will": true, "would": true, "could": true, "should": true,
	"may": true, "might": true, "shall": true, "can": true, "need": true,
	"dare": true, "ought": true, "used": true, "to": true, "of": true,
	"in": true, "for": true, "on": true, "with": true, "at": true,
	"by": true, "from": true, "as": true, "into": true, "through": true,
	"during": true, "before": true, "after": true, "above": true,
	"below": true, "between": true, "out": true, "off": true, "over": true,
	"under": true, "again": true, "further": true, "then": true,
	"once": true, "here": true, "there": true, "when": true, "where": true,
	"why": true, "how": true, "all": true, "both": true, "each": true,
	"few": true, "more": true, "most": true, "other": true, "some": true,
	"such": true, "no": true, "nor": true, "not": true, "only": true,
	"own": true, "same": true, "so": true, "than": true, "too": true,
	"very": true, "just": true, "because": true, "but": true, "and": true,
	"or": true, "if": true, "while": true, "about": true, "this": true,
	"that": true, "these": true, "those": true, "it": true, "its": true,
}

// ---------------------------------------------------------------------------
// Topic keywords used by the Auto-Tagger
// ---------------------------------------------------------------------------

var topicKeywords = map[string][]string{
	"technology": {"software", "hardware", "computer", "programming", "code", "api", "server", "database", "algorithm", "tech"},
	"science":    {"research", "experiment", "hypothesis", "theory", "study", "data", "analysis", "biology", "physics", "chemistry"},
	"personal":   {"journal", "diary", "feeling", "emotion", "reflection", "thought", "life", "dream", "memory", "gratitude"},
	"work":       {"meeting", "deadline", "client", "report", "presentation", "email", "office", "colleague", "manager", "review"},
	"project":    {"milestone", "task", "sprint", "deliverable", "requirement", "scope", "timeline", "roadmap", "backlog", "release"},
	"meeting":    {"agenda", "minutes", "attendees", "discussion", "action", "decision", "followup", "standup", "sync", "retro"},
	"idea":       {"concept", "brainstorm", "innovation", "creative", "proposal", "prototype", "sketch", "vision", "possibility", "explore"},
	"todo":       {"task", "checklist", "priority", "urgent", "reminder", "deadline", "assign", "complete", "pending", "done"},
	"recipe":     {"ingredient", "cook", "bake", "recipe", "meal", "food", "kitchen", "serve", "prep", "dish"},
	"travel":     {"trip", "destination", "flight", "hotel", "itinerary", "vacation", "explore", "adventure", "passport", "booking"},
	"finance":    {"budget", "expense", "income", "investment", "savings", "tax", "payment", "invoice", "cost", "profit"},
	"health":     {"exercise", "workout", "diet", "nutrition", "sleep", "wellness", "meditation", "yoga", "symptom", "doctor"},
	"book":       {"reading", "chapter", "author", "novel", "review", "summary", "fiction", "nonfiction", "library", "bookmark"},
	"music":      {"song", "album", "artist", "playlist", "guitar", "piano", "concert", "lyric", "melody", "rhythm"},
	"film":       {"movie", "director", "actor", "scene", "cinema", "documentary", "series", "episode", "watch", "review"},
}

// ---------------------------------------------------------------------------
// Passive voice helper patterns
// ---------------------------------------------------------------------------

// Common past-participle endings used for a rough passive-voice check.
var pastParticipleEndings = []string{"ed", "en", "own", "ung", "ept"}

func looksLikePastParticiple(w string) bool {
	if len(w) < 4 {
		return false
	}
	for _, suf := range pastParticipleEndings {
		if strings.HasSuffix(w, suf) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Bots component
// ---------------------------------------------------------------------------

type botsState int

const (
	botsStateList    botsState = iota // selecting a bot
	botsStateInput                    // Question Bot input
	botsStateResults                  // showing results
)

// Bots is an overlay component that provides local AI-assistant bots.
type Bots struct {
	active bool
	width  int
	height int

	state  botsState
	cursor int // cursor in the bot list
	scroll int // scroll offset for results

	// Vault data
	notes       map[string]string   // notePath -> content
	tags        map[string][]string // tag -> []notePaths
	currentPath string
	currentBody string

	// Question Bot input
	questionInput string

	// Results
	resultLines []string // rendered result lines for display
	resultReady bool
	pendingResult BotResult

	// Which bot was run
	activeBot botKind
}

// NewBots creates a new Bots overlay in its initial state.
func NewBots() Bots {
	return Bots{}
}

// SetSize updates the overlay dimensions.
func (b *Bots) SetSize(width, height int) {
	b.width = width
	b.height = height
}

// Open activates the overlay and resets to the bot list.
func (b *Bots) Open() {
	b.active = true
	b.state = botsStateList
	b.cursor = 0
	b.scroll = 0
	b.resultLines = nil
	b.resultReady = false
	b.pendingResult = BotResult{}
	b.questionInput = ""
}

// Close deactivates the overlay.
func (b *Bots) Close() {
	b.active = false
}

// IsActive reports whether the overlay is visible.
func (b *Bots) IsActive() bool {
	return b.active
}

// SetVaultData passes the full vault contents and tag index to the bots.
func (b *Bots) SetVaultData(notes map[string]string, tags map[string][]string) {
	b.notes = notes
	b.tags = tags
}

// SetCurrentNote sets which note is currently being edited / viewed.
func (b *Bots) SetCurrentNote(path, content string) {
	b.currentPath = path
	b.currentBody = content
}

// GetResult returns any pending BotResult and clears it.
func (b *Bots) GetResult() BotResult {
	r := b.pendingResult
	b.pendingResult = BotResult{}
	b.resultReady = false
	return r
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

// Update handles keyboard input for the Bots overlay.
func (b Bots) Update(msg tea.Msg) (Bots, tea.Cmd) {
	if !b.active {
		return b, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch b.state {
		case botsStateList:
			return b.updateList(msg)
		case botsStateInput:
			return b.updateInput(msg)
		case botsStateResults:
			return b.updateResults(msg)
		}
	}
	return b, nil
}

func (b Bots) updateList(msg tea.KeyMsg) (Bots, tea.Cmd) {
	switch msg.String() {
	case "esc":
		b.active = false
	case "up", "k":
		if b.cursor > 0 {
			b.cursor--
		}
	case "down", "j":
		if b.cursor < len(botList)-1 {
			b.cursor++
		}
	case "enter":
		if b.cursor >= 0 && b.cursor < len(botList) {
			b.activeBot = botList[b.cursor].kind
			if b.activeBot == botQuestionBot {
				b.state = botsStateInput
				b.questionInput = ""
			} else {
				b.runBot()
			}
		}
	}
	return b, nil
}

func (b Bots) updateInput(msg tea.KeyMsg) (Bots, tea.Cmd) {
	switch msg.String() {
	case "esc":
		b.state = botsStateList
	case "enter":
		if strings.TrimSpace(b.questionInput) != "" {
			b.runBot()
		}
	case "backspace":
		if len(b.questionInput) > 0 {
			b.questionInput = b.questionInput[:len(b.questionInput)-1]
		}
	default:
		if len(msg.String()) == 1 || msg.String() == " " {
			b.questionInput += msg.String()
		}
	}
	return b, nil
}

func (b Bots) updateResults(msg tea.KeyMsg) (Bots, tea.Cmd) {
	switch msg.String() {
	case "esc":
		b.state = botsStateList
		b.scroll = 0
		b.resultLines = nil
	case "up", "k":
		if b.scroll > 0 {
			b.scroll--
		}
	case "down", "j":
		b.scroll++
	case "enter":
		if b.resultReady {
			b.active = false
			return b, nil
		}
	}
	return b, nil
}

// ---------------------------------------------------------------------------
// Bot runners
// ---------------------------------------------------------------------------

func (b *Bots) runBot() {
	b.state = botsStateResults
	b.scroll = 0
	b.resultLines = nil
	b.resultReady = false
	b.pendingResult = BotResult{Action: "none", NotePath: b.currentPath}

	switch b.activeBot {
	case botAutoTagger:
		b.runAutoTagger()
	case botLinkSuggester:
		b.runLinkSuggester()
	case botSummarizer:
		b.runSummarizer()
	case botQuestionBot:
		b.runQuestionBot()
	case botWritingAssistant:
		b.runWritingAssistant()
	case botDailyDigest:
		b.runDailyDigest()
	}
}

// ── Auto-Tagger ──────────────────────────────────────────────────────────

func (b *Bots) runAutoTagger() {
	content := strings.ToLower(b.currentBody)
	words := strings.Fields(content)
	wordSet := make(map[string]bool, len(words))
	for _, w := range words {
		w = stripPunctuation(w)
		if len(w) > 2 && !stopwords[w] {
			wordSet[w] = true
		}
	}

	type tagScore struct {
		tag   string
		score int
	}
	var scored []tagScore

	// 1. Score against topic keywords
	for topic, keywords := range topicKeywords {
		score := 0
		for _, kw := range keywords {
			if wordSet[kw] {
				score += 10
			}
		}
		if score > 0 {
			scored = append(scored, tagScore{tag: topic, score: score})
		}
	}

	// 2. Check existing tags from other notes and score by shared keywords
	for tag, notePaths := range b.tags {
		already := false
		for _, ts := range scored {
			if ts.tag == tag {
				already = true
				break
			}
		}
		if already {
			continue
		}
		sharedCount := 0
		for _, np := range notePaths {
			if np == b.currentPath {
				continue
			}
			otherContent, ok := b.notes[np]
			if !ok {
				continue
			}
			otherWords := significantWords(otherContent)
			for ow := range otherWords {
				if wordSet[ow] {
					sharedCount++
				}
			}
		}
		if sharedCount > 0 {
			scored = append(scored, tagScore{tag: tag, score: sharedCount})
		}
	}

	// 3. Extract hashtags already in the note
	for _, w := range strings.Fields(b.currentBody) {
		if strings.HasPrefix(w, "#") && len(w) > 1 {
			ht := strings.TrimRight(w[1:], ".,;:!?)")
			ht = strings.ToLower(ht)
			if ht != "" && !strings.HasPrefix(ht, "#") {
				already := false
				for i, ts := range scored {
					if ts.tag == ht {
						scored[i].score += 20
						already = true
						break
					}
				}
				if !already {
					scored = append(scored, tagScore{tag: ht, score: 20})
				}
			}
		}
	}

	sort.Slice(scored, func(i, j int) bool { return scored[i].score > scored[j].score })
	if len(scored) > 5 {
		scored = scored[:5]
	}

	// Normalise scores to percentages
	maxScore := 1
	if len(scored) > 0 && scored[0].score > 0 {
		maxScore = scored[0].score
	}

	noteName := strings.TrimSuffix(b.currentPath, ".md")
	var lines []string
	lines = append(lines, lipgloss.NewStyle().Foreground(text).Render(
		fmt.Sprintf("  Suggested tags for \"%s\":", noteName)))
	lines = append(lines, "")

	var resultTags []string
	for _, ts := range scored {
		pct := ts.score * 100 / maxScore
		if pct > 99 {
			pct = 99
		}
		if pct < 10 {
			pct = 10
		}
		bullet := lipgloss.NewStyle().Foreground(green).Render("  \u25CF")
		tagStr := lipgloss.NewStyle().Foreground(blue).Bold(true).Render(" #" + ts.tag)
		conf := DimStyle.Render(fmt.Sprintf("  (confidence: %d%%)", pct))
		lines = append(lines, bullet+tagStr+conf)
		resultTags = append(resultTags, ts.tag)
	}

	if len(scored) == 0 {
		lines = append(lines, DimStyle.Render("  No tag suggestions found."))
	}

	b.resultLines = lines
	if len(resultTags) > 0 {
		b.pendingResult = BotResult{
			Action:   "tag",
			Tags:     resultTags,
			NotePath: b.currentPath,
		}
		b.resultReady = true
	}
}

// ── Link Suggester ───────────────────────────────────────────────────────

func (b *Bots) runLinkSuggester() {
	currentWords := significantWords(b.currentBody)

	type linkScore struct {
		path  string
		score int
		total int // total significant words in the other note for percentage
	}
	var scored []linkScore

	for path, content := range b.notes {
		if path == b.currentPath {
			continue
		}
		otherWords := significantWords(content)
		shared := 0
		for w := range currentWords {
			if otherWords[w] {
				shared++
			}
		}
		if shared > 0 {
			total := len(otherWords)
			if total < 1 {
				total = 1
			}
			scored = append(scored, linkScore{path: path, score: shared, total: total})
		}
	}

	sort.Slice(scored, func(i, j int) bool { return scored[i].score > scored[j].score })
	if len(scored) > 5 {
		scored = scored[:5]
	}

	maxShared := 1
	if len(scored) > 0 && scored[0].score > 0 {
		maxShared = scored[0].score
	}

	noteName := strings.TrimSuffix(b.currentPath, ".md")
	var lines []string
	lines = append(lines, lipgloss.NewStyle().Foreground(text).Render(
		fmt.Sprintf("  Related notes for \"%s\":", noteName)))
	lines = append(lines, "")

	var resultLinks []string
	for _, ls := range scored {
		pct := ls.score * 100 / maxShared
		if pct > 99 {
			pct = 99
		}
		if pct < 5 {
			pct = 5
		}
		bullet := lipgloss.NewStyle().Foreground(green).Render("  \u25CF")
		name := lipgloss.NewStyle().Foreground(blue).Bold(true).Render(
			" " + strings.TrimSuffix(ls.path, ".md"))
		rel := DimStyle.Render(fmt.Sprintf("  (relevance: %d%%)", pct))
		lines = append(lines, bullet+name+rel)
		resultLinks = append(resultLinks, ls.path)
	}

	if len(scored) == 0 {
		lines = append(lines, DimStyle.Render("  No related notes found."))
	}

	b.resultLines = lines
	if len(resultLinks) > 0 {
		b.pendingResult = BotResult{
			Action:   "link",
			Links:    resultLinks,
			NotePath: b.currentPath,
		}
		b.resultReady = true
	}
}

// ── Summarizer ───────────────────────────────────────────────────────────

func (b *Bots) runSummarizer() {
	// Extract keywords from title + headings
	keywords := make(map[string]bool)
	titleWords := strings.Fields(strings.ToLower(strings.TrimSuffix(b.currentPath, ".md")))
	for _, w := range titleWords {
		w = stripPunctuation(w)
		if len(w) > 2 && !stopwords[w] {
			keywords[w] = true
		}
	}
	// Scan for headings
	for _, line := range strings.Split(b.currentBody, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			headingText := strings.TrimLeft(trimmed, "# ")
			for _, w := range strings.Fields(strings.ToLower(headingText)) {
				w = stripPunctuation(w)
				if len(w) > 2 && !stopwords[w] {
					keywords[w] = true
				}
			}
		}
	}

	// Split into paragraphs, pick first sentence of each
	paragraphs := splitParagraphs(b.currentBody)
	var candidates []string
	for _, para := range paragraphs {
		// Skip frontmatter lines
		if strings.HasPrefix(strings.TrimSpace(para), "---") {
			continue
		}
		// Skip heading-only paragraphs
		if strings.HasPrefix(strings.TrimSpace(para), "#") && !strings.Contains(para, "\n") {
			continue
		}
		sentence := firstSentence(para)
		if sentence == "" {
			continue
		}
		candidates = append(candidates, sentence)
	}

	// Score and pick sentences containing keywords
	type scored struct {
		sentence string
		score    int
		idx      int
	}
	var ss []scored
	for i, s := range candidates {
		sc := 0
		lower := strings.ToLower(s)
		for kw := range keywords {
			if strings.Contains(lower, kw) {
				sc++
			}
		}
		ss = append(ss, scored{sentence: s, score: sc, idx: i})
	}

	// Sort by score desc, then by original order
	sort.SliceStable(ss, func(i, j int) bool { return ss[i].score > ss[j].score })

	limit := 5
	if limit > len(ss) {
		limit = len(ss)
	}
	if limit < 3 && len(ss) >= 3 {
		limit = 3
	}
	selected := ss[:limit]

	// Re-sort by original order for coherence
	sort.Slice(selected, func(i, j int) bool { return selected[i].idx < selected[j].idx })

	var summaryParts []string
	for _, s := range selected {
		cleaned := stripMarkdown(s.sentence)
		if cleaned != "" {
			summaryParts = append(summaryParts, cleaned)
		}
	}

	summary := strings.Join(summaryParts, " ")

	var lines []string
	noteName := strings.TrimSuffix(b.currentPath, ".md")
	lines = append(lines, lipgloss.NewStyle().Foreground(text).Render(
		fmt.Sprintf("  Summary of \"%s\":", noteName)))
	lines = append(lines, "")

	if summary == "" {
		lines = append(lines, DimStyle.Render("  Note is too short to summarize."))
	} else {
		// Word-wrap the summary
		wrapped := wordWrap(summary, b.overlayInnerWidth()-4)
		for _, wl := range strings.Split(wrapped, "\n") {
			lines = append(lines, "  "+lipgloss.NewStyle().Foreground(text).Render(wl))
		}
	}

	b.resultLines = lines
	if summary != "" {
		b.pendingResult = BotResult{
			Action:   "summary",
			Summary:  summary,
			NotePath: b.currentPath,
		}
		b.resultReady = true
	}
}

// ── Question Bot ─────────────────────────────────────────────────────────

func (b *Bots) runQuestionBot() {
	query := strings.ToLower(strings.TrimSpace(b.questionInput))
	queryWords := make(map[string]bool)
	for _, w := range strings.Fields(query) {
		w = stripPunctuation(w)
		if len(w) > 2 && !stopwords[w] {
			queryWords[w] = true
		}
	}

	type match struct {
		path    string
		snippet string
		score   int
	}
	var matches []match

	for path, content := range b.notes {
		lines := strings.Split(content, "\n")
		bestScore := 0
		bestSnippet := ""
		for _, line := range lines {
			lineScore := 0
			lower := strings.ToLower(line)
			for qw := range queryWords {
				if strings.Contains(lower, qw) {
					lineScore++
				}
			}
			if lineScore > bestScore {
				bestScore = lineScore
				snippet := strings.TrimSpace(line)
				if len(snippet) > 120 {
					snippet = snippet[:117] + "..."
				}
				bestSnippet = snippet
			}
		}
		if bestScore > 0 {
			matches = append(matches, match{path: path, snippet: bestSnippet, score: bestScore})
		}
	}

	sort.Slice(matches, func(i, j int) bool { return matches[i].score > matches[j].score })
	if len(matches) > 5 {
		matches = matches[:5]
	}

	var lines []string
	lines = append(lines, lipgloss.NewStyle().Foreground(text).Render(
		fmt.Sprintf("  Results for: \"%s\"", b.questionInput)))
	lines = append(lines, "")

	if len(matches) == 0 {
		lines = append(lines, DimStyle.Render("  No matching notes found."))
	} else {
		for _, m := range matches {
			bullet := lipgloss.NewStyle().Foreground(green).Render("  \u25CF")
			name := lipgloss.NewStyle().Foreground(blue).Bold(true).Render(
				" " + strings.TrimSuffix(m.path, ".md"))
			lines = append(lines, bullet+name)
			if m.snippet != "" {
				snip := stripMarkdown(m.snippet)
				lines = append(lines, "    "+DimStyle.Render(snip))
			}
			lines = append(lines, "")
		}
	}

	b.resultLines = lines
	b.pendingResult = BotResult{Action: "none", NotePath: b.currentPath}
}

// ── Writing Assistant ────────────────────────────────────────────────────

func (b *Bots) runWritingAssistant() {
	var lines []string
	noteName := strings.TrimSuffix(b.currentPath, ".md")
	lines = append(lines, lipgloss.NewStyle().Foreground(text).Render(
		fmt.Sprintf("  Writing analysis for \"%s\":", noteName)))
	lines = append(lines, "")

	allWords := strings.Fields(b.currentBody)
	totalWords := len(allWords)
	paragraphs := splitParagraphs(b.currentBody)
	issueCount := 0

	warnIcon := lipgloss.NewStyle().Foreground(yellow).Render("  \u26A0")
	infoIcon := lipgloss.NewStyle().Foreground(blue).Render("  \u25CB")

	// 1. Check for very long paragraphs (>200 words)
	for i, para := range paragraphs {
		pWords := strings.Fields(para)
		if len(pWords) > 200 {
			lines = append(lines, warnIcon+" "+lipgloss.NewStyle().Foreground(yellow).Render(
				fmt.Sprintf("Paragraph %d is very long (%d words) — consider breaking it up.", i+1, len(pWords))))
			issueCount++
		}
	}

	// 2. Detect passive voice patterns
	passiveCount := 0
	for _, para := range paragraphs {
		words := strings.Fields(strings.ToLower(para))
		for i := 0; i < len(words)-1; i++ {
			aux := stripPunctuation(words[i])
			if aux == "was" || aux == "were" || aux == "been" {
				next := stripPunctuation(words[i+1])
				if looksLikePastParticiple(next) {
					passiveCount++
				}
			}
		}
	}
	if passiveCount > 0 {
		lines = append(lines, warnIcon+" "+lipgloss.NewStyle().Foreground(yellow).Render(
			fmt.Sprintf("Found %d possible passive voice construction(s).", passiveCount)))
		issueCount++
	}

	// 3. Find repeated words (same word >3 times in a paragraph)
	for i, para := range paragraphs {
		freq := make(map[string]int)
		for _, w := range strings.Fields(strings.ToLower(para)) {
			w = stripPunctuation(w)
			if len(w) > 3 && !stopwords[w] {
				freq[w]++
			}
		}
		for word, count := range freq {
			if count > 3 {
				lines = append(lines, warnIcon+" "+lipgloss.NewStyle().Foreground(yellow).Render(
					fmt.Sprintf("\"%s\" repeated %d times in paragraph %d.", word, count, i+1)))
				issueCount++
			}
		}
	}

	// 4. Suggest adding headings if note is long with none
	hasHeadings := false
	for _, line := range strings.Split(b.currentBody, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			hasHeadings = true
			break
		}
	}
	if totalWords > 500 && !hasHeadings {
		lines = append(lines, warnIcon+" "+lipgloss.NewStyle().Foreground(yellow).Render(
			fmt.Sprintf("Note has %d words but no headings — consider adding structure.", totalWords)))
		issueCount++
	}

	// 5. Readability metrics
	sentences := splitSentences(b.currentBody)
	sentenceCount := len(sentences)
	if sentenceCount < 1 {
		sentenceCount = 1
	}
	avgSentenceLen := float64(totalWords) / float64(sentenceCount)

	totalChars := 0
	wordCount := 0
	for _, w := range allWords {
		cleaned := stripPunctuation(strings.ToLower(w))
		if cleaned != "" {
			totalChars += len(cleaned)
			wordCount++
		}
	}
	avgWordLen := 0.0
	if wordCount > 0 {
		avgWordLen = float64(totalChars) / float64(wordCount)
	}

	lines = append(lines, "")
	lines = append(lines, lipgloss.NewStyle().Foreground(blue).Bold(true).Render("  Readability Metrics"))
	lines = append(lines, DimStyle.Render("  "+strings.Repeat("\u2500", 30)))
	lines = append(lines, infoIcon+" "+lipgloss.NewStyle().Foreground(text).Render(
		fmt.Sprintf("Total words: %d", totalWords)))
	lines = append(lines, infoIcon+" "+lipgloss.NewStyle().Foreground(text).Render(
		fmt.Sprintf("Sentences: %d", len(sentences))))
	lines = append(lines, infoIcon+" "+lipgloss.NewStyle().Foreground(text).Render(
		fmt.Sprintf("Avg sentence length: %.1f words", avgSentenceLen)))
	lines = append(lines, infoIcon+" "+lipgloss.NewStyle().Foreground(text).Render(
		fmt.Sprintf("Avg word length: %.1f chars", avgWordLen)))

	if issueCount == 0 {
		lines = append(lines, "")
		lines = append(lines, lipgloss.NewStyle().Foreground(green).Render(
			"  No issues found — looking good!"))
	}

	b.resultLines = lines
	b.pendingResult = BotResult{Action: "none", NotePath: b.currentPath}
}

// ── Daily Digest ─────────────────────────────────────────────────────────

func (b *Bots) runDailyDigest() {
	var lines []string

	sectionStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(text)
	numStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)

	totalNotes := len(b.notes)

	// Most linked notes (count how many times each note is referenced via [[link]])
	linkCounts := make(map[string]int)
	for _, content := range b.notes {
		// Count [[wikilinks]]
		rest := content
		for {
			start := strings.Index(rest, "[[")
			if start < 0 {
				break
			}
			rest = rest[start+2:]
			end := strings.Index(rest, "]]")
			if end < 0 {
				break
			}
			target := rest[:end]
			// Handle [[link|alias]]
			if pipe := strings.Index(target, "|"); pipe >= 0 {
				target = target[:pipe]
			}
			target = strings.TrimSpace(target)
			if target != "" {
				// Try with .md suffix
				if !strings.HasSuffix(target, ".md") {
					target = target + ".md"
				}
				linkCounts[target]++
			}
			rest = rest[end+2:]
		}
	}

	type entry struct {
		name  string
		count int
	}

	// Orphan notes — no incoming or outgoing links
	outgoing := make(map[string]bool)
	for path, content := range b.notes {
		rest := content
		hasOutgoing := false
		for {
			start := strings.Index(rest, "[[")
			if start < 0 {
				break
			}
			rest = rest[start+2:]
			end := strings.Index(rest, "]]")
			if end < 0 {
				break
			}
			hasOutgoing = true
			rest = rest[end+2:]
		}
		if hasOutgoing {
			outgoing[path] = true
		}
	}

	var orphans []string
	for path := range b.notes {
		if linkCounts[path] == 0 && !outgoing[path] {
			orphans = append(orphans, path)
		}
	}
	sort.Strings(orphans)

	// Top linked
	var topLinked []entry
	for path, count := range linkCounts {
		topLinked = append(topLinked, entry{name: path, count: count})
	}
	sort.Slice(topLinked, func(i, j int) bool { return topLinked[i].count > topLinked[j].count })
	if len(topLinked) > 5 {
		topLinked = topLinked[:5]
	}

	// Tag distribution
	type tagCount struct {
		tag   string
		count int
	}
	var tagDist []tagCount
	for tag, paths := range b.tags {
		tagDist = append(tagDist, tagCount{tag: tag, count: len(paths)})
	}
	sort.Slice(tagDist, func(i, j int) bool { return tagDist[i].count > tagDist[j].count })
	if len(tagDist) > 8 {
		tagDist = tagDist[:8]
	}

	// Build output
	lines = append(lines, sectionStyle.Render("  Vault Overview"))
	lines = append(lines, DimStyle.Render("  "+strings.Repeat("\u2500", 30)))
	lines = append(lines, labelStyle.Render("  Total notes: ")+numStyle.Render(smallNum(totalNotes)))
	lines = append(lines, labelStyle.Render("  Orphan notes: ")+numStyle.Render(smallNum(len(orphans))))
	lines = append(lines, "")

	// Most linked
	if len(topLinked) > 0 {
		lines = append(lines, sectionStyle.Render("  Most Linked Notes"))
		lines = append(lines, DimStyle.Render("  "+strings.Repeat("\u2500", 30)))
		for _, e := range topLinked {
			bullet := lipgloss.NewStyle().Foreground(green).Render("  \u25CF")
			name := lipgloss.NewStyle().Foreground(blue).Render(
				" " + strings.TrimSuffix(e.name, ".md"))
			count := DimStyle.Render(fmt.Sprintf(" (%d links)", e.count))
			lines = append(lines, bullet+name+count)
		}
		lines = append(lines, "")
	}

	// Orphans
	if len(orphans) > 0 {
		show := orphans
		if len(show) > 5 {
			show = show[:5]
		}
		lines = append(lines, sectionStyle.Render("  Orphan Notes (no links in or out)"))
		lines = append(lines, DimStyle.Render("  "+strings.Repeat("\u2500", 30)))
		for _, o := range show {
			bullet := lipgloss.NewStyle().Foreground(yellow).Render("  \u25CB")
			name := lipgloss.NewStyle().Foreground(text).Render(
				" " + strings.TrimSuffix(o, ".md"))
			lines = append(lines, bullet+name)
		}
		if len(orphans) > 5 {
			lines = append(lines, DimStyle.Render(
				fmt.Sprintf("  ... and %d more", len(orphans)-5)))
		}
		lines = append(lines, "")
	}

	// Tag distribution
	if len(tagDist) > 0 {
		lines = append(lines, sectionStyle.Render("  Tag Distribution"))
		lines = append(lines, DimStyle.Render("  "+strings.Repeat("\u2500", 30)))
		maxCount := tagDist[0].count
		if maxCount < 1 {
			maxCount = 1
		}
		for _, tc := range tagDist {
			barLen := tc.count * 15 / maxCount
			if barLen < 1 && tc.count > 0 {
				barLen = 1
			}
			bar := lipgloss.NewStyle().Foreground(mauve).Render(strings.Repeat("\u2588", barLen))
			rest := DimStyle.Render(strings.Repeat("\u2591", 15-barLen))
			tagPill := lipgloss.NewStyle().Foreground(crust).Background(blue).Render(" #" + tc.tag + " ")
			cnt := numStyle.Render(" " + smallNum(tc.count))
			lines = append(lines, "  "+tagPill+" "+bar+rest+cnt)
		}
	}

	b.resultLines = lines
	b.pendingResult = BotResult{Action: "none", NotePath: b.currentPath}
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

// View renders the Bots overlay.
func (b Bots) View() string {
	if !b.active {
		return ""
	}

	width := b.overlayWidth()

	switch b.state {
	case botsStateList:
		return b.viewList(width)
	case botsStateInput:
		return b.viewInput(width)
	case botsStateResults:
		return b.viewResults(width)
	}
	return ""
}

func (b Bots) overlayWidth() int {
	w := b.width / 2
	if w < 50 {
		w = 50
	}
	if w > 72 {
		w = 72
	}
	return w
}

func (b Bots) overlayInnerWidth() int {
	return b.overlayWidth() - 6 // border(2) + padding(2*2)
}

// ── Bot list view ────────────────────────────────────────────────────────

func (b Bots) viewList(width int) string {
	var buf strings.Builder

	title := lipgloss.NewStyle().
		Foreground(sapphire).
		Bold(true).
		Render("  \U0001F916 Granit Bots")
	buf.WriteString(title)
	buf.WriteString("\n")
	buf.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	buf.WriteString("\n\n")
	buf.WriteString(DimStyle.Render("  Select a bot:"))
	buf.WriteString("\n\n")

	for i, bd := range botList {
		icon := bd.icon
		name := bd.name
		desc := bd.desc

		if i == b.cursor {
			pointer := lipgloss.NewStyle().Foreground(sapphire).Bold(true).Render("  \u25B8 ")
			nameStyled := lipgloss.NewStyle().Foreground(peach).Bold(true).Render(icon + " " + name)
			buf.WriteString(lipgloss.NewStyle().
				Background(surface0).
				Width(width - 6).
				Render(pointer + nameStyled))
			buf.WriteString("\n")
			descStyled := lipgloss.NewStyle().
				Background(surface0).
				Foreground(overlay1).
				Width(width - 6).
				Render("      " + desc)
			buf.WriteString(descStyled)
		} else {
			nameStyled := lipgloss.NewStyle().Foreground(text).Render("    " + icon + " " + name)
			buf.WriteString(nameStyled)
			buf.WriteString("\n")
			buf.WriteString(DimStyle.Render("      " + desc))
		}

		if i < len(botList)-1 {
			buf.WriteString("\n\n")
		}
	}

	buf.WriteString("\n\n")
	buf.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", width-10)))
	buf.WriteString("\n")
	buf.WriteString(DimStyle.Render("  \u2191\u2193: select  Enter: run  Esc: close"))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(sapphire).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(buf.String())
}

// ── Question input view ──────────────────────────────────────────────────

func (b Bots) viewInput(width int) string {
	var buf strings.Builder

	title := lipgloss.NewStyle().
		Foreground(sapphire).
		Bold(true).
		Render("  \u2753 Question Bot")
	buf.WriteString(title)
	buf.WriteString("\n")
	buf.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	buf.WriteString("\n\n")

	buf.WriteString(lipgloss.NewStyle().Foreground(text).Render("  Ask a question about your notes:"))
	buf.WriteString("\n\n")

	prompt := SearchPromptStyle.Render("  > ")
	input := SearchInputStyle.Render(b.questionInput + "\u2588")
	buf.WriteString(prompt + input)

	buf.WriteString("\n\n")
	buf.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", width-10)))
	buf.WriteString("\n")
	buf.WriteString(DimStyle.Render("  Enter: search  Esc: back"))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(sapphire).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(buf.String())
}

// ── Results view ─────────────────────────────────────────────────────────

func (b Bots) viewResults(width int) string {
	var buf strings.Builder

	// Find bot info for the title
	bd := botList[0]
	for _, d := range botList {
		if d.kind == b.activeBot {
			bd = d
			break
		}
	}

	title := lipgloss.NewStyle().
		Foreground(sapphire).
		Bold(true).
		Render("  " + bd.icon + " " + bd.name + " Results")
	buf.WriteString(title)
	buf.WriteString("\n")
	buf.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	buf.WriteString("\n")

	// Scrollable result lines
	visH := b.height - 10
	if visH < 5 {
		visH = 5
	}

	maxScroll := len(b.resultLines) - visH
	if maxScroll < 0 {
		maxScroll = 0
	}
	scroll := b.scroll
	if scroll > maxScroll {
		scroll = maxScroll
	}
	end := scroll + visH
	if end > len(b.resultLines) {
		end = len(b.resultLines)
	}

	for i := scroll; i < end; i++ {
		buf.WriteString(b.resultLines[i])
		if i < end-1 {
			buf.WriteString("\n")
		}
	}

	buf.WriteString("\n\n")
	buf.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", width-10)))
	buf.WriteString("\n")

	// Footer depends on whether there's an actionable result
	if b.resultReady {
		actionHint := ""
		switch b.pendingResult.Action {
		case "tag":
			actionHint = "Enter: apply tags"
		case "link":
			actionHint = "Enter: insert link"
		case "summary":
			actionHint = "Enter: copy summary"
		}
		if actionHint != "" {
			buf.WriteString(DimStyle.Render("  " + actionHint + "  Esc: back"))
		} else {
			buf.WriteString(DimStyle.Render("  Esc: back"))
		}
	} else {
		buf.WriteString(DimStyle.Render("  j/k: scroll  Esc: back"))
	}

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(sapphire).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(buf.String())
}

// ---------------------------------------------------------------------------
// Text-processing helpers
// ---------------------------------------------------------------------------

// significantWords returns a set of lowercase words longer than 4 chars that
// are not stopwords.
func significantWords(content string) map[string]bool {
	result := make(map[string]bool)
	for _, w := range strings.Fields(strings.ToLower(content)) {
		w = stripPunctuation(w)
		if len(w) > 4 && !stopwords[w] {
			result[w] = true
		}
	}
	return result
}

// stripPunctuation removes leading/trailing punctuation from a word.
func stripPunctuation(w string) string {
	return strings.Trim(w, ".,;:!?\"'`()[]{}#*_~<>@/\\|+-=")
}

// splitParagraphs splits text on blank lines.
func splitParagraphs(text string) []string {
	raw := strings.Split(text, "\n\n")
	var result []string
	for _, p := range raw {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// firstSentence extracts the first sentence from a paragraph.
func firstSentence(para string) string {
	// Join all lines
	para = strings.ReplaceAll(para, "\n", " ")
	para = strings.TrimSpace(para)
	if para == "" {
		return ""
	}

	// Find sentence-ending punctuation
	for i, ch := range para {
		if (ch == '.' || ch == '!' || ch == '?') && i > 10 {
			return para[:i+1]
		}
	}
	// No punctuation found — take the whole thing if short, else truncate
	if len(para) <= 150 {
		return para
	}
	// Find a word boundary near 150
	idx := strings.LastIndex(para[:150], " ")
	if idx < 50 {
		idx = 150
	}
	return para[:idx] + "..."
}

// stripMarkdown removes common markdown formatting.
func stripMarkdown(s string) string {
	// Remove headers
	for strings.HasPrefix(s, "#") {
		s = strings.TrimLeft(s, "# ")
	}
	// Remove bold/italic markers
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "__", "")
	s = strings.ReplaceAll(s, "*", "")
	s = strings.ReplaceAll(s, "_", " ")
	// Remove inline code
	s = strings.ReplaceAll(s, "`", "")
	// Remove wiki links — [[link|text]] -> text, [[link]] -> link
	for {
		start := strings.Index(s, "[[")
		if start < 0 {
			break
		}
		end := strings.Index(s[start:], "]]")
		if end < 0 {
			break
		}
		inner := s[start+2 : start+end]
		if pipe := strings.Index(inner, "|"); pipe >= 0 {
			inner = inner[pipe+1:]
		}
		s = s[:start] + inner + s[start+end+2:]
	}
	// Remove markdown links [text](url)
	for {
		start := strings.Index(s, "[")
		if start < 0 {
			break
		}
		mid := strings.Index(s[start:], "](")
		if mid < 0 {
			break
		}
		end := strings.Index(s[start+mid:], ")")
		if end < 0 {
			break
		}
		linkText := s[start+1 : start+mid]
		s = s[:start] + linkText + s[start+mid+end+1:]
	}
	return strings.TrimSpace(s)
}

// splitSentences splits text into rough sentence chunks.
func splitSentences(text string) []string {
	text = strings.ReplaceAll(text, "\n", " ")
	var sentences []string
	current := strings.Builder{}
	for _, ch := range text {
		current.WriteRune(ch)
		if ch == '.' || ch == '!' || ch == '?' {
			s := strings.TrimSpace(current.String())
			if s != "" {
				sentences = append(sentences, s)
			}
			current.Reset()
		}
	}
	// Remaining text as a sentence
	s := strings.TrimSpace(current.String())
	if s != "" {
		sentences = append(sentences, s)
	}
	return sentences
}

// wordWrap wraps text to a given width at word boundaries.
func wordWrap(text string, width int) string {
	if width < 10 {
		width = 10
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return ""
	}

	var lines []string
	current := words[0]

	for _, w := range words[1:] {
		if len(current)+1+len(w) > width {
			lines = append(lines, current)
			current = w
		} else {
			current += " " + w
		}
	}
	lines = append(lines, current)
	return strings.Join(lines, "\n")
}

