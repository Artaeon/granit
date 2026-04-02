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
// Messages
// ---------------------------------------------------------------------------

type blogOutlineResultMsg struct {
	sections []blogSection
	err      error
}

type blogDraftResultMsg struct {
	draft   string
	section int
	err     error
}

type blogTickMsg struct{}

func blogTickCmd() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
		return blogTickMsg{}
	})
}

// ---------------------------------------------------------------------------
// Stages
// ---------------------------------------------------------------------------

const (
	blogStageInput    = iota // Topic, audience, tone input
	blogStageOutline         // AI generates outline, user edits it
	blogStageDrafting        // AI drafts section by section
	blogStageReview          // Final review, title, save
)

// ---------------------------------------------------------------------------
// BlogDraft overlay
// ---------------------------------------------------------------------------

type blogSection struct {
	Heading   string
	KeyPoints []string
	Draft     string // filled during drafting stage
}

type BlogDraft struct {
	active bool
	width  int
	height int

	ai        AIConfig
	vaultRoot string

	// Stage
	stage int

	// Input stage
	topic      string
	audience   string // "general", "technical", "beginner"
	tone       string // "professional", "casual", "academic", "conversational"
	focusField int    // 0=topic, 1=audience, 2=tone

	// Outline stage
	outline        []blogSection
	outlineCursor  int
	editingOutline bool
	editBuf        string

	// Drafting stage
	currentSection int
	sections       []blogSection
	draftBuf       string // accumulated draft text for current section
	fullDraft      string // complete draft so far

	// Review stage
	title  string
	tags   string
	scroll int

	// AI state
	loading     bool
	loadingTick int

	// Result
	resultReady   bool
	resultTitle   string
	resultContent string

	// Error
	errMsg string
}

var audienceOptions = []string{"general", "technical", "beginner"}
var toneOptions = []string{"professional", "casual", "academic", "conversational"}

// NewBlogDraft creates a new BlogDraft overlay.
func NewBlogDraft() BlogDraft {
	return BlogDraft{}
}

// ---------------------------------------------------------------------------
// Overlay interface
// ---------------------------------------------------------------------------

func (bd BlogDraft) IsActive() bool { return bd.active }

func (bd *BlogDraft) Open(vaultRoot string, ai AIConfig) {
	bd.active = true
	bd.ai = ai
	if bd.ai.OllamaURL == "" {
		bd.ai.OllamaURL = "http://localhost:11434"
	}
	bd.vaultRoot = vaultRoot
	bd.stage = blogStageInput
	bd.topic = ""
	bd.audience = "general"
	bd.tone = "professional"
	bd.focusField = 0
	bd.outline = nil
	bd.outlineCursor = 0
	bd.editingOutline = false
	bd.editBuf = ""
	bd.currentSection = 0
	bd.sections = nil
	bd.draftBuf = ""
	bd.fullDraft = ""
	bd.title = ""
	bd.tags = ""
	bd.scroll = 0
	bd.loading = false
	bd.loadingTick = 0
	bd.resultReady = false
	bd.resultTitle = ""
	bd.resultContent = ""
	bd.errMsg = ""
}

func (bd *BlogDraft) Close() {
	bd.active = false
}

func (bd *BlogDraft) SetSize(w, h int) {
	bd.width = w
	bd.height = h
}

// GetResult returns (title, content, ok) and clears the result. Consumed once.
func (bd *BlogDraft) GetResult() (title, content string, ok bool) {
	if !bd.resultReady {
		return "", "", false
	}
	t := bd.resultTitle
	ct := bd.resultContent
	bd.resultReady = false
	bd.resultTitle = ""
	bd.resultContent = ""
	return t, ct, true
}

// ---------------------------------------------------------------------------
// AI prompt builders
// ---------------------------------------------------------------------------

func (bd BlogDraft) buildOutlinePrompt() (string, string) {
	systemPrompt := "You are a skilled blog writer. Generate a clear outline for a blog post."

	var userBuf strings.Builder
	userBuf.WriteString(fmt.Sprintf("TOPIC: %s\n", bd.topic))
	userBuf.WriteString(fmt.Sprintf("AUDIENCE: %s\n", bd.audience))
	userBuf.WriteString(fmt.Sprintf("TONE: %s\n\n", bd.tone))
	userBuf.WriteString("Generate an outline with 4-7 sections. For each section, include a heading and 2-3 key points.\n\n")
	userBuf.WriteString("Format:\n")
	userBuf.WriteString("## Section Heading\n")
	userBuf.WriteString("- Key point 1\n")
	userBuf.WriteString("- Key point 2\n")
	userBuf.WriteString("- Key point 3\n\n")
	userBuf.WriteString("## Next Section\n")
	userBuf.WriteString("...\n")

	return systemPrompt, userBuf.String()
}

func (bd BlogDraft) buildSectionPrompt(sectionIdx int) (string, string) {
	if sectionIdx < 0 || sectionIdx >= len(bd.sections) {
		return "You are a blog writer.", "Draft a blog section."
	}
	systemPrompt := fmt.Sprintf(
		"You are a skilled blog writer. Draft one section of a blog post.\n"+
			"Write in a %s tone for a %s audience.\n"+
			"Keep the section focused and engaging. Use markdown formatting.\n"+
			"Do NOT include the section heading — just the content.",
		bd.tone, bd.audience,
	)

	var userBuf strings.Builder
	userBuf.WriteString(fmt.Sprintf("BLOG TOPIC: %s\n\n", bd.topic))

	// Full outline
	userBuf.WriteString("FULL OUTLINE:\n")
	for _, sec := range bd.sections {
		userBuf.WriteString(fmt.Sprintf("## %s\n", sec.Heading))
		for _, kp := range sec.KeyPoints {
			userBuf.WriteString(fmt.Sprintf("- %s\n", kp))
		}
	}
	userBuf.WriteString("\n")

	// Previous sections (truncated to last 500 chars)
	if bd.fullDraft != "" {
		prev := bd.fullDraft
		if len(prev) > 500 {
			prev = prev[len(prev)-500:]
		}
		userBuf.WriteString("PREVIOUS SECTIONS:\n")
		userBuf.WriteString(prev)
		userBuf.WriteString("\n\n")
	}

	sec := bd.sections[sectionIdx]
	userBuf.WriteString(fmt.Sprintf("NOW DRAFT THIS SECTION:\n## %s\n", sec.Heading))
	userBuf.WriteString("Key points to cover:\n")
	for _, kp := range sec.KeyPoints {
		userBuf.WriteString(fmt.Sprintf("- %s\n", kp))
	}
	userBuf.WriteString("\nWrite 150-300 words for this section.\n")

	return systemPrompt, userBuf.String()
}

// ---------------------------------------------------------------------------
// AI dispatch
// ---------------------------------------------------------------------------

func (bd BlogDraft) generateOutline() tea.Cmd {
	systemPrompt, userPrompt := bd.buildOutlinePrompt()
	ai := bd.ai

	return func() tea.Msg {
		resp, err := blogCallAI(ai, systemPrompt, userPrompt)
		if err != nil {
			return blogOutlineResultMsg{err: err}
		}
		sections := parseBlogOutline(resp)
		if len(sections) == 0 {
			return blogOutlineResultMsg{err: fmt.Errorf("AI returned no outline sections")}
		}
		return blogOutlineResultMsg{sections: sections}
	}
}

func (bd BlogDraft) draftSection(sectionIdx int) tea.Cmd {
	systemPrompt, userPrompt := bd.buildSectionPrompt(sectionIdx)
	ai := bd.ai
	idx := sectionIdx

	return func() tea.Msg {
		resp, err := blogCallAI(ai, systemPrompt, userPrompt)
		if err != nil {
			return blogDraftResultMsg{err: err, section: idx}
		}
		return blogDraftResultMsg{draft: resp, section: idx}
	}
}

// blogCallAI dispatches to the configured provider and returns the response.
func blogCallAI(ai AIConfig, systemPrompt, userPrompt string) (string, error) {
	switch ai.Provider {
	case "openai":
		return blogOpenAI(ai.APIKey, ai.ModelOrDefault("gpt-4o-mini"), systemPrompt, userPrompt)
	case "nous":
		client := ai.NewNous()
		return client.Chat(systemPrompt + "\n\n" + userPrompt)
	case "nerve":
		client := ai.NewNerve()
		return client.Chat(systemPrompt, userPrompt, 120*time.Second)
	default: // "ollama"
		return blogOllama(ai.OllamaEndpoint(), ai.ModelOrDefault("llama3.2"), systemPrompt, userPrompt)
	}
}

func blogOllama(url, model, systemPrompt, userPrompt string) (string, error) {
	type ollamaMsg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type ollamaChatReq struct {
		Model    string      `json:"model"`
		Messages []ollamaMsg `json:"messages"`
		Stream   bool        `json:"stream"`
	}

	reqBody := ollamaChatReq{
		Model: model,
		Messages: []ollamaMsg{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Stream: false,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: 120 * time.Second}
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
		return "", fmt.Errorf("Ollama returned status %d: %s", resp.StatusCode, string(body))
	}

	var chatResp struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Error string `json:"error,omitempty"`
	}
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", err
	}
	if chatResp.Error != "" {
		return "", fmt.Errorf("Ollama error: %s", chatResp.Error)
	}

	return chatResp.Message.Content, nil
}

func blogOpenAI(apiKey, model, systemPrompt, userPrompt string) (string, error) {
	type msg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type req struct {
		Model    string `json:"model"`
		Messages []msg  `json:"messages"`
	}

	reqBody := req{
		Model: model,
		Messages: []msg{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: 60 * time.Second}
	httpReq, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	httpResp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("cannot connect to OpenAI: %w", err)
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return "", err
	}

	var openaiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &openaiResp); err != nil {
		return "", err
	}
	if openaiResp.Error != nil {
		return "", fmt.Errorf("OpenAI error: %s", openaiResp.Error.Message)
	}
	if len(openaiResp.Choices) == 0 {
		return "", fmt.Errorf("OpenAI returned no choices")
	}

	return openaiResp.Choices[0].Message.Content, nil
}

// ---------------------------------------------------------------------------
// Outline parser
// ---------------------------------------------------------------------------

func parseBlogOutline(raw string) []blogSection {
	var sections []blogSection
	lines := strings.Split(raw, "\n")

	var current *blogSection
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Detect heading: "## Something"
		if strings.HasPrefix(trimmed, "## ") {
			heading := strings.TrimPrefix(trimmed, "## ")
			heading = strings.TrimSpace(heading)
			if heading == "" {
				continue
			}
			sections = append(sections, blogSection{Heading: heading})
			current = &sections[len(sections)-1]
			continue
		}

		// Key points under a heading: "- Something"
		if current != nil && strings.HasPrefix(trimmed, "- ") {
			point := strings.TrimPrefix(trimmed, "- ")
			point = strings.TrimSpace(point)
			if point != "" {
				current.KeyPoints = append(current.KeyPoints, point)
				sections[len(sections)-1] = *current
			}
		}
	}

	return sections
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (bd BlogDraft) Update(msg tea.Msg) (BlogDraft, tea.Cmd) {
	if !bd.active {
		return bd, nil
	}

	switch msg := msg.(type) {

	case blogTickMsg:
		if bd.loading {
			bd.loadingTick++
			return bd, blogTickCmd()
		}
		return bd, nil

	case blogOutlineResultMsg:
		bd.loading = false
		if msg.err != nil {
			bd.errMsg = msg.err.Error()
			bd.stage = blogStageInput
			return bd, nil
		}
		bd.outline = msg.sections
		bd.outlineCursor = 0
		bd.editingOutline = false
		bd.stage = blogStageOutline
		return bd, nil

	case blogDraftResultMsg:
		bd.loading = false
		if msg.err != nil {
			bd.errMsg = msg.err.Error()
			return bd, nil
		}
		if msg.section >= 0 && msg.section < len(bd.sections) {
			bd.sections[msg.section].Draft = msg.draft
			bd.draftBuf = msg.draft

			// Rebuild full draft
			var full strings.Builder
			for i := 0; i <= msg.section; i++ {
				if bd.sections[i].Draft != "" {
					full.WriteString("## " + bd.sections[i].Heading + "\n\n")
					full.WriteString(bd.sections[i].Draft)
					full.WriteString("\n\n")
				}
			}
			bd.fullDraft = full.String()
		}
		return bd, nil

	case tea.KeyMsg:
		switch bd.stage {
		case blogStageInput:
			return bd.updateInput(msg)
		case blogStageOutline:
			return bd.updateOutline(msg)
		case blogStageDrafting:
			return bd.updateDrafting(msg)
		case blogStageReview:
			return bd.updateReview(msg)
		}
	}

	return bd, nil
}

// ---------------------------------------------------------------------------
// Input stage
// ---------------------------------------------------------------------------

func (bd BlogDraft) updateInput(msg tea.KeyMsg) (BlogDraft, tea.Cmd) {
	switch msg.String() {
	case "esc":
		bd.active = false
		return bd, nil

	case "tab":
		bd.focusField = (bd.focusField + 1) % 3
		return bd, nil

	case "shift+tab":
		bd.focusField = (bd.focusField + 2) % 3
		return bd, nil

	case "left":
		if bd.focusField == 1 {
			bd.audience = blogCyclePrev(audienceOptions, bd.audience)
		} else if bd.focusField == 2 {
			bd.tone = blogCyclePrev(toneOptions, bd.tone)
		}
		return bd, nil

	case "right":
		if bd.focusField == 1 {
			bd.audience = blogCycleNext(audienceOptions, bd.audience)
		} else if bd.focusField == 2 {
			bd.tone = blogCycleNext(toneOptions, bd.tone)
		}
		return bd, nil

	case "enter":
		if strings.TrimSpace(bd.topic) == "" {
			bd.errMsg = "Topic is required"
			return bd, nil
		}
		bd.loading = true
		bd.loadingTick = 0
		bd.errMsg = ""
		return bd, tea.Batch(bd.generateOutline(), blogTickCmd())

	case "backspace":
		if bd.focusField == 0 && len(bd.topic) > 0 {
			bd.topic = bd.topic[:len(bd.topic)-1]
		}
		return bd, nil

	case "ctrl+u":
		if bd.focusField == 0 {
			bd.topic = ""
		}
		return bd, nil

	default:
		if bd.focusField == 0 {
			if len(msg.String()) == 1 || msg.Type == tea.KeySpace || msg.Type == tea.KeyRunes {
				bd.topic += msg.String()
			}
		}
		return bd, nil
	}
}

// ---------------------------------------------------------------------------
// Outline stage
// ---------------------------------------------------------------------------

func (bd BlogDraft) updateOutline(msg tea.KeyMsg) (BlogDraft, tea.Cmd) {
	if bd.loading {
		if msg.String() == "esc" {
			bd.loading = false
			bd.stage = blogStageInput
		}
		return bd, nil
	}

	// If editing a section heading
	if bd.editingOutline {
		switch msg.String() {
		case "esc":
			bd.editingOutline = false
			return bd, nil
		case "enter":
			if bd.outlineCursor >= 0 && bd.outlineCursor < len(bd.outline) {
				bd.outline[bd.outlineCursor].Heading = bd.editBuf
			}
			bd.editingOutline = false
			return bd, nil
		case "backspace":
			if len(bd.editBuf) > 0 {
				bd.editBuf = bd.editBuf[:len(bd.editBuf)-1]
			}
			return bd, nil
		default:
			if len(msg.String()) == 1 || msg.Type == tea.KeySpace || msg.Type == tea.KeyRunes {
				bd.editBuf += msg.String()
			}
			return bd, nil
		}
	}

	switch msg.String() {
	case "esc":
		bd.stage = blogStageInput
		return bd, nil

	case "j", "down":
		if bd.outlineCursor < len(bd.outline)-1 {
			bd.outlineCursor++
		}
		return bd, nil

	case "k", "up":
		if bd.outlineCursor > 0 {
			bd.outlineCursor--
		}
		return bd, nil

	case "e":
		if bd.outlineCursor >= 0 && bd.outlineCursor < len(bd.outline) {
			bd.editingOutline = true
			bd.editBuf = bd.outline[bd.outlineCursor].Heading
		}
		return bd, nil

	case "a":
		// Add a new section after cursor
		newSec := blogSection{Heading: "New Section", KeyPoints: []string{"Key point"}}
		pos := bd.outlineCursor + 1
		if pos > len(bd.outline) {
			pos = len(bd.outline)
		}
		bd.outline = append(bd.outline, blogSection{})
		copy(bd.outline[pos+1:], bd.outline[pos:])
		bd.outline[pos] = newSec
		bd.outlineCursor = pos
		return bd, nil

	case "d":
		if len(bd.outline) > 1 && bd.outlineCursor >= 0 && bd.outlineCursor < len(bd.outline) {
			bd.outline = append(bd.outline[:bd.outlineCursor], bd.outline[bd.outlineCursor+1:]...)
			if bd.outlineCursor >= len(bd.outline) {
				bd.outlineCursor = len(bd.outline) - 1
			}
		}
		return bd, nil

	case "enter":
		if len(bd.outline) == 0 {
			return bd, nil
		}
		// Accept outline, start drafting
		bd.sections = make([]blogSection, len(bd.outline))
		copy(bd.sections, bd.outline)
		bd.currentSection = 0
		bd.fullDraft = ""
		bd.draftBuf = ""
		bd.stage = blogStageDrafting
		bd.loading = true
		bd.loadingTick = 0
		bd.errMsg = ""
		return bd, tea.Batch(bd.draftSection(0), blogTickCmd())
	}

	return bd, nil
}

// ---------------------------------------------------------------------------
// Drafting stage
// ---------------------------------------------------------------------------

func (bd BlogDraft) updateDrafting(msg tea.KeyMsg) (BlogDraft, tea.Cmd) {
	if bd.loading {
		if msg.String() == "esc" {
			bd.loading = false
		}
		return bd, nil
	}

	switch msg.String() {
	case "esc":
		bd.stage = blogStageOutline
		return bd, nil

	case "enter", "n":
		// Accept current section draft, move to next
		if bd.currentSection < len(bd.sections)-1 {
			bd.currentSection++
			bd.draftBuf = ""
			bd.loading = true
			bd.loadingTick = 0
			bd.errMsg = ""
			return bd, tea.Batch(bd.draftSection(bd.currentSection), blogTickCmd())
		}
		// All sections done, move to review
		bd.stage = blogStageReview
		bd.scroll = 0
		// Derive title from topic
		bd.title = bd.topic
		if len(bd.title) > 80 {
			bd.title = bd.title[:80]
		}
		bd.tags = ""
		return bd, nil

	case "r":
		// Regenerate current section
		bd.loading = true
		bd.loadingTick = 0
		bd.errMsg = ""
		return bd, tea.Batch(bd.draftSection(bd.currentSection), blogTickCmd())

	case "e":
		// Hint: manual editing not yet implemented
		bd.errMsg = "Manual editing not yet implemented"
		return bd, nil
	}

	return bd, nil
}

// ---------------------------------------------------------------------------
// Review stage
// ---------------------------------------------------------------------------

func (bd BlogDraft) updateReview(msg tea.KeyMsg) (BlogDraft, tea.Cmd) {
	switch msg.String() {
	case "esc":
		bd.stage = blogStageDrafting
		return bd, nil

	case "t":
		// Switch focus to title editing — use focusField as 0=title, 1=tags
		bd.focusField = 0
		return bd, nil

	case "tab":
		bd.focusField = (bd.focusField + 1) % 2
		return bd, nil

	case "j", "down":
		bd.scroll++
		return bd, nil

	case "k", "up":
		if bd.scroll > 0 {
			bd.scroll--
		}
		return bd, nil

	case "enter", "s":
		// Save as new note
		bd.resultTitle = bd.title
		bd.resultContent = bd.buildFinalContent()
		bd.resultReady = true
		bd.active = false
		return bd, nil

	case "backspace":
		if bd.focusField == 0 && len(bd.title) > 0 {
			bd.title = bd.title[:len(bd.title)-1]
		} else if bd.focusField == 1 && len(bd.tags) > 0 {
			bd.tags = bd.tags[:len(bd.tags)-1]
		}
		return bd, nil

	default:
		if bd.focusField == 0 || bd.focusField == 1 {
			if len(msg.String()) == 1 || msg.Type == tea.KeySpace || msg.Type == tea.KeyRunes {
				if bd.focusField == 0 {
					bd.title += msg.String()
				} else {
					bd.tags += msg.String()
				}
			}
		}
		return bd, nil
	}
}

func (bd BlogDraft) buildFinalContent() string {
	var b strings.Builder

	// Frontmatter
	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("title: \"%s\"\n", bd.title))
	b.WriteString(fmt.Sprintf("date: %s\n", time.Now().Format("2006-01-02")))
	b.WriteString(fmt.Sprintf("audience: %s\n", bd.audience))
	b.WriteString(fmt.Sprintf("tone: %s\n", bd.tone))
	if bd.tags != "" {
		b.WriteString("tags:\n")
		for _, tag := range strings.Split(bd.tags, ",") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				b.WriteString(fmt.Sprintf("  - %s\n", tag))
			}
		}
	}
	b.WriteString("---\n\n")

	// Title
	b.WriteString("# " + bd.title + "\n\n")

	// Sections
	for _, sec := range bd.sections {
		b.WriteString("## " + sec.Heading + "\n\n")
		if sec.Draft != "" {
			b.WriteString(sec.Draft)
			b.WriteString("\n\n")
		}
	}

	return b.String()
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (bd BlogDraft) View() string {
	width := bd.width * 3 / 5
	if width < 60 {
		width = 60
	}
	if width > 100 {
		width = 100
	}
	innerW := width - 6

	var b strings.Builder

	// Title
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	b.WriteString(titleStyle.Render(IconEditChar + " Blog Draft"))
	b.WriteString("\n\n")

	switch {
	case bd.loading:
		bd.viewLoading(&b, innerW)
	case bd.stage == blogStageInput:
		bd.viewInput(&b, innerW)
	case bd.stage == blogStageOutline:
		bd.viewOutline(&b, innerW)
	case bd.stage == blogStageDrafting:
		bd.viewDrafting(&b, innerW)
	case bd.stage == blogStageReview:
		bd.viewReview(&b, innerW)
	}

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(mauve).
		Padding(1, 2).
		Width(width)

	return border.Render(b.String())
}

func (bd BlogDraft) viewLoading(b *strings.Builder, _ int) {
	spinnerFrames := []string{"\u280b", "\u2819", "\u2838", "\u2834", "\u2826", "\u2807"}
	frame := spinnerFrames[bd.loadingTick%len(spinnerFrames)]

	spinStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(overlay0)

	var label string
	switch bd.stage {
	case blogStageInput:
		label = "Generating outline..."
	case blogStageOutline:
		label = "Generating outline..."
	case blogStageDrafting:
		if bd.currentSection < len(bd.sections) {
			label = fmt.Sprintf("Drafting section %d of %d: %s",
				bd.currentSection+1, len(bd.sections), bd.sections[bd.currentSection].Heading)
		} else {
			label = "Drafting..."
		}
	default:
		label = "Generating..."
	}

	b.WriteString(spinStyle.Render(frame + " " + label))
	b.WriteString("\n\n")
	b.WriteString(dimStyle.Render(fmt.Sprintf("Topic: %s", bd.topic)))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render(fmt.Sprintf("Provider: %s  Model: %s", bd.ai.Provider, bd.ai.Model)))
	b.WriteString("\n\n")
	b.WriteString(dimStyle.Render("Esc=cancel"))
}

func (bd BlogDraft) viewInput(b *strings.Builder, innerW int) {
	labelStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	inputBg := lipgloss.NewStyle().Background(surface0).Foreground(text).Width(innerW-2).Padding(0, 1)
	activeBg := lipgloss.NewStyle().Background(surface1).Foreground(text).Width(innerW-2).Padding(0, 1)
	cursor := "\u2588"
	dimStyle := lipgloss.NewStyle().Foreground(overlay0)
	errStyle := lipgloss.NewStyle().Foreground(red)
	optStyle := lipgloss.NewStyle().Foreground(teal).Bold(true)
	optDim := lipgloss.NewStyle().Foreground(overlay0)

	// Topic field
	b.WriteString(labelStyle.Render("Topic"))
	b.WriteString("\n")
	topicDisplay := bd.topic
	if bd.focusField == 0 {
		topicDisplay += cursor
		b.WriteString(activeBg.Render(topicDisplay))
	} else {
		b.WriteString(inputBg.Render(topicDisplay))
	}
	b.WriteString("\n\n")

	// Audience field
	b.WriteString(labelStyle.Render("Audience"))
	b.WriteString("  ")
	for i, opt := range audienceOptions {
		if opt == bd.audience {
			b.WriteString(optStyle.Render("[" + opt + "]"))
		} else {
			b.WriteString(optDim.Render(" " + opt + " "))
		}
		if i < len(audienceOptions)-1 {
			b.WriteString(" ")
		}
	}
	if bd.focusField == 1 {
		b.WriteString(dimStyle.Render("  </>"))
	}
	b.WriteString("\n\n")

	// Tone field
	b.WriteString(labelStyle.Render("Tone"))
	b.WriteString("      ")
	for i, opt := range toneOptions {
		if opt == bd.tone {
			b.WriteString(optStyle.Render("[" + opt + "]"))
		} else {
			b.WriteString(optDim.Render(" " + opt + " "))
		}
		if i < len(toneOptions)-1 {
			b.WriteString(" ")
		}
	}
	if bd.focusField == 2 {
		b.WriteString(dimStyle.Render("  </>"))
	}
	b.WriteString("\n\n")

	// Error
	if bd.errMsg != "" {
		b.WriteString(errStyle.Render("Error: " + bd.errMsg))
		b.WriteString("\n\n")
	}

	b.WriteString(dimStyle.Render("Tab=next field  Left/Right=cycle options  Enter=generate outline  Esc=close"))
}

func (bd BlogDraft) viewOutline(b *strings.Builder, innerW int) {
	headingStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
	activeStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	pointStyle := lipgloss.NewStyle().Foreground(text)
	dimStyle := lipgloss.NewStyle().Foreground(overlay0)
	errStyle := lipgloss.NewStyle().Foreground(red)

	b.WriteString(lipgloss.NewStyle().Foreground(blue).Bold(true).Render("Outline"))
	b.WriteString(dimStyle.Render(fmt.Sprintf("  (%d sections)", len(bd.outline))))
	b.WriteString("\n\n")

	for i, sec := range bd.outline {
		prefix := fmt.Sprintf("  %d. ", i+1)
		if i == bd.outlineCursor {
			if bd.editingOutline {
				b.WriteString(activeStyle.Render(prefix))
				editStyle := lipgloss.NewStyle().Background(surface1).Foreground(text).Padding(0, 1)
				b.WriteString(editStyle.Render(bd.editBuf + "\u2588"))
			} else {
				b.WriteString(activeStyle.Render(prefix + sec.Heading + " <"))
			}
		} else {
			b.WriteString(headingStyle.Render(prefix + sec.Heading))
		}
		b.WriteString("\n")

		for _, kp := range sec.KeyPoints {
			line := "     - " + kp
			if len(line) > innerW {
				line = line[:innerW-1] + "\u2026"
			}
			b.WriteString(pointStyle.Render(line))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if bd.errMsg != "" {
		b.WriteString(errStyle.Render("Error: " + bd.errMsg))
		b.WriteString("\n\n")
	}

	b.WriteString(dimStyle.Render("j/k=navigate  e=edit  a=add  d=delete  Enter=start drafting  Esc=back"))
}

func (bd BlogDraft) viewDrafting(b *strings.Builder, innerW int) {
	dimStyle := lipgloss.NewStyle().Foreground(overlay0)
	headingStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
	contentStyle := lipgloss.NewStyle().Foreground(text)
	errStyle := lipgloss.NewStyle().Foreground(red)
	progressStyle := lipgloss.NewStyle().Foreground(teal).Bold(true)

	// Progress bar
	total := len(bd.sections)
	current := bd.currentSection + 1
	barWidth := innerW - 20
	if barWidth < 10 {
		barWidth = 10
	}
	filled := 0
	if total > 0 {
		filled = (bd.currentSection * barWidth) / total
	}
	bar := strings.Repeat("\u2588", filled) + strings.Repeat("\u2591", barWidth-filled)
	b.WriteString(progressStyle.Render(fmt.Sprintf("Section %d/%d ", current, total)))
	b.WriteString(dimStyle.Render(bar))
	b.WriteString("\n\n")

	// Current section heading
	if bd.currentSection < len(bd.sections) {
		b.WriteString(headingStyle.Render("## " + bd.sections[bd.currentSection].Heading))
		b.WriteString("\n\n")
	}

	// Draft content
	if bd.draftBuf != "" {
		lines := strings.Split(bd.draftBuf, "\n")
		maxLines := bd.height/2 - 10
		if maxLines < 5 {
			maxLines = 5
		}
		if len(lines) > maxLines {
			lines = lines[:maxLines]
		}
		for _, line := range lines {
			if len(line) > innerW {
				line = line[:innerW]
			}
			b.WriteString(contentStyle.Render("  " + line))
			b.WriteString("\n")
		}
	} else if !bd.loading {
		b.WriteString(dimStyle.Render("  (awaiting draft...)"))
		b.WriteString("\n")
	}

	if bd.errMsg != "" {
		b.WriteString("\n")
		b.WriteString(errStyle.Render("Error: " + bd.errMsg))
	}

	b.WriteString("\n")
	if bd.currentSection < len(bd.sections)-1 {
		b.WriteString(dimStyle.Render("Enter/n=accept & next  r=regenerate  e=edit (coming soon)  Esc=back"))
	} else {
		b.WriteString(dimStyle.Render("Enter/n=finish & review  r=regenerate  Esc=back"))
	}
}

func (bd BlogDraft) viewReview(b *strings.Builder, innerW int) {
	labelStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	inputBg := lipgloss.NewStyle().Background(surface0).Foreground(text).Width(innerW-2).Padding(0, 1)
	activeBg := lipgloss.NewStyle().Background(surface1).Foreground(text).Width(innerW-2).Padding(0, 1)
	dimStyle := lipgloss.NewStyle().Foreground(overlay0)
	headingStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
	contentStyle := lipgloss.NewStyle().Foreground(text)
	successStyle := lipgloss.NewStyle().Foreground(green).Bold(true)

	b.WriteString(successStyle.Render("Blog Post Review"))
	b.WriteString("\n\n")

	// Title field
	b.WriteString(labelStyle.Render("Title"))
	b.WriteString("\n")
	titleDisplay := bd.title
	if bd.focusField == 0 {
		titleDisplay += "\u2588"
		b.WriteString(activeBg.Render(titleDisplay))
	} else {
		b.WriteString(inputBg.Render(titleDisplay))
	}
	b.WriteString("\n\n")

	// Tags field
	b.WriteString(labelStyle.Render("Tags"))
	b.WriteString(dimStyle.Render("  (comma-separated)"))
	b.WriteString("\n")
	tagsDisplay := bd.tags
	if bd.focusField == 1 {
		tagsDisplay += "\u2588"
		b.WriteString(activeBg.Render(tagsDisplay))
	} else {
		b.WriteString(inputBg.Render(tagsDisplay))
	}
	b.WriteString("\n\n")

	// Full preview
	b.WriteString(labelStyle.Render("Preview"))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render(strings.Repeat("\u2500", innerW)))
	b.WriteString("\n")

	// Build preview lines
	var previewLines []string
	for _, sec := range bd.sections {
		previewLines = append(previewLines, headingStyle.Render("## "+sec.Heading))
		if sec.Draft != "" {
			for _, line := range strings.Split(sec.Draft, "\n") {
				if len(line) > innerW {
					line = line[:innerW]
				}
				previewLines = append(previewLines, contentStyle.Render("  "+line))
			}
		}
		previewLines = append(previewLines, "")
	}

	// Scrollable view
	maxVisible := bd.height/2 - 16
	if maxVisible < 5 {
		maxVisible = 5
	}

	start := bd.scroll
	if start > len(previewLines) {
		start = len(previewLines)
	}
	end := start + maxVisible
	if end > len(previewLines) {
		end = len(previewLines)
	}

	for _, line := range previewLines[start:end] {
		b.WriteString(line)
		b.WriteString("\n")
	}

	if end < len(previewLines) {
		remaining := len(previewLines) - end
		b.WriteString(dimStyle.Render(fmt.Sprintf("... %d more lines (j/k to scroll)", remaining)))
		b.WriteString("\n")
	}

	b.WriteString(dimStyle.Render(strings.Repeat("\u2500", innerW)))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("t=edit title  Tab=switch field  j/k=scroll  Enter/s=save  Esc=back"))
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func blogCycleNext(options []string, current string) string {
	for i, opt := range options {
		if opt == current {
			return options[(i+1)%len(options)]
		}
	}
	return options[0]
}

func blogCyclePrev(options []string, current string) string {
	for i, opt := range options {
		if opt == current {
			return options[(i+len(options)-1)%len(options)]
		}
	}
	return options[0]
}
