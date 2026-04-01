package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ---------------------------------------------------------------------------
// Ghost text suggestion messages
// ---------------------------------------------------------------------------

// ghostSuggestionMsg carries the AI completion back to the caller.
type ghostSuggestionMsg struct {
	text string
	err  error
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
	provider      string
	model         string
	ollamaURL     string
	apiKey        string
	nousURL       string
	nousAPIKey    string
	nerveBinary   string
	nerveModel    string
	nerveProvider string

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

	// Internal: shuttled from OnEdit to debounce handler
	contextBuf  string
	requestTime time.Time

	// Last error from AI provider (cleared on next successful suggestion)
	lastError string
}

// NewGhostWriter creates a GhostWriter with sensible defaults.
func NewGhostWriter() *GhostWriter {
	return &GhostWriter{
		enabled:      false,
		provider:     "ollama",
		model:        "llama3.2",
		ollamaURL:    "http://localhost:11434",
		debounceMs:   800,
		contextLines: 30,
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

// SetConfig sets the AI provider configuration.
func (gw *GhostWriter) SetConfig(provider, model, ollamaURL, apiKey string, nousOpts ...string) {
	if provider != "" {
		gw.provider = provider
	}
	if model != "" {
		gw.model = model
	}
	if ollamaURL != "" {
		gw.ollamaURL = ollamaURL
	}
	gw.apiKey = apiKey
	if len(nousOpts) > 0 && nousOpts[0] != "" {
		gw.nousURL = nousOpts[0]
	}
	if len(nousOpts) > 1 {
		gw.nousAPIKey = nousOpts[1]
	}
	if len(nousOpts) > 2 {
		gw.nerveBinary = nousOpts[2]
	}
	if len(nousOpts) > 3 {
		gw.nerveModel = nousOpts[3]
	}
	if len(nousOpts) > 4 {
		gw.nerveProvider = nousOpts[4]
	}
}

// SetNoteTitle updates the note title used for ghost writer context.
func (gw *GhostWriter) SetNoteTitle(title string) {
	gw.noteTitle = title
}

// SetNoteTags updates the tags used for ghost writer context grounding.
func (gw *GhostWriter) SetNoteTags(tags string) {
	gw.noteTags = tags
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
// caller should insert the returned text at the cursor position.
func (gw *GhostWriter) Accept() string {
	s := gw.suggestion
	gw.suggestion = ""
	gw.pending = false
	return s
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

	// Capture values for the closure.
	editTime := gw.lastEdit
	debounce := time.Duration(gw.debounceMs) * time.Millisecond

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
	startLine := cursorLine - gw.contextLines
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

// ---------------------------------------------------------------------------
// AI request
// ---------------------------------------------------------------------------

// ghostOllamaRequest is the JSON body for Ollama /api/generate.
type ghostOllamaRequest struct {
	Model   string             `json:"model"`
	Prompt  string             `json:"prompt"`
	Stream  bool               `json:"stream"`
	Options ghostOllamaOptions `json:"options"`
}

type ghostOllamaOptions struct {
	NumPredict  int     `json:"num_predict"`
	Temperature float64 `json:"temperature"`
}

type ghostOllamaResponse struct {
	Response string `json:"response"`
}

// ghostOpenAIRequest is the JSON body for OpenAI /v1/chat/completions.
type ghostOpenAIRequest struct {
	Model       string               `json:"model"`
	Messages    []ghostOpenAIMessage `json:"messages"`
	MaxTokens   int                  `json:"max_tokens"`
	Temperature float64              `json:"temperature"`
}

type ghostOpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ghostOpenAIResponse struct {
	Choices []ghostOpenAIChoice `json:"choices"`
	Error   *ghostOpenAIError   `json:"error,omitempty"`
}

type ghostOpenAIChoice struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
}

type ghostOpenAIError struct {
	Message string `json:"message"`
}

// requestCompletion returns a tea.Cmd that calls the configured AI provider
// and sends back a ghostSuggestionMsg with the cleaned-up completion.
func (gw *GhostWriter) requestCompletion(context string) tea.Cmd {
	provider := gw.provider
	model := gw.model
	ollamaURL := gw.ollamaURL
	apiKey := gw.apiKey
	maxTokens := gw.maxTokens

	nousURL := gw.nousURL
	nousAPIKey := gw.nousAPIKey
	nerveBinary := gw.nerveBinary
	nerveModel := gw.nerveModel
	nerveProvider := gw.nerveProvider

	return func() tea.Msg {
		var raw string
		var err error

		switch provider {
		case "openai":
			raw, err = ghostCallOpenAI(apiKey, model, context, maxTokens)
		case "nous":
			raw, err = ghostCallNous(nousURL, nousAPIKey, context)
		case "nerve":
			raw, err = ghostCallNerve(nerveBinary, nerveModel, nerveProvider, context)
		default: // "ollama"
			raw, err = ghostCallOllama(ollamaURL, model, context, maxTokens)
		}

		if err != nil {
			return ghostSuggestionMsg{err: err}
		}

		cleaned := ghostCleanCompletion(raw)
		if cleaned == "" {
			return ghostSuggestionMsg{err: fmt.Errorf("empty completion")}
		}

		return ghostSuggestionMsg{text: cleaned}
	}
}

// ---------------------------------------------------------------------------
// Provider calls
// ---------------------------------------------------------------------------

func ghostCallOllama(url, model, context string, maxTokens int) (string, error) {
	reqBody := ghostOllamaRequest{
		Model:  model,
		Prompt: context,
		Stream: false,
		Options: ghostOllamaOptions{
			NumPredict:  maxTokens,
			Temperature: 0.3,
		},
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(url+"/api/generate", "application/json", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("Ghost Writer: cannot connect to Ollama at %s — is it running? Try: ollama serve", url)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Ghost Writer: Ollama error (status %d) — check model is pulled: ollama pull %s", resp.StatusCode, model)
	}

	var olResp ghostOllamaResponse
	if err := json.Unmarshal(body, &olResp); err != nil {
		return "", err
	}

	return olResp.Response, nil
}

func ghostCallOpenAI(apiKey, model, context string, maxTokens int) (string, error) {
	reqBody := ghostOpenAIRequest{
		Model: model,
		Messages: []ghostOpenAIMessage{
			{
				Role:    "system",
				Content: "Continue the text naturally. Only output the continuation, no explanation.",
			},
			{
				Role:    "user",
				Content: context,
			},
		},
		MaxTokens:   maxTokens,
		Temperature: 0.3,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("Ghost Writer: cannot reach OpenAI API — check your internet connection")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var oaiResp ghostOpenAIResponse
	if err := json.Unmarshal(body, &oaiResp); err != nil {
		return "", err
	}

	if oaiResp.Error != nil {
		hint := oaiResp.Error.Message
		if strings.Contains(hint, "auth") || strings.Contains(hint, "key") {
			hint += " — check your API key in Settings (Ctrl+,)"
		}
		return "", fmt.Errorf("Ghost Writer: OpenAI: %s", hint)
	}

	if len(oaiResp.Choices) == 0 {
		return "", fmt.Errorf("Ghost Writer: OpenAI returned an empty response")
	}

	return oaiResp.Choices[0].Message.Content, nil
}

func ghostCallNerve(binary, model, provider, context string) (string, error) {
	client := NewNerveClient(binary, model, provider)
	prompt := "Continue the following text naturally. Only output the continuation, no explanation.\n\n" + context
	resp, err := client.Chat("", prompt, 30*time.Second)
	if err != nil {
		return "", fmt.Errorf("Ghost Writer: %v", err)
	}
	return resp, nil
}

func ghostCallNous(url, apiKey, context string) (string, error) {
	client := NewNousClient(url, apiKey)
	prompt := "Continue the following text naturally. Only output the continuation, no explanation.\n\n" + context
	resp, err := client.Chat(prompt)
	if err != nil {
		return "", fmt.Errorf("Ghost Writer: %v", err)
	}
	return resp, nil
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

	// Stop at the first ". " (period followed by space) — keep the period.
	if idx := strings.Index(s, ". "); idx >= 0 {
		s = s[:idx+1]
	}

	return strings.TrimSpace(s)
}
