package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// listPrefixRegex matches common list item prefixes: "1.", "1)", "1:", "- ", "* ", "• "
var listPrefixRegex = regexp.MustCompile(`^(\d+[.):\-]|[-*•])\s+`)

// stripListPrefix removes common list item markers from the start of a line.
func stripListPrefix(line string) string {
	return listPrefixRegex.ReplaceAllString(strings.TrimSpace(line), "")
}

// ---------------------------------------------------------------------------
// BotResult – returned to the caller after a bot finishes
// ---------------------------------------------------------------------------

type BotResult struct {
	Action   string   // "tag", "link", "summary", "none"
	Tags     []string // suggested tags
	Links    []string // suggested links (note paths)
	Summary  string   // generated summary
	NotePath string   // which note this applies to
}

// botAIResultMsg carries the AI response back to Update (unified for all providers).
type botAIResultMsg struct {
	response string
	err      error
	botKind  botKind
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
	botTitleSuggester
	botActionItems
	botMOCGenerator
	botAutoLinker
	botFlashcardGen
	botToneAdjuster
	botOutliner
	botExplainSimple
	botKeyTerms
	botCounterArgument
	botTLDR
	botProsCons
	botExpand
)

type botDescriptor struct {
	kind     botKind
	icon     string
	name     string
	desc     string
	category string
}

// Bot categories are shown as section headers in the list view.
const (
	catSummarize = "Summarize"
	catWriting   = "Writing"
	catAnalysis  = "Analysis"
	catOrganize  = "Organize"
	catLearning  = "Learning"
	catVault     = "Vault"
)

// categoryOrder controls the display order of category sections.
var categoryOrder = []string{catSummarize, catWriting, catAnalysis, catOrganize, catLearning, catVault}

var botList = []botDescriptor{
	// Summarize
	{botTLDR, IconFileChar, "TL;DR", "One-sentence summary of the note (AI)", catSummarize},
	{botSummarizer, IconFileChar, "Summarizer", "Create a brief summary (AI)", catSummarize},
	{botExplainSimple, IconBookmarkChar, "Explain Simply", "Explain the note like I'm 5 (AI)", catSummarize},

	// Writing
	{botTitleSuggester, IconEditChar, "Title Suggester", "Suggest better titles (AI)", catWriting},
	{botWritingAssistant, IconBookmarkChar, "Writing Assistant", "Suggest improvements (AI)", catWriting},
	{botToneAdjuster, IconEditChar, "Tone Adjuster", "Rewrite selection as formal/casual/concise (AI)", catWriting},
	{botExpand, IconEditChar, "Expand", "Flesh out a terse note with more detail (AI)", catWriting},

	// Analysis
	{botQuestionBot, IconSearchChar, "Question Bot", "Ask questions about your notes (AI)", catAnalysis},
	{botCounterArgument, IconSearchChar, "Counter-Argument", "Generate opposing viewpoints to sharpen thinking (AI)", catAnalysis},
	{botProsCons, IconOutlineChar, "Pros & Cons", "Generate a pros/cons list for decision-making (AI)", catAnalysis},
	{botActionItems, IconOutlineChar, "Action Items", "Extract todos & action items (AI)", catAnalysis},

	// Organize
	{botAutoTagger, IconTagChar, "Auto-Tagger", "Suggest tags for current note (AI)", catOrganize},
	{botLinkSuggester, IconSearchChar, "Link Suggester", "Find related notes (AI)", catOrganize},
	{botAutoLinker, IconSearchChar, "Auto-Link", "Find and suggest [[wikilinks]] to insert (AI)", catOrganize},
	{botOutliner, IconOutlineChar, "Outline Generator", "Build a structured outline from note (AI)", catOrganize},

	// Learning
	{botFlashcardGen, IconBookmarkChar, "Flashcard Generator", "Generate Q&A flashcards from note (AI)", catLearning},
	{botKeyTerms, IconTagChar, "Key Terms", "Extract glossary of key terms & definitions (AI)", catLearning},

	// Vault
	{botMOCGenerator, IconGraphChar, "MOC Generator", "Create a Map of Content (AI)", catVault},
	{botDailyDigest, IconCalendarChar, "Daily Digest", "Summarize vault activity", catVault},
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

// Topic keywords used by the local Auto-Tagger fallback
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

// Passive voice helper
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
	botsStateLoading                  // waiting for Ollama
	botsStateResults                  // showing results
)

type Bots struct {
	OverlayBase

	state  botsState
	cursor int
	scroll int

	// Type-ahead filter for the bot list
	filter string

	// Vault data
	notes       map[string]string
	tags        map[string][]string
	currentPath string
	currentBody string

	// Question Bot input
	questionInput string

	// Results
	resultLines   []string
	resultReady   bool
	pendingResult BotResult
	// Raw AI response text, used for clipboard copy / save-to-note actions.
	rawResponse string
	// Transient status message shown in the results footer (e.g. "Copied!")
	statusMsg string

	// Which bot was run
	activeBot botKind
	// Most recently used bot, used to prefill the cursor on next open.
	lastBot botKind
	lastBotSet bool

	// Vault root (used by save-to-note)
	vaultRoot string

	// AI config
	ai AIConfig

	// Loading animation
	loadingTick  int
	loadingStart time.Time
	// Elapsed time of the last completed request (ms), shown in results header.
	lastElapsed time.Duration
}

func NewBots() Bots {
	return Bots{
		ai: AIConfig{
			Provider:  "local",
			Model:     "qwen2.5:0.5b",
			OllamaURL: "http://localhost:11434",
		},
	}
}

func (b *Bots) SetAIConfig(cfg AIConfig) {
	b.ai = cfg
	if b.ai.Provider == "" {
		b.ai.Provider = "local"
	}
}

// SetVaultRoot provides the vault root so save-to-note actions know where
// to write the generated file.
func (b *Bots) SetVaultRoot(root string) {
	b.vaultRoot = root
}

func (b *Bots) Open() {
	b.Activate()
	b.state = botsStateList
	b.cursor = 0
	b.scroll = 0
	b.filter = ""
	b.resultLines = nil
	// Restore cursor on the most recently used bot.
	if b.lastBotSet {
		for i, bd := range botList {
			if bd.kind == b.lastBot {
				b.cursor = i
				break
			}
		}
	}
	b.resultReady = false
	b.pendingResult = BotResult{}
	b.questionInput = ""
	b.loadingTick = 0
}


func (b *Bots) SetVaultData(notes map[string]string, tags map[string][]string) {
	b.notes = notes
	b.tags = tags
}

func (b *Bots) SetCurrentNote(path, content string) {
	b.currentPath = path
	b.currentBody = content
}

func (b *Bots) GetResult() BotResult {
	r := b.pendingResult
	b.pendingResult = BotResult{}
	b.resultReady = false
	return r
}

// ---------------------------------------------------------------------------
// AI call (unified via AIConfig.Chat)
// ---------------------------------------------------------------------------

// botSystemPrompt returns a role-specific system prompt for the given bot.
// Small models get a minimal version that emphasizes format compliance.
func botSystemPrompt(kind botKind, small bool) string {
	if small {
		switch kind {
		case botAutoTagger:
			return "You are a tagger. Output only a comma-separated list of lowercase tags."
		case botTitleSuggester:
			return "You are a title generator. Output only titles, one per line."
		case botActionItems:
			return "You are an action-item extractor. Output only markdown tasks (- [ ] ...)."
		case botSummarizer:
			return "You are a summarizer. Output only the summary, no preamble."
		case botFlashcardGen:
			return "You are a flashcard generator. Output only Q:/A: pairs."
		case botToneAdjuster:
			return "You are a rewriter. Output only the rewritten versions with FORMAL/CASUAL/CONCISE labels."
		case botOutliner:
			return "You are an outliner. Output only markdown headings and bullets. No preamble."
		case botExplainSimple:
			return "Explain simply for a 12-year-old. Short sentences, no jargon."
		case botKeyTerms:
			return "Extract key terms. Output only TERM: definition pairs, one per line."
		case botCounterArgument:
			return "Generate counter-arguments. Output only CLAIM/COUNTER pairs."
		case botTLDR:
			return "Output one sentence. No preamble, no quotes."
		case botProsCons:
			return "Output only PROS: and CONS: sections with bullet points."
		case botExpand:
			return "Expand the note with detail. Output markdown only, no preamble."
		}
		return "Follow the format exactly. Be concise. No explanations."
	}
	switch kind {
	case botAutoTagger:
		return "You are a tagging assistant for a personal knowledge base. Suggest concise, reusable tags. Follow the requested output format exactly."
	case botLinkSuggester, botAutoLinker:
		return "You are a note linking assistant. Identify strong conceptual matches between notes. Follow the requested output format exactly."
	case botSummarizer:
		return "You are a summarization assistant. Capture key ideas faithfully and concisely."
	case botQuestionBot:
		return "You are a knowledge assistant. Answer from the provided notes only. If the answer isn't in the notes, say so."
	case botWritingAssistant:
		return "You are a writing coach. Provide specific, actionable feedback."
	case botTitleSuggester:
		return "You are a title generator. Create clear, descriptive titles. Follow the requested output format exactly."
	case botActionItems:
		return "You are an action-item extractor. Return concrete tasks in markdown checkbox format."
	case botMOCGenerator:
		return "You are an information architect. Create well-organized Maps of Content."
	case botFlashcardGen:
		return "You are a flashcard generator focused on durable, specific Q/A pairs."
	case botToneAdjuster:
		return "You are a rewriting assistant. Preserve meaning, only change style."
	case botOutliner:
		return "You are an information architect. Produce a clean hierarchical outline using markdown headings and bullets."
	case botExplainSimple:
		return "You are a science communicator. Explain complex ideas using simple words and everyday analogies."
	case botKeyTerms:
		return "You are a glossary builder. Extract key terms and write concise, accurate definitions from the note's own context."
	case botCounterArgument:
		return "You are a devil's advocate. Surface strong, intellectually honest counter-arguments to sharpen the author's thinking."
	case botTLDR:
		return "You are a summarization assistant producing single-sentence TL;DRs. Capture the single most important idea, nothing more."
	case botProsCons:
		return "You are a decision analysis assistant. Produce balanced, specific pros and cons lists."
	case botExpand:
		return "You are an expansion assistant. Flesh out terse notes with useful detail while preserving the author's voice and structure."
	}
	return "You are a helpful note-taking assistant. Be concise and actionable. Follow the requested output format exactly."
}

func callAIForBot(ai AIConfig, prompt string, kind botKind) tea.Cmd {
	return func() tea.Msg {
		systemPrompt := botSystemPrompt(kind, ai.IsSmallModel())
		// Hard 3-minute cap per bot request so a stuck model doesn't leave
		// the loading overlay spinning forever.
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		resp, err := ai.ChatCtx(ctx, systemPrompt, prompt)
		return botAIResultMsg{response: resp, err: err, botKind: kind}
	}
}

// ---------------------------------------------------------------------------
// Prompt builders
// ---------------------------------------------------------------------------

func (b *Bots) buildAutoTaggerPrompt() string {
	noteName := strings.TrimSuffix(b.currentPath, ".md")

	// Count how many notes use each tag to help AI understand the taxonomy.
	tagCounts := make(map[string]int)
	for _, body := range b.notes {
		for _, tag := range extractFrontmatterTags(body) {
			tagCounts[tag]++
		}
	}

	existingTags := make([]string, 0, len(b.tags))
	for t := range b.tags {
		if c, ok := tagCounts[t]; ok && c > 1 {
			existingTags = append(existingTags, fmt.Sprintf("%s (%d notes)", t, c))
		} else {
			existingTags = append(existingTags, t)
		}
	}
	sort.Strings(existingTags)
	tagList := strings.Join(existingTags, ", ")
	if tagList == "" {
		tagList = "(none)"
	}

	content := b.currentBody
	maxContent := 2000
	if b.ai.IsSmallModel() {
		maxContent = 800
	}
	content = truncateAtBoundary(content, maxContent)

	// Find 2-3 similar notes by shared words to provide few-shot examples.
	examples := ""
	type noteScore struct {
		path string
		tags []string
		score int
	}
	currentWords := make(map[string]bool)
	for _, w := range strings.Fields(strings.ToLower(noteName)) {
		if len(w) > 3 {
			currentWords[w] = true
		}
	}
	var scored []noteScore
	for path, body := range b.notes {
		if path == b.currentPath {
			continue
		}
		tags := extractFrontmatterTags(body)
		if len(tags) == 0 {
			continue
		}
		score := 0
		lower := strings.ToLower(filepath.Base(path))
		for w := range currentWords {
			if strings.Contains(lower, w) {
				score += 2
			}
		}
		for _, tag := range tags {
			for w := range currentWords {
				if strings.Contains(strings.ToLower(tag), w) {
					score++
				}
			}
		}
		if score > 0 {
			scored = append(scored, noteScore{path, tags, score})
		}
	}
	sort.Slice(scored, func(i, j int) bool { return scored[i].score > scored[j].score })
	if len(scored) > 3 {
		scored = scored[:3]
	}
	if len(scored) > 0 {
		var exLines []string
		for _, s := range scored {
			name := strings.TrimSuffix(filepath.Base(s.path), ".md")
			exLines = append(exLines, fmt.Sprintf("  %s -> %s", name, strings.Join(s.tags, ", ")))
		}
		examples = "\n\nExamples of how similar notes are tagged:\n" + strings.Join(exLines, "\n")
	}

	return fmt.Sprintf(`You are a note tagging assistant. Suggest 3-5 tags for this note. Prefer existing tags from the vault when they fit.

Note title: %s
Existing tags in vault: %s%s

Note content:
---
%s
---

Rules:
- Use existing vault tags when possible (reduces tag sprawl)
- Tags should be specific and descriptive (not generic like "note" or "misc")
- Lowercase, no # prefix

Respond with ONLY a comma-separated list of tags.`, noteName, tagList, examples, content)
}

func (b *Bots) buildLinkSuggesterPrompt() string {
	noteName := strings.TrimSuffix(b.currentPath, ".md")
	content := b.currentBody
	maxContent := 1500
	if b.ai.IsSmallModel() {
		maxContent = 600
	}
	content = truncateAtBoundary(content, maxContent)

	// Build a brief index of other notes — include all titles, with
	// content previews for the first N to give the AI enough context.
	maxPreviews := 30
	if b.ai.IsSmallModel() {
		maxPreviews = 10
	}
	var noteIndex strings.Builder
	count := 0
	for path, body := range b.notes {
		if path == b.currentPath {
			continue
		}
		name := strings.TrimSuffix(path, ".md")
		if count < maxPreviews {
			preview := body
			if len(preview) > 200 {
				preview = preview[:200]
			}
			preview = strings.ReplaceAll(preview, "\n", " ")
			noteIndex.WriteString(fmt.Sprintf("- %s: %s\n", name, preview))
		} else {
			noteIndex.WriteString(fmt.Sprintf("- %s\n", name))
		}
		count++
	}

	return fmt.Sprintf(`You are a note linking assistant. Given the current note and a list of other notes in the vault, suggest 3-5 notes that are most related and should be linked.

Current note: %s
Content:
---
%s
---

Other notes in vault:
%s

Respond with ONLY the note names (one per line) that should be linked to the current note, with a brief reason. Format: "note-name - reason"`, noteName, content, noteIndex.String())
}

func (b *Bots) buildSummarizerPrompt() string {
	noteName := strings.TrimSuffix(b.currentPath, ".md")
	content := b.currentBody
	maxContent := 3000
	if b.ai.IsSmallModel() {
		maxContent = 1200
	}
	content = truncateAtBoundary(content, maxContent)

	// Extract metadata for better grounding
	tags := extractFrontmatterTags(b.currentBody)
	tagLine := ""
	if len(tags) > 0 {
		tagLine = fmt.Sprintf("\nTags: %s", strings.Join(tags, ", "))
	}
	folder := ""
	if dir := filepath.Dir(b.currentPath); dir != "." && dir != "" {
		folder = fmt.Sprintf("\nFolder: %s", dir)
	}

	return fmt.Sprintf(`Summarize the following note in 2-4 concise sentences. Focus on the key ideas and actionable points.

Note: %s%s%s
---
%s
---

Provide ONLY the summary, no preamble.`, noteName, tagLine, folder, content)
}

func (b *Bots) buildQuestionPrompt() string {
	// Build context from all notes (trimmed)
	maxTotal := 6000
	maxPerNote := 500
	if b.ai.IsSmallModel() {
		maxTotal = 2000
		maxPerNote = 200
	}
	// Sort paths for deterministic iteration order.
	paths := make([]string, 0, len(b.notes))
	for p := range b.notes {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	var context strings.Builder
	totalLen := 0
	for _, path := range paths {
		if totalLen > maxTotal {
			break
		}
		body := b.notes[path]
		name := strings.TrimSuffix(path, ".md")
		preview := truncateAtBoundary(body, maxPerNote)
		context.WriteString(fmt.Sprintf("## %s\n%s\n\n", name, preview))
		totalLen += len(preview)
	}

	return fmt.Sprintf(`You are a knowledge assistant. Answer the following question based on the user's notes.

Question: %s

Notes context:
%s

Answer concisely based on what's in the notes. If the answer isn't in the notes, say so.`, b.questionInput, context.String())
}

func (b *Bots) buildWritingAssistantPrompt() string {
	noteName := strings.TrimSuffix(b.currentPath, ".md")
	content := b.currentBody
	maxContent := 3000
	if b.ai.IsSmallModel() {
		maxContent = 1200
	}
	content = truncateAtBoundary(content, maxContent)

	return fmt.Sprintf(`Analyze the following note for writing quality. Provide:
1. A brief readability assessment (word count, sentence structure)
2. 3-5 specific suggestions to improve clarity, structure, or style
3. Any issues like passive voice, repetition, or missing structure

Note: %s
---
%s
---

Be concise and actionable.`, noteName, content)
}

func (b *Bots) buildTitleSuggesterPrompt() string {
	noteName := strings.TrimSuffix(filepath.Base(b.currentPath), ".md")
	content := b.currentBody
	maxContent := 2000
	if b.ai.IsSmallModel() {
		maxContent = 800
	}
	content = truncateAtBoundary(content, maxContent)
	tags := extractFrontmatterTags(b.currentBody)
	tagLine := ""
	if len(tags) > 0 {
		tagLine = "\nTags: " + strings.Join(tags, ", ")
	}

	return fmt.Sprintf(`Suggest 5 alternative titles for this note. Current title: "%s"%s

Content:
---
%s
---

Rules:
- Clear, descriptive, and concise (3-8 words)
- Reflect the main topic, not just a detail
- Use title case

Respond with ONLY 5 titles, one per line.`, noteName, tagLine, content)
}

func (b *Bots) buildActionItemsPrompt() string {
	noteName := strings.TrimSuffix(filepath.Base(b.currentPath), ".md")
	content := b.currentBody
	maxContent := 3000
	if b.ai.IsSmallModel() {
		maxContent = 1200
	}
	content = truncateAtBoundary(content, maxContent)

	return fmt.Sprintf(`Extract ALL action items from this meeting note / document. Include:
- Explicit todos (checkboxes, "need to", "should", "will")
- Implicit actions (decisions that require follow-up)
- Deadlines mentioned

Note: %s
---
%s
---

Format each as a markdown task:
- [ ] Action description @person (if mentioned) by date (if mentioned)

List ONLY the action items.`, noteName, content)
}

func (b *Bots) buildMOCGeneratorPrompt() string {
	var noteIndex strings.Builder
	maxNotes := 30
	if b.ai.IsSmallModel() {
		maxNotes = 15
	}
	// Sort paths for deterministic output.
	paths := make([]string, 0, len(b.notes))
	for p := range b.notes {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	count := 0
	for _, path := range paths {
		if count >= maxNotes {
			break
		}
		body := b.notes[path]
		name := strings.TrimSuffix(path, ".md")
		preview := truncateAtBoundary(body, 150)
		preview = strings.ReplaceAll(preview, "\n", " ")
		noteIndex.WriteString(fmt.Sprintf("- %s: %s\n", name, preview))
		count++
	}

	return fmt.Sprintf(`Create a Map of Content (MOC) for this vault. Group the notes into logical categories and create a structured index with wiki-links.

Notes in vault:
%s

Generate a MOC in markdown format using [[wiki-links]] to link to notes. Group notes into 3-6 categories with headings.`, noteIndex.String())
}

// ---------------------------------------------------------------------------
// Loading tick message
// ---------------------------------------------------------------------------

type botsTickMsg struct{}

func (b *Bots) buildAutoLinkerPrompt() string {
	content := b.currentBody
	maxContent := 2000
	if b.ai.IsSmallModel() {
		maxContent = 800
	}
	content = truncateAtBoundary(content, maxContent)

	// Build list of existing note names for linking
	var noteNames []string
	for path := range b.notes {
		name := strings.TrimSuffix(filepath.Base(path), ".md")
		if name != strings.TrimSuffix(filepath.Base(b.currentPath), ".md") {
			noteNames = append(noteNames, name)
		}
	}
	sort.Strings(noteNames)
	if len(noteNames) > 50 {
		noteNames = noteNames[:50]
	}

	return fmt.Sprintf(`Find concepts in this note that match existing notes and should be linked with [[wikilinks]].

Available notes to link to:
%s

Note content:
---
%s
---

For each match, output:
LINK: original text -> [[Note Name]]

Only suggest links where the concept clearly matches. Output ONLY the LINK lines.`, strings.Join(noteNames, ", "), content)
}

func (b *Bots) buildFlashcardPrompt() string {
	noteName := strings.TrimSuffix(filepath.Base(b.currentPath), ".md")
	content := b.currentBody
	maxContent := 2500
	if b.ai.IsSmallModel() {
		maxContent = 1000
	}
	content = truncateAtBoundary(content, maxContent)

	return fmt.Sprintf(`Generate 5-8 flashcards (question and answer pairs) from this note. Focus on key facts, definitions, and concepts worth remembering.

Note: %s
---
%s
---

Format each as:
Q: question
A: answer

Make questions specific and answers concise (1-2 sentences).`, noteName, content)
}

func (b *Bots) buildToneAdjusterPrompt() string {
	content := b.currentBody
	maxContent := 2000
	if b.ai.IsSmallModel() {
		maxContent = 800
	}
	content = truncateAtBoundary(content, maxContent)

	return fmt.Sprintf(`Rewrite the following text in three different tones. Keep the same meaning and information.

Original:
---
%s
---

Provide three versions:

FORMAL:
(rewritten in professional, formal tone)

CASUAL:
(rewritten in friendly, conversational tone)

CONCISE:
(rewritten as briefly as possible, bullet points if appropriate)`, content)
}

func (b *Bots) buildOutlinerPrompt() string {
	noteName := strings.TrimSuffix(filepath.Base(b.currentPath), ".md")
	content := b.currentBody
	maxContent := 3000
	if b.ai.IsSmallModel() {
		maxContent = 1200
	}
	content = truncateAtBoundary(content, maxContent)

	return fmt.Sprintf(`Generate a structured hierarchical outline for this note. Use markdown headings (##, ###) and bullet points.

Note: %s
---
%s
---

Rules:
- Start with 2-5 top-level sections (## headings)
- Add 2-4 sub-points per section as bullet points
- Keep each point short (1 line)
- Cover the main ideas, not every detail

Output ONLY the outline in markdown, no preamble.`, noteName, content)
}

func (b *Bots) buildExplainSimplePrompt() string {
	noteName := strings.TrimSuffix(filepath.Base(b.currentPath), ".md")
	content := b.currentBody
	maxContent := 2500
	if b.ai.IsSmallModel() {
		maxContent = 1000
	}
	content = truncateAtBoundary(content, maxContent)

	return fmt.Sprintf(`Explain the following note as if to a curious 12-year-old. Use simple words, everyday analogies, and short sentences. Avoid jargon.

Note: %s
---
%s
---

Write 3-5 short paragraphs. Start directly with the explanation, no preamble.`, noteName, content)
}

func (b *Bots) buildKeyTermsPrompt() string {
	noteName := strings.TrimSuffix(filepath.Base(b.currentPath), ".md")
	content := b.currentBody
	maxContent := 2500
	if b.ai.IsSmallModel() {
		maxContent = 1000
	}
	content = truncateAtBoundary(content, maxContent)

	return fmt.Sprintf(`Extract the key terms from this note and write a brief definition for each. Focus on domain-specific terms, proper nouns, and concepts that might need clarification.

Note: %s
---
%s
---

Format each as:
TERM: Brief 1-sentence definition grounded in the note's context.

Output 5-10 entries. Only entries that are actually in the note.`, noteName, content)
}

func (b *Bots) buildTLDRPrompt() string {
	noteName := strings.TrimSuffix(filepath.Base(b.currentPath), ".md")
	content := b.currentBody
	maxContent := 3000
	if b.ai.IsSmallModel() {
		maxContent = 1200
	}
	content = truncateAtBoundary(content, maxContent)

	return fmt.Sprintf(`Write a single-sentence TL;DR for this note. It should be the shortest possible capture of the most important idea.

Note: %s
---
%s
---

Output ONLY the one-sentence summary. No preamble, no quotes, no explanation.`, noteName, content)
}

func (b *Bots) buildProsConsPrompt() string {
	noteName := strings.TrimSuffix(filepath.Base(b.currentPath), ".md")
	content := b.currentBody
	maxContent := 2500
	if b.ai.IsSmallModel() {
		maxContent = 1000
	}
	content = truncateAtBoundary(content, maxContent)

	return fmt.Sprintf(`Generate a pros and cons list for the topic of this note. Identify the main decision or option being discussed and list its benefits and drawbacks.

Note: %s
---
%s
---

Format:
PROS:
- pro 1
- pro 2
- pro 3

CONS:
- con 1
- con 2
- con 3

Aim for 3-5 items per side. Be specific, not generic.`, noteName, content)
}

func (b *Bots) buildExpandPrompt() string {
	noteName := strings.TrimSuffix(filepath.Base(b.currentPath), ".md")
	content := b.currentBody
	maxContent := 2000
	if b.ai.IsSmallModel() {
		maxContent = 800
	}
	content = truncateAtBoundary(content, maxContent)

	return fmt.Sprintf(`Take this terse or incomplete note and expand it with additional detail, context, and examples. Preserve the author's voice and the original structure. Do not add information that contradicts what's already written.

Note: %s
---
%s
---

Output the expanded version as markdown. No preamble.`, noteName, content)
}

func (b *Bots) buildCounterArgumentPrompt() string {
	noteName := strings.TrimSuffix(filepath.Base(b.currentPath), ".md")
	content := b.currentBody
	maxContent := 2500
	if b.ai.IsSmallModel() {
		maxContent = 1000
	}
	content = truncateAtBoundary(content, maxContent)

	return fmt.Sprintf(`Read the following note and generate 3-5 strong counter-arguments or opposing viewpoints to its main claims. Help the author sharpen their thinking by surfacing blind spots.

Note: %s
---
%s
---

Format each as:
- CLAIM: [the original claim being challenged]
  COUNTER: [the opposing viewpoint in 1-2 sentences]

Be intellectually honest. Prefer strong counters over weak ones.`, noteName, content)
}

func botsTickCmd() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
		return botsTickMsg{}
	})
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (b Bots) Update(msg tea.Msg) (Bots, tea.Cmd) {
	if !b.active {
		return b, nil
	}

	switch msg := msg.(type) {
	case botsTickMsg:
		if b.state == botsStateLoading {
			b.loadingTick++
			return b, botsTickCmd()
		}

	case botAIResultMsg:
		if b.state != botsStateLoading {
			return b, nil
		}
		if msg.err != nil {
			var lines []string
			lines = append(lines, lipgloss.NewStyle().Foreground(yellow).Render(
				"  "+IconBookmarkChar+" AI unavailable, using local analysis"))
			lines = append(lines, DimStyle.Render("  "+msg.err.Error()))
			lines = append(lines, "")
			b.runLocalBot()
			b.resultLines = append(lines, b.resultLines...)
			b.state = botsStateResults
			return b, nil
		}
		// If the AI returned an empty response, fall back to local analysis
		// with a clear note so the user knows what happened.
		if strings.TrimSpace(msg.response) == "" {
			var lines []string
			lines = append(lines, lipgloss.NewStyle().Foreground(yellow).Render(
				"  "+IconBookmarkChar+" AI returned an empty response — using local analysis"))
			lines = append(lines, "")
			b.runLocalBot()
			b.resultLines = append(lines, b.resultLines...)
			b.state = botsStateResults
			return b, nil
		}
		providerLabel := b.ai.Provider + ": " + b.ai.Model
		b.lastElapsed = time.Since(b.loadingStart)
		b.rawResponse = msg.response
		b.processAIResponse(msg.response, providerLabel)
		b.state = botsStateResults
		return b, nil

	case tea.KeyMsg:
		switch b.state {
		case botsStateList:
			return b.updateList(msg)
		case botsStateInput:
			return b.updateInput(msg)
		case botsStateLoading:
			if msg.String() == "esc" {
				b.state = botsStateList
				return b, nil
			}
		case botsStateResults:
			return b.updateResults(msg)
		}
	}
	return b, nil
}

// filteredBots returns the bots matching the current filter string.
// An empty filter returns the full list.
func (b Bots) filteredBots() []botDescriptor {
	if b.filter == "" {
		return botList
	}
	needle := strings.ToLower(b.filter)
	var out []botDescriptor
	for _, bd := range botList {
		if strings.Contains(strings.ToLower(bd.name), needle) ||
			strings.Contains(strings.ToLower(bd.desc), needle) {
			out = append(out, bd)
		}
	}
	return out
}

func (b Bots) updateList(msg tea.KeyMsg) (Bots, tea.Cmd) {
	visible := b.filteredBots()
	key := msg.String()
	switch key {
	case "esc":
		if b.filter != "" {
			b.filter = ""
			b.cursor = 0
			return b, nil
		}
		b.active = false
	case "up", "k":
		if b.cursor > 0 {
			b.cursor--
		} else if len(visible) > 0 {
			// Wrap to last item.
			b.cursor = len(visible) - 1
		}
	case "down", "j":
		if b.cursor < len(visible)-1 {
			b.cursor++
		} else if len(visible) > 0 {
			// Wrap to first item.
			b.cursor = 0
		}
	case "home":
		b.cursor = 0
	case "end":
		if len(visible) > 0 {
			b.cursor = len(visible) - 1
		}
	case "enter":
		if b.cursor >= 0 && b.cursor < len(visible) {
			b.activeBot = visible[b.cursor].kind
			b.lastBot = b.activeBot
			b.lastBotSet = true
			if b.activeBot == botQuestionBot {
				b.state = botsStateInput
				b.questionInput = ""
			} else {
				return b.startBot()
			}
		}
	case "backspace":
		if len(b.filter) > 0 {
			runes := []rune(b.filter)
			b.filter = string(runes[:len(runes)-1])
			b.cursor = 0
		}
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		// Number-key hotkeys: only when not filtering (digits are rare in
		// bot names, and we want digits to be usable as filter input too,
		// but prefer hotkey behavior when filter is empty).
		if b.filter == "" {
			idx := int(key[0] - '1')
			if idx >= 0 && idx < len(visible) {
				b.activeBot = visible[idx].kind
				b.lastBot = b.activeBot
				b.lastBotSet = true
				if b.activeBot == botQuestionBot {
					b.state = botsStateInput
					b.questionInput = ""
					return b, nil
				}
				return b.startBot()
			}
			return b, nil
		}
		// Filter mode: digits become part of the filter.
		b.filter += key
		b.cursor = 0
	default:
		// Type-ahead: single printable character adds to filter.
		if len(key) == 1 && key[0] >= 32 && key[0] < 127 {
			b.filter += key
			b.cursor = 0
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
			return b.startBot()
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
	// Approximate visible height for page-wise scrolling.
	page := b.height - 12
	if page < 5 {
		page = 5
	}
	maxScroll := len(b.resultLines) - page
	if maxScroll < 0 {
		maxScroll = 0
	}

	switch msg.String() {
	case "esc":
		b.state = botsStateList
		b.scroll = 0
		b.resultLines = nil
		b.statusMsg = ""
	case "up", "k":
		if b.scroll > 0 {
			b.scroll--
		}
	case "down", "j":
		if b.scroll < maxScroll {
			b.scroll++
		}
	case "pgup", "ctrl+u":
		b.scroll -= page
		if b.scroll < 0 {
			b.scroll = 0
		}
	case "pgdown", "ctrl+d":
		b.scroll += page
		if b.scroll > maxScroll {
			b.scroll = maxScroll
		}
	case "g", "home":
		b.scroll = 0
	case "G", "end":
		b.scroll = maxScroll
	case "r":
		// Re-run the same bot — useful when a small model produced a
		// weird response and the user wants another attempt.
		if b.activeBot != botDailyDigest {
			return b.startBot()
		}
	case "c", "y":
		// Copy raw response to system clipboard.
		if b.rawResponse != "" {
			if err := ClipboardCopy(b.rawResponse); err != nil {
				b.statusMsg = "Copy failed: " + err.Error()
			} else {
				b.statusMsg = "Copied to clipboard"
			}
		}
	case "s":
		// Save result as a new note in the vault.
		if b.rawResponse != "" && b.vaultRoot != "" {
			if path, err := b.saveResultToNote(); err != nil {
				b.statusMsg = "Save failed: " + err.Error()
			} else {
				b.statusMsg = "Saved to " + path
			}
		}
	case "enter":
		if b.resultReady {
			b.active = false
			return b, nil
		}
	}
	return b, nil
}

// saveResultToNote writes the raw AI response to a new markdown file in the
// vault's "Bots" subdirectory and returns the relative path.
func (b *Bots) saveResultToNote() (string, error) {
	dir := filepath.Join(b.vaultRoot, "Bots")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	noteName := strings.TrimSuffix(filepath.Base(b.currentPath), ".md")
	if noteName == "" {
		noteName = "note"
	}
	// Find the bot's human-readable name for the filename.
	botName := "bot"
	for _, bd := range botList {
		if bd.kind == b.activeBot {
			botName = strings.ReplaceAll(strings.ToLower(bd.name), " ", "-")
			break
		}
	}
	timestamp := time.Now().Format("2006-01-02-150405")
	filename := fmt.Sprintf("%s-%s-%s.md", noteName, botName, timestamp)
	fullPath := filepath.Join(dir, filename)

	// Build note with frontmatter for context.
	var buf strings.Builder
	buf.WriteString("---\n")
	buf.WriteString(fmt.Sprintf("title: %s (%s)\n", noteName, botName))
	buf.WriteString(fmt.Sprintf("date: %s\n", time.Now().Format("2006-01-02")))
	buf.WriteString(fmt.Sprintf("source-note: \"[[%s]]\"\n", noteName))
	buf.WriteString(fmt.Sprintf("bot: %s\n", botName))
	buf.WriteString(fmt.Sprintf("ai-provider: %s\n", b.ai.Provider))
	if b.ai.Model != "" {
		buf.WriteString(fmt.Sprintf("ai-model: %s\n", b.ai.Model))
	}
	buf.WriteString("tags: [ai-generated]\n")
	buf.WriteString("---\n\n")
	buf.WriteString(b.rawResponse)
	buf.WriteString("\n")

	if err := atomicWriteNote(fullPath, buf.String()); err != nil {
		return "", err
	}
	return filepath.Join("Bots", filename), nil
}

// startBot decides whether to use Ollama, OpenAI, or local analysis
func (b Bots) startBot() (Bots, tea.Cmd) {
	useAI := (b.ai.Provider == "ollama" || b.ai.Provider == "openai" || b.ai.Provider == "nous") && b.activeBot != botDailyDigest

	if useAI {
		b.state = botsStateLoading
		b.loadingTick = 0
		b.loadingStart = time.Now()
		b.resultLines = nil
		b.rawResponse = ""
		b.statusMsg = ""
		b.pendingResult = BotResult{Action: "none", NotePath: b.currentPath}

		prompt := b.buildPrompt()
		if prompt == "" {
			// Fallback for bots with no AI prompt
			b.state = botsStateResults
			b.runLocalBot()
			return b, nil
		}

		return b, tea.Batch(
			callAIForBot(b.ai, prompt, b.activeBot),
			botsTickCmd(),
		)
	}

	// Local analysis (no AI)
	b.state = botsStateResults
	b.scroll = 0
	b.resultLines = nil
	b.resultReady = false
	b.pendingResult = BotResult{Action: "none", NotePath: b.currentPath}
	b.runLocalBot()
	return b, nil
}

func (b *Bots) buildPrompt() string {
	switch b.activeBot {
	case botAutoTagger:
		return b.buildAutoTaggerPrompt()
	case botLinkSuggester:
		return b.buildLinkSuggesterPrompt()
	case botSummarizer:
		return b.buildSummarizerPrompt()
	case botQuestionBot:
		return b.buildQuestionPrompt()
	case botWritingAssistant:
		return b.buildWritingAssistantPrompt()
	case botTitleSuggester:
		return b.buildTitleSuggesterPrompt()
	case botActionItems:
		return b.buildActionItemsPrompt()
	case botMOCGenerator:
		return b.buildMOCGeneratorPrompt()
	case botAutoLinker:
		return b.buildAutoLinkerPrompt()
	case botFlashcardGen:
		return b.buildFlashcardPrompt()
	case botToneAdjuster:
		return b.buildToneAdjusterPrompt()
	case botOutliner:
		return b.buildOutlinerPrompt()
	case botExplainSimple:
		return b.buildExplainSimplePrompt()
	case botKeyTerms:
		return b.buildKeyTermsPrompt()
	case botCounterArgument:
		return b.buildCounterArgumentPrompt()
	case botTLDR:
		return b.buildTLDRPrompt()
	case botProsCons:
		return b.buildProsConsPrompt()
	case botExpand:
		return b.buildExpandPrompt()
	}
	return ""
}

func (b *Bots) runLocalBot() {
	switch b.activeBot {
	case botAutoTagger:
		b.runLocalAutoTagger()
	case botLinkSuggester:
		b.runLocalLinkSuggester()
	case botSummarizer:
		b.runLocalSummarizer()
	case botQuestionBot:
		b.runLocalQuestionBot()
	case botWritingAssistant:
		b.runLocalWritingAssistant()
	case botTitleSuggester:
		b.runLocalTitleSuggester()
	case botActionItems:
		b.runLocalActionItems()
	case botMOCGenerator:
		b.runLocalMOCGenerator()
	case botAutoLinker:
		b.runLocalAutoLinker()
	case botFlashcardGen:
		b.runLocalFlashcardGen()
	case botToneAdjuster:
		b.runLocalToneAdjuster()
	case botOutliner:
		b.runLocalOutliner()
	case botExplainSimple, botKeyTerms, botCounterArgument, botTLDR, botProsCons, botExpand:
		b.runLocalRequiresAI()
	case botDailyDigest:
		b.runDailyDigest()
	}
}

// runLocalRequiresAI shows a message that the bot requires an AI provider.
func (b *Bots) runLocalRequiresAI() {
	b.resultLines = []string{
		"",
		lipgloss.NewStyle().Foreground(yellow).Render("  " + IconBookmarkChar + " This bot requires an AI provider."),
		"",
		DimStyle.Render("  Configure Ollama, OpenAI, or Nous in Settings (Ctrl+,)"),
		DimStyle.Render("  or install Ollama: curl -fsSL https://ollama.ai/install.sh | sh"),
	}
}

// runLocalOutliner builds a simple outline from markdown headings found in the note.
func (b *Bots) runLocalOutliner() {
	var lines []string
	lines = append(lines, lipgloss.NewStyle().Foreground(text).Render("  Outline (from existing headings):"))
	lines = append(lines, "")
	found := 0
	for _, line := range strings.Split(b.currentBody, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") || strings.HasPrefix(trimmed, "## ") ||
			strings.HasPrefix(trimmed, "### ") || strings.HasPrefix(trimmed, "#### ") {
			style := lipgloss.NewStyle().Foreground(blue).Bold(true)
			lines = append(lines, "  "+style.Render(trimmed))
			found++
		}
	}
	if found == 0 {
		lines = append(lines, DimStyle.Render("  (no headings found — enable AI for a structured outline)"))
	}
	b.resultLines = lines
}

// processAIResponse parses the AI response for the active bot
func (b *Bots) processAIResponse(response string, providerLabel string) {
	noteName := strings.TrimSuffix(b.currentPath, ".md")
	aiIcon := lipgloss.NewStyle().Foreground(sapphire).Render("  " + IconSearchChar)

	switch b.activeBot {
	case botAutoTagger:
		var lines []string
		lines = append(lines, lipgloss.NewStyle().Foreground(text).Render(
			fmt.Sprintf("  AI-suggested tags for \"%s\":", noteName)))
		lines = append(lines, aiIcon+" "+lipgloss.NewStyle().Foreground(sapphire).Italic(true).Render(
			"Powered by "+providerLabel))
		lines = append(lines, "")

		// Parse comma-separated tags, de-duplicating case-insensitively.
		// Also handle newline-separated output as a fallback.
		raw := strings.ReplaceAll(response, "\n", ",")
		seen := make(map[string]bool)
		var resultTags []string
		for _, part := range strings.Split(raw, ",") {
			tag := strings.TrimSpace(part)
			tag = strings.Trim(tag, "#\"'`[]")
			tag = strings.ToLower(tag)
			if tag == "" || len(tag) >= 30 || seen[tag] {
				continue
			}
			seen[tag] = true
			resultTags = append(resultTags, tag)
			bullet := lipgloss.NewStyle().Foreground(green).Render("  " + IconTagChar)
			tagStr := lipgloss.NewStyle().Foreground(blue).Bold(true).Render(" #" + tag)
			lines = append(lines, bullet+tagStr)
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

	case botLinkSuggester:
		var lines []string
		lines = append(lines, lipgloss.NewStyle().Foreground(text).Render(
			fmt.Sprintf("  AI-suggested links for \"%s\":", noteName)))
		lines = append(lines, aiIcon+" "+lipgloss.NewStyle().Foreground(sapphire).Italic(true).Render(
			"Powered by "+providerLabel))
		lines = append(lines, "")

		var resultLinks []string
		for _, line := range strings.Split(response, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			line = stripListPrefix(line)
			if line == "" {
				continue
			}
			bullet := lipgloss.NewStyle().Foreground(green).Render("  " + IconFileChar)
			linkLine := lipgloss.NewStyle().Foreground(blue).Render(" " + line)
			lines = append(lines, bullet+linkLine)

			// Extract note name for linking. Handle "-", "—", "→" separators.
			line = strings.ReplaceAll(line, " — ", " - ")
			line = strings.ReplaceAll(line, " → ", " - ")
			parts := strings.SplitN(line, " - ", 2)
			if len(parts) > 0 {
				name := strings.TrimSpace(parts[0])
				if !strings.HasSuffix(name, ".md") {
					name += ".md"
				}
				resultLinks = append(resultLinks, name)
			}
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

	case botSummarizer:
		var lines []string
		lines = append(lines, lipgloss.NewStyle().Foreground(text).Render(
			fmt.Sprintf("  AI summary of \"%s\":", noteName)))
		lines = append(lines, aiIcon+" "+lipgloss.NewStyle().Foreground(sapphire).Italic(true).Render(
			"Powered by "+providerLabel))
		lines = append(lines, "")

		wrapped := wordWrap(response, b.overlayInnerWidth()-4)
		for _, wl := range strings.Split(wrapped, "\n") {
			lines = append(lines, "  "+lipgloss.NewStyle().Foreground(text).Render(wl))
		}

		b.resultLines = lines
		if response != "" {
			b.pendingResult = BotResult{
				Action:   "summary",
				Summary:  response,
				NotePath: b.currentPath,
			}
			b.resultReady = true
		}

	case botQuestionBot:
		var lines []string
		lines = append(lines, lipgloss.NewStyle().Foreground(text).Render(
			fmt.Sprintf("  Answer to: \"%s\"", b.questionInput)))
		lines = append(lines, aiIcon+" "+lipgloss.NewStyle().Foreground(sapphire).Italic(true).Render(
			"Powered by "+providerLabel))
		lines = append(lines, "")

		wrapped := wordWrap(response, b.overlayInnerWidth()-4)
		for _, wl := range strings.Split(wrapped, "\n") {
			lines = append(lines, "  "+lipgloss.NewStyle().Foreground(text).Render(wl))
		}
		b.resultLines = lines

	case botWritingAssistant:
		var lines []string
		lines = append(lines, lipgloss.NewStyle().Foreground(text).Render(
			fmt.Sprintf("  AI writing analysis for \"%s\":", noteName)))
		lines = append(lines, aiIcon+" "+lipgloss.NewStyle().Foreground(sapphire).Italic(true).Render(
			"Powered by "+providerLabel))
		lines = append(lines, "")

		for _, respLine := range strings.Split(response, "\n") {
			if respLine == "" {
				lines = append(lines, "")
				continue
			}
			wrapped := wordWrap(respLine, b.overlayInnerWidth()-4)
			for _, wl := range strings.Split(wrapped, "\n") {
				lines = append(lines, "  "+lipgloss.NewStyle().Foreground(text).Render(wl))
			}
		}
		b.resultLines = lines

	case botTitleSuggester:
		var lines []string
		lines = append(lines, lipgloss.NewStyle().Foreground(text).Render(
			fmt.Sprintf("  Title suggestions for \"%s\":", noteName)))
		lines = append(lines, aiIcon+" "+lipgloss.NewStyle().Foreground(sapphire).Italic(true).Render(
			"Powered by "+providerLabel))
		lines = append(lines, "")

		for _, respLine := range strings.Split(response, "\n") {
			respLine = strings.TrimSpace(respLine)
			if respLine == "" {
				continue
			}
			respLine = stripListPrefix(respLine)
			if respLine == "" {
				continue
			}
			bullet := lipgloss.NewStyle().Foreground(green).Render("  " + IconEditChar)
			titleStr := lipgloss.NewStyle().Foreground(blue).Bold(true).Render(" " + respLine)
			lines = append(lines, bullet+titleStr)
		}
		b.resultLines = lines

	case botActionItems:
		var lines []string
		lines = append(lines, lipgloss.NewStyle().Foreground(text).Render(
			fmt.Sprintf("  Action items from \"%s\":", noteName)))
		lines = append(lines, aiIcon+" "+lipgloss.NewStyle().Foreground(sapphire).Italic(true).Render(
			"Powered by "+providerLabel))
		lines = append(lines, "")

		for _, respLine := range strings.Split(response, "\n") {
			respLine = strings.TrimSpace(respLine)
			if respLine == "" {
				continue
			}
			if (strings.HasPrefix(respLine, "- [ ]") || strings.HasPrefix(respLine, "- [x]")) && len(respLine) > 6 {
				bullet := lipgloss.NewStyle().Foreground(yellow).Render("  " + IconOutlineChar)
				task := lipgloss.NewStyle().Foreground(text).Render(" " + respLine[6:])
				lines = append(lines, bullet+task)
			} else {
				wrapped := wordWrap(respLine, b.overlayInnerWidth()-4)
				for _, wl := range strings.Split(wrapped, "\n") {
					lines = append(lines, "  "+lipgloss.NewStyle().Foreground(text).Render(wl))
				}
			}
		}
		b.resultLines = lines

	case botMOCGenerator:
		var lines []string
		lines = append(lines, lipgloss.NewStyle().Foreground(text).Render(
			"  Map of Content"))
		lines = append(lines, aiIcon+" "+lipgloss.NewStyle().Foreground(sapphire).Italic(true).Render(
			"Powered by "+providerLabel))
		lines = append(lines, "")

		for _, respLine := range strings.Split(response, "\n") {
			if respLine == "" {
				lines = append(lines, "")
				continue
			}
			if strings.HasPrefix(respLine, "# ") || strings.HasPrefix(respLine, "## ") {
				lines = append(lines, lipgloss.NewStyle().Foreground(blue).Bold(true).Render("  "+respLine))
			} else {
				wrapped := wordWrap(respLine, b.overlayInnerWidth()-4)
				for _, wl := range strings.Split(wrapped, "\n") {
					lines = append(lines, "  "+lipgloss.NewStyle().Foreground(text).Render(wl))
				}
			}
		}
		b.resultLines = lines
		if response != "" {
			b.pendingResult = BotResult{
				Action:   "summary",
				Summary:  response,
				NotePath: "MOC.md",
			}
			b.resultReady = true
		}

	case botAutoLinker:
		var lines []string
		lines = append(lines, lipgloss.NewStyle().Foreground(text).Render(
			fmt.Sprintf("  AI wikilink suggestions for \"%s\":", noteName)))
		lines = append(lines, aiIcon+" "+lipgloss.NewStyle().Foreground(sapphire).Italic(true).Render(
			"Powered by "+providerLabel))
		lines = append(lines, "")
		for _, respLine := range strings.Split(response, "\n") {
			respLine = strings.TrimSpace(respLine)
			if respLine == "" {
				continue
			}
			lines = append(lines, "  "+lipgloss.NewStyle().Foreground(text).Render(respLine))
		}
		b.resultLines = lines

	case botFlashcardGen:
		var lines []string
		lines = append(lines, lipgloss.NewStyle().Foreground(text).Render(
			fmt.Sprintf("  AI flashcards for \"%s\":", noteName)))
		lines = append(lines, aiIcon+" "+lipgloss.NewStyle().Foreground(sapphire).Italic(true).Render(
			"Powered by "+providerLabel))
		lines = append(lines, "")
		for _, respLine := range strings.Split(response, "\n") {
			respLine = strings.TrimSpace(respLine)
			if respLine == "" {
				lines = append(lines, "")
				continue
			}
			if strings.HasPrefix(respLine, "Q:") {
				lines = append(lines, "  "+lipgloss.NewStyle().Foreground(blue).Bold(true).Render(respLine))
			} else if strings.HasPrefix(respLine, "A:") {
				lines = append(lines, "  "+lipgloss.NewStyle().Foreground(green).Render(respLine))
			} else {
				lines = append(lines, "  "+lipgloss.NewStyle().Foreground(text).Render(respLine))
			}
		}
		b.resultLines = lines

	case botToneAdjuster:
		var lines []string
		lines = append(lines, lipgloss.NewStyle().Foreground(text).Render("  AI tone adjustments:"))
		lines = append(lines, aiIcon+" "+lipgloss.NewStyle().Foreground(sapphire).Italic(true).Render(
			"Powered by "+providerLabel))
		lines = append(lines, "")
		for _, respLine := range strings.Split(response, "\n") {
			respLine = strings.TrimSpace(respLine)
			if respLine == "" {
				lines = append(lines, "")
				continue
			}
			if strings.HasPrefix(respLine, "FORMAL:") || strings.HasPrefix(respLine, "CASUAL:") || strings.HasPrefix(respLine, "CONCISE:") {
				lines = append(lines, "  "+lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(respLine))
			} else {
				wrapped := wordWrap(respLine, b.overlayInnerWidth()-4)
				for _, wl := range strings.Split(wrapped, "\n") {
					lines = append(lines, "  "+lipgloss.NewStyle().Foreground(text).Render(wl))
				}
			}
		}
		b.resultLines = lines

	case botOutliner:
		var lines []string
		lines = append(lines, lipgloss.NewStyle().Foreground(text).Render(
			fmt.Sprintf("  AI outline for \"%s\":", noteName)))
		lines = append(lines, aiIcon+" "+lipgloss.NewStyle().Foreground(sapphire).Italic(true).Render(
			"Powered by "+providerLabel))
		lines = append(lines, "")
		for _, respLine := range strings.Split(response, "\n") {
			if respLine == "" {
				lines = append(lines, "")
				continue
			}
			trimmed := strings.TrimSpace(respLine)
			if strings.HasPrefix(trimmed, "#") {
				lines = append(lines, "  "+lipgloss.NewStyle().Foreground(blue).Bold(true).Render(trimmed))
			} else {
				lines = append(lines, "  "+lipgloss.NewStyle().Foreground(text).Render(trimmed))
			}
		}
		b.resultLines = lines
		if strings.TrimSpace(response) != "" {
			b.pendingResult = BotResult{Action: "summary", Summary: response, NotePath: b.currentPath}
			b.resultReady = true
		}

	case botExplainSimple:
		var lines []string
		lines = append(lines, lipgloss.NewStyle().Foreground(text).Render(
			fmt.Sprintf("  Simple explanation of \"%s\":", noteName)))
		lines = append(lines, aiIcon+" "+lipgloss.NewStyle().Foreground(sapphire).Italic(true).Render(
			"Powered by "+providerLabel))
		lines = append(lines, "")
		wrapped := wordWrap(response, b.overlayInnerWidth()-4)
		for _, wl := range strings.Split(wrapped, "\n") {
			lines = append(lines, "  "+lipgloss.NewStyle().Foreground(text).Render(wl))
		}
		b.resultLines = lines
		if strings.TrimSpace(response) != "" {
			b.pendingResult = BotResult{Action: "summary", Summary: response, NotePath: b.currentPath}
			b.resultReady = true
		}

	case botKeyTerms:
		var lines []string
		lines = append(lines, lipgloss.NewStyle().Foreground(text).Render(
			fmt.Sprintf("  Key terms in \"%s\":", noteName)))
		lines = append(lines, aiIcon+" "+lipgloss.NewStyle().Foreground(sapphire).Italic(true).Render(
			"Powered by "+providerLabel))
		lines = append(lines, "")
		for _, respLine := range strings.Split(response, "\n") {
			respLine = strings.TrimSpace(respLine)
			if respLine == "" {
				lines = append(lines, "")
				continue
			}
			// Parse "TERM: definition" format.
			if idx := strings.Index(respLine, ":"); idx > 0 && idx < 60 {
				term := strings.TrimSpace(respLine[:idx])
				def := strings.TrimSpace(respLine[idx+1:])
				termStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(term)
				defStyle := lipgloss.NewStyle().Foreground(text).Render(" — " + def)
				wrapped := wordWrap("  "+termStyle+defStyle, b.overlayInnerWidth()-4)
				lines = append(lines, strings.Split(wrapped, "\n")...)
			} else {
				lines = append(lines, "  "+lipgloss.NewStyle().Foreground(text).Render(respLine))
			}
		}
		b.resultLines = lines

	case botTLDR:
		var lines []string
		lines = append(lines, lipgloss.NewStyle().Foreground(text).Render(
			fmt.Sprintf("  TL;DR of \"%s\":", noteName)))
		lines = append(lines, aiIcon+" "+lipgloss.NewStyle().Foreground(sapphire).Italic(true).Render(
			"Powered by "+providerLabel))
		lines = append(lines, "")
		summary := strings.TrimSpace(response)
		summary = strings.Trim(summary, "\"'`")
		wrapped := wordWrap(summary, b.overlayInnerWidth()-4)
		for _, wl := range strings.Split(wrapped, "\n") {
			lines = append(lines, "  "+lipgloss.NewStyle().Foreground(text).Bold(true).Render(wl))
		}
		b.resultLines = lines
		if summary != "" {
			b.pendingResult = BotResult{Action: "summary", Summary: summary, NotePath: b.currentPath}
			b.resultReady = true
		}

	case botProsCons:
		var lines []string
		lines = append(lines, lipgloss.NewStyle().Foreground(text).Render(
			fmt.Sprintf("  Pros & Cons for \"%s\":", noteName)))
		lines = append(lines, aiIcon+" "+lipgloss.NewStyle().Foreground(sapphire).Italic(true).Render(
			"Powered by "+providerLabel))
		lines = append(lines, "")
		inPros := false
		inCons := false
		for _, respLine := range strings.Split(response, "\n") {
			trimmed := strings.TrimSpace(respLine)
			if trimmed == "" {
				lines = append(lines, "")
				continue
			}
			upper := strings.ToUpper(trimmed)
			if strings.HasPrefix(upper, "PROS") {
				inPros = true
				inCons = false
				lines = append(lines, "  "+lipgloss.NewStyle().Foreground(green).Bold(true).Render("PROS:"))
				continue
			}
			if strings.HasPrefix(upper, "CONS") {
				inPros = false
				inCons = true
				lines = append(lines, "")
				lines = append(lines, "  "+lipgloss.NewStyle().Foreground(red).Bold(true).Render("CONS:"))
				continue
			}
			cleaned := stripListPrefix(trimmed)
			if cleaned == "" {
				continue
			}
			var style lipgloss.Style
			switch {
			case inPros:
				style = lipgloss.NewStyle().Foreground(green)
			case inCons:
				style = lipgloss.NewStyle().Foreground(red)
			default:
				style = lipgloss.NewStyle().Foreground(text)
			}
			lines = append(lines, "    • "+style.Render(cleaned))
		}
		b.resultLines = lines

	case botExpand:
		var lines []string
		lines = append(lines, lipgloss.NewStyle().Foreground(text).Render(
			fmt.Sprintf("  Expanded \"%s\":", noteName)))
		lines = append(lines, aiIcon+" "+lipgloss.NewStyle().Foreground(sapphire).Italic(true).Render(
			"Powered by "+providerLabel))
		lines = append(lines, "")
		for _, respLine := range strings.Split(response, "\n") {
			if respLine == "" {
				lines = append(lines, "")
				continue
			}
			trimmed := strings.TrimSpace(respLine)
			if strings.HasPrefix(trimmed, "#") {
				lines = append(lines, "  "+lipgloss.NewStyle().Foreground(blue).Bold(true).Render(trimmed))
			} else {
				wrapped := wordWrap(trimmed, b.overlayInnerWidth()-4)
				for _, wl := range strings.Split(wrapped, "\n") {
					lines = append(lines, "  "+lipgloss.NewStyle().Foreground(text).Render(wl))
				}
			}
		}
		b.resultLines = lines
		if strings.TrimSpace(response) != "" {
			b.pendingResult = BotResult{Action: "summary", Summary: response, NotePath: b.currentPath}
			b.resultReady = true
		}

	case botCounterArgument:
		var lines []string
		lines = append(lines, lipgloss.NewStyle().Foreground(text).Render(
			fmt.Sprintf("  Counter-arguments for \"%s\":", noteName)))
		lines = append(lines, aiIcon+" "+lipgloss.NewStyle().Foreground(sapphire).Italic(true).Render(
			"Powered by "+providerLabel))
		lines = append(lines, "")
		for _, respLine := range strings.Split(response, "\n") {
			if respLine == "" {
				lines = append(lines, "")
				continue
			}
			trimmed := strings.TrimSpace(respLine)
			if strings.HasPrefix(trimmed, "- CLAIM:") || strings.HasPrefix(trimmed, "CLAIM:") {
				lines = append(lines, "  "+lipgloss.NewStyle().Foreground(yellow).Bold(true).Render(trimmed))
			} else if strings.HasPrefix(trimmed, "COUNTER:") || strings.HasPrefix(trimmed, "- COUNTER:") {
				lines = append(lines, "  "+lipgloss.NewStyle().Foreground(red).Render(trimmed))
			} else {
				wrapped := wordWrap(trimmed, b.overlayInnerWidth()-6)
				for _, wl := range strings.Split(wrapped, "\n") {
					lines = append(lines, "    "+lipgloss.NewStyle().Foreground(text).Render(wl))
				}
			}
		}
		b.resultLines = lines
	}
}

// ---------------------------------------------------------------------------
// Local analysis runners (fallback when Ollama is not available)
// ---------------------------------------------------------------------------

func (b *Bots) runLocalAutoTagger() {
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

	maxScore := 1
	if len(scored) > 0 && scored[0].score > 0 {
		maxScore = scored[0].score
	}

	noteName := strings.TrimSuffix(b.currentPath, ".md")
	var lines []string
	lines = append(lines, lipgloss.NewStyle().Foreground(text).Render(
		fmt.Sprintf("  Suggested tags for \"%s\":", noteName)))
	lines = append(lines, DimStyle.Render("  Local analysis (set AI Provider to 'ollama' for AI)"))
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
		bullet := lipgloss.NewStyle().Foreground(green).Render("  " + IconTagChar)
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

func (b *Bots) runLocalLinkSuggester() {
	currentWords := significantWords(b.currentBody)

	type linkScore struct {
		path  string
		score int
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
			scored = append(scored, linkScore{path: path, score: shared})
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
	lines = append(lines, DimStyle.Render("  Local analysis (set AI Provider to 'ollama' for AI)"))
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
		bullet := lipgloss.NewStyle().Foreground(green).Render("  " + IconFileChar)
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

func (b *Bots) runLocalSummarizer() {
	keywords := make(map[string]bool)
	titleWords := strings.Fields(strings.ToLower(strings.TrimSuffix(b.currentPath, ".md")))
	for _, w := range titleWords {
		w = stripPunctuation(w)
		if len(w) > 2 && !stopwords[w] {
			keywords[w] = true
		}
	}
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

	paragraphs := splitParagraphs(b.currentBody)
	var candidates []string
	for _, para := range paragraphs {
		if strings.HasPrefix(strings.TrimSpace(para), "---") {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(para), "#") && !strings.Contains(para, "\n") {
			continue
		}
		sentence := firstSentence(para)
		if sentence == "" {
			continue
		}
		candidates = append(candidates, sentence)
	}

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
	sort.SliceStable(ss, func(i, j int) bool { return ss[i].score > ss[j].score })
	limit := 5
	if limit > len(ss) {
		limit = len(ss)
	}
	selected := ss[:limit]
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
	lines = append(lines, DimStyle.Render("  Local analysis (set AI Provider to 'ollama' for AI)"))
	lines = append(lines, "")

	if summary == "" {
		lines = append(lines, DimStyle.Render("  Note is too short to summarize."))
	} else {
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

func (b *Bots) runLocalQuestionBot() {
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
		contentLines := strings.Split(content, "\n")
		bestScore := 0
		bestSnippet := ""
		for _, line := range contentLines {
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
	lines = append(lines, DimStyle.Render("  Local search (set AI Provider to 'ollama' for AI answers)"))
	lines = append(lines, "")

	if len(matches) == 0 {
		lines = append(lines, DimStyle.Render("  No matching notes found."))
	} else {
		for _, m := range matches {
			bullet := lipgloss.NewStyle().Foreground(green).Render("  " + IconFileChar)
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
}

func (b *Bots) runLocalWritingAssistant() {
	var lines []string
	noteName := strings.TrimSuffix(b.currentPath, ".md")
	lines = append(lines, lipgloss.NewStyle().Foreground(text).Render(
		fmt.Sprintf("  Writing analysis for \"%s\":", noteName)))
	lines = append(lines, DimStyle.Render("  Local analysis (set AI Provider to 'ollama' for AI)"))
	lines = append(lines, "")

	allWords := strings.Fields(b.currentBody)
	totalWords := len(allWords)
	paragraphs := splitParagraphs(b.currentBody)
	issueCount := 0

	warnIcon := lipgloss.NewStyle().Foreground(yellow).Render("  " + IconBookmarkChar)
	infoIcon := lipgloss.NewStyle().Foreground(blue).Render("  " + IconFileChar)

	for i, para := range paragraphs {
		pWords := strings.Fields(para)
		if len(pWords) > 200 {
			lines = append(lines, warnIcon+" "+lipgloss.NewStyle().Foreground(yellow).Render(
				fmt.Sprintf("Paragraph %d is very long (%d words)", i+1, len(pWords))))
			issueCount++
		}
	}

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
			fmt.Sprintf("Found %d possible passive voice construction(s)", passiveCount)))
		issueCount++
	}

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
					fmt.Sprintf("\"%s\" repeated %d times in paragraph %d", word, count, i+1)))
				issueCount++
			}
		}
	}

	hasHeadings := false
	for _, line := range strings.Split(b.currentBody, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			hasHeadings = true
			break
		}
	}
	if totalWords > 500 && !hasHeadings {
		lines = append(lines, warnIcon+" "+lipgloss.NewStyle().Foreground(yellow).Render(
			fmt.Sprintf("Note has %d words but no headings", totalWords)))
		issueCount++
	}

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
	lines = append(lines, DimStyle.Render("  "+strings.Repeat("─", 30)))
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
}

func (b *Bots) runLocalTitleSuggester() {
	noteName := strings.TrimSuffix(b.currentPath, ".md")
	var lines []string
	lines = append(lines, lipgloss.NewStyle().Foreground(text).Render(
		fmt.Sprintf("  Title suggestions for \"%s\":", noteName)))
	lines = append(lines, DimStyle.Render("  Local analysis (set AI Provider for AI suggestions)"))
	lines = append(lines, "")

	// Extract significant words from content
	keywords := significantWords(b.currentBody)
	var topWords []string
	for w := range keywords {
		topWords = append(topWords, w)
	}
	sort.Strings(topWords)
	if len(topWords) > 6 {
		topWords = topWords[:6]
	}

	// Generate simple title variations
	suggestions := []string{}
	if len(topWords) >= 2 {
		suggestions = append(suggestions, titleCase(topWords[0])+" and "+titleCase(topWords[1]))
	}
	// Extract first heading as a suggestion
	for _, line := range strings.Split(b.currentBody, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			heading := strings.TrimPrefix(trimmed, "# ")
			if heading != noteName {
				suggestions = append(suggestions, heading)
			}
		} else if strings.HasPrefix(trimmed, "## ") {
			heading := strings.TrimPrefix(trimmed, "## ")
			suggestions = append(suggestions, heading)
		}
		if len(suggestions) >= 5 {
			break
		}
	}

	if len(suggestions) == 0 {
		lines = append(lines, DimStyle.Render("  Not enough content to suggest titles."))
	} else {
		for _, s := range suggestions {
			bullet := lipgloss.NewStyle().Foreground(green).Render("  " + IconEditChar)
			titleStr := lipgloss.NewStyle().Foreground(blue).Bold(true).Render(" " + s)
			lines = append(lines, bullet+titleStr)
		}
	}

	b.resultLines = lines
}

func (b *Bots) runLocalActionItems() {
	var lines []string
	noteName := strings.TrimSuffix(b.currentPath, ".md")
	lines = append(lines, lipgloss.NewStyle().Foreground(text).Render(
		fmt.Sprintf("  Action items from \"%s\":", noteName)))
	lines = append(lines, DimStyle.Render("  Local analysis (set AI Provider for deeper extraction)"))
	lines = append(lines, "")

	found := 0
	for _, line := range strings.Split(b.currentBody, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- [ ] ") {
			task := strings.TrimPrefix(trimmed, "- [ ] ")
			bullet := lipgloss.NewStyle().Foreground(yellow).Render("  " + IconOutlineChar)
			taskStr := lipgloss.NewStyle().Foreground(text).Render(" " + task)
			lines = append(lines, bullet+taskStr)
			found++
		} else if strings.HasPrefix(trimmed, "- [x] ") || strings.HasPrefix(trimmed, "- [X] ") {
			task := trimmed[6:]
			bullet := lipgloss.NewStyle().Foreground(green).Render("  " + IconOutlineChar)
			taskStr := lipgloss.NewStyle().Foreground(overlay0).Strikethrough(true).Render(" " + task)
			lines = append(lines, bullet+taskStr)
			found++
		}
	}

	if found == 0 {
		lines = append(lines, DimStyle.Render("  No action items found in this note."))
	} else {
		lines = append(lines, "")
		lines = append(lines, DimStyle.Render(fmt.Sprintf("  Found %d action item(s)", found)))
	}

	b.resultLines = lines
}

func (b *Bots) runLocalMOCGenerator() {
	var lines []string
	lines = append(lines, lipgloss.NewStyle().Foreground(text).Render(
		"  Map of Content (Local Analysis)"))
	lines = append(lines, DimStyle.Render("  Set AI Provider for AI-generated MOC"))
	lines = append(lines, "")

	// Group notes by shared keywords/tags
	groups := make(map[string][]string) // tag -> note paths
	ungrouped := []string{}

	for path := range b.notes {
		tagged := false
		for tag, paths := range b.tags {
			for _, tp := range paths {
				if tp == path {
					groups[tag] = append(groups[tag], path)
					tagged = true
				}
			}
		}
		if !tagged {
			ungrouped = append(ungrouped, path)
		}
	}

	sectionStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)

	// Sort groups by size
	type group struct {
		name  string
		paths []string
	}
	var sorted []group
	for name, paths := range groups {
		sort.Strings(paths)
		sorted = append(sorted, group{name, paths})
	}
	sort.Slice(sorted, func(i, j int) bool { return len(sorted[i].paths) > len(sorted[j].paths) })

	for _, g := range sorted {
		lines = append(lines, sectionStyle.Render("  ## "+titleCase(g.name)))
		for _, p := range g.paths {
			bullet := lipgloss.NewStyle().Foreground(green).Render("  " + IconFileChar)
			name := lipgloss.NewStyle().Foreground(blue).Render(" [[" + strings.TrimSuffix(p, ".md") + "]]")
			lines = append(lines, bullet+name)
		}
		lines = append(lines, "")
	}

	if len(ungrouped) > 0 {
		sort.Strings(ungrouped)
		lines = append(lines, sectionStyle.Render("  ## Uncategorized"))
		for _, p := range ungrouped {
			bullet := lipgloss.NewStyle().Foreground(yellow).Render("  " + IconFileChar)
			name := lipgloss.NewStyle().Foreground(text).Render(" [[" + strings.TrimSuffix(p, ".md") + "]]")
			lines = append(lines, bullet+name)
		}
	}

	b.resultLines = lines
}

func (b *Bots) runDailyDigest() {
	var lines []string

	sectionStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(text)
	numStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)

	totalNotes := len(b.notes)

	linkCounts := make(map[string]int)
	for _, content := range b.notes {
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
			if pipe := strings.Index(target, "|"); pipe >= 0 {
				target = target[:pipe]
			}
			target = strings.TrimSpace(target)
			if target != "" {
				if !strings.HasSuffix(target, ".md") {
					target += ".md"
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

	outgoing := make(map[string]bool)
	for path, content := range b.notes {
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
			outgoing[path] = true
			rest = rest[end+2:]
		}
	}

	var orphans []string
	for path := range b.notes {
		if linkCounts[path] == 0 && !outgoing[path] {
			orphans = append(orphans, path)
		}
	}
	sort.Strings(orphans)

	var topLinked []entry
	for path, count := range linkCounts {
		topLinked = append(topLinked, entry{name: path, count: count})
	}
	sort.Slice(topLinked, func(i, j int) bool { return topLinked[i].count > topLinked[j].count })
	if len(topLinked) > 5 {
		topLinked = topLinked[:5]
	}

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

	lines = append(lines, sectionStyle.Render("  "+IconCanvasChar+" Vault Overview"))
	lines = append(lines, DimStyle.Render("  "+strings.Repeat("─", 30)))
	lines = append(lines, labelStyle.Render("  Total notes: ")+numStyle.Render(smallNum(totalNotes)))
	lines = append(lines, labelStyle.Render("  Orphan notes: ")+numStyle.Render(smallNum(len(orphans))))
	lines = append(lines, "")

	if len(topLinked) > 0 {
		lines = append(lines, sectionStyle.Render("  "+IconFileChar+" Most Linked Notes"))
		lines = append(lines, DimStyle.Render("  "+strings.Repeat("─", 30)))
		for _, e := range topLinked {
			bullet := lipgloss.NewStyle().Foreground(green).Render("  " + IconFileChar)
			name := lipgloss.NewStyle().Foreground(blue).Render(
				" " + strings.TrimSuffix(e.name, ".md"))
			count := DimStyle.Render(fmt.Sprintf(" (%d links)", e.count))
			lines = append(lines, bullet+name+count)
		}
		lines = append(lines, "")
	}

	if len(orphans) > 0 {
		show := orphans
		if len(show) > 5 {
			show = show[:5]
		}
		lines = append(lines, sectionStyle.Render("  "+IconBookmarkChar+" Orphan Notes"))
		lines = append(lines, DimStyle.Render("  "+strings.Repeat("─", 30)))
		for _, o := range show {
			bullet := lipgloss.NewStyle().Foreground(yellow).Render("  " + IconFileChar)
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

	if len(tagDist) > 0 {
		lines = append(lines, sectionStyle.Render("  "+IconTagChar+" Tag Distribution"))
		lines = append(lines, DimStyle.Render("  "+strings.Repeat("─", 30)))
		maxCount := tagDist[0].count
		if maxCount < 1 {
			maxCount = 1
		}
		for _, tc := range tagDist {
			barLen := tc.count * 15 / maxCount
			if barLen < 1 && tc.count > 0 {
				barLen = 1
			}
			bar := lipgloss.NewStyle().Foreground(mauve).Render(strings.Repeat("█", barLen))
			rest := DimStyle.Render(strings.Repeat("░", 15-barLen))
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
	case botsStateLoading:
		return b.viewLoading(width)
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
	if w > 80 {
		w = 80
	}
	return w
}

func (b Bots) overlayInnerWidth() int {
	return b.overlayWidth() - 6
}

func (b Bots) viewList(width int) string {
	var buf strings.Builder

	title := lipgloss.NewStyle().
		Foreground(sapphire).
		Bold(true).
		Render("  " + IconSearchChar + " Granit AI Bots")
	buf.WriteString(title)
	buf.WriteString("\n")

	// Show AI provider status
	providerLabel := "Local Analysis"
	providerColor := overlay1
	switch b.ai.Provider {
	case "ollama":
		providerLabel = "Ollama: " + b.ai.Model
		providerColor = green
	case "openai":
		providerLabel = "OpenAI: " + b.ai.Model
		providerColor = green
	case "nous":
		providerLabel = "Nous (local)"
		providerColor = green
	}
	providerStatus := lipgloss.NewStyle().Foreground(providerColor).Render("  " + IconSearchChar + " " + providerLabel)
	buf.WriteString(providerStatus)
	buf.WriteString("\n")

	buf.WriteString(DimStyle.Render(strings.Repeat("─", width-6)))
	buf.WriteString("\n\n")
	if b.filter != "" {
		filterLabel := lipgloss.NewStyle().Foreground(yellow).Render("  Filter: ")
		filterText := lipgloss.NewStyle().Foreground(peach).Bold(true).Render(b.filter)
		buf.WriteString(filterLabel + filterText + DimStyle.Render("  (Esc to clear)"))
	} else {
		buf.WriteString(DimStyle.Render("  Select a bot (type to filter):"))
	}
	buf.WriteString("\n\n")

	visible := b.filteredBots()
	if len(visible) == 0 {
		buf.WriteString(DimStyle.Render("    (no bots match \"" + b.filter + "\")"))
		buf.WriteString("\n")
	}

	// Render grouped by category. Headers are shown only when the filter
	// is empty (filtered view is a flat list to emphasize results).
	currentCat := ""
	showCategories := b.filter == ""
	for i, bd := range visible {
		icon := bd.icon
		name := bd.name
		desc := bd.desc

		if showCategories && bd.category != currentCat {
			if currentCat != "" {
				buf.WriteString("\n")
			}
			currentCat = bd.category
			catStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("  " + strings.ToUpper(bd.category))
			buf.WriteString(catStyle)
			buf.WriteString("\n")
		}

		// Show hotkey number for the first 9 bots (1-9).
		hotkey := "  "
		if i < 9 {
			hotkey = lipgloss.NewStyle().Foreground(overlay1).Render(fmt.Sprintf(" %d", i+1))
		}

		if i == b.cursor {
			pointer := lipgloss.NewStyle().Foreground(sapphire).Bold(true).Render("▸ ")
			nameStyled := lipgloss.NewStyle().Foreground(peach).Bold(true).Render(icon + " " + name)
			buf.WriteString(lipgloss.NewStyle().
				Background(surface0).
				Width(width - 6).
				Render(hotkey + " " + pointer + nameStyled))
			buf.WriteString("\n")
			descStyled := lipgloss.NewStyle().
				Background(surface0).
				Foreground(overlay1).
				Width(width - 6).
				Render("       " + desc)
			buf.WriteString(descStyled)
		} else {
			nameStyled := lipgloss.NewStyle().Foreground(text).Render(hotkey + "   " + icon + " " + name)
			buf.WriteString(nameStyled)
			buf.WriteString("\n")
			buf.WriteString(DimStyle.Render("       " + desc))
		}

		if i < len(visible)-1 {
			buf.WriteString("\n")
		}
	}

	buf.WriteString("\n\n")
	buf.WriteString(DimStyle.Render("  " + strings.Repeat("─", width-10)))
	buf.WriteString("\n")
	buf.WriteString(DimStyle.Render("  ↑↓: select  Enter: run  1-9: quick pick  type: filter  Esc: close"))

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(sapphire).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(buf.String())
}

func (b Bots) viewInput(width int) string {
	var buf strings.Builder

	title := lipgloss.NewStyle().
		Foreground(sapphire).
		Bold(true).
		Render("  " + IconSearchChar + " Question Bot")
	buf.WriteString(title)
	buf.WriteString("\n")
	buf.WriteString(DimStyle.Render(strings.Repeat("─", width-6)))
	buf.WriteString("\n\n")

	buf.WriteString(lipgloss.NewStyle().Foreground(text).Render("  Ask a question about your notes:"))
	buf.WriteString("\n\n")

	prompt := SearchPromptStyle.Render("  > ")
	input := SearchInputStyle.Render(b.questionInput + "█")
	buf.WriteString(prompt + input)

	buf.WriteString("\n\n")
	buf.WriteString(DimStyle.Render("  " + strings.Repeat("─", width-10)))
	buf.WriteString("\n")
	switch b.ai.Provider {
	case "ollama":
		buf.WriteString(DimStyle.Render("  AI will answer using Ollama: " + b.ai.Model))
		buf.WriteString("\n")
	case "openai":
		buf.WriteString(DimStyle.Render("  AI will answer using OpenAI: " + b.ai.Model))
		buf.WriteString("\n")
	case "nous":
		buf.WriteString(DimStyle.Render("  AI will answer using Nous (local)"))
		buf.WriteString("\n")
	}
	buf.WriteString(DimStyle.Render("  Enter: search  Esc: back"))

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(sapphire).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(buf.String())
}

func (b Bots) viewLoading(width int) string {
	var buf strings.Builder

	bd := botList[0]
	for _, d := range botList {
		if d.kind == b.activeBot {
			bd = d
			break
		}
	}

	// Title with category pill
	titleText := lipgloss.NewStyle().
		Foreground(sapphire).
		Bold(true).
		Render("  " + bd.icon + " " + bd.name)
	catPill := ""
	if bd.category != "" {
		catPill = " " + lipgloss.NewStyle().
			Foreground(crust).
			Background(mauve).
			Render(" " + bd.category + " ")
	}
	buf.WriteString(titleText + catPill)
	buf.WriteString("\n")
	buf.WriteString(DimStyle.Render(strings.Repeat("─", width-6)))
	buf.WriteString("\n\n")

	// Animated spinner
	spinFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	frame := spinFrames[b.loadingTick%len(spinFrames)]
	spinner := lipgloss.NewStyle().Foreground(sapphire).Bold(true).Render(frame)

	elapsed := time.Since(b.loadingStart).Truncate(time.Second)
	elapsedStr := lipgloss.NewStyle().Foreground(yellow).Render(fmt.Sprintf(" %s", elapsed))
	thinkingLabel := "Thinking with " + b.ai.Model
	connectLabel := "Connecting to Ollama at " + b.ai.OllamaURL
	if b.ai.Provider == "openai" {
		thinkingLabel = "Thinking with " + b.ai.Model
		connectLabel = "Connecting to OpenAI API..."
	} else if b.ai.Provider == "nous" {
		thinkingLabel = "Thinking with Nous"
		connectLabel = "Connecting to Nous at " + b.ai.NousURL
	}
	buf.WriteString("  " + spinner + " " + lipgloss.NewStyle().Foreground(text).Render(thinkingLabel) + elapsedStr)
	buf.WriteString("\n\n")

	// Animated progress bar (indeterminate, just moves) to add visual life.
	barWidth := width - 12
	if barWidth < 10 {
		barWidth = 10
	}
	pos := b.loadingTick % (barWidth + 6)
	var bar strings.Builder
	for i := 0; i < barWidth; i++ {
		// Draw a moving "comet" of 3 filled blocks.
		if i >= pos-2 && i <= pos {
			bar.WriteString("█")
		} else {
			bar.WriteString("░")
		}
	}
	buf.WriteString("  " + lipgloss.NewStyle().Foreground(sapphire).Render(bar.String()))
	buf.WriteString("\n\n")

	buf.WriteString(DimStyle.Render("  " + connectLabel))
	if elapsed > 15*time.Second && b.ai.IsSmallModel() {
		buf.WriteString("\n")
		buf.WriteString(DimStyle.Render("  Small models can be slow — consider a larger model for complex tasks"))
	} else if elapsed > 30*time.Second {
		buf.WriteString("\n")
		buf.WriteString(DimStyle.Render("  Taking longer than usual — check if Ollama is responsive"))
	}
	buf.WriteString("\n\n")
	buf.WriteString(DimStyle.Render("  Esc: cancel"))

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(sapphire).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(buf.String())
}

func (b Bots) viewResults(width int) string {
	var buf strings.Builder

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
	// Metadata line: model + elapsed time.
	if b.lastElapsed > 0 {
		metaParts := []string{}
		if b.ai.Model != "" && b.ai.Provider != "local" {
			metaParts = append(metaParts, b.ai.Model)
		}
		metaParts = append(metaParts, fmt.Sprintf("%.1fs", b.lastElapsed.Seconds()))
		meta := DimStyle.Render("  " + strings.Join(metaParts, " • "))
		buf.WriteString(meta)
	}
	buf.WriteString("\n")
	buf.WriteString(DimStyle.Render(strings.Repeat("─", width-6)))
	buf.WriteString("\n")

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
	buf.WriteString(DimStyle.Render("  " + strings.Repeat("─", width-10)))
	buf.WriteString("\n")

	// Transient status line (e.g. "Copied to clipboard", "Saved to Bots/...").
	if b.statusMsg != "" {
		buf.WriteString(lipgloss.NewStyle().Foreground(green).Bold(true).Render("  " + b.statusMsg))
		buf.WriteString("\n")
	}

	actions := "c:copy  s:save  r:re-run"
	if b.rawResponse == "" {
		actions = "r:re-run"
	}
	if b.resultReady {
		actionHint := ""
		switch b.pendingResult.Action {
		case "tag":
			actionHint = "Enter: apply tags"
		case "link":
			actionHint = "Enter: insert link"
		case "summary":
			actionHint = "Enter: close"
		}
		parts := []string{}
		if actionHint != "" {
			parts = append(parts, actionHint)
		}
		parts = append(parts, actions, "j/k g/G: scroll", "Esc: back")
		buf.WriteString(DimStyle.Render("  " + strings.Join(parts, "  ")))
	} else {
		parts := []string{actions, "j/k g/G: scroll", "Esc: back"}
		buf.WriteString(DimStyle.Render("  " + strings.Join(parts, "  ")))
	}

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(sapphire).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(buf.String())
}

// ---------------------------------------------------------------------------
// Text-processing helpers
// ---------------------------------------------------------------------------

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

func stripPunctuation(w string) string {
	return strings.Trim(w, ".,;:!?\"'`()[]{}#*_~<>@/\\|+-=")
}

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

func firstSentence(para string) string {
	para = strings.ReplaceAll(para, "\n", " ")
	para = strings.TrimSpace(para)
	if para == "" {
		return ""
	}
	for i, ch := range para {
		if (ch == '.' || ch == '!' || ch == '?') && i > 10 {
			return para[:i+1]
		}
	}
	if len(para) <= 150 {
		return para
	}
	idx := strings.LastIndex(para[:150], " ")
	if idx < 50 {
		idx = 150
	}
	return para[:idx] + "..."
}

func stripMarkdown(s string) string {
	for strings.HasPrefix(s, "#") {
		s = strings.TrimLeft(s, "# ")
	}
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "__", "")
	s = strings.ReplaceAll(s, "*", "")
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.ReplaceAll(s, "`", "")
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
	s := strings.TrimSpace(current.String())
	if s != "" {
		sentences = append(sentences, s)
	}
	return sentences
}

func titleCase(s string) string {
	if s == "" {
		return ""
	}
	words := strings.Fields(s)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

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

// ---------------------------------------------------------------------------
// Local fallbacks: Auto-Linker, Flashcard Generator, Tone Adjuster
// ---------------------------------------------------------------------------

func (b *Bots) runLocalAutoLinker() {
	var result []string
	result = append(result, lipgloss.NewStyle().Foreground(text).Bold(true).Render(
		"  Auto-Link Suggestions (Local Analysis)"))
	result = append(result, "")

	currentName := strings.TrimSuffix(filepath.Base(b.currentPath), ".md")
	bodyLower := strings.ToLower(b.currentBody)

	found := 0
	for path := range b.notes {
		name := strings.TrimSuffix(filepath.Base(path), ".md")
		if name == currentName || len(name) < 3 {
			continue
		}
		if strings.Contains(bodyLower, strings.ToLower(name)) {
			// Check it's not already linked
			if !strings.Contains(b.currentBody, "[["+name+"]]") {
				result = append(result, lipgloss.NewStyle().Foreground(green).Render(
					fmt.Sprintf("  LINK: %s -> [[%s]]", name, name)))
				found++
				if found >= 10 {
					break
				}
			}
		}
	}
	if found == 0 {
		result = append(result, DimStyle.Render("  No link suggestions found."))
	}

	b.resultLines = result
}

func (b *Bots) runLocalFlashcardGen() {
	var result []string
	result = append(result, lipgloss.NewStyle().Foreground(text).Bold(true).Render(
		"  Flashcards (Local Extraction)"))
	result = append(result, "")

	// Extract headings and first sentences as Q&A pairs
	lines := strings.Split(b.currentBody, "\n")
	cardNum := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") || strings.HasPrefix(trimmed, "### ") {
			topic := strings.TrimLeft(trimmed, "# ")
			// Find first non-empty line after heading
			answer := ""
			for j := i + 1; j < len(lines) && j < i+4; j++ {
				a := strings.TrimSpace(lines[j])
				if a != "" && !strings.HasPrefix(a, "#") {
					answer = a
					break
				}
			}
			if answer != "" && len(answer) > 10 {
				cardNum++
				if len(answer) > 120 {
					answer = answer[:120] + "..."
				}
				result = append(result, lipgloss.NewStyle().Foreground(blue).Render(
					fmt.Sprintf("  Q%d: What is %s?", cardNum, topic)))
				result = append(result, DimStyle.Render(
					fmt.Sprintf("  A%d: %s", cardNum, answer)))
				result = append(result, "")
				if cardNum >= 8 {
					break
				}
			}
		}
	}
	if cardNum == 0 {
		result = append(result, DimStyle.Render("  Add headings (##) to generate flashcards locally."))
		result = append(result, DimStyle.Render("  Use Ollama for AI-powered flashcard generation."))
	}

	b.resultLines = result
}

func (b *Bots) runLocalToneAdjuster() {
	var result []string
	result = append(result, lipgloss.NewStyle().Foreground(text).Bold(true).Render(
		"  Tone Adjustment (Local)"))
	result = append(result, "")
	result = append(result, DimStyle.Render("  Tone adjustment requires an AI provider (Ollama recommended)."))
	result = append(result, DimStyle.Render("  Set ai_provider to 'ollama' in Settings (Ctrl+,)."))

	b.resultLines = result
}
