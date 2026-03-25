package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// AutoTagger — AI-powered tag suggestions on save
// ---------------------------------------------------------------------------

// AutoTagger classifies notes and suggests tags using a small AI model.
// It is NOT an overlay; it is a helper invoked by app.go when a note is saved.
type AutoTagger struct {
	enabled bool

	// AI config
	provider   string
	model      string
	ollamaURL  string
	apiKey     string
	nousURL    string
	nousAPIKey string

	// Existing tags in the vault for consistency
	vaultTags []string
}

// autoTagResultMsg carries suggested tags back to the Update loop.
type autoTagResultMsg struct {
	tags []string
	err  error
}

// NewAutoTagger creates an AutoTagger with sensible defaults.
func NewAutoTagger() *AutoTagger {
	return &AutoTagger{
		enabled:   false,
		provider:  "ollama",
		model:     "llama3.2",
		ollamaURL: "http://localhost:11434",
	}
}

// SetEnabled enables or disables automatic tagging.
func (at *AutoTagger) SetEnabled(enabled bool) {
	at.enabled = enabled
}

// IsEnabled reports whether the auto-tagger is active.
func (at *AutoTagger) IsEnabled() bool {
	return at.enabled
}

// SetConfig configures the AI provider and connection details.
func (at *AutoTagger) SetConfig(provider, model, ollamaURL, apiKey string, nousOpts ...string) {
	if provider != "" {
		at.provider = provider
	}
	if model != "" {
		at.model = model
	}
	if ollamaURL != "" {
		at.ollamaURL = ollamaURL
	}
	at.apiKey = apiKey
	if len(nousOpts) > 0 && nousOpts[0] != "" {
		at.nousURL = nousOpts[0]
	}
	if len(nousOpts) > 1 {
		at.nousAPIKey = nousOpts[1]
	}
}

// SetVaultTags provides the set of tags already present in the vault so the
// AI can prefer them for consistency.
func (at *AutoTagger) SetVaultTags(tags []string) {
	at.vaultTags = tags
}

// TagNote sends the note content to the configured AI and returns a tea.Cmd
// that will produce an autoTagResultMsg.
func (at *AutoTagger) TagNote(content string) tea.Cmd {
	if !at.enabled {
		return nil
	}

	// Truncate to first 1500 characters for speed.
	userPrompt := content
	if len(userPrompt) > 1500 {
		userPrompt = userPrompt[:1500]
	}

	// Build system prompt.
	systemPrompt := "You are a note classifier. Given a note's content, suggest 2-5 relevant tags. Return ONLY a comma-separated list of lowercase tags, nothing else."
	if len(at.vaultTags) > 0 {
		systemPrompt += " Prefer tags from this existing list when applicable: " + strings.Join(at.vaultTags, ", ")
	}

	provider := at.provider
	model := at.model
	ollamaURL := at.ollamaURL
	apiKey := at.apiKey
	nousURL := at.nousURL
	nousAPIKey := at.nousAPIKey

	return func() tea.Msg {
		var response string
		var err error

		switch provider {
		case "openai":
			response, err = atCallOpenAI(apiKey, model, systemPrompt, userPrompt)
		case "nous":
			client := NewNousClient(nousURL, nousAPIKey)
			response, err = client.Chat(systemPrompt + "\n\n" + userPrompt)
		default: // "ollama"
			response, err = atCallOllama(ollamaURL, model, systemPrompt, userPrompt)
		}

		if err != nil {
			return autoTagResultMsg{err: err}
		}

		tags := atParseSuggestedTags(response)
		return autoTagResultMsg{tags: tags}
	}
}

// ParseSuggestedTags extracts tags from a raw AI response string.
func (at *AutoTagger) ParseSuggestedTags(response string) []string {
	return atParseSuggestedTags(response)
}

// ---------------------------------------------------------------------------
// AutoTagger — Ollama API types & call
// ---------------------------------------------------------------------------

type atOllamaChatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type atOllamaChatReq struct {
	Model    string            `json:"model"`
	Messages []atOllamaChatMsg `json:"messages"`
	Stream   bool              `json:"stream"`
}

type atOllamaChatResp struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	Error string `json:"error,omitempty"`
}

func atCallOllama(url, model, systemPrompt, userPrompt string) (string, error) {
	reqBody := atOllamaChatReq{
		Model: model,
		Messages: []atOllamaChatMsg{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Stream: false,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Post(url+"/api/chat", "application/json", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("cannot connect to Ollama at %s: %w", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, string(body))
	}

	var chatResp atOllamaChatResp
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", err
	}

	if chatResp.Error != "" {
		return "", fmt.Errorf("ollama error: %s", chatResp.Error)
	}

	return chatResp.Message.Content, nil
}

// ---------------------------------------------------------------------------
// AutoTagger — OpenAI API types & call
// ---------------------------------------------------------------------------

type atOpenAIMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type atOpenAIReq struct {
	Model       string        `json:"model"`
	Messages    []atOpenAIMsg `json:"messages"`
	MaxTokens   int           `json:"max_tokens"`
	Temperature float64       `json:"temperature"`
}

type atOpenAIResp struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func atCallOpenAI(apiKey, model, systemPrompt, userPrompt string) (string, error) {
	reqBody := atOpenAIReq{
		Model: model,
		Messages: []atOpenAIMsg{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		MaxTokens:   50,
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
		return "", fmt.Errorf("cannot connect to OpenAI: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var oaiResp atOpenAIResp
	if err := json.Unmarshal(body, &oaiResp); err != nil {
		return "", err
	}

	if oaiResp.Error != nil {
		return "", fmt.Errorf("OpenAI error: %s", oaiResp.Error.Message)
	}

	if len(oaiResp.Choices) == 0 {
		return "", fmt.Errorf("OpenAI returned no choices")
	}

	return oaiResp.Choices[0].Message.Content, nil
}

// ---------------------------------------------------------------------------
// AutoTagger — tag parsing
// ---------------------------------------------------------------------------

// atParseSuggestedTags splits a comma-separated AI response into clean tags.
// Each tag is lowercased and stripped of non-alphanumeric characters (hyphens
// are preserved).
func atParseSuggestedTags(response string) []string {
	parts := strings.Split(response, ",")
	var tags []string
	for _, p := range parts {
		t := strings.TrimSpace(p)
		t = strings.ToLower(t)

		// Remove any character that is not alphanumeric or hyphen.
		var cleaned strings.Builder
		for _, ch := range t {
			if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' {
				cleaned.WriteRune(ch)
			}
		}
		t = cleaned.String()
		t = strings.Trim(t, "-")

		if t != "" {
			tags = append(tags, t)
		}
	}
	return tags
}

// ===========================================================================
// NoteChat — "chat with a specific note" overlay
// ===========================================================================

// noteChatMessage represents a single message in the note-focused chat.
type noteChatMessage struct {
	Role    string
	Content string
	Time    time.Time
}

// noteChatResultMsg carries the AI response back to the Update loop.
type noteChatResultMsg struct {
	content string
	err     error
}

// noteChatTickMsg drives the loading animation.
type noteChatTickMsg struct{}

// NoteChat implements a chat overlay focused on a single note.
type NoteChat struct {
	active bool
	width  int
	height int

	notePath    string
	noteContent string

	messages    []noteChatMessage
	input       string
	scroll      int
	loading     bool
	loadingTick int

	provider   string
	model      string
	ollamaURL  string
	apiKey     string
	nousURL    string
	nousAPIKey string
}

// NewNoteChat creates an empty NoteChat with sensible defaults.
func NewNoteChat() NoteChat {
	return NoteChat{
		provider:  "ollama",
		model:     "llama3.2",
		ollamaURL: "http://localhost:11434",
	}
}

// ---------------------------------------------------------------------------
// NoteChat — overlay interface
// ---------------------------------------------------------------------------

// IsActive reports whether the note chat overlay is visible.
func (nc *NoteChat) IsActive() bool {
	return nc.active
}

// Open activates the chat overlay, focused on a specific note.
func (nc *NoteChat) Open(notePath, noteContent string) {
	nc.active = true
	nc.notePath = notePath
	nc.noteContent = noteContent
	nc.input = ""
	nc.scroll = 0
	nc.loading = false
	nc.loadingTick = 0
	nc.messages = []noteChatMessage{
		{
			Role:    "system",
			Content: fmt.Sprintf("Chatting about: %s", filepath.Base(notePath)),
			Time:    time.Now(),
		},
	}
}

// Close deactivates the note chat overlay.
func (nc *NoteChat) Close() {
	nc.active = false
}

// SetSize updates the available dimensions for the overlay.
func (nc *NoteChat) SetSize(w, h int) {
	nc.width = w
	nc.height = h
}

// SetConfig configures the AI provider and connection details.
func (nc *NoteChat) SetConfig(provider, model, ollamaURL, apiKey string, nousOpts ...string) {
	if provider != "" {
		nc.provider = provider
	}
	if model != "" {
		nc.model = model
	}
	if ollamaURL != "" {
		nc.ollamaURL = ollamaURL
	}
	nc.apiKey = apiKey
	if len(nousOpts) > 0 && nousOpts[0] != "" {
		nc.nousURL = nousOpts[0]
	}
	if len(nousOpts) > 1 {
		nc.nousAPIKey = nousOpts[1]
	}
}

// ---------------------------------------------------------------------------
// NoteChat — AI calling helpers
// ---------------------------------------------------------------------------

// ncOllamaMsg is the message format for the Ollama chat API.
type ncOllamaMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ncOllamaChatReq struct {
	Model    string        `json:"model"`
	Messages []ncOllamaMsg `json:"messages"`
	Stream   bool          `json:"stream"`
}

type ncOllamaChatResp struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	Error string `json:"error,omitempty"`
}

type ncOpenAIMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ncOpenAIReq struct {
	Model    string        `json:"model"`
	Messages []ncOpenAIMsg `json:"messages"`
}

type ncOpenAIResp struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// ncBuildSystemPrompt returns the system prompt for note-focused chat.
func ncBuildSystemPrompt(notePath, noteContent string) string {
	return fmt.Sprintf(
		"You are an assistant helping the user understand and work with a specific note. "+
			"The note is titled '%s'. Here is the full content:\n\n%s\n\n"+
			"Answer questions about this note. Be specific and reference parts of the note.",
		notePath, noteContent,
	)
}

// ncSendToOllama calls the Ollama chat API with the full conversation.
func ncSendToOllama(url, model, systemPrompt string, history []noteChatMessage) tea.Cmd {
	return func() tea.Msg {
		var msgs []ncOllamaMsg
		msgs = append(msgs, ncOllamaMsg{Role: "system", Content: systemPrompt})
		for _, m := range history {
			if m.Role == "system" {
				continue
			}
			msgs = append(msgs, ncOllamaMsg{Role: m.Role, Content: m.Content})
		}

		reqBody := ncOllamaChatReq{
			Model:    model,
			Messages: msgs,
			Stream:   false,
		}

		data, err := json.Marshal(reqBody)
		if err != nil {
			return noteChatResultMsg{err: err}
		}

		client := &http.Client{Timeout: 120 * time.Second}
		resp, err := client.Post(url+"/api/chat", "application/json", bytes.NewReader(data))
		if err != nil {
			return noteChatResultMsg{err: fmt.Errorf("cannot connect to Ollama at %s: %w", url, err)}
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return noteChatResultMsg{err: err}
		}

		if resp.StatusCode != 200 {
			return noteChatResultMsg{err: fmt.Errorf("Ollama returned status %d: %s", resp.StatusCode, string(body))}
		}

		var chatResp ncOllamaChatResp
		if err := json.Unmarshal(body, &chatResp); err != nil {
			return noteChatResultMsg{err: err}
		}

		if chatResp.Error != "" {
			return noteChatResultMsg{err: fmt.Errorf("Ollama error: %s", chatResp.Error)}
		}

		return noteChatResultMsg{content: chatResp.Message.Content}
	}
}

// ncSendToOpenAI calls the OpenAI chat completions API with the full
// conversation history.
func ncSendToNous(url, apiKey, systemPrompt string, history []noteChatMessage) tea.Cmd {
	return func() tea.Msg {
		// Build combined prompt from system + history
		var prompt strings.Builder
		prompt.WriteString(systemPrompt)
		prompt.WriteString("\n\n")
		for _, m := range history {
			if m.Role == "system" {
				continue
			}
			prompt.WriteString(m.Role + ": " + m.Content + "\n")
		}
		client := NewNousClient(url, apiKey)
		resp, err := client.Chat(prompt.String())
		if err != nil {
			return noteChatResultMsg{err: err}
		}
		return noteChatResultMsg{content: resp}
	}
}

func ncSendToOpenAI(apiKey, model, systemPrompt string, history []noteChatMessage) tea.Cmd {
	return func() tea.Msg {
		var msgs []ncOpenAIMsg
		msgs = append(msgs, ncOpenAIMsg{Role: "system", Content: systemPrompt})
		for _, m := range history {
			if m.Role == "system" {
				continue
			}
			msgs = append(msgs, ncOpenAIMsg{Role: m.Role, Content: m.Content})
		}

		reqBody := ncOpenAIReq{
			Model:    model,
			Messages: msgs,
		}

		data, err := json.Marshal(reqBody)
		if err != nil {
			return noteChatResultMsg{err: err}
		}

		client := &http.Client{Timeout: 60 * time.Second}
		req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(data))
		if err != nil {
			return noteChatResultMsg{err: err}
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)

		resp, err := client.Do(req)
		if err != nil {
			return noteChatResultMsg{err: fmt.Errorf("cannot connect to OpenAI: %w", err)}
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return noteChatResultMsg{err: err}
		}

		var oaiResp ncOpenAIResp
		if err := json.Unmarshal(body, &oaiResp); err != nil {
			return noteChatResultMsg{err: err}
		}

		if oaiResp.Error != nil {
			return noteChatResultMsg{err: fmt.Errorf("OpenAI error: %s", oaiResp.Error.Message)}
		}

		if len(oaiResp.Choices) == 0 {
			return noteChatResultMsg{err: fmt.Errorf("OpenAI returned no choices")}
		}

		return noteChatResultMsg{content: oaiResp.Choices[0].Message.Content}
	}
}

// ---------------------------------------------------------------------------
// NoteChat — loading animation tick
// ---------------------------------------------------------------------------

func noteChatTick() tea.Cmd {
	return tea.Tick(400*time.Millisecond, func(time.Time) tea.Msg {
		return noteChatTickMsg{}
	})
}

// ---------------------------------------------------------------------------
// NoteChat — Update (value receiver)
// ---------------------------------------------------------------------------

// Update handles key presses and incoming AI responses for the note chat.
func (nc NoteChat) Update(msg tea.Msg) (NoteChat, tea.Cmd) {
	if !nc.active {
		return nc, nil
	}

	switch msg := msg.(type) {
	case noteChatTickMsg:
		if nc.loading {
			nc.loadingTick++
			return nc, noteChatTick()
		}

	case noteChatResultMsg:
		nc.loading = false
		if msg.err != nil {
			nc.messages = append(nc.messages, noteChatMessage{
				Role:    "system",
				Content: fmt.Sprintf("Error: %v", msg.err),
				Time:    time.Now(),
			})
		} else {
			nc.messages = append(nc.messages, noteChatMessage{
				Role:    "assistant",
				Content: msg.content,
				Time:    time.Now(),
			})
		}
		// Auto-scroll to bottom.
		nc.scroll = nc.maxScroll()
		return nc, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			nc.active = false
			return nc, nil

		case "enter":
			if nc.loading {
				return nc, nil
			}
			trimmed := strings.TrimSpace(nc.input)
			if trimmed == "" {
				return nc, nil
			}

			// Add user message.
			nc.messages = append(nc.messages, noteChatMessage{
				Role:    "user",
				Content: trimmed,
				Time:    time.Now(),
			})
			nc.input = ""
			nc.loading = true
			nc.loadingTick = 0

			systemPrompt := ncBuildSystemPrompt(nc.notePath, nc.noteContent)

			var cmd tea.Cmd
			switch nc.provider {
			case "openai":
				cmd = ncSendToOpenAI(nc.apiKey, nc.model, systemPrompt, nc.messages)
			case "nous":
				cmd = ncSendToNous(nc.nousURL, nc.nousAPIKey, systemPrompt, nc.messages)
			default: // "ollama"
				cmd = ncSendToOllama(nc.ollamaURL, nc.model, systemPrompt, nc.messages)
			}

			return nc, tea.Batch(cmd, noteChatTick())

		case "up":
			if nc.scroll > 0 {
				nc.scroll--
			}
		case "down":
			ms := nc.maxScroll()
			if nc.scroll < ms {
				nc.scroll++
			}

		case "ctrl+u":
			nc.input = ""

		case "backspace":
			if len(nc.input) > 0 {
				nc.input = nc.input[:len(nc.input)-1]
			}

		default:
			// Accept printable runes.
			if len(msg.String()) == 1 || msg.Type == tea.KeyRunes {
				nc.input += msg.String()
			}
		}
	}

	return nc, nil
}

// ---------------------------------------------------------------------------
// NoteChat — scroll helpers
// ---------------------------------------------------------------------------

// ncChatWidth returns the inner width of the chat panel.
func (nc *NoteChat) ncChatWidth() int {
	w := nc.width * 3 / 5
	if w < 50 {
		w = 50
	}
	if w > 100 {
		w = 100
	}
	return w
}

// ncChatHeight returns the number of lines available for messages.
func (nc *NoteChat) ncChatHeight() int {
	// Subtract: title(2) + note-name(1) + separator(1) + input(2) + help(2) + border/padding(6)
	h := nc.height - 14
	if h < 5 {
		h = 5
	}
	return h
}

// ncMessageLineCount estimates how many terminal lines a message occupies.
func (nc *NoteChat) ncMessageLineCount(m noteChatMessage, width int) int {
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

// maxScroll returns the maximum scroll offset for the message list.
func (nc *NoteChat) maxScroll() int {
	totalLines := 0
	chatWidth := nc.ncChatWidth() - 4
	if chatWidth < 20 {
		chatWidth = 20
	}

	for _, m := range nc.messages {
		totalLines += nc.ncMessageLineCount(m, chatWidth)
		totalLines++ // spacing between messages
	}

	if nc.loading {
		totalLines += 2
	}

	viewable := nc.ncChatHeight()
	if totalLines <= viewable {
		return 0
	}
	return totalLines - viewable
}

// ---------------------------------------------------------------------------
// NoteChat — View (value receiver)
// ---------------------------------------------------------------------------

// View renders the note chat overlay.
func (nc NoteChat) View() string {
	panelWidth := nc.ncChatWidth()
	innerWidth := panelWidth - 6 // border(2) + padding(2*2)

	var b strings.Builder

	// Title
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	noteName := filepath.Base(nc.notePath)
	b.WriteString(titleStyle.Render("  Chat: " + noteName))
	b.WriteString("\n")

	// Note name subtitle
	noteInfo := DimStyle.Render("  Note: " + nc.notePath)
	b.WriteString(noteInfo)
	b.WriteString("\n")

	// Separator
	b.WriteString(lipgloss.NewStyle().Foreground(overlay0).Render(strings.Repeat("-", innerWidth)))
	b.WriteString("\n")

	// Render messages
	viewHeight := nc.ncChatHeight()
	var messageLines []string

	for _, m := range nc.messages {
		rendered := nc.renderNCMessage(m, innerWidth)
		messageLines = append(messageLines, rendered)
	}

	// Loading indicator
	if nc.loading {
		dots := strings.Repeat(".", (nc.loadingTick%3)+1)
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
	start := nc.scroll
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

	// Separator
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(overlay0).Render(strings.Repeat("-", innerWidth)))
	b.WriteString("\n")

	// Input area
	promptStyle := SearchPromptStyle
	inputStyle := lipgloss.NewStyle().Foreground(text)

	inputLine := promptStyle.Render("> ") + inputStyle.Render(nc.input)
	if !nc.loading {
		inputLine += lipgloss.NewStyle().Foreground(text).Background(surface0).Render(" ")
	}
	b.WriteString(inputLine)
	b.WriteString("\n")

	// Help bar
	helpText := "Enter: send  Esc: close  Up/Down: scroll  Ctrl+U: clear"
	if nc.loading {
		helpText = "Waiting for response...  Esc: close"
	}
	b.WriteString(DimStyle.Render("  " + helpText))

	// Wrap in panel border
	panel := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(mauve).
		Padding(1, 2).
		Width(panelWidth).
		Background(mantle)

	return panel.Render(b.String())
}

// renderNCMessage renders a single note-chat message as a styled bubble.
func (nc *NoteChat) renderNCMessage(m noteChatMessage, width int) string {
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
