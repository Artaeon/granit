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
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Tea messages for Matrix AI results
// ---------------------------------------------------------------------------

type matrixAISummaryMsg struct {
	Result string
	Err    error
}

type matrixAIActionsMsg struct {
	Result string
	Err    error
}

type matrixAIReplyMsg struct {
	Suggestions []string
	Err         error
}

type matrixAITranslateMsg struct {
	Result string
	Err    error
}

type matrixAINoteMsg struct {
	Result string
	Err    error
}

type matrixAIContextMsg struct {
	Result string
	Err    error
}

// ---------------------------------------------------------------------------
// MatrixAI result types
// ---------------------------------------------------------------------------

type matrixAIResultType int

const (
	matrixAIResultNone matrixAIResultType = iota
	matrixAIResultSummary
	matrixAIResultActions
	matrixAIResultReply
	matrixAIResultTranslate
	matrixAIResultNote
	matrixAIResultContext
)

// ---------------------------------------------------------------------------
// MatrixAI component
// ---------------------------------------------------------------------------

// MatrixAI provides AI-powered chat analysis features for the Matrix overlay.
type MatrixAI struct {
	// AI configuration
	provider    string // "ollama", "openai", "local"
	ollamaURL   string
	ollamaModel string
	openaiKey   string
	openaiModel string

	// State
	active     bool
	processing bool
	lastResult string
	lastError  string
	resultType matrixAIResultType

	// Reply suggestions (Feature 3)
	replySuggestions []string
	replySelected   int

	// Scroll for long results
	scroll int

	// Loading animation
	loadingTick int

	// Width/height for rendering
	width  int
	height int
}

// NewMatrixAI creates a new MatrixAI instance.
func NewMatrixAI() MatrixAI {
	return MatrixAI{
		provider:    "local",
		ollamaModel: "llama3.2",
		ollamaURL:   "http://localhost:11434",
		openaiModel: "gpt-4o-mini",
	}
}

// SetAIConfig updates the AI provider settings.
func (mai *MatrixAI) SetAIConfig(provider, ollamaModel, ollamaURL, openaiKey, openaiModel string) {
	mai.provider = provider
	if mai.provider == "" {
		mai.provider = "local"
	}
	mai.ollamaModel = ollamaModel
	if mai.ollamaModel == "" {
		mai.ollamaModel = "qwen2.5:0.5b"
	}
	mai.ollamaURL = ollamaURL
	if mai.ollamaURL == "" {
		mai.ollamaURL = "http://localhost:11434"
	}
	mai.openaiKey = openaiKey
	mai.openaiModel = openaiModel
	if mai.openaiModel == "" {
		mai.openaiModel = "gpt-4o-mini"
	}
}

// SetSize sets the available render area.
func (mai *MatrixAI) SetSize(width, height int) {
	mai.width = width
	mai.height = height
}

// IsActive returns whether the AI result panel is visible.
func (mai *MatrixAI) IsActive() bool {
	return mai.active
}

// IsProcessing returns whether an AI request is in progress.
func (mai *MatrixAI) IsProcessing() bool {
	return mai.processing
}

// Close dismisses the AI result panel.
func (mai *MatrixAI) Close() {
	mai.active = false
	mai.processing = false
	mai.lastResult = ""
	mai.lastError = ""
	mai.resultType = matrixAIResultNone
	mai.replySuggestions = nil
	mai.replySelected = 0
	mai.scroll = 0
}

// GetCaptureContent returns the current result text suitable for saving as a note.
func (mai *MatrixAI) GetCaptureContent() string {
	if mai.lastResult == "" {
		return ""
	}
	var title string
	switch mai.resultType {
	case matrixAIResultSummary:
		title = "# Chat Summary"
	case matrixAIResultActions:
		title = "# Action Items from Chat"
	case matrixAIResultNote:
		return mai.lastResult // already has a title
	case matrixAIResultContext:
		title = "# Chat-Note Connections"
	default:
		title = "# Chat AI Result"
	}
	return title + "\n\n" + mai.lastResult + "\n"
}

// GetSelectedReply returns the currently picked reply suggestion, or empty.
func (mai *MatrixAI) GetSelectedReply() string {
	if mai.resultType != matrixAIResultReply {
		return ""
	}
	if mai.replySelected < 0 || mai.replySelected >= len(mai.replySuggestions) {
		return ""
	}
	return mai.replySuggestions[mai.replySelected]
}

// ---------------------------------------------------------------------------
// Loading tick
// ---------------------------------------------------------------------------

type matrixAITickMsg struct{}

func matrixAITickCmd() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
		return matrixAITickMsg{}
	})
}

// ---------------------------------------------------------------------------
// HandleKey processes a keybinding and returns a tea.Cmd if an AI action
// should be triggered. Called from the Matrix overlay's Update.
// ---------------------------------------------------------------------------

func (mai *MatrixAI) HandleKey(key string, messages []matrixChatMessage, currentNote string) tea.Cmd {
	switch key {
	case "ctrl+s":
		return mai.startSummary(messages)
	case "ctrl+a":
		return mai.startActions(messages)
	case "ctrl+r":
		return mai.startReplySuggestions(messages, currentNote)
	case "ctrl+t":
		return mai.startTranslation(messages)
	case "ctrl+n":
		return mai.startNoteGeneration(messages)
	case "ctrl+i":
		return mai.startContextAnalysis(messages, currentNote)
	}
	return nil
}

// ---------------------------------------------------------------------------
// AI action starters
// ---------------------------------------------------------------------------

func (mai *MatrixAI) collectConversation(messages []matrixChatMessage, limit int) string {
	start := 0
	if len(messages) > limit {
		start = len(messages) - limit
	}
	var b strings.Builder
	for _, msg := range messages[start:] {
		ts := msg.Timestamp.Format("15:04")
		b.WriteString(fmt.Sprintf("[%s] %s: %s\n", ts, msg.Sender, msg.Body))
	}
	return b.String()
}

func (mai *MatrixAI) startSummary(messages []matrixChatMessage) tea.Cmd {
	if len(messages) == 0 {
		mai.active = true
		mai.lastError = "No messages to summarize"
		mai.resultType = matrixAIResultSummary
		return nil
	}

	conversation := mai.collectConversation(messages, 50)
	prompt := fmt.Sprintf(`Summarize this conversation concisely. Highlight key decisions, action items, and important points.

Conversation:
---
%s
---

Provide a clear, structured summary.`, conversation)

	mai.active = true
	mai.processing = true
	mai.lastResult = ""
	mai.lastError = ""
	mai.resultType = matrixAIResultSummary
	mai.scroll = 0

	return tea.Batch(mai.callAI(prompt, matrixAIResultSummary), matrixAITickCmd())
}

func (mai *MatrixAI) startActions(messages []matrixChatMessage) tea.Cmd {
	if len(messages) == 0 {
		mai.active = true
		mai.lastError = "No messages to analyze"
		mai.resultType = matrixAIResultActions
		return nil
	}

	conversation := mai.collectConversation(messages, 50)
	prompt := fmt.Sprintf(`Extract all action items, tasks, and commitments from this conversation. Format each as a checkbox item: - [ ] description (assigned to: person, deadline: date if mentioned)

Conversation:
---
%s
---

List ONLY the action items, nothing else.`, conversation)

	mai.active = true
	mai.processing = true
	mai.lastResult = ""
	mai.lastError = ""
	mai.resultType = matrixAIResultActions
	mai.scroll = 0

	return tea.Batch(mai.callAI(prompt, matrixAIResultActions), matrixAITickCmd())
}

func (mai *MatrixAI) startReplySuggestions(messages []matrixChatMessage, currentNote string) tea.Cmd {
	if len(messages) == 0 {
		mai.active = true
		mai.lastError = "No messages for context"
		mai.resultType = matrixAIResultReply
		return nil
	}

	conversation := mai.collectConversation(messages, 10)
	noteContext := ""
	if currentNote != "" {
		nc := currentNote
		if len(nc) > 500 {
			nc = nc[:500]
		}
		noteContext = fmt.Sprintf("\n\nNote being edited:\n---\n%s\n---\n", nc)
	}

	prompt := fmt.Sprintf(`Given this conversation context, suggest 3 brief, natural replies. Consider the tone and topic. Format as a numbered list (1. 2. 3.).%s

Conversation:
---
%s
---

Provide ONLY the 3 numbered suggestions, nothing else.`, noteContext, conversation)

	mai.active = true
	mai.processing = true
	mai.lastResult = ""
	mai.lastError = ""
	mai.resultType = matrixAIResultReply
	mai.replySuggestions = nil
	mai.replySelected = 0
	mai.scroll = 0

	return tea.Batch(mai.callAI(prompt, matrixAIResultReply), matrixAITickCmd())
}

func (mai *MatrixAI) startTranslation(messages []matrixChatMessage) tea.Cmd {
	if len(messages) == 0 {
		mai.active = true
		mai.lastError = "No messages to translate"
		mai.resultType = matrixAIResultTranslate
		return nil
	}

	// Translate the last message
	lastMsg := messages[len(messages)-1]
	prompt := fmt.Sprintf(`Translate the following message to English. If already in English, translate to German. Provide only the translation, nothing else.

Message from %s:
%s`, lastMsg.Sender, lastMsg.Body)

	mai.active = true
	mai.processing = true
	mai.lastResult = ""
	mai.lastError = ""
	mai.resultType = matrixAIResultTranslate
	mai.scroll = 0

	return tea.Batch(mai.callAI(prompt, matrixAIResultTranslate), matrixAITickCmd())
}

func (mai *MatrixAI) startNoteGeneration(messages []matrixChatMessage) tea.Cmd {
	if len(messages) == 0 {
		mai.active = true
		mai.lastError = "No messages to convert"
		mai.resultType = matrixAIResultNote
		return nil
	}

	conversation := mai.collectConversation(messages, 50)
	prompt := fmt.Sprintf(`Convert this conversation into a well-structured markdown note. Include: a title (as # heading), key topics as headers, important information organized logically, action items at the end. Use proper markdown formatting.

Conversation:
---
%s
---`, conversation)

	mai.active = true
	mai.processing = true
	mai.lastResult = ""
	mai.lastError = ""
	mai.resultType = matrixAIResultNote
	mai.scroll = 0

	return tea.Batch(mai.callAI(prompt, matrixAIResultNote), matrixAITickCmd())
}

func (mai *MatrixAI) startContextAnalysis(messages []matrixChatMessage, currentNote string) tea.Cmd {
	if len(messages) == 0 || currentNote == "" {
		mai.active = true
		mai.lastError = "Need both chat messages and a note open"
		mai.resultType = matrixAIResultContext
		return nil
	}

	conversation := mai.collectConversation(messages, 20)
	nc := currentNote
	if len(nc) > 1500 {
		nc = nc[:1500]
	}

	prompt := fmt.Sprintf(`Given this note context and the recent chat conversation, identify connections. What information from the chat is relevant to this note? Suggest additions or updates.

Note content:
---
%s
---

Chat conversation:
---
%s
---

List specific suggestions for updating the note based on the chat.`, nc, conversation)

	mai.active = true
	mai.processing = true
	mai.lastResult = ""
	mai.lastError = ""
	mai.resultType = matrixAIResultContext
	mai.scroll = 0

	return tea.Batch(mai.callAI(prompt, matrixAIResultContext), matrixAITickCmd())
}

// ---------------------------------------------------------------------------
// AI call dispatcher
// ---------------------------------------------------------------------------

func (mai *MatrixAI) callAI(prompt string, resultType matrixAIResultType) tea.Cmd {
	switch mai.provider {
	case "ollama":
		return mai.callOllama(prompt, resultType)
	case "openai":
		if mai.openaiKey != "" {
			return mai.callOpenAI(prompt, resultType)
		}
		// Fall through to local
		return mai.runLocal(prompt, resultType)
	default:
		return mai.runLocal(prompt, resultType)
	}
}

func (mai *MatrixAI) callOllama(prompt string, resultType matrixAIResultType) tea.Cmd {
	url := mai.ollamaURL
	model := mai.ollamaModel
	return func() tea.Msg {
		reqBody := ollamaRequest{
			Model:  model,
			Prompt: prompt,
			Stream: false,
		}
		data, err := json.Marshal(reqBody)
		if err != nil {
			return matrixAIResultToMsg(resultType, "", err)
		}

		client := &http.Client{Timeout: 120 * time.Second}
		resp, err := client.Post(url+"/api/generate", "application/json", bytes.NewReader(data))
		if err != nil {
			return matrixAIResultToMsg(resultType, "", fmt.Errorf("cannot connect to Ollama at %s: %w", url, err))
		}
		if resp != nil && resp.Body != nil {
			defer resp.Body.Close()
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return matrixAIResultToMsg(resultType, "", err)
		}

		if resp.StatusCode != 200 {
			return matrixAIResultToMsg(resultType, "", fmt.Errorf("Ollama returned status %d: %s", resp.StatusCode, string(body)))
		}

		var ollamaResp ollamaResponse
		if err := json.Unmarshal(body, &ollamaResp); err != nil {
			return matrixAIResultToMsg(resultType, "", err)
		}

		return matrixAIResultToMsg(resultType, ollamaResp.Response, nil)
	}
}

func (mai *MatrixAI) callOpenAI(prompt string, resultType matrixAIResultType) tea.Cmd {
	apiKey := mai.openaiKey
	model := mai.openaiModel
	return func() tea.Msg {
		reqBody := openaiRequest{
			Model: model,
			Messages: []openaiMessage{
				{Role: "system", Content: "You are a helpful chat analysis assistant. Be concise and actionable."},
				{Role: "user", Content: prompt},
			},
		}
		data, err := json.Marshal(reqBody)
		if err != nil {
			return matrixAIResultToMsg(resultType, "", err)
		}

		client := &http.Client{Timeout: 60 * time.Second}
		req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(data))
		if err != nil {
			return matrixAIResultToMsg(resultType, "", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)

		resp, err := client.Do(req)
		if err != nil {
			return matrixAIResultToMsg(resultType, "", fmt.Errorf("cannot connect to OpenAI: %w", err))
		}
		if resp != nil && resp.Body != nil {
			defer resp.Body.Close()
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return matrixAIResultToMsg(resultType, "", err)
		}

		var openaiResp openaiResponse
		if err := json.Unmarshal(body, &openaiResp); err != nil {
			return matrixAIResultToMsg(resultType, "", err)
		}

		if openaiResp.Error != nil {
			return matrixAIResultToMsg(resultType, "", fmt.Errorf("OpenAI error: %s", openaiResp.Error.Message))
		}

		if len(openaiResp.Choices) == 0 {
			return matrixAIResultToMsg(resultType, "", fmt.Errorf("OpenAI returned no choices"))
		}

		return matrixAIResultToMsg(resultType, openaiResp.Choices[0].Message.Content, nil)
	}
}

// runLocal provides a basic local fallback without AI.
func (mai *MatrixAI) runLocal(prompt string, resultType matrixAIResultType) tea.Cmd {
	return func() tea.Msg {
		result := matrixAILocalFallback(prompt, resultType)
		return matrixAIResultToMsg(resultType, result, nil)
	}
}

// matrixAILocalFallback performs basic keyword extraction as a local fallback.
func matrixAILocalFallback(prompt string, resultType matrixAIResultType) string {
	// Extract the conversation from the prompt (between --- markers)
	parts := strings.SplitN(prompt, "---\n", 3)
	conversation := ""
	if len(parts) >= 2 {
		conversation = parts[1]
	}
	if conversation == "" {
		return "Local analysis: no conversation content found."
	}

	lines := strings.Split(strings.TrimSpace(conversation), "\n")

	switch resultType {
	case matrixAIResultSummary:
		return matrixAILocalSummary(lines)
	case matrixAIResultActions:
		return matrixAILocalActions(lines)
	case matrixAIResultReply:
		return matrixAILocalReplies(lines)
	case matrixAIResultTranslate:
		return "Local mode cannot translate. Set AI Provider to 'ollama' or 'openai' for translation."
	case matrixAIResultNote:
		return matrixAILocalNote(lines)
	case matrixAIResultContext:
		return "Local mode cannot analyze context. Set AI Provider to 'ollama' or 'openai' for AI analysis."
	}
	return "Local analysis complete."
}

func matrixAILocalSummary(lines []string) string {
	if len(lines) == 0 {
		return "No messages to summarize."
	}

	// Count messages per person and extract topic keywords
	senders := make(map[string]int)
	wordFreq := make(map[string]int)
	for _, line := range lines {
		// Parse "[HH:MM] sender: body" format
		parts := strings.SplitN(line, ": ", 2)
		if len(parts) < 2 {
			continue
		}
		senderPart := parts[0]
		body := parts[1]

		// Extract sender name from "[HH:MM] sender"
		if idx := strings.Index(senderPart, "] "); idx >= 0 {
			sender := senderPart[idx+2:]
			senders[sender]++
		}

		// Count significant words
		for _, w := range strings.Fields(strings.ToLower(body)) {
			w = strings.Trim(w, ".,!?;:'\"()[]{}@#")
			if len(w) > 3 && !stopwords[w] {
				wordFreq[w]++
			}
		}
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Conversation with %d messages from %d participants.\n\n", len(lines), len(senders)))

	b.WriteString("Participants: ")
	first := true
	for sender, count := range senders {
		if !first {
			b.WriteString(", ")
		}
		b.WriteString(fmt.Sprintf("%s (%d msgs)", sender, count))
		first = false
	}
	b.WriteString("\n\n")

	// Top topics
	type wc struct {
		word  string
		count int
	}
	var topWords []wc
	for w, c := range wordFreq {
		if c >= 2 {
			topWords = append(topWords, wc{w, c})
		}
	}
	if len(topWords) > 0 {
		// Sort by count descending
		for i := 0; i < len(topWords); i++ {
			for j := i + 1; j < len(topWords); j++ {
				if topWords[j].count > topWords[i].count {
					topWords[i], topWords[j] = topWords[j], topWords[i]
				}
			}
		}
		if len(topWords) > 8 {
			topWords = topWords[:8]
		}
		b.WriteString("Key topics: ")
		for i, tw := range topWords {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(tw.word)
		}
		b.WriteString("\n")
	}

	return b.String()
}

func matrixAILocalActions(lines []string) string {
	var actions []string
	actionKeywords := []string{"todo", "task", "need to", "should", "must", "will do", "action", "deadline",
		"assign", "responsible", "follow up", "follow-up", "by tomorrow", "by next", "complete", "finish"}

	for _, line := range lines {
		lower := strings.ToLower(line)
		for _, kw := range actionKeywords {
			if strings.Contains(lower, kw) {
				// Extract the body part
				parts := strings.SplitN(line, ": ", 2)
				if len(parts) >= 2 {
					actions = append(actions, "- [ ] "+strings.TrimSpace(parts[1]))
				}
				break
			}
		}
	}

	if len(actions) == 0 {
		return "No action items detected.\n\n(Set AI Provider to 'ollama' or 'openai' for deeper analysis)"
	}

	return strings.Join(actions, "\n")
}

func matrixAILocalReplies(lines []string) string {
	if len(lines) == 0 {
		return "1. Got it, thanks!\n2. I'll look into that.\n3. Sounds good to me."
	}
	// Provide generic contextual replies
	lastLine := lines[len(lines)-1]
	lower := strings.ToLower(lastLine)

	if strings.Contains(lower, "?") {
		return "1. Yes, that sounds right.\n2. Let me check and get back to you.\n3. I'm not sure, could you clarify?"
	}
	if strings.Contains(lower, "thanks") || strings.Contains(lower, "thank you") {
		return "1. You're welcome!\n2. Happy to help!\n3. No problem at all."
	}
	if strings.Contains(lower, "meeting") || strings.Contains(lower, "call") {
		return "1. I'll be there.\n2. Can we reschedule?\n3. Works for me, see you then."
	}
	return "1. Got it, thanks for the update!\n2. Makes sense, I agree.\n3. Let me think about it and get back to you."
}

func matrixAILocalNote(lines []string) string {
	if len(lines) == 0 {
		return "# Chat Notes\n\nNo messages to convert."
	}

	var b strings.Builder
	b.WriteString("# Chat Notes\n\n")
	b.WriteString("## Messages\n\n")
	for _, line := range lines {
		b.WriteString("- " + line + "\n")
	}
	return b.String()
}

// matrixAIResultToMsg converts a result into the appropriate tea.Msg.
func matrixAIResultToMsg(resultType matrixAIResultType, result string, err error) tea.Msg {
	switch resultType {
	case matrixAIResultSummary:
		return matrixAISummaryMsg{Result: result, Err: err}
	case matrixAIResultActions:
		return matrixAIActionsMsg{Result: result, Err: err}
	case matrixAIResultReply:
		// Parse numbered suggestions
		var suggestions []string
		for _, line := range strings.Split(result, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			// Strip leading number and punctuation (e.g., "1. ", "1) ", "1: ")
			cleaned := strings.TrimLeft(line, "0123456789")
			cleaned = strings.TrimLeft(cleaned, ".) :- ")
			if cleaned != "" {
				suggestions = append(suggestions, cleaned)
			}
		}
		if len(suggestions) > 3 {
			suggestions = suggestions[:3]
		}
		return matrixAIReplyMsg{Suggestions: suggestions, Err: err}
	case matrixAIResultTranslate:
		return matrixAITranslateMsg{Result: result, Err: err}
	case matrixAIResultNote:
		return matrixAINoteMsg{Result: result, Err: err}
	case matrixAIResultContext:
		return matrixAIContextMsg{Result: result, Err: err}
	}
	return matrixAISummaryMsg{Result: result, Err: err}
}

// ---------------------------------------------------------------------------
// Update — process AI result messages and key input on the result panel
// ---------------------------------------------------------------------------

func (mai MatrixAI) Update(msg tea.Msg) (MatrixAI, tea.Cmd) {
	switch msg := msg.(type) {
	case matrixAITickMsg:
		if mai.processing {
			mai.loadingTick++
			return mai, matrixAITickCmd()
		}

	case matrixAISummaryMsg:
		mai.processing = false
		if msg.Err != nil {
			mai.lastError = msg.Err.Error()
			mai.lastResult = ""
		} else {
			mai.lastResult = msg.Result
			mai.lastError = ""
		}
		return mai, nil

	case matrixAIActionsMsg:
		mai.processing = false
		if msg.Err != nil {
			mai.lastError = msg.Err.Error()
			mai.lastResult = ""
		} else {
			mai.lastResult = msg.Result
			mai.lastError = ""
		}
		return mai, nil

	case matrixAIReplyMsg:
		mai.processing = false
		if msg.Err != nil {
			mai.lastError = msg.Err.Error()
			mai.replySuggestions = nil
		} else {
			mai.replySuggestions = msg.Suggestions
			mai.lastError = ""
			// Also store the raw result for display
			var displayLines []string
			for i, s := range msg.Suggestions {
				displayLines = append(displayLines, fmt.Sprintf("%d. %s", i+1, s))
			}
			mai.lastResult = strings.Join(displayLines, "\n")
		}
		return mai, nil

	case matrixAITranslateMsg:
		mai.processing = false
		if msg.Err != nil {
			mai.lastError = msg.Err.Error()
			mai.lastResult = ""
		} else {
			mai.lastResult = msg.Result
			mai.lastError = ""
		}
		return mai, nil

	case matrixAINoteMsg:
		mai.processing = false
		if msg.Err != nil {
			mai.lastError = msg.Err.Error()
			mai.lastResult = ""
		} else {
			mai.lastResult = msg.Result
			mai.lastError = ""
		}
		return mai, nil

	case matrixAIContextMsg:
		mai.processing = false
		if msg.Err != nil {
			mai.lastError = msg.Err.Error()
			mai.lastResult = ""
		} else {
			mai.lastResult = msg.Result
			mai.lastError = ""
		}
		return mai, nil

	case tea.KeyMsg:
		return mai.handleKey(msg)
	}

	return mai, nil
}

func (mai MatrixAI) handleKey(msg tea.KeyMsg) (MatrixAI, tea.Cmd) {
	key := msg.String()

	switch key {
	case "esc":
		mai.Close()
		return mai, nil

	case "j", "down":
		mai.scroll++
		return mai, nil

	case "k", "up":
		if mai.scroll > 0 {
			mai.scroll--
		}
		return mai, nil

	case "1":
		if mai.resultType == matrixAIResultReply && len(mai.replySuggestions) > 0 {
			mai.replySelected = 0
			// Signal that a reply was picked — caller will read GetSelectedReply()
			mai.active = false
		}
		return mai, nil

	case "2":
		if mai.resultType == matrixAIResultReply && len(mai.replySuggestions) > 1 {
			mai.replySelected = 1
			mai.active = false
		}
		return mai, nil

	case "3":
		if mai.resultType == matrixAIResultReply && len(mai.replySuggestions) > 2 {
			mai.replySelected = 2
			mai.active = false
		}
		return mai, nil

	case "enter":
		// For note generation, enter signals "save to vault"
		if mai.resultType == matrixAIResultNote && mai.lastResult != "" {
			// The caller will read GetCaptureContent() and save
			mai.active = false
		}
		return mai, nil
	}

	return mai, nil
}

// ---------------------------------------------------------------------------
// View — render the AI result panel
// ---------------------------------------------------------------------------

func (mai MatrixAI) View(width, height int) string {
	panelW := width - 4
	if panelW < 40 {
		panelW = 40
	}
	if panelW > 90 {
		panelW = 90
	}
	innerW := panelW - 6

	panelH := height - 4
	if panelH < 10 {
		panelH = 10
	}
	if panelH > 35 {
		panelH = 35
	}

	var b strings.Builder

	// Header
	headerStyle := lipgloss.NewStyle().Foreground(sapphire).Bold(true)
	title := mai.resultTitle()
	b.WriteString(headerStyle.Render("  " + IconBotChar + " " + title))
	b.WriteString("\n")

	// Provider badge
	providerLabel := "Local Analysis"
	providerColor := overlay1
	switch mai.provider {
	case "ollama":
		providerLabel = "Ollama: " + mai.ollamaModel
		providerColor = green
	case "openai":
		providerLabel = "OpenAI: " + mai.openaiModel
		providerColor = blue
	}
	b.WriteString(lipgloss.NewStyle().Foreground(providerColor).Render("  " + IconSearchChar + " " + providerLabel))
	b.WriteString("\n")

	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("─", innerW)))
	b.WriteString("\n")

	contentHeight := panelH - 7 // header + provider + separator + footer

	if mai.processing {
		b.WriteString(mai.renderLoading(innerW, contentHeight))
	} else if mai.lastError != "" {
		b.WriteString(mai.renderError(innerW))
	} else {
		b.WriteString(mai.renderResult(innerW, contentHeight))
	}

	// Footer
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("─", innerW)))
	b.WriteString("\n")
	b.WriteString(mai.renderFooter())

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(sapphire).
		Padding(1, 1).
		Width(panelW).
		Background(mantle)

	return border.Render(b.String())
}

func (mai MatrixAI) resultTitle() string {
	switch mai.resultType {
	case matrixAIResultSummary:
		return "AI: Conversation Summary"
	case matrixAIResultActions:
		return "AI: Action Items"
	case matrixAIResultReply:
		return "AI: Reply Suggestions"
	case matrixAIResultTranslate:
		return "AI: Translation"
	case matrixAIResultNote:
		return "AI: Thread to Note"
	case matrixAIResultContext:
		return "AI: Chat-Note Connections"
	}
	return "AI Analysis"
}

func (mai MatrixAI) renderLoading(width, height int) string {
	var b strings.Builder
	spinChars := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	spinIdx := mai.loadingTick % len(spinChars)
	spinner := lipgloss.NewStyle().Foreground(sapphire).Bold(true).Render(spinChars[spinIdx])

	b.WriteString("\n")
	b.WriteString("  " + spinner + " " + lipgloss.NewStyle().Foreground(text).Render("Analyzing conversation..."))
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Foreground(overlay0).Render("  This may take a moment."))
	for i := 3; i < height; i++ {
		b.WriteString("\n")
	}
	return b.String()
}

func (mai MatrixAI) renderError(width int) string {
	var b strings.Builder
	b.WriteString("\n")
	errStyle := lipgloss.NewStyle().Foreground(red)
	b.WriteString(errStyle.Render("  Error: " + mai.lastError))
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Foreground(overlay0).Render("  Check your AI provider settings."))
	return b.String()
}

func (mai MatrixAI) renderResult(width, maxLines int) string {
	var b strings.Builder
	b.WriteString("\n")

	if mai.lastResult == "" {
		b.WriteString(lipgloss.NewStyle().Foreground(overlay0).Render("  No results."))
		return b.String()
	}

	// Wrap and render the result text
	var contentLines []string
	for _, line := range strings.Split(mai.lastResult, "\n") {
		if line == "" {
			contentLines = append(contentLines, "")
			continue
		}

		// Style special lines
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- [ ]") || strings.HasPrefix(trimmed, "- [x]") {
			// Action item
			contentLines = append(contentLines, lipgloss.NewStyle().Foreground(green).Render("  "+trimmed))
		} else if strings.HasPrefix(trimmed, "# ") || strings.HasPrefix(trimmed, "## ") {
			// Heading
			contentLines = append(contentLines, lipgloss.NewStyle().Foreground(blue).Bold(true).Render("  "+trimmed))
		} else if mai.resultType == matrixAIResultReply && (strings.HasPrefix(trimmed, "1.") ||
			strings.HasPrefix(trimmed, "2.") || strings.HasPrefix(trimmed, "3.")) {
			// Numbered reply suggestion
			contentLines = append(contentLines, lipgloss.NewStyle().Foreground(peach).Bold(true).Render("  "+trimmed))
		} else {
			// Regular text, word wrap
			wrapped := wordWrap(line, width-4)
			for _, wl := range strings.Split(wrapped, "\n") {
				contentLines = append(contentLines, "  "+lipgloss.NewStyle().Foreground(text).Render(wl))
			}
		}
	}

	// Apply scroll
	if maxLines < 1 {
		maxLines = 1
	}
	totalLines := len(contentLines)
	start := mai.scroll
	if start >= totalLines {
		start = totalLines - 1
	}
	if start < 0 {
		start = 0
	}
	end := start + maxLines
	if end > totalLines {
		end = totalLines
	}

	for i := start; i < end; i++ {
		b.WriteString(contentLines[i])
		b.WriteString("\n")
	}

	// Scroll indicator
	if totalLines > maxLines {
		pos := "top"
		if start > 0 && end < totalLines {
			pos = "middle"
		} else if end >= totalLines {
			pos = "bottom"
		}
		scrollInfo := fmt.Sprintf("  [%d-%d of %d lines] %s", start+1, end, totalLines, pos)
		b.WriteString(lipgloss.NewStyle().Foreground(overlay0).Render(scrollInfo))
	}

	return b.String()
}

func (mai MatrixAI) renderFooter() string {
	dimStyle := lipgloss.NewStyle().Foreground(overlay0)
	keyStyle := lipgloss.NewStyle().Foreground(lavender).Bold(true)

	if mai.processing {
		return dimStyle.Render("  ") + keyStyle.Render("Esc") + dimStyle.Render(" cancel")
	}

	switch mai.resultType {
	case matrixAIResultReply:
		if len(mai.replySuggestions) > 0 {
			return "  " + keyStyle.Render("1") + dimStyle.Render("/") +
				keyStyle.Render("2") + dimStyle.Render("/") +
				keyStyle.Render("3") + dimStyle.Render(" pick reply  ") +
				keyStyle.Render("Esc") + dimStyle.Render(" close")
		}
		return "  " + keyStyle.Render("Esc") + dimStyle.Render(" close")

	case matrixAIResultNote:
		if mai.lastResult != "" {
			return "  " + keyStyle.Render("Enter") + dimStyle.Render(" save as note  ") +
				keyStyle.Render("Esc") + dimStyle.Render(" close")
		}
		return "  " + keyStyle.Render("Esc") + dimStyle.Render(" close")

	case matrixAIResultSummary, matrixAIResultActions, matrixAIResultContext:
		if mai.lastResult != "" {
			return "  " + keyStyle.Render("j/k") + dimStyle.Render(" scroll  ") +
				keyStyle.Render("Esc") + dimStyle.Render(" close")
		}
		return "  " + keyStyle.Render("Esc") + dimStyle.Render(" close")

	default:
		return "  " + keyStyle.Render("j/k") + dimStyle.Render(" scroll  ") +
			keyStyle.Render("Esc") + dimStyle.Render(" close")
	}
}
