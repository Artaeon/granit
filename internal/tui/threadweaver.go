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
// Message types
// ---------------------------------------------------------------------------

type threadWeaverResultMsg struct {
	content string
	err     error
}

type threadWeaverTickMsg struct{}

// ---------------------------------------------------------------------------
// ThreadWeaver — AI-powered note synthesis overlay
// ---------------------------------------------------------------------------

type ThreadWeaver struct {
	active bool
	width  int
	height int

	// Mode: 0=select notes, 1=configuring, 2=generating, 3=preview
	mode int

	// Note selection
	allNotes []string
	filtered []string
	query    string
	cursor   int
	selected []string // paths of selected notes (2-5)

	// Generation config
	style int // 0=essay, 1=summary, 2=comparison, 3=outline

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

	// Generation state
	loading     bool
	loadingTick int
	generated   string
	title       string

	// Note contents for context
	noteContents map[string]string

	// Result
	resultReady   bool
	resultTitle   string
	resultContent string

	// Preview scroll
	scroll int

	// Error
	errMsg string
}

// ---------------------------------------------------------------------------
// Constructor
// ---------------------------------------------------------------------------

func NewThreadWeaver() ThreadWeaver {
	return ThreadWeaver{
		provider:  "ollama",
		model:     "llama3.2",
		ollamaURL: "http://localhost:11434",
	}
}

// ---------------------------------------------------------------------------
// Overlay interface
// ---------------------------------------------------------------------------

func (tw *ThreadWeaver) IsActive() bool { return tw.active }

func (tw *ThreadWeaver) Open() {
	tw.active = true
	tw.mode = 0
	tw.query = ""
	tw.cursor = 0
	tw.selected = nil
	tw.style = 0
	tw.loading = false
	tw.loadingTick = 0
	tw.generated = ""
	tw.title = ""
	tw.scroll = 0
	tw.resultReady = false
	tw.resultTitle = ""
	tw.resultContent = ""
	tw.errMsg = ""
	tw.filtered = make([]string, len(tw.allNotes))
	copy(tw.filtered, tw.allNotes)
}

func (tw *ThreadWeaver) Close() {
	tw.active = false
}

func (tw *ThreadWeaver) SetSize(w, h int) {
	tw.width = w
	tw.height = h
}

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

func (tw *ThreadWeaver) SetConfig(provider, model, ollamaURL, apiKey string, nousOpts ...string) {
	tw.provider = provider
	if tw.provider == "" {
		tw.provider = "ollama"
	}
	tw.model = model
	if tw.model == "" {
		tw.model = "llama3.2"
	}
	tw.ollamaURL = ollamaURL
	if tw.ollamaURL == "" {
		tw.ollamaURL = "http://localhost:11434"
	}
	tw.apiKey = apiKey
	if len(nousOpts) > 0 && nousOpts[0] != "" {
		tw.nousURL = nousOpts[0]
	}
	if len(nousOpts) > 1 {
		tw.nousAPIKey = nousOpts[1]
	}
	if len(nousOpts) > 2 {
		tw.nerveBinary = nousOpts[2]
	}
	if len(nousOpts) > 3 {
		tw.nerveModel = nousOpts[3]
	}
	if len(nousOpts) > 4 {
		tw.nerveProvider = nousOpts[4]
	}
}

func (tw *ThreadWeaver) SetNotes(paths []string, contents map[string]string) {
	tw.allNotes = paths
	tw.noteContents = contents
}

// ---------------------------------------------------------------------------
// GetResult — consumed once after user accepts a generated note
// ---------------------------------------------------------------------------

func (tw *ThreadWeaver) GetResult() (title, content string, ok bool) {
	if !tw.resultReady {
		return "", "", false
	}
	t := tw.resultTitle
	ct := tw.resultContent
	tw.resultReady = false
	tw.resultTitle = ""
	tw.resultContent = ""
	return t, ct, true
}

// ---------------------------------------------------------------------------
// Fuzzy matching (unique name to avoid collision)
// ---------------------------------------------------------------------------

func twFuzzyMatch(str, pattern string) bool {
	pi := 0
	for si := 0; si < len(str) && pi < len(pattern); si++ {
		if str[si] == pattern[pi] {
			pi++
		}
	}
	return pi == len(pattern)
}

// ---------------------------------------------------------------------------
// Note filtering
// ---------------------------------------------------------------------------

func (tw *ThreadWeaver) filterNotes() {
	if tw.query == "" {
		tw.filtered = make([]string, len(tw.allNotes))
		copy(tw.filtered, tw.allNotes)
	} else {
		q := strings.ToLower(tw.query)
		tw.filtered = nil
		for _, p := range tw.allNotes {
			if twFuzzyMatch(strings.ToLower(p), q) {
				tw.filtered = append(tw.filtered, p)
			}
		}
	}
	if tw.cursor >= len(tw.filtered) {
		tw.cursor = len(tw.filtered) - 1
	}
	if tw.cursor < 0 {
		tw.cursor = 0
	}
}

func (tw *ThreadWeaver) isSelected(path string) bool {
	for _, s := range tw.selected {
		if s == path {
			return true
		}
	}
	return false
}

func (tw *ThreadWeaver) toggleSelected(path string) {
	for i, s := range tw.selected {
		if s == path {
			tw.selected = append(tw.selected[:i], tw.selected[i+1:]...)
			return
		}
	}
	if len(tw.selected) < 5 {
		tw.selected = append(tw.selected, path)
	}
}

// ---------------------------------------------------------------------------
// Style descriptions
// ---------------------------------------------------------------------------

var twStyleNames = []string{"Essay", "Summary", "Comparison", "Outline"}

var twStyleDescriptions = []string{
	"Weave selected notes into a cohesive essay",
	"Create a concise summary of key points",
	"Compare and contrast the topics",
	"Create a structured outline from all notes",
}

// ---------------------------------------------------------------------------
// AI prompt construction
// ---------------------------------------------------------------------------

func (tw *ThreadWeaver) buildSystemPrompt() string {
	return "You are a knowledge synthesis assistant. You create well-structured markdown notes that connect ideas from multiple source notes. Always use [[wikilinks]] to reference source notes. Write in a clear, academic style."
}

func (tw *ThreadWeaver) buildUserPrompt() string {
	var b strings.Builder

	// Style instruction
	switch tw.style {
	case 0:
		b.WriteString("Weave the following notes into a cohesive essay that connects their ideas and themes.\n\n")
	case 1:
		b.WriteString("Create a concise summary of the key points from the following notes.\n\n")
	case 2:
		b.WriteString("Compare and contrast the topics and ideas from the following notes.\n\n")
	case 3:
		b.WriteString("Create a structured outline that organizes the ideas from the following notes.\n\n")
	}

	// Include each source note
	for _, path := range tw.selected {
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		content := tw.noteContents[path]
		b.WriteString(fmt.Sprintf("--- Note: [[%s]] ---\n", name))
		b.WriteString(content)
		b.WriteString("\n\n")
	}

	b.WriteString("Synthesize the above notes into a single well-structured markdown document. Include [[wikilinks]] back to each source note.")

	return b.String()
}

// ---------------------------------------------------------------------------
// AI generation
// ---------------------------------------------------------------------------

func (tw *ThreadWeaver) generate() tea.Cmd {
	systemPrompt := tw.buildSystemPrompt()
	userPrompt := tw.buildUserPrompt()
	provider := tw.provider
	model := tw.model
	ollamaURL := tw.ollamaURL
	apiKey := tw.apiKey
	nousURL := tw.nousURL
	nousAPIKey := tw.nousAPIKey
	nerveBinary := tw.nerveBinary
	nerveModel := tw.nerveModel
	nerveProvider := tw.nerveProvider

	return func() tea.Msg {
		switch provider {
		case "openai":
			return twCallOpenAI(apiKey, model, systemPrompt, userPrompt)
		case "nous":
			client := NewNousClient(nousURL, nousAPIKey)
			resp, err := client.Chat(systemPrompt + "\n\n" + userPrompt)
			return threadWeaverResultMsg{content: resp, err: err}
		case "nerve":
			client := NewNerveClient(nerveBinary, nerveModel, nerveProvider)
			resp, err := client.Chat(systemPrompt, userPrompt, 120*time.Second)
			return threadWeaverResultMsg{content: resp, err: err}
		default:
			return twCallOllama(ollamaURL, model, systemPrompt, userPrompt)
		}
	}
}

// ---------------------------------------------------------------------------
// Ollama API
// ---------------------------------------------------------------------------

type twOllamaMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type twOllamaReq struct {
	Model    string        `json:"model"`
	Messages []twOllamaMsg `json:"messages"`
	Stream   bool          `json:"stream"`
}

type twOllamaResp struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	Error string `json:"error,omitempty"`
}

func twCallOllama(url, model, systemPrompt, userPrompt string) threadWeaverResultMsg {
	reqBody := twOllamaReq{
		Model: model,
		Messages: []twOllamaMsg{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Stream: false,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return threadWeaverResultMsg{err: err}
	}

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Post(url+"/api/chat", "application/json", bytes.NewReader(data))
	if err != nil {
		return threadWeaverResultMsg{err: fmt.Errorf("cannot connect to Ollama at %s: %w", url, err)}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return threadWeaverResultMsg{err: err}
	}

	if resp.StatusCode != 200 {
		return threadWeaverResultMsg{err: fmt.Errorf("Ollama returned status %d: %s", resp.StatusCode, string(body))}
	}

	var chatResp twOllamaResp
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return threadWeaverResultMsg{err: err}
	}

	if chatResp.Error != "" {
		return threadWeaverResultMsg{err: fmt.Errorf("Ollama error: %s", chatResp.Error)}
	}

	return threadWeaverResultMsg{content: chatResp.Message.Content}
}

// ---------------------------------------------------------------------------
// OpenAI API
// ---------------------------------------------------------------------------

type twOpenAIMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type twOpenAIReq struct {
	Model    string        `json:"model"`
	Messages []twOpenAIMsg `json:"messages"`
}

type twOpenAIResp struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func twCallOpenAI(apiKey, model, systemPrompt, userPrompt string) threadWeaverResultMsg {
	reqBody := twOpenAIReq{
		Model: model,
		Messages: []twOpenAIMsg{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return threadWeaverResultMsg{err: err}
	}

	client := &http.Client{Timeout: 60 * time.Second}
	httpReq, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(data))
	if err != nil {
		return threadWeaverResultMsg{err: err}
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	httpResp, err := client.Do(httpReq)
	if err != nil {
		return threadWeaverResultMsg{err: fmt.Errorf("cannot connect to OpenAI: %w", err)}
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return threadWeaverResultMsg{err: err}
	}

	var openaiResp twOpenAIResp
	if err := json.Unmarshal(body, &openaiResp); err != nil {
		return threadWeaverResultMsg{err: err}
	}

	if openaiResp.Error != nil {
		return threadWeaverResultMsg{err: fmt.Errorf("OpenAI error: %s", openaiResp.Error.Message)}
	}

	if len(openaiResp.Choices) == 0 {
		return threadWeaverResultMsg{err: fmt.Errorf("OpenAI returned no choices")}
	}

	return threadWeaverResultMsg{content: openaiResp.Choices[0].Message.Content}
}

// ---------------------------------------------------------------------------
// Loading tick
// ---------------------------------------------------------------------------

func twTickCmd() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
		return threadWeaverTickMsg{}
	})
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (tw ThreadWeaver) Update(msg tea.Msg) (ThreadWeaver, tea.Cmd) {
	if !tw.active {
		return tw, nil
	}

	switch msg := msg.(type) {

	case threadWeaverResultMsg:
		tw.loading = false
		if msg.err != nil {
			tw.errMsg = msg.err.Error()
			tw.mode = 1 // back to config
			return tw, nil
		}
		tw.generated = msg.content
		tw.mode = 3 // preview
		tw.scroll = 0
		tw.title = twDeriveTitle(tw.selected)
		return tw, nil

	case threadWeaverTickMsg:
		if tw.loading {
			tw.loadingTick++
			return tw, twTickCmd()
		}
		return tw, nil

	case tea.KeyMsg:
		switch tw.mode {
		case 0:
			return tw.updateSelectNotes(msg)
		case 1:
			return tw.updateConfigure(msg)
		case 2:
			return tw.updateGenerating(msg)
		case 3:
			return tw.updatePreview(msg)
		case 4:
			return tw.updateTitleEdit(msg)
		}
	}

	return tw, nil
}

// ---------------------------------------------------------------------------
// Mode 0 — Note Selection
// ---------------------------------------------------------------------------

func (tw ThreadWeaver) updateSelectNotes(msg tea.KeyMsg) (ThreadWeaver, tea.Cmd) {
	switch msg.String() {
	case "esc":
		tw.active = false
		return tw, nil

	case "ctrl+j":
		// Ctrl+Enter equivalent: proceed to configuration if 2+ notes selected
		if len(tw.selected) >= 2 {
			tw.mode = 1
			tw.style = 0
		}
		return tw, nil

	case "enter":
		// Toggle selection of the current note
		if len(tw.filtered) > 0 && tw.cursor < len(tw.filtered) {
			tw.toggleSelected(tw.filtered[tw.cursor])
		}
		return tw, nil

	case "up", "k":
		if tw.cursor > 0 {
			tw.cursor--
		}
		return tw, nil

	case "down", "j":
		if tw.cursor < len(tw.filtered)-1 {
			tw.cursor++
		}
		return tw, nil

	case "backspace":
		if len(tw.query) > 0 {
			tw.query = tw.query[:len(tw.query)-1]
			tw.filterNotes()
		}
		return tw, nil

	case "ctrl+u":
		tw.query = ""
		tw.filterNotes()
		return tw, nil

	default:
		if len(msg.String()) == 1 || msg.Type == tea.KeyRunes {
			tw.query += msg.String()
			tw.filterNotes()
		}
		return tw, nil
	}
}

// ---------------------------------------------------------------------------
// Mode 1 — Style Configuration
// ---------------------------------------------------------------------------

func (tw ThreadWeaver) updateConfigure(msg tea.KeyMsg) (ThreadWeaver, tea.Cmd) {
	switch msg.String() {
	case "esc":
		tw.mode = 0 // back to note selection
		return tw, nil

	case "up", "k":
		if tw.style > 0 {
			tw.style--
		}
		return tw, nil

	case "down", "j":
		if tw.style < 3 {
			tw.style++
		}
		return tw, nil

	case "enter":
		// Start generation
		tw.mode = 2
		tw.loading = true
		tw.loadingTick = 0
		tw.errMsg = ""
		return tw, tea.Batch(tw.generate(), twTickCmd())
	}

	return tw, nil
}

// ---------------------------------------------------------------------------
// Mode 2 — Generating
// ---------------------------------------------------------------------------

func (tw ThreadWeaver) updateGenerating(msg tea.KeyMsg) (ThreadWeaver, tea.Cmd) {
	switch msg.String() {
	case "esc":
		tw.loading = false
		tw.mode = 1
		return tw, nil
	}
	return tw, nil
}

// ---------------------------------------------------------------------------
// Mode 3 — Preview
// ---------------------------------------------------------------------------

func (tw ThreadWeaver) updatePreview(msg tea.KeyMsg) (ThreadWeaver, tea.Cmd) {
	switch msg.String() {
	case "esc":
		tw.mode = 1
		tw.scroll = 0
		return tw, nil

	case "enter":
		// Accept: store the result
		tw.resultTitle = tw.title
		tw.resultContent = tw.generated
		tw.resultReady = true
		tw.active = false
		return tw, nil

	case "e":
		// Edit title inline — switch to a simple title edit sub-mode
		// We reuse mode 3 but track via a small state machine in the key handler
		// Actually, let's do an inline approach: just read chars in a dedicated path
		// For simplicity, cycle to title editing (reuse mode value 4)
		tw.mode = 4
		return tw, nil

	case "r":
		// Regenerate
		tw.mode = 2
		tw.loading = true
		tw.loadingTick = 0
		tw.generated = ""
		tw.errMsg = ""
		return tw, tea.Batch(tw.generate(), twTickCmd())

	case "up", "k":
		if tw.scroll > 0 {
			tw.scroll--
		}
		return tw, nil

	case "down", "j":
		lines := strings.Count(tw.generated, "\n") + 1
		maxScroll := lines - (tw.height - 16)
		if maxScroll < 0 {
			maxScroll = 0
		}
		if tw.scroll < maxScroll {
			tw.scroll++
		}
		return tw, nil
	}

	return tw, nil
}

// ---------------------------------------------------------------------------
// Mode 4 — Title editing (sub-mode of preview)
// ---------------------------------------------------------------------------

func (tw ThreadWeaver) updateTitleEdit(msg tea.KeyMsg) (ThreadWeaver, tea.Cmd) {
	switch msg.String() {
	case "esc":
		tw.mode = 3 // back to preview
		return tw, nil
	case "enter":
		if strings.TrimSpace(tw.title) != "" {
			tw.mode = 3
		}
		return tw, nil
	case "backspace":
		if len(tw.title) > 0 {
			tw.title = tw.title[:len(tw.title)-1]
		}
		return tw, nil
	case "ctrl+u":
		tw.title = ""
		return tw, nil
	default:
		if len(msg.String()) == 1 || msg.Type == tea.KeySpace || msg.Type == tea.KeyRunes {
			tw.title += msg.String()
		}
		return tw, nil
	}
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (tw ThreadWeaver) View() string {
	width := tw.width / 2
	if width < 60 {
		width = 60
	}
	if width > 100 {
		width = 100
	}

	var b strings.Builder

	switch {
	case tw.mode == 0:
		b.WriteString(tw.viewSelectNotes(width))
	case tw.mode == 1:
		b.WriteString(tw.viewConfigure(width))
	case tw.mode == 2:
		b.WriteString(tw.viewGenerating(width))
	case tw.mode == 3:
		b.WriteString(tw.viewPreview(width))
	case tw.mode == 4:
		b.WriteString(tw.viewTitleEdit(width))
	}

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

// ---------------------------------------------------------------------------
// View: Mode 0 — Note Selection
// ---------------------------------------------------------------------------

func (tw ThreadWeaver) viewSelectNotes(width int) string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	b.WriteString(titleStyle.Render("  Thread Weaver"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n\n")

	b.WriteString(NormalItemStyle.Render("  Select 2-5 notes to synthesize:"))
	b.WriteString("\n\n")

	// Search input
	promptStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	inputStyle := lipgloss.NewStyle().
		Foreground(text).
		Background(surface0).
		Padding(0, 1).
		Width(width - 12)

	b.WriteString("  " + promptStyle.Render(SearchPromptStyle.Render("Search: ")))
	displayQuery := tw.query + "\u2588"
	b.WriteString(inputStyle.Render(displayQuery))
	b.WriteString("\n\n")

	// Selected count
	countStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
	b.WriteString("  " + countStyle.Render(fmt.Sprintf("Selected: %d/5", len(tw.selected))))

	// Show selected note names
	if len(tw.selected) > 0 {
		selNames := make([]string, len(tw.selected))
		for i, s := range tw.selected {
			selNames[i] = strings.TrimSuffix(filepath.Base(s), filepath.Ext(s))
		}
		selStyle := lipgloss.NewStyle().Foreground(green)
		b.WriteString("  " + selStyle.Render("["+strings.Join(selNames, ", ")+"]"))
	}
	b.WriteString("\n\n")

	// Note list
	visibleHeight := tw.height - 22
	if visibleHeight < 5 {
		visibleHeight = 5
	}

	start := 0
	if tw.cursor >= visibleHeight {
		start = tw.cursor - visibleHeight + 1
	}
	end := start + visibleHeight
	if end > len(tw.filtered) {
		end = len(tw.filtered)
	}

	checkStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
	cursorStyle := lipgloss.NewStyle().Foreground(crust).Background(mauve).Bold(true).Padding(0, 1)
	normalNoteStyle := lipgloss.NewStyle().Foreground(text)
	selectedNoteStyle := lipgloss.NewStyle().Foreground(green)

	for i := start; i < end; i++ {
		path := tw.filtered[i]
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))

		prefix := "  "
		if tw.isSelected(path) {
			prefix = checkStyle.Render(" \u2713")
		}

		if i == tw.cursor {
			b.WriteString(prefix + " " + cursorStyle.Render(name))
		} else if tw.isSelected(path) {
			b.WriteString(prefix + " " + selectedNoteStyle.Render(name))
		} else {
			b.WriteString(prefix + " " + normalNoteStyle.Render(name))
		}
		b.WriteString("\n")
	}

	if len(tw.filtered) == 0 {
		b.WriteString(DimStyle.Render("    No notes match your search"))
		b.WriteString("\n")
	}

	// Pad remaining lines
	rendered := end - start
	if len(tw.filtered) == 0 {
		rendered = 1
	}
	for i := rendered; i < visibleHeight; i++ {
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n")

	helpParts := []string{"Enter: toggle  j/k: navigate  Esc: close"}
	if len(tw.selected) >= 2 {
		continueStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
		helpParts = append(helpParts, continueStyle.Render("Ctrl+J: continue"))
	}
	b.WriteString(DimStyle.Render("  " + strings.Join(helpParts, "  ")))

	return b.String()
}

// ---------------------------------------------------------------------------
// View: Mode 1 — Style Configuration
// ---------------------------------------------------------------------------

func (tw ThreadWeaver) viewConfigure(width int) string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	b.WriteString(titleStyle.Render("  Thread Weaver \u2014 Style"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n\n")

	// Show selected notes
	b.WriteString(DimStyle.Render("  Weaving " + fmt.Sprintf("%d", len(tw.selected)) + " notes:"))
	b.WriteString("\n")
	noteNameStyle := lipgloss.NewStyle().Foreground(blue)
	for _, s := range tw.selected {
		name := strings.TrimSuffix(filepath.Base(s), filepath.Ext(s))
		b.WriteString("    " + noteNameStyle.Render("[["+name+"]]"))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	b.WriteString(NormalItemStyle.Render("  Choose synthesis style:"))
	b.WriteString("\n\n")

	if tw.errMsg != "" {
		errStyle := lipgloss.NewStyle().Foreground(red)
		b.WriteString("  " + errStyle.Render("Error: "+tw.errMsg))
		b.WriteString("\n\n")
	}

	// Style options
	activeStyle := lipgloss.NewStyle().Foreground(crust).Background(mauve).Bold(true).Padding(0, 1)
	inactiveStyle := lipgloss.NewStyle().Foreground(text)
	descStyle := lipgloss.NewStyle().Foreground(overlay0).Italic(true)

	for i, name := range twStyleNames {
		marker := "  "
		if i == tw.style {
			marker = lipgloss.NewStyle().Foreground(peach).Bold(true).Render("\u25b8 ")
			b.WriteString("  " + marker + activeStyle.Render(name))
		} else {
			b.WriteString("  " + marker + inactiveStyle.Render(name))
		}
		b.WriteString("\n")
		b.WriteString("    " + descStyle.Render(twStyleDescriptions[i]))
		b.WriteString("\n\n")
	}

	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  j/k: navigate  Enter: generate  Esc: back"))

	return b.String()
}

// ---------------------------------------------------------------------------
// View: Mode 2 — Generating
// ---------------------------------------------------------------------------

func (tw ThreadWeaver) viewGenerating(width int) string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	b.WriteString(titleStyle.Render("  Thread Weaver"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n\n")

	spinner := []string{"\u280b", "\u2819", "\u2838", "\u2834", "\u2826", "\u2807"}
	frame := spinner[tw.loadingTick%len(spinner)]

	loadStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
	b.WriteString("  " + loadStyle.Render(frame+" Weaving threads..."))
	b.WriteString("\n\n")

	b.WriteString(DimStyle.Render("  Synthesizing notes:"))
	b.WriteString("\n")
	noteStyle := lipgloss.NewStyle().Foreground(blue)
	for _, s := range tw.selected {
		name := strings.TrimSuffix(filepath.Base(s), filepath.Ext(s))
		b.WriteString("    " + noteStyle.Render("[["+name+"]]"))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	styleLabel := lipgloss.NewStyle().Foreground(yellow)
	b.WriteString("  " + DimStyle.Render("Style: ") + styleLabel.Render(twStyleNames[tw.style]))
	b.WriteString("\n\n")

	providerDisplay := lipgloss.NewStyle().Foreground(overlay0)
	b.WriteString("  " + providerDisplay.Render("Provider: "+tw.provider+"  Model: "+tw.model))

	return b.String()
}

// ---------------------------------------------------------------------------
// View: Mode 3 — Preview
// ---------------------------------------------------------------------------

func (tw ThreadWeaver) viewPreview(width int) string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
	b.WriteString(titleStyle.Render("  Threads Woven"))
	b.WriteString("\n")

	filenameStyle := lipgloss.NewStyle().Foreground(peach)
	b.WriteString("  " + DimStyle.Render("File: ") + filenameStyle.Render(tw.title+".md"))
	b.WriteString("\n")

	// Source notes
	srcStyle := lipgloss.NewStyle().Foreground(blue)
	srcNames := make([]string, len(tw.selected))
	for i, s := range tw.selected {
		srcNames[i] = strings.TrimSuffix(filepath.Base(s), filepath.Ext(s))
	}
	b.WriteString("  " + DimStyle.Render("Sources: ") + srcStyle.Render(strings.Join(srcNames, ", ")))
	b.WriteString("\n")

	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n")

	// Render preview with basic syntax highlighting
	contentLines := strings.Split(tw.generated, "\n")

	visibleHeight := tw.height - 18
	if visibleHeight < 5 {
		visibleHeight = 5
	}

	start := tw.scroll
	if start > len(contentLines) {
		start = len(contentLines)
	}
	end := start + visibleHeight
	if end > len(contentLines) {
		end = len(contentLines)
	}

	contentWidth := width - 8
	if contentWidth < 20 {
		contentWidth = 20
	}

	for i := start; i < end; i++ {
		line := contentLines[i]
		rendered := twHighlightLine(line, contentWidth)
		b.WriteString("  " + rendered)
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	if end < len(contentLines) {
		b.WriteString("\n")
		scrollInfo := lipgloss.NewStyle().Foreground(overlay0)
		remaining := len(contentLines) - end
		b.WriteString("  " + scrollInfo.Render(fmt.Sprintf("... %d more lines (j/k to scroll)", remaining)))
	}

	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Enter: accept  r: regenerate  e: edit title  Esc: back"))

	return b.String()
}

// ---------------------------------------------------------------------------
// View: Mode 4 — Title Edit
// ---------------------------------------------------------------------------

func (tw ThreadWeaver) viewTitleEdit(width int) string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	b.WriteString(titleStyle.Render("  Edit Note Title"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n\n")

	b.WriteString(NormalItemStyle.Render("  Enter a filename for the woven note:"))
	b.WriteString("\n\n")

	promptStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	inputStyle := lipgloss.NewStyle().
		Foreground(text).
		Background(surface0).
		Padding(0, 1).
		Width(width - 12)

	b.WriteString("  " + promptStyle.Render("> "))
	displayTitle := tw.title + "\u2588"
	b.WriteString(inputStyle.Render(displayTitle))
	b.WriteString("\n\n")

	extStyle := lipgloss.NewStyle().Foreground(overlay0)
	b.WriteString("  " + extStyle.Render("Extension .md will be added automatically"))
	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Enter: confirm  Esc: back to preview"))

	return b.String()
}

// ---------------------------------------------------------------------------
// Syntax highlighting (basic)
// ---------------------------------------------------------------------------

func twHighlightLine(line string, maxWidth int) string {
	if maxWidth > 0 && len(line) > maxWidth {
		line = line[:maxWidth]
	}

	headingStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	boldStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
	wikilinkStyle := lipgloss.NewStyle().Foreground(blue).Underline(true)
	codeStyle := lipgloss.NewStyle().Foreground(yellow)
	frontmatterStyle := lipgloss.NewStyle().Foreground(overlay0)
	normalStyle := lipgloss.NewStyle().Foreground(text)

	trimmed := strings.TrimSpace(line)

	// Frontmatter delimiters
	if trimmed == "---" {
		return frontmatterStyle.Render(line)
	}

	// Headings
	if strings.HasPrefix(trimmed, "# ") || strings.HasPrefix(trimmed, "## ") ||
		strings.HasPrefix(trimmed, "### ") || strings.HasPrefix(trimmed, "#### ") {
		return headingStyle.Render(line)
	}

	// Code block markers
	if strings.HasPrefix(trimmed, "```") {
		return codeStyle.Render(line)
	}

	// Process inline elements
	return twHighlightInline(line, normalStyle, boldStyle, wikilinkStyle, codeStyle)
}

func twHighlightInline(line string, normal, bold, wikilink, code lipgloss.Style) string {
	var b strings.Builder
	i := 0
	n := len(line)

	for i < n {
		// Wikilinks [[...]]
		if i+1 < n && line[i] == '[' && line[i+1] == '[' {
			end := strings.Index(line[i+2:], "]]")
			if end >= 0 {
				linkText := line[i : i+2+end+2]
				b.WriteString(wikilink.Render(linkText))
				i = i + 2 + end + 2
				continue
			}
		}

		// Inline code `...`
		if line[i] == '`' {
			end := strings.Index(line[i+1:], "`")
			if end >= 0 {
				codeText := line[i : i+1+end+1]
				b.WriteString(code.Render(codeText))
				i = i + 1 + end + 1
				continue
			}
		}

		// Bold **...**
		if i+1 < n && line[i] == '*' && line[i+1] == '*' {
			end := strings.Index(line[i+2:], "**")
			if end >= 0 {
				boldText := line[i+2 : i+2+end]
				b.WriteString(bold.Render("**" + boldText + "**"))
				i = i + 2 + end + 2
				continue
			}
		}

		b.WriteString(normal.Render(string(line[i])))
		i++
	}

	return b.String()
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// twDeriveTitle creates a default filename from selected note names.
func twDeriveTitle(selected []string) string {
	if len(selected) == 0 {
		return "Woven Note"
	}
	names := make([]string, len(selected))
	for i, s := range selected {
		names[i] = strings.TrimSuffix(filepath.Base(s), filepath.Ext(s))
	}

	// Build a title like "Synthesis - Note1, Note2, Note3"
	title := "Synthesis - " + strings.Join(names, ", ")
	if len(title) > 80 {
		title = title[:80]
	}

	// Replace characters that are problematic in filenames
	replacer := strings.NewReplacer(
		"/", "-",
		"\\", "-",
		":", "-",
		"*", "",
		"?", "",
		"\"", "",
		"<", "",
		">", "",
		"|", "",
	)
	return replacer.Replace(title)
}
