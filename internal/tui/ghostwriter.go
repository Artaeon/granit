package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)


// ---------------------------------------------------------------------------
// Ghost text suggestion messages
// ---------------------------------------------------------------------------

// ghostSuggestionMsg carries the AI completion back to the caller.
type ghostSuggestionMsg struct {
	text       string
	err        error
	contextKey string // cache key so the handler can store the result
}

// ghostDebounceMsg is sent after the debounce delay to check whether a
// completion request should be fired.
type ghostDebounceMsg struct {
	editTime time.Time // the edit time this debounce was scheduled for
}

// ---------------------------------------------------------------------------
// GhostWriter — inline AI writing assistant (ghost text completions)
// ---------------------------------------------------------------------------

// GhostWriter generates inline completion suggestions ("ghost text") as the
// user types in the editor. After a brief pause it sends the surrounding
// context to an AI provider and stores the resulting suggestion. The caller
// is responsible for rendering the dimmed ghost text and handling accept/dismiss.
type GhostWriter struct {
	enabled bool

	// AI config
	ai AIConfig

	// State
	suggestion string    // the current ghost text suggestion
	pending    bool      // waiting for AI response
	lastEdit   time.Time // when the user last typed
	debounceMs int       // milliseconds to wait before requesting (default: 800)

	// Context
	contextLines int    // number of lines before cursor to send as context (default: 30)
	maxTokens    int    // max tokens for completion (default: 50)
	noteTitle    string // current note title for context
	noteTags     string // comma-separated tags for topical grounding

	// Vault-grounded suggestions: note path -> content (set externally)
	vaultNotes map[string]string

	// Accept-and-continue: flag set after accepting a suggestion
	acceptedJustNow bool

	// Internal: shuttled from OnEdit to debounce handler
	contextBuf  string
	requestTime time.Time

	// Last error from AI provider (cleared on next successful suggestion)
	lastError string

	// Completion cache — maps context hash → cleaned suggestion.
	// When the user backspaces and retypes the same content, we reuse the
	// prior completion instead of calling the AI again. Bounded in size.
	cache     map[string]string
	cacheKeys []string // insertion order for simple FIFO eviction
}

const ghostCacheMaxEntries = 32

// SetAI updates the AI configuration and invalidates the completion cache
// if the provider or model has changed (different model → different style).
func (gw *GhostWriter) SetAI(cfg AIConfig) {
	if cfg.Provider != gw.ai.Provider || cfg.Model != gw.ai.Model {
		gw.cache = nil
		gw.cacheKeys = nil
	}
	gw.ai = cfg
}

// cacheGet returns a cached completion for the given context, if any.
func (gw *GhostWriter) cacheGet(key string) (string, bool) {
	if gw.cache == nil {
		return "", false
	}
	v, ok := gw.cache[key]
	return v, ok
}

// cachePut stores a completion, evicting the oldest entry if at capacity.
func (gw *GhostWriter) cachePut(key, value string) {
	if gw.cache == nil {
		gw.cache = make(map[string]string, ghostCacheMaxEntries)
	}
	if _, exists := gw.cache[key]; exists {
		gw.cache[key] = value
		return
	}
	if len(gw.cacheKeys) >= ghostCacheMaxEntries {
		oldest := gw.cacheKeys[0]
		delete(gw.cache, oldest)
		gw.cacheKeys = gw.cacheKeys[1:]
	}
	gw.cache[key] = value
	gw.cacheKeys = append(gw.cacheKeys, key)
}

// NewGhostWriter creates a GhostWriter with sensible defaults.
func NewGhostWriter() *GhostWriter {
	return &GhostWriter{
		enabled: false,
		ai: AIConfig{
			Provider:  "ollama",
			Model:     "qwen2.5:0.5b",
			OllamaURL: "http://localhost:11434",
		},
		debounceMs:   800,
		contextLines: 50,
		maxTokens:    50,
	}
}

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

// SetEnabled enables or disables the ghost writer.
func (gw *GhostWriter) SetEnabled(enabled bool) {
	gw.enabled = enabled
	if !enabled {
		gw.suggestion = ""
		gw.pending = false
	}
}

// IsEnabled returns whether the ghost writer is active.
func (gw *GhostWriter) IsEnabled() bool {
	return gw.enabled
}

// SetNoteTitle updates the note title used for ghost writer context.
func (gw *GhostWriter) SetNoteTitle(title string) {
	gw.noteTitle = title
}

// SetNoteTags updates the tags used for ghost writer context grounding.
func (gw *GhostWriter) SetNoteTags(tags string) {
	gw.noteTags = tags
}

// SetVaultNotes provides vault note titles and content for grounded suggestions.
func (gw *GhostWriter) SetVaultNotes(notes map[string]string) {
	if notes == nil {
		notes = make(map[string]string)
	}
	gw.vaultNotes = notes
}

// ---------------------------------------------------------------------------
// Suggestion accessors
// ---------------------------------------------------------------------------

// GetSuggestion returns the current ghost text suggestion (empty string if
// none is available).
func (gw *GhostWriter) GetSuggestion() string {
	return gw.suggestion
}

// Accept returns the current suggestion and clears the internal state. The
// caller should insert the returned text at the cursor position. It also sets
// acceptedJustNow so the caller can trigger an immediate follow-up request.
func (gw *GhostWriter) Accept() string {
	s := gw.suggestion
	gw.suggestion = ""
	gw.pending = false
	gw.acceptedJustNow = true
	return s
}

// ShouldAutoRequest returns true once after a suggestion was accepted, allowing
// the caller to immediately trigger a new completion without waiting for debounce.
func (gw *GhostWriter) ShouldAutoRequest() bool {
	if gw.acceptedJustNow {
		gw.acceptedJustNow = false
		return true
	}
	return false
}

// Dismiss clears the current suggestion without inserting it.
func (gw *GhostWriter) Dismiss() {
	gw.suggestion = ""
}

// ConsumeError returns and clears the last error message, if any.
func (gw *GhostWriter) ConsumeError() string {
	e := gw.lastError
	gw.lastError = ""
	return e
}

// ---------------------------------------------------------------------------
// Edit hook & debounce
// ---------------------------------------------------------------------------

// OnEdit should be called whenever the user modifies the editor content. It
// records the edit time, clears any existing suggestion, and returns a Cmd
// that will fire a ghostDebounceMsg after the configured delay.
func (gw *GhostWriter) OnEdit(content []string, cursorLine, cursorCol int) tea.Cmd {
	if !gw.enabled {
		return nil
	}

	gw.lastEdit = time.Now()
	gw.suggestion = ""

	// Build and store context for the upcoming debounce handler.
	gw.buildContext(content, cursorLine, cursorCol)

	// Capture values for the closure. Small models benefit from a longer
	// debounce since they are slower — avoids firing requests that will
	// be superseded before they return.
	editTime := gw.lastEdit
	debounceMs := gw.debounceMs
	if gw.ai.IsSmallModel() && debounceMs < 1500 {
		debounceMs = 1500
	}
	debounce := time.Duration(debounceMs) * time.Millisecond

	return func() tea.Msg {
		time.Sleep(debounce)
		return ghostDebounceMsg{editTime: editTime}
	}
}

// ---------------------------------------------------------------------------
// Message handler
// ---------------------------------------------------------------------------

// HandleMsg processes ghostDebounceMsg and ghostSuggestionMsg. It returns a
// tea.Cmd when an AI request needs to be dispatched.
func (gw *GhostWriter) HandleMsg(msg tea.Msg) tea.Cmd {
	if !gw.enabled {
		return nil
	}

	switch msg := msg.(type) {
	case ghostDebounceMsg:
		// Only proceed if no newer edit has occurred since this debounce was
		// scheduled.
		if msg.editTime.Equal(gw.lastEdit) && !gw.pending {
			if gw.contextBuf == "" {
				return nil
			}
			// Check cache first — avoids a round-trip when the user
			// backspaced and retyped the same content.
			if cached, ok := gw.cacheGet(gw.contextBuf); ok {
				gw.suggestion = cached
				gw.lastError = ""
				return nil
			}
			gw.pending = true
			gw.requestTime = gw.lastEdit
			return gw.requestCompletion(gw.contextBuf)
		}

	case ghostSuggestionMsg:
		gw.pending = false
		if msg.err != nil {
			gw.lastError = msg.err.Error()
			return nil
		}
		gw.lastError = ""
		// Only accept the suggestion if no newer edits happened while
		// the request was in flight.
		if gw.requestTime.Equal(gw.lastEdit) {
			gw.suggestion = msg.text
		}
		// Cache regardless of staleness so future typing can reuse it.
		if msg.contextKey != "" && msg.text != "" {
			gw.cachePut(msg.contextKey, msg.text)
		}
	}

	return nil
}

// ---------------------------------------------------------------------------
// Context building
// ---------------------------------------------------------------------------

// buildContext extracts the text surrounding the cursor and stores it in
// gw.contextBuf for the debounce handler. Called from OnEdit.
func (gw *GhostWriter) buildContext(content []string, cursorLine, cursorCol int) {
	if len(content) == 0 {
		gw.contextBuf = ""
		return
	}

	// Clamp cursor line.
	if cursorLine < 0 {
		cursorLine = 0
	}
	if cursorLine >= len(content) {
		cursorLine = len(content) - 1
	}

	// Determine the range of lines to include (up to contextLines before cursor).
	// Small models benefit from less context — faster inference, better focus.
	ctxLines := gw.contextLines
	if gw.ai.IsSmallModel() && ctxLines > 15 {
		ctxLines = 15
	}
	startLine := cursorLine - ctxLines
	if startLine < 0 {
		startLine = 0
	}

	var b strings.Builder

	// Prepend note title for topic awareness.
	if gw.noteTitle != "" {
		b.WriteString("# ")
		b.WriteString(strings.TrimSuffix(gw.noteTitle, ".md"))
		b.WriteString("\n")
	}

	// Add tags for topical grounding (helps 0.5b stay on topic).
	if gw.noteTags != "" {
		b.WriteString("Tags: " + gw.noteTags + "\n")
	}
	b.WriteString("\n")

	// Document outline — all headings for structural awareness.
	if outline := gw.buildOutline(content); outline != "" {
		b.WriteString("[Document Outline]\n")
		b.WriteString(outline)
		b.WriteString("\n\n")
	}

	// Vault-grounded context — find the most relevant vault note by keyword
	// overlap with the current line.
	if len(gw.vaultNotes) > 0 && cursorLine < len(content) {
		if title, snippet := gw.findRelatedVaultNote(content[cursorLine]); title != "" {
			b.WriteString("[Related Note: ")
			b.WriteString(title)
			b.WriteString("]\n")
			b.WriteString(snippet)
			b.WriteString("\n\n")
		}
	}

	// Skip YAML frontmatter in context window.
	fmEnd := 0
	if len(content) > 0 && strings.TrimSpace(content[0]) == "---" {
		for i := 1; i < len(content); i++ {
			if strings.TrimSpace(content[i]) == "---" {
				fmEnd = i + 1
				break
			}
		}
	}
	if startLine < fmEnd {
		startLine = fmEnd
	}

	// Find the nearest heading above the cursor (if outside context window).
	if startLine > 0 {
		for i := startLine - 1; i >= 0; i-- {
			trimmed := strings.TrimSpace(content[i])
			if strings.HasPrefix(trimmed, "#") {
				b.WriteString(trimmed)
				b.WriteString("\n...\n")
				break
			}
		}
	}

	// Full lines before the cursor line.
	for i := startLine; i < cursorLine; i++ {
		b.WriteString(content[i])
		b.WriteString("\n")
	}

	// Current line up to cursor position.
	line := content[cursorLine]
	col := cursorCol
	if col < 0 {
		col = 0
	}
	if col > len(line) {
		col = len(line)
	}
	b.WriteString(line[:col])

	gw.contextBuf = b.String()
}

// buildOutline extracts all heading lines (starting with #) from the content.
func (gw *GhostWriter) buildOutline(content []string) string {
	var headings []string
	for _, line := range content {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			headings = append(headings, trimmed)
		}
	}
	return strings.Join(headings, "\n")
}

// findRelatedVaultNote returns the vault note title and a snippet whose title
// best matches the given line. Uses whole-word matching to avoid false
// positives like "test" matching "testing" or "contest". Returns empty
// strings if no strong match is found.
func (gw *GhostWriter) findRelatedVaultNote(currentLine string) (string, string) {
	if gw.vaultNotes == nil {
		return "", ""
	}
	words := strings.Fields(strings.ToLower(currentLine))
	if len(words) == 0 {
		return "", ""
	}
	// Filter to meaningful words.
	var keywords []string
	for _, w := range words {
		w = strings.Trim(w, ".,;:!?\"'()[]{}#-")
		if len(w) >= 4 && !aiChatStopwords[w] {
			keywords = append(keywords, w)
		}
	}
	if len(keywords) == 0 {
		return "", ""
	}

	bestTitle := ""
	bestScore := 0
	for title := range gw.vaultNotes {
		// Tokenize title on non-letter boundaries for whole-word matching.
		titleWords := make(map[string]bool)
		for _, tw := range strings.FieldsFunc(strings.ToLower(title), func(r rune) bool {
			return r == ' ' || r == '-' || r == '_' || r == '.' || r == '/'
		}) {
			if len(tw) >= 3 {
				titleWords[tw] = true
			}
		}
		score := 0
		for _, w := range keywords {
			if titleWords[w] {
				score += 2
			} else if strings.Contains(strings.ToLower(title), w) {
				// Weak substring match still counts, but less.
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			bestTitle = title
		}
	}

	// Require at least one strong (whole-word) match to avoid weak noise.
	if bestScore < 2 {
		return "", ""
	}

	snippet := gw.vaultNotes[bestTitle]
	if len(snippet) > 200 {
		snippet = truncateAtBoundary(snippet, 200) + "..."
	}
	return bestTitle, snippet
}

// ---------------------------------------------------------------------------
// AI request
// ---------------------------------------------------------------------------

// requestCompletion returns a tea.Cmd that calls the configured AI provider
// and sends back a ghostSuggestionMsg with the cleaned-up completion.
func (gw *GhostWriter) requestCompletion(noteCtx string) tea.Cmd {
	ai := gw.ai
	cacheKey := noteCtx

	return func() tea.Msg {
		var raw string
		var err error

		systemPrompt := "Continue the text naturally. Only output the continuation, no explanation."

		// Hard cap per-completion so slow models don't pile up stale requests.
		// Small models get more headroom.
		deadline := 15 * time.Second
		if ai.IsSmallModel() {
			deadline = 30 * time.Second
		}
		ctx, cancel := context.WithTimeout(context.Background(), deadline)
		defer cancel()

		raw, err = ai.ChatShortCtx(ctx, systemPrompt, noteCtx)

		if err != nil {
			return ghostSuggestionMsg{err: fmt.Errorf("ghost writer: %v", err), contextKey: cacheKey}
		}

		cleaned := ghostCleanCompletion(raw)
		if cleaned == "" {
			return ghostSuggestionMsg{err: fmt.Errorf("empty completion"), contextKey: cacheKey}
		}

		return ghostSuggestionMsg{text: cleaned, contextKey: cacheKey}
	}
}

// ---------------------------------------------------------------------------
// Completion cleanup
// ---------------------------------------------------------------------------

// ghostCleanCompletion takes the raw AI output and returns only the first
// line or sentence — whichever is shorter. This keeps ghost text brief.
func ghostCleanCompletion(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}

	// Stop at the first newline.
	if idx := strings.Index(s, "\n"); idx >= 0 {
		s = s[:idx]
	}

	// Stop at a sentence boundary — ". " followed by an uppercase letter,
	// but only when the word before the period is ≥4 letters long (so that
	// abbreviations like "Dr.", "Mr.", "e.g." don't trigger a premature cut).
	for i := 0; i < len(s)-2; i++ {
		if s[i] != '.' || s[i+1] != ' ' || s[i+2] < 'A' || s[i+2] > 'Z' {
			continue
		}
		// Count contiguous letters immediately before the period.
		letters := 0
		for j := i - 1; j >= 0; j-- {
			c := s[j]
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
				letters++
			} else {
				break
			}
		}
		if letters >= 4 {
			s = s[:i+1]
			break
		}
	}

	return strings.TrimSpace(s)
}
