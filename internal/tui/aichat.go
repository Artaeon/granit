package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Chat message types
// ---------------------------------------------------------------------------

// ChatMessage represents a single message in the AI chat conversation.
type ChatMessage struct {
	Role    string // "user", "assistant", "system"
	Content string
	Time    time.Time
}

// aiChatResultMsg carries the AI response back to the Update loop.
type aiChatResultMsg struct {
	response string
	err      error
}

// aiChatTickMsg drives the loading animation.
type aiChatTickMsg struct{}

// ---------------------------------------------------------------------------
// AI chat stopwords (used for keyword extraction)
// ---------------------------------------------------------------------------

var aiChatStopwords = map[string]bool{
	"the": true, "a": true, "an": true, "is": true, "are": true,
	"was": true, "were": true, "be": true, "been": true,
	"have": true, "has": true, "had": true, "do": true, "does": true,
	"did": true, "will": true, "would": true, "could": true, "should": true,
	"to": true, "of": true, "in": true, "for": true, "on": true,
	"with": true, "at": true, "by": true, "from": true, "as": true,
	"and": true, "or": true, "but": true, "if": true, "not": true,
	"no": true, "this": true, "that": true, "it": true, "my": true,
	"your": true, "what": true, "how": true, "why": true, "when": true,
	"where": true, "which": true,
}

// ---------------------------------------------------------------------------
// System prompt
// ---------------------------------------------------------------------------

const aiChatSystemPrompt = "You are a helpful assistant answering questions about the user's personal knowledge base. Use the provided note excerpts to give accurate, specific answers. If the notes don't contain relevant information, say so. Keep answers concise and reference specific notes when possible."

// ---------------------------------------------------------------------------
// AIChat component
// ---------------------------------------------------------------------------

// AIChat implements a conversational overlay for asking questions about vault
// contents. It finds relevant notes via keyword matching, sends context plus
// the user's question to Ollama or OpenAI, and renders the conversation in a
// chat-style UI.
type AIChat struct {
	active bool
	width  int
	height int

	messages     []ChatMessage
	input        string
	scroll       int
	loading      bool
	provider      string // "ollama", "openai", "nous", or "nerve"
	model         string
	ollamaURL     string
	apiKey        string
	nousURL       string
	nousAPIKey    string
	nerveBinary   string
	nerveModel    string
	nerveProvider string
	noteContents map[string]string
	maxContext   int

	// Track which notes were used for context in the last answer
	lastContextNotes []string
	loadingTick      int
}

// NewAIChat creates a new AIChat with sensible defaults.
func NewAIChat() AIChat {
	return AIChat{
		provider:   "ollama",
		model:      "llama3.2",
		ollamaURL:  "http://localhost:11434",
		maxContext:  4000,
	}
}

// ---------------------------------------------------------------------------
// Overlay interface
// ---------------------------------------------------------------------------

// IsActive returns whether the chat overlay is visible.
func (ac *AIChat) IsActive() bool {
	return ac.active
}

// Open activates the chat overlay.
func (ac *AIChat) Open() {
	ac.active = true
	ac.input = ""
	ac.scroll = 0
	ac.loading = false
	ac.loadingTick = 0
	if len(ac.messages) == 0 {
		ac.messages = []ChatMessage{
			{
				Role:    "system",
				Content: "Ask me anything about your notes.",
				Time:    time.Now(),
			},
		}
	}
}

// Close deactivates the chat overlay.
func (ac *AIChat) Close() {
	ac.active = false
}

// SetSize updates the available dimensions for the overlay.
func (ac *AIChat) SetSize(width, height int) {
	ac.width = width
	ac.height = height
}

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

// SetConfig configures the AI provider and connection details.
func (ac *AIChat) SetConfig(provider, model, ollamaURL, apiKey string, nousOpts ...string) {
	if provider != "" {
		ac.provider = provider
	}
	if model != "" {
		ac.model = model
	}
	if ollamaURL != "" {
		ac.ollamaURL = ollamaURL
	}
	ac.apiKey = apiKey
	if len(nousOpts) > 0 && nousOpts[0] != "" {
		ac.nousURL = nousOpts[0]
	}
	if len(nousOpts) > 1 {
		ac.nousAPIKey = nousOpts[1]
	}
	if len(nousOpts) > 2 {
		ac.nerveBinary = nousOpts[2]
	}
	if len(nousOpts) > 3 {
		ac.nerveModel = nousOpts[3]
	}
	if len(nousOpts) > 4 {
		ac.nerveProvider = nousOpts[4]
	}
}

// SetNotes provides the vault contents for context lookup.
func (ac *AIChat) SetNotes(notes map[string]string) {
	ac.noteContents = notes
}

// ---------------------------------------------------------------------------
// Context finding
// ---------------------------------------------------------------------------

// findRelevantNotes scores every note by keyword frequency against the query,
// picks the top matches, and concatenates their content up to maxChars.
func findRelevantNotes(query string, notes map[string]string, maxChars int) (context string, usedNotes []string) {
	if len(notes) == 0 {
		return "", nil
	}

	// Tokenize query into keywords, removing stopwords.
	words := strings.Fields(strings.ToLower(query))
	var keywords []string
	for _, w := range words {
		// Strip basic punctuation.
		w = strings.Trim(w, ".,;:!?\"'()[]{}#")
		if len(w) < 2 {
			continue
		}
		if aiChatStopwords[w] {
			continue
		}
		keywords = append(keywords, w)
	}

	if len(keywords) == 0 {
		// Fall back to all words if everything was filtered.
		for _, w := range words {
			w = strings.Trim(w, ".,;:!?\"'()[]{}#")
			if len(w) >= 2 {
				keywords = append(keywords, w)
			}
		}
	}

	if len(keywords) == 0 {
		return "", nil
	}

	// Build word-boundary regexes for each keyword to avoid matching
	// "test" inside "testing" or "contest".
	kwRegexps := make([]*regexp.Regexp, len(keywords))
	for i, kw := range keywords {
		pattern := `(?i)\b` + regexp.QuoteMeta(kw) + `\b`
		kwRegexps[i] = regexp.MustCompile(pattern)
	}

	// Score each note.
	type scored struct {
		path  string
		score int
	}
	var scores []scored

	for path, body := range notes {
		lowerPath := strings.ToLower(path)
		score := 0
		for i, kw := range keywords {
			// Count whole-word occurrences in body.
			matches := kwRegexps[i].FindAllStringIndex(body, -1)
			score += len(matches)
			// Bonus for keyword in note name.
			if strings.Contains(lowerPath, kw) {
				score += 5
			}
			// Bonus for keyword in headings (higher signal)
			for _, line := range strings.Split(body, "\n") {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "#") && kwRegexps[i].MatchString(trimmed) {
					score += 3
				}
			}
		}
		// Bonus for tag matches
		tags := extractFrontmatterTags(body)
		for _, tag := range tags {
			for _, kw := range keywords {
				if strings.EqualFold(tag, kw) {
					score += 4
				}
			}
		}
		if score > 0 {
			scores = append(scores, scored{path: path, score: score})
		}
	}

	if len(scores) == 0 {
		return "", nil
	}

	// Sort by score descending.
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].score > scores[j].score
	})

	// Collect top notes until we hit maxChars.
	var b strings.Builder
	totalLen := 0
	for _, s := range scores {
		body := notes[s.path]

		// Extract tags from frontmatter for extra context.
		tags := extractFrontmatterTags(body)
		tagStr := ""
		if len(tags) > 0 {
			tagStr = fmt.Sprintf("  Tags: %s\n", strings.Join(tags, ", "))
		}

		// Include folder path for structural context.
		folder := ""
		if idx := strings.LastIndex(s.path, "/"); idx > 0 {
			folder = fmt.Sprintf("  Folder: %s\n", s.path[:idx])
		}

		noteName := strings.TrimSuffix(s.path, ".md")
		header := fmt.Sprintf("\n--- Note: %s ---\n%s%s", noteName, folder, tagStr)
		headerLen := len(header)

		remaining := maxChars - totalLen - headerLen
		if remaining <= 0 {
			break
		}

		if r := []rune(body); len(r) > remaining {
			body = string(r[:remaining])
		}

		b.WriteString(header)
		b.WriteString(body)
		totalLen += headerLen + len(body)
		usedNotes = append(usedNotes, noteName)

		if totalLen >= maxChars {
			break
		}
	}

	return b.String(), usedNotes
}

// extractFrontmatterTags extracts tags from a note's YAML frontmatter block.
func extractFrontmatterTags(body string) []string {
	if !strings.HasPrefix(body, "---") {
		return nil
	}
	end := strings.Index(body[3:], "---")
	if end == -1 {
		return nil
	}
	block := body[3 : 3+end]
	for _, line := range strings.Split(block, "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		if strings.TrimSpace(parts[0]) != "tags" {
			continue
		}
		value := strings.TrimSpace(parts[1])
		// Handle [tag1, tag2] format
		if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
			value = value[1 : len(value)-1]
		}
		var tags []string
		for _, t := range strings.Split(value, ",") {
			t = strings.TrimSpace(t)
			t = strings.Trim(t, "\"'")
			if t != "" {
				tags = append(tags, t)
			}
		}
		return tags
	}
	return nil
}

// ---------------------------------------------------------------------------
// AI calling
// ---------------------------------------------------------------------------

// sendToOllama creates a tea.Cmd that calls the Ollama generate API and
// returns an aiChatResultMsg.
func sendToOllama(url, model, systemPrompt, userMsg string) tea.Cmd {
	return func() tea.Msg {
		fullPrompt := fmt.Sprintf("System: %s\n\nUser: %s", systemPrompt, userMsg)

		reqBody := ollamaRequest{
			Model:  model,
			Prompt: fullPrompt,
			Stream: false,
		}
		data, err := json.Marshal(reqBody)
		if err != nil {
			return aiChatResultMsg{err: err}
		}

		client := &http.Client{Timeout: 120 * time.Second}
		resp, err := client.Post(url+"/api/generate", "application/json", bytes.NewReader(data))
		if err != nil {
			return aiChatResultMsg{err: fmt.Errorf("cannot connect to Ollama at %s — is it running? Try: ollama serve", url)}
		}
		if resp != nil && resp.Body != nil {
			defer resp.Body.Close()
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return aiChatResultMsg{err: err}
		}

		if resp.StatusCode != 200 {
			return aiChatResultMsg{err: fmt.Errorf("Ollama error (status %d). Check model is pulled: ollama pull %s", resp.StatusCode, model)}
		}

		var olResp ollamaResponse
		if err := json.Unmarshal(body, &olResp); err != nil {
			return aiChatResultMsg{err: err}
		}

		return aiChatResultMsg{response: olResp.Response}
	}
}

// sendToOpenAI creates a tea.Cmd that calls the OpenAI chat completions API
// and returns an aiChatResultMsg.
func sendToOpenAI(apiKey, model string, chatMessages []ChatMessage, systemPrompt string) tea.Cmd {
	return func() tea.Msg {
		var msgs []openaiMessage
		msgs = append(msgs, openaiMessage{Role: "system", Content: systemPrompt})
		for _, m := range chatMessages {
			if m.Role == "system" {
				continue
			}
			msgs = append(msgs, openaiMessage{Role: m.Role, Content: m.Content})
		}

		reqBody := openaiRequest{
			Model:    model,
			Messages: msgs,
		}
		data, err := json.Marshal(reqBody)
		if err != nil {
			return aiChatResultMsg{err: err}
		}

		client := &http.Client{Timeout: 60 * time.Second}
		req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(data))
		if err != nil {
			return aiChatResultMsg{err: err}
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)

		resp, err := client.Do(req)
		if err != nil {
			return aiChatResultMsg{err: fmt.Errorf("cannot reach OpenAI API — check your internet connection")}
		}
		if resp != nil && resp.Body != nil {
			defer resp.Body.Close()
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return aiChatResultMsg{err: err}
		}

		var oaiResp openaiResponse
		if err := json.Unmarshal(body, &oaiResp); err != nil {
			return aiChatResultMsg{err: err}
		}

		if oaiResp.Error != nil {
			hint := oaiResp.Error.Message
			if strings.Contains(hint, "auth") || strings.Contains(hint, "key") {
				hint += " — check your API key in Settings (Ctrl+,)"
			}
			return aiChatResultMsg{err: fmt.Errorf("OpenAI: %s", hint)}
		}

		if len(oaiResp.Choices) == 0 {
			return aiChatResultMsg{err: fmt.Errorf("OpenAI returned an empty response — try a different model in Settings (Ctrl+,)")}
		}

		return aiChatResultMsg{response: oaiResp.Choices[0].Message.Content}
	}
}

// sendToNous creates a tea.Cmd that calls the Nous chat API and returns an
// aiChatResultMsg.
func sendToNous(url, apiKey, userMsg string) tea.Cmd {
	return func() tea.Msg {
		client := NewNousClient(url, apiKey)
		resp, err := client.Chat(userMsg)
		if err != nil {
			return aiChatResultMsg{err: err}
		}
		return aiChatResultMsg{response: resp}
	}
}

// sendToNerve creates a tea.Cmd that calls the nerve binary and returns an
// aiChatResultMsg.
func sendToNerve(binary, model, provider, userMsg string) tea.Cmd {
	return func() tea.Msg {
		client := NewNerveClient(binary, model, provider)
		resp, err := client.Chat(aiChatSystemPrompt, userMsg, 120*time.Second)
		if err != nil {
			return aiChatResultMsg{err: err}
		}
		return aiChatResultMsg{response: resp}
	}
}

// localChatFallback provides a keyword-based response when no AI provider
// is available. It returns the matched note excerpts as the "answer".
func localChatFallback(query string, usedNotes []string, contextText string) tea.Cmd {
	return func() tea.Msg {
		if len(usedNotes) == 0 {
			return aiChatResultMsg{response: "No notes matched your query. Try different keywords or switch to an AI provider (Ollama/OpenAI) in Settings (Ctrl+,) for smarter search."}
		}

		var b strings.Builder
		b.WriteString(fmt.Sprintf("Found %d relevant note(s): **%s**\n\n", len(usedNotes), strings.Join(usedNotes, "**, **")))
		b.WriteString("Here are the matching excerpts:\n")
		b.WriteString(contextText)
		b.WriteString("\n\n_Using local keyword search. For AI-powered answers, configure Ollama or OpenAI in Settings (Ctrl+,)._")
		return aiChatResultMsg{response: b.String()}
	}
}

// ---------------------------------------------------------------------------
// Loading animation tick
// ---------------------------------------------------------------------------

func aiChatTick() tea.Cmd {
	return tea.Tick(400*time.Millisecond, func(time.Time) tea.Msg {
		return aiChatTickMsg{}
	})
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

// Update handles key presses and incoming AI responses.
func (ac AIChat) Update(msg tea.Msg) (AIChat, tea.Cmd) {
	if !ac.active {
		return ac, nil
	}

	switch msg := msg.(type) {
	case aiChatTickMsg:
		if ac.loading {
			ac.loadingTick++
			return ac, aiChatTick()
		}

	case aiChatResultMsg:
		ac.loading = false
		if msg.err != nil {
			ac.messages = append(ac.messages, ChatMessage{
				Role:    "system",
				Content: fmt.Sprintf("Error: %v\n\nTip: Set AI provider to \"local\" in Settings (Ctrl+,) for offline keyword search.", msg.err),
				Time:    time.Now(),
			})
		} else {
			ac.messages = append(ac.messages, ChatMessage{
				Role:    "assistant",
				Content: msg.response,
				Time:    time.Now(),
			})
		}
		// Auto-scroll to bottom.
		ac.scroll = ac.maxScroll()
		return ac, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			ac.active = false
			return ac, nil

		case "enter":
			if ac.loading {
				return ac, nil
			}
			trimmed := strings.TrimSpace(ac.input)
			if trimmed == "" {
				return ac, nil
			}

			// Add user message.
			ac.messages = append(ac.messages, ChatMessage{
				Role:    "user",
				Content: trimmed,
				Time:    time.Now(),
			})
			ac.input = ""
			ac.loading = true
			ac.loadingTick = 0

			// Find relevant context.
			contextText, usedNotes := findRelevantNotes(trimmed, ac.noteContents, ac.maxContext)
			ac.lastContextNotes = usedNotes

			// Build the message with context.
			var userMsg string
			if contextText != "" {
				userMsg = fmt.Sprintf("Here are relevant excerpts from the user's notes:\n%s\n\nQuestion: %s", contextText, trimmed)
			} else {
				userMsg = fmt.Sprintf("No specific notes matched the query. The user asks: %s", trimmed)
			}

			// Dispatch to AI provider.
			var cmd tea.Cmd
			switch ac.provider {
			case "openai":
				// For OpenAI, build full conversation with context injected
				// into the latest user message.
				convMsgs := make([]ChatMessage, len(ac.messages))
				copy(convMsgs, ac.messages)
				// Replace the last user message content with the context-enriched version.
				if len(convMsgs) > 0 {
					convMsgs[len(convMsgs)-1].Content = userMsg
				}
				cmd = sendToOpenAI(ac.apiKey, ac.model, convMsgs, aiChatSystemPrompt)
			case "nous":
				cmd = sendToNous(ac.nousURL, ac.nousAPIKey, userMsg)
			case "nerve":
				cmd = sendToNerve(ac.nerveBinary, ac.nerveModel, ac.nerveProvider, userMsg)
			case "local":
				// Local fallback: return matched note excerpts directly.
				cmd = localChatFallback(trimmed, usedNotes, contextText)
			default: // "ollama"
				cmd = sendToOllama(ac.ollamaURL, ac.model, aiChatSystemPrompt, userMsg)
			}

			return ac, tea.Batch(cmd, aiChatTick())

		case "up":
			if ac.scroll > 0 {
				ac.scroll--
			}
		case "down":
			ms := ac.maxScroll()
			if ac.scroll < ms {
				ac.scroll++
			}

		case "backspace":
			if len(ac.input) > 0 {
				ac.input = ac.input[:len(ac.input)-1]
			}

		case "ctrl+u":
			ac.input = ""

		default:
			// Only accept printable runes.
			if len(msg.String()) == 1 || msg.Type == tea.KeyRunes {
				ac.input += msg.String()
			}
		}
	}

	return ac, nil
}

// maxScroll returns the maximum scroll offset for the message list.
func (ac *AIChat) maxScroll() int {
	totalLines := 0
	chatWidth := ac.chatWidth() - 4 // account for padding
	if chatWidth < 20 {
		chatWidth = 20
	}

	for _, m := range ac.messages {
		totalLines += ac.messageLineCount(m, chatWidth)
		totalLines++ // spacing between messages
	}

	if ac.loading {
		totalLines += 2
	}

	if len(ac.lastContextNotes) > 0 && len(ac.messages) > 0 && ac.messages[len(ac.messages)-1].Role == "assistant" {
		totalLines += len(ac.lastContextNotes) + 1
	}

	viewable := ac.chatHeight()
	if totalLines <= viewable {
		return 0
	}
	return totalLines - viewable
}

// messageLineCount estimates how many terminal lines a message occupies.
func (ac *AIChat) messageLineCount(m ChatMessage, width int) int {
	if width <= 0 {
		width = 40
	}
	bubbleWidth := width * 2 / 3
	if bubbleWidth < 20 {
		bubbleWidth = 20
	}

	lines := strings.Split(m.Content, "\n")
	total := 0
	for _, line := range lines {
		if len(line) == 0 {
			total++
			continue
		}
		wrapped := (len(line) + bubbleWidth - 1) / bubbleWidth
		total += wrapped
	}
	// Add 1 for timestamp line.
	total++
	return total
}

// chatWidth returns the inner width of the chat panel.
func (ac *AIChat) chatWidth() int {
	w := ac.width * 3 / 5
	if w < 50 {
		w = 50
	}
	if w > 100 {
		w = 100
	}
	return w
}

// chatHeight returns the number of lines available for messages.
func (ac *AIChat) chatHeight() int {
	// Subtract: title(2) + separator(1) + input(2) + help(2) + border/padding(6)
	h := ac.height - 13
	if h < 5 {
		h = 5
	}
	return h
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

// View renders the chat overlay.
func (ac AIChat) View() string {
	panelWidth := ac.chatWidth()
	innerWidth := panelWidth - 6 // border(2) + padding(2*2)

	var b strings.Builder

	// Title
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	b.WriteString(titleStyle.Render("  " + IconSearchChar + " AI Chat"))
	b.WriteString("  ")
	providerInfo := fmt.Sprintf("[%s / %s]", ac.provider, ac.model)
	b.WriteString(lipgloss.NewStyle().Foreground(overlay0).Render(providerInfo))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(overlay0).Render(strings.Repeat("-", innerWidth)))
	b.WriteString("\n")

	// Render messages
	viewHeight := ac.chatHeight()
	var messageLines []string

	for i, m := range ac.messages {
		rendered := ac.renderMessage(m, innerWidth)
		messageLines = append(messageLines, rendered)

		// Context notes shown after the last assistant message
		if m.Role == "assistant" && i == len(ac.messages)-1 && len(ac.lastContextNotes) > 0 {
			contextLine := lipgloss.NewStyle().Foreground(overlay0).Italic(true).Render("  Sources: " + strings.Join(ac.lastContextNotes, ", "))
			messageLines = append(messageLines, contextLine)
		}
	}

	// Loading indicator
	if ac.loading {
		dots := strings.Repeat(".", (ac.loadingTick%3)+1)
		loadingLine := lipgloss.NewStyle().Foreground(blue).Italic(true).Render("  Thinking" + dots)
		messageLines = append(messageLines, loadingLine)
	}

	// Flatten to individual terminal lines for scrolling.
	var allLines []string
	for _, ml := range messageLines {
		parts := strings.Split(ml, "\n")
		allLines = append(allLines, parts...)
	}

	// Apply scroll offset.
	start := ac.scroll
	if start > len(allLines) {
		start = len(allLines)
	}
	end := start + viewHeight
	if end > len(allLines) {
		end = len(allLines)
	}

	visible := allLines[start:end]
	b.WriteString(strings.Join(visible, "\n"))

	// Pad remaining height.
	remaining := viewHeight - len(visible)
	for i := 0; i < remaining; i++ {
		b.WriteString("\n")
	}

	// Separator.
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(overlay0).Render(strings.Repeat("-", innerWidth)))
	b.WriteString("\n")

	// Input area.
	promptStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	inputStyle := lipgloss.NewStyle().Foreground(text)

	inputLine := promptStyle.Render("> ") + inputStyle.Render(ac.input)
	if !ac.loading {
		inputLine += lipgloss.NewStyle().Foreground(text).Background(surface0).Render(" ")
	}
	b.WriteString(inputLine)
	b.WriteString("\n")

	// Help bar.
	if ac.loading {
		b.WriteString(RenderHelpBar([]struct{ Key, Desc string }{
			{"...", "waiting"}, {"Esc", "close"},
		}))
	} else {
		b.WriteString(RenderHelpBar([]struct{ Key, Desc string }{
			{"Enter", "send"}, {"↑↓", "scroll"}, {"Esc", "close"},
		}))
	}

	// Wrap in panel border.
	panel := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(panelWidth).
		Background(mantle)

	return panel.Render(b.String())
}

// renderMessage renders a single chat message as a styled bubble.
func (ac *AIChat) renderMessage(m ChatMessage, width int) string {
	bubbleWidth := width * 2 / 3
	if bubbleWidth < 20 {
		bubbleWidth = 20
	}

	timeStr := m.Time.Format("15:04")

	switch m.Role {
	case "user":
		// Right-aligned, peach colored.
		msgStyle := lipgloss.NewStyle().
			Foreground(mantle).
			Background(peach).
			Padding(0, 1).
			Width(bubbleWidth).
			Align(lipgloss.Right)

		timeStyle := lipgloss.NewStyle().
			Foreground(overlay0).
			Width(width).
			Align(lipgloss.Right)

		rendered := msgStyle.Render(m.Content)
		// Right-align the bubble.
		padLeft := width - lipgloss.Width(rendered)
		if padLeft < 0 {
			padLeft = 0
		}
		line := strings.Repeat(" ", padLeft) + rendered
		line += "\n" + timeStyle.Render(timeStr)
		return line

	case "assistant":
		// Left-aligned, blue colored.
		msgStyle := lipgloss.NewStyle().
			Foreground(mantle).
			Background(blue).
			Padding(0, 1).
			Width(bubbleWidth)

		timeStyle := lipgloss.NewStyle().
			Foreground(overlay0)

		rendered := msgStyle.Render(m.Content)
		line := rendered
		line += "\n" + timeStyle.Render("  " + timeStr)
		return line

	case "system":
		// Centered, dim.
		msgStyle := lipgloss.NewStyle().
			Foreground(overlay0).
			Italic(true).
			Width(width).
			Align(lipgloss.Center)

		return msgStyle.Render(m.Content)

	default:
		return m.Content
	}
}
