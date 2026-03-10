package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	provider     string // "ollama" or "openai"
	model        string
	ollamaURL    string
	apiKey       string
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
func (ac *AIChat) SetConfig(provider, model, ollamaURL, apiKey string) {
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

	// Score each note.
	type scored struct {
		path  string
		score int
	}
	var scores []scored

	for path, body := range notes {
		lowerBody := strings.ToLower(body)
		lowerPath := strings.ToLower(path)
		score := 0
		for _, kw := range keywords {
			// Count occurrences in body.
			score += strings.Count(lowerBody, kw)
			// Bonus for keyword in note name.
			if strings.Contains(lowerPath, kw) {
				score += 3
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
		header := fmt.Sprintf("\n--- Note: %s ---\n", s.path)
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
		usedNotes = append(usedNotes, s.path)

		if totalLen >= maxChars {
			break
		}
	}

	return b.String(), usedNotes
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
			return aiChatResultMsg{err: fmt.Errorf("cannot connect to Ollama at %s: %w", url, err)}
		}
		if resp != nil && resp.Body != nil {
			defer resp.Body.Close()
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return aiChatResultMsg{err: err}
		}

		if resp.StatusCode != 200 {
			return aiChatResultMsg{err: fmt.Errorf("Ollama returned status %d: %s", resp.StatusCode, string(body))}
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
			return aiChatResultMsg{err: fmt.Errorf("cannot connect to OpenAI: %w", err)}
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
			return aiChatResultMsg{err: fmt.Errorf("OpenAI error: %s", oaiResp.Error.Message)}
		}

		if len(oaiResp.Choices) == 0 {
			return aiChatResultMsg{err: fmt.Errorf("OpenAI returned no choices")}
		}

		return aiChatResultMsg{response: oaiResp.Choices[0].Message.Content}
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
				Content: fmt.Sprintf("Error: %v", msg.err),
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
	b.WriteString(titleStyle.Render("  AI Chat"))
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
	helpText := "Enter: send  Esc: close  Up/Down: scroll"
	if ac.loading {
		helpText = "Waiting for response...  Esc: close"
	}
	b.WriteString(lipgloss.NewStyle().Foreground(overlay0).Render("  " + helpText))

	// Wrap in panel border.
	panel := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(blue).
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
