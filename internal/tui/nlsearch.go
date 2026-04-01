package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Async message types
// ---------------------------------------------------------------------------

// nlSearchResultMsg carries the AI search response back to Update.
type nlSearchResultMsg struct {
	response string
	err      error
}

// nlSearchTickMsg drives the spinner animation.
type nlSearchTickMsg struct{}

// ---------------------------------------------------------------------------
// Data types
// ---------------------------------------------------------------------------

// nlSearchResult represents a single search result.
type nlSearchResult struct {
	Path      string
	Title     string
	Relevance string // AI explanation of why it's relevant
	Snippet   string // first matching lines from the note
}

// noteEntry is a cached representation of a vault note used for search.
type noteEntry struct {
	Path    string
	Title   string
	Content string
	Tags    []string
}

// ---------------------------------------------------------------------------
// NLSearch overlay component
// ---------------------------------------------------------------------------

// NLSearch provides natural language vault search powered by AI or local
// keyword matching as a fallback.
type NLSearch struct {
	active bool
	width  int
	height int

	// Vault
	vaultRoot string
	noteIndex []noteEntry

	// Search state
	query   string
	results []nlSearchResult
	cursor  int
	scroll  int
	phase   int // 0=input, 1=searching, 2=results
	spinner int

	// AI configuration
	ai AIConfig

	// Consumed-once selected note
	selectedNote string
	hasResult    bool
}

// ---------------------------------------------------------------------------
// Constructor
// ---------------------------------------------------------------------------

// NewNLSearch creates a new NLSearch overlay with sensible defaults.
func NewNLSearch() NLSearch {
	return NLSearch{
		ai: AIConfig{
			Provider:  "local",
			OllamaURL: "http://localhost:11434",
			Model:     "llama3.2",
		},
	}
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

// SetSize sets the overlay dimensions.
func (nls *NLSearch) SetSize(w, h int) {
	nls.width = w
	nls.height = h
}

// Open activates the NL search overlay and builds the note index.
func (nls *NLSearch) Open(vaultRoot string, cfg AIConfig) {
	nls.active = true
	nls.vaultRoot = vaultRoot
	nls.query = ""
	nls.results = nil
	nls.cursor = 0
	nls.scroll = 0
	nls.phase = 0
	nls.spinner = 0
	nls.selectedNote = ""
	nls.hasResult = false

	// AI config
	nls.ai = cfg
	if nls.ai.Provider == "" {
		nls.ai.Provider = "local"
	}
	if nls.ai.OllamaURL == "" {
		nls.ai.OllamaURL = "http://localhost:11434"
	}
	if nls.ai.Model == "" {
		nls.ai.Model = "llama3.2"
	}

	// Build index
	nls.buildNoteIndex()
}

// IsActive returns whether the overlay is currently visible.
func (nls NLSearch) IsActive() bool {
	return nls.active
}

// GetSelectedNote returns the path of the note the user selected and
// clears it so subsequent calls return empty until a new selection is made.
func (nls *NLSearch) GetSelectedNote() (string, bool) {
	if !nls.hasResult {
		return "", false
	}
	path := nls.selectedNote
	nls.selectedNote = ""
	nls.hasResult = false
	return path, true
}

// ---------------------------------------------------------------------------
// Note index builder
// ---------------------------------------------------------------------------

func (nls *NLSearch) buildNoteIndex() {
	nls.noteIndex = nil

	_ = filepath.Walk(nls.vaultRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		// Skip hidden directories and the trash folder
		name := info.Name()
		if info.IsDir() {
			if strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}

		// Only markdown files
		if !strings.HasSuffix(strings.ToLower(name), ".md") {
			return nil
		}

		// Read first 200 chars of the file
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}

		content := string(data)
		preview := content
		if len(preview) > 200 {
			preview = preview[:200]
		}

		// Extract title: first # heading, or fall back to filename
		title := strings.TrimSuffix(name, ".md")
		for _, line := range strings.Split(content, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "# ") {
				title = strings.TrimPrefix(trimmed, "# ")
				break
			}
		}

		// Extract inline tags
		var tags []string
		for _, word := range strings.Fields(content) {
			if strings.HasPrefix(word, "#") && len(word) > 1 {
				tag := strings.TrimRight(word[1:], ".,;:!?)")
				tag = strings.ToLower(tag)
				if tag != "" && !strings.HasPrefix(tag, "#") {
					tags = append(tags, tag)
				}
			}
		}

		// Compute relative path from vault root
		relPath, relErr := filepath.Rel(nls.vaultRoot, path)
		if relErr != nil {
			relPath = path
		}

		nls.noteIndex = append(nls.noteIndex, noteEntry{
			Path:    relPath,
			Title:   title,
			Content: preview,
			Tags:    tags,
		})

		return nil
	})
}

// ---------------------------------------------------------------------------
// Spinner tick command
// ---------------------------------------------------------------------------

func nlSearchTickCmd() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
		return nlSearchTickMsg{}
	})
}

// ---------------------------------------------------------------------------
// AI API calls
// ---------------------------------------------------------------------------

// nlSearchOllamaRequest is the request body for Ollama /api/generate.
type nlSearchOllamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

// nlSearchOllamaResponse is the response from Ollama /api/generate.
type nlSearchOllamaResponse struct {
	Response string `json:"response"`
}

// nlSearchOpenAIRequest is the request body for OpenAI chat completions.
type nlSearchOpenAIRequest struct {
	Model    string                  `json:"model"`
	Messages []nlSearchOpenAIMessage `json:"messages"`
}

// nlSearchOpenAIMessage is a single message in the OpenAI chat format.
type nlSearchOpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// nlSearchOpenAIResponse is the response from OpenAI chat completions.
type nlSearchOpenAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func callNLSearchOllama(url, model, prompt string) tea.Cmd {
	return func() tea.Msg {
		reqBody := nlSearchOllamaRequest{
			Model:  model,
			Prompt: prompt,
			Stream: false,
		}
		data, err := json.Marshal(reqBody)
		if err != nil {
			return nlSearchResultMsg{err: err}
		}

		client := &http.Client{Timeout: 120 * time.Second}
		resp, err := client.Post(url+"/api/generate", "application/json", bytes.NewReader(data))
		if err != nil {
			return nlSearchResultMsg{err: fmt.Errorf("cannot connect to Ollama at %s: %w", url, err)}
		}
		defer resp.Body.Close()

		var buf bytes.Buffer
		_, err = buf.ReadFrom(resp.Body)
		if err != nil {
			return nlSearchResultMsg{err: err}
		}

		if resp.StatusCode != 200 {
			return nlSearchResultMsg{err: fmt.Errorf("Ollama returned status %d: %s", resp.StatusCode, buf.String())}
		}

		var ollamaResp nlSearchOllamaResponse
		if err := json.Unmarshal(buf.Bytes(), &ollamaResp); err != nil {
			return nlSearchResultMsg{err: err}
		}

		return nlSearchResultMsg{response: ollamaResp.Response}
	}
}

func callNLSearchOpenAI(apiKey, model, prompt string) tea.Cmd {
	return func() tea.Msg {
		reqBody := nlSearchOpenAIRequest{
			Model: model,
			Messages: []nlSearchOpenAIMessage{
				{Role: "system", Content: "You are a helpful note-taking assistant. Be concise and actionable."},
				{Role: "user", Content: prompt},
			},
		}
		data, err := json.Marshal(reqBody)
		if err != nil {
			return nlSearchResultMsg{err: err}
		}

		client := &http.Client{Timeout: 60 * time.Second}
		req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(data))
		if err != nil {
			return nlSearchResultMsg{err: err}
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)

		resp, err := client.Do(req)
		if err != nil {
			return nlSearchResultMsg{err: fmt.Errorf("cannot connect to OpenAI: %w", err)}
		}
		defer resp.Body.Close()

		var buf bytes.Buffer
		_, err = buf.ReadFrom(resp.Body)
		if err != nil {
			return nlSearchResultMsg{err: err}
		}

		var openaiResp nlSearchOpenAIResponse
		if err := json.Unmarshal(buf.Bytes(), &openaiResp); err != nil {
			return nlSearchResultMsg{err: err}
		}

		if openaiResp.Error != nil {
			return nlSearchResultMsg{err: fmt.Errorf("OpenAI error: %s", openaiResp.Error.Message)}
		}

		if len(openaiResp.Choices) == 0 {
			return nlSearchResultMsg{err: fmt.Errorf("OpenAI returned no choices")}
		}

		return nlSearchResultMsg{response: openaiResp.Choices[0].Message.Content}
	}
}

// ---------------------------------------------------------------------------
// Prompt builder
// ---------------------------------------------------------------------------

func (nls NLSearch) buildSearchPrompt() string {
	var noteList strings.Builder
	for i, entry := range nls.noteIndex {
		if i >= 100 {
			break
		}
		preview := strings.ReplaceAll(entry.Content, "\n", " ")
		if len(preview) > 100 {
			preview = preview[:100]
		}
		tagStr := ""
		if len(entry.Tags) > 0 {
			tagStr = " [" + strings.Join(entry.Tags, ", ") + "]"
		}
		noteList.WriteString(fmt.Sprintf("- %s%s: %s\n", entry.Path, tagStr, preview))
	}

	return fmt.Sprintf(`Given this vault of notes, find the most relevant ones for the user's query.
Return results as one per line in the format: PATH | RELEVANCE_REASON
Return at most 10 results, ordered by relevance (most relevant first).
Only return notes that are genuinely relevant.

User query: %s

Notes in vault:
%s

Results:`, nls.query, noteList.String())
}

// ---------------------------------------------------------------------------
// Local fallback search
// ---------------------------------------------------------------------------

// nlSearchStopwords are words to skip when extracting keywords from the query.
var nlSearchStopwords = map[string]bool{
	"the": true, "a": true, "an": true, "is": true,
	"was": true, "were": true, "been": true, "about": true,
	"with": true, "from": true, "that": true, "this": true,
	"what": true, "which": true, "who": true, "how": true,
	"when": true, "where": true, "find": true, "notes": true,
	"note": true, "show": true, "me": true, "my": true,
	"and": true, "or": true, "in": true,
	"of": true, "to": true, "for": true, "on": true,
	"at": true, "by": true, "it": true, "its": true,
	"are": true, "be": true, "has": true, "have": true,
	"had": true, "do": true, "does": true, "did": true,
	"will": true, "would": true, "could": true, "should": true,
	"can": true, "may": true, "might": true, "shall": true,
	"not": true, "no": true, "but": true, "if": true,
	"so": true, "as": true, "all": true, "any": true,
	"some": true, "just": true, "also": true, "than": true,
	"then": true, "too": true, "very": true, "into": true,
	"through": true, "during": true, "before": true, "after": true,
	"between": true, "up": true, "down": true, "out": true,
	"off": true, "over": true, "under": true, "again": true,
	"there": true, "here": true, "get": true, "got": true,
	"look": true, "looking": true, "search": true, "related": true,
}

// nlSearchExtractKeywords pulls meaningful words out of a query string.
func nlSearchExtractKeywords(query string) []string {
	words := strings.Fields(strings.ToLower(query))
	var keywords []string
	for _, w := range words {
		w = nlSearchStripPunct(w)
		if len(w) > 1 && !nlSearchStopwords[w] {
			keywords = append(keywords, w)
		}
	}
	return keywords
}

// nlSearchStripPunct removes leading/trailing punctuation from a word.
func nlSearchStripPunct(w string) string {
	w = strings.Trim(w, ".,;:!?\"'`()[]{}#*-_~<>/\\|@$%^&+=")
	return w
}

type nlSearchScore struct {
	index   int
	score   int
	snippet string
}

func (nls NLSearch) runLocalSearch() []nlSearchResult {
	keywords := nlSearchExtractKeywords(nls.query)
	if len(keywords) == 0 {
		return nil
	}

	var scored []nlSearchScore

	for i, entry := range nls.noteIndex {
		score := 0
		var matchedKeywords []string

		titleLower := strings.ToLower(entry.Title)
		contentLower := strings.ToLower(entry.Content)
		pathLower := strings.ToLower(entry.Path)
		tagsLower := strings.ToLower(strings.Join(entry.Tags, " "))

		for _, kw := range keywords {
			titleHit := strings.Contains(titleLower, kw)
			contentHit := strings.Contains(contentLower, kw)
			pathHit := strings.Contains(pathLower, kw)
			tagHit := strings.Contains(tagsLower, kw)

			if titleHit {
				score += 30
				matchedKeywords = append(matchedKeywords, kw)
			}
			if tagHit {
				score += 25
				if !titleHit {
					matchedKeywords = append(matchedKeywords, kw)
				}
			}
			if pathHit && !titleHit {
				score += 20
				matchedKeywords = append(matchedKeywords, kw)
			}
			if contentHit {
				score += 10
				if !titleHit && !pathHit && !tagHit {
					matchedKeywords = append(matchedKeywords, kw)
				}
			}

			// Fuzzy matching on title
			if !titleHit && nlSearchFuzzyContains(titleLower, kw) {
				score += 15
				matchedKeywords = append(matchedKeywords, kw+"~")
			}
		}

		if score > 0 {
			// Extract a snippet: find the first line containing a keyword
			snippet := ""
			if len(entry.Content) > 0 {
				lines := strings.Split(entry.Content, "\n")
				for _, line := range lines {
					lineLower := strings.ToLower(line)
					for _, kw := range keywords {
						if strings.Contains(lineLower, kw) {
							snippet = strings.TrimSpace(line)
							snippet = TruncateDisplay(snippet, 80)
							break
						}
					}
					if snippet != "" {
						break
					}
				}
				// Fallback: first non-empty line
				if snippet == "" {
					for _, line := range lines {
						trimmed := strings.TrimSpace(line)
						if trimmed != "" && !strings.HasPrefix(trimmed, "---") && !strings.HasPrefix(trimmed, "#") {
							snippet = trimmed
							snippet = TruncateDisplay(snippet, 80)
							break
						}
					}
				}
			}

			// Deduplicate matched keywords
			seen := make(map[string]bool)
			var unique []string
			for _, mk := range matchedKeywords {
				if !seen[mk] {
					seen[mk] = true
					unique = append(unique, mk)
				}
			}

			relevance := "Matched: " + strings.Join(unique, ", ")

			scored = append(scored, nlSearchScore{
				index:   i,
				score:   score,
				snippet: snippet,
			})

			_ = relevance // used below after sorting
		}
	}

	// Sort by score descending
	for i := 0; i < len(scored); i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].score > scored[i].score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}

	// Limit to top 15
	if len(scored) > 15 {
		scored = scored[:15]
	}

	var results []nlSearchResult
	for _, s := range scored {
		entry := nls.noteIndex[s.index]

		// Rebuild relevance description
		var matchedKWs []string
		titleLower := strings.ToLower(entry.Title)
		contentLower := strings.ToLower(entry.Content)
		tagsLower := strings.ToLower(strings.Join(entry.Tags, " "))
		for _, kw := range keywords {
			if strings.Contains(titleLower, kw) {
				matchedKWs = append(matchedKWs, kw+" (title)")
			} else if strings.Contains(tagsLower, kw) {
				matchedKWs = append(matchedKWs, kw+" (tag)")
			} else if strings.Contains(contentLower, kw) {
				matchedKWs = append(matchedKWs, kw+" (content)")
			} else if nlSearchFuzzyContains(titleLower, kw) {
				matchedKWs = append(matchedKWs, kw+" (fuzzy)")
			}
		}
		relevance := "Matched: " + strings.Join(matchedKWs, ", ")

		results = append(results, nlSearchResult{
			Path:      entry.Path,
			Title:     entry.Title,
			Relevance: relevance,
			Snippet:   s.snippet,
		})
	}

	return results
}

// nlSearchFuzzyContains returns true if all characters of needle appear
// in order within haystack.
func nlSearchFuzzyContains(haystack, needle string) bool {
	hi := 0
	for ni := 0; ni < len(needle); ni++ {
		found := false
		for hi < len(haystack) {
			if haystack[hi] == needle[ni] {
				hi++
				found = true
				break
			}
			hi++
		}
		if !found {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// Parse AI response
// ---------------------------------------------------------------------------

func (nls *NLSearch) parseAIResponse(response string) {
	nls.results = nil
	lines := strings.Split(response, "\n")

	// Build a path->entry lookup for snippet extraction
	entryByPath := make(map[string]*noteEntry, len(nls.noteIndex))
	for i := range nls.noteIndex {
		entryByPath[nls.noteIndex[i].Path] = &nls.noteIndex[i]
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Strip leading bullet / numbering
		line = strings.TrimLeft(line, "0123456789.-) ")
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "|", 2)
		if len(parts) < 2 {
			// Try " - " as separator
			parts = strings.SplitN(line, " - ", 2)
		}
		if len(parts) < 2 {
			continue
		}

		path := strings.TrimSpace(parts[0])
		relevance := strings.TrimSpace(parts[1])

		// Try to match path against index
		var matchedEntry *noteEntry
		for i := range nls.noteIndex {
			ep := nls.noteIndex[i].Path
			if ep == path || strings.TrimSuffix(ep, ".md") == strings.TrimSuffix(path, ".md") {
				matchedEntry = &nls.noteIndex[i]
				break
			}
		}

		// Fuzzy path matching: AI might return partial names
		if matchedEntry == nil {
			pathLower := strings.ToLower(path)
			for i := range nls.noteIndex {
				titleLower := strings.ToLower(nls.noteIndex[i].Title)
				epLower := strings.ToLower(nls.noteIndex[i].Path)
				if strings.Contains(epLower, pathLower) || strings.Contains(titleLower, pathLower) {
					matchedEntry = &nls.noteIndex[i]
					break
				}
			}
		}

		if matchedEntry == nil {
			// Keep the result even if we can't find the note
			nls.results = append(nls.results, nlSearchResult{
				Path:      path,
				Title:     strings.TrimSuffix(filepath.Base(path), ".md"),
				Relevance: relevance,
				Snippet:   "",
			})
			continue
		}

		// Extract a snippet from the matched entry
		snippet := ""
		if len(matchedEntry.Content) > 0 {
			contentLines := strings.Split(matchedEntry.Content, "\n")
			for _, cl := range contentLines {
				trimmed := strings.TrimSpace(cl)
				if trimmed != "" && !strings.HasPrefix(trimmed, "---") && !strings.HasPrefix(trimmed, "# ") {
					snippet = trimmed
					snippet = TruncateDisplay(snippet, 80)
					break
				}
			}
		}

		nls.results = append(nls.results, nlSearchResult{
			Path:      matchedEntry.Path,
			Title:     matchedEntry.Title,
			Relevance: relevance,
			Snippet:   snippet,
		})

		if len(nls.results) >= 15 {
			break
		}
	}
}

// ---------------------------------------------------------------------------
// Start search
// ---------------------------------------------------------------------------

func (nls NLSearch) startSearch() (NLSearch, tea.Cmd) {
	nls.phase = 1
	nls.spinner = 0
	nls.results = nil
	nls.cursor = 0
	nls.scroll = 0

	if nls.ai.Provider == "ollama" {
		prompt := nls.buildSearchPrompt()
		return nls, tea.Batch(
			callNLSearchOllama(nls.ai.OllamaURL, nls.ai.Model, prompt),
			nlSearchTickCmd(),
		)
	}

	if nls.ai.Provider == "openai" && nls.ai.APIKey != "" {
		prompt := nls.buildSearchPrompt()
		return nls, tea.Batch(
			callNLSearchOpenAI(nls.ai.APIKey, nls.ai.Model, prompt),
			nlSearchTickCmd(),
		)
	}

	// Local fallback — no async needed
	nls.results = nls.runLocalSearch()
	nls.phase = 2
	return nls, nil
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

// Update processes messages for the NLSearch overlay.
func (nls NLSearch) Update(msg tea.Msg) (NLSearch, tea.Cmd) {
	if !nls.active {
		return nls, nil
	}

	switch msg := msg.(type) {
	case nlSearchTickMsg:
		if nls.phase == 1 {
			nls.spinner++
			return nls, nlSearchTickCmd()
		}

	case nlSearchResultMsg:
		if nls.phase != 1 {
			return nls, nil
		}
		if msg.err != nil {
			// AI failed — fall back to local search
			nls.results = nls.runLocalSearch()
			// Prepend a warning line as the first result's relevance
			if len(nls.results) == 0 {
				nls.results = []nlSearchResult{{
					Title:     "No results",
					Relevance: msg.err.Error(),
				}}
			}
			nls.phase = 2
			nls.cursor = 0
			nls.scroll = 0
			return nls, nil
		}
		nls.parseAIResponse(msg.response)
		if len(nls.results) == 0 {
			nls.results = []nlSearchResult{{
				Title:     "No results",
				Relevance: "AI did not return any matching notes",
			}}
		}
		nls.phase = 2
		nls.cursor = 0
		nls.scroll = 0
		return nls, nil

	case tea.KeyMsg:
		switch nls.phase {
		case 0:
			return nls.updateInput(msg)
		case 1:
			return nls.updateSearching(msg)
		case 2:
			return nls.updateResults(msg)
		}
	}

	return nls, nil
}

func (nls NLSearch) updateInput(msg tea.KeyMsg) (NLSearch, tea.Cmd) {
	switch msg.String() {
	case "esc":
		nls.active = false
		return nls, nil
	case "enter":
		if strings.TrimSpace(nls.query) != "" {
			return nls.startSearch()
		}
	case "backspace":
		if len(nls.query) > 0 {
			nls.query = nls.query[:len(nls.query)-1]
		}
	case "ctrl+u":
		nls.query = ""
	default:
		ch := msg.String()
		if len(ch) == 1 && ch[0] >= 32 {
			nls.query += ch
		} else if ch == " " {
			nls.query += " "
		}
	}
	return nls, nil
}

func (nls NLSearch) updateSearching(msg tea.KeyMsg) (NLSearch, tea.Cmd) {
	switch msg.String() {
	case "esc":
		nls.phase = 0
		return nls, nil
	}
	return nls, nil
}

func (nls NLSearch) updateResults(msg tea.KeyMsg) (NLSearch, tea.Cmd) {
	switch msg.String() {
	case "esc":
		nls.phase = 0
		nls.results = nil
		nls.cursor = 0
		nls.scroll = 0
		return nls, nil
	case "q":
		nls.active = false
		return nls, nil
	case "up", "k":
		if nls.cursor > 0 {
			nls.cursor--
			nls.ensureVisible()
		}
	case "down", "j":
		if nls.cursor < len(nls.results)-1 {
			nls.cursor++
			nls.ensureVisible()
		}
	case "enter":
		if len(nls.results) > 0 && nls.cursor < len(nls.results) {
			selected := nls.results[nls.cursor]
			if selected.Path != "" {
				nls.selectedNote = selected.Path
				nls.hasResult = true
				nls.active = false
			}
		}
		return nls, nil
	}
	return nls, nil
}

func (nls *NLSearch) ensureVisible() {
	maxVisible := nls.maxVisibleResults()
	if nls.cursor < nls.scroll {
		nls.scroll = nls.cursor
	}
	if nls.cursor >= nls.scroll+maxVisible {
		nls.scroll = nls.cursor - maxVisible + 1
	}
}

func (nls NLSearch) maxVisibleResults() int {
	// Each result takes ~4 lines (title + relevance + snippet + spacing)
	available := nls.height - 14
	maxItems := available / 4
	if maxItems < 3 {
		maxItems = 3
	}
	if maxItems > len(nls.results) {
		maxItems = len(nls.results)
	}
	return maxItems
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

// View renders the NLSearch overlay.
func (nls NLSearch) View() string {
	if !nls.active {
		return ""
	}

	switch nls.phase {
	case 0:
		return nls.viewInput()
	case 1:
		return nls.viewSearching()
	case 2:
		return nls.viewResults()
	}
	return ""
}

func (nls NLSearch) overlayWidth() int {
	w := nls.width * 2 / 3
	if w < 55 {
		w = 55
	}
	if w > 90 {
		w = 90
	}
	return w
}

func (nls NLSearch) overlayInnerWidth() int {
	return nls.overlayWidth() - 6
}

// viewInput renders the query input phase.
func (nls NLSearch) viewInput() string {
	width := nls.overlayWidth()
	innerWidth := nls.overlayInnerWidth()

	var b strings.Builder

	// Title
	titleIcon := lipgloss.NewStyle().Foreground(mauve).Render(IconSearchChar)
	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  " + titleIcon + " Natural Language Search")
	b.WriteString(title)
	b.WriteString("\n")

	// Provider badge
	providerLabel := "Local Analysis"
	providerColor := overlay0
	switch nls.ai.Provider {
	case "ollama":
		providerLabel = "Ollama: " + nls.ai.Model
		providerColor = green
	case "openai":
		providerLabel = "OpenAI: " + nls.ai.Model
		providerColor = blue
	}
	providerBadge := lipgloss.NewStyle().Foreground(providerColor).Render(
		"  " + IconBotChar + " " + providerLabel)
	b.WriteString(providerBadge)
	b.WriteString("\n")

	b.WriteString(DimStyle.Render(strings.Repeat("─", innerWidth)))
	b.WriteString("\n\n")

	// Instruction
	b.WriteString(lipgloss.NewStyle().Foreground(text).Render(
		"  Describe what you're looking for:"))
	b.WriteString("\n\n")

	// Input field
	prompt := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("  > ")
	inputText := nls.query + lipgloss.NewStyle().
		Background(text).
		Foreground(mantle).
		Render(" ")
	inputStyled := lipgloss.NewStyle().Foreground(text).Render(inputText)
	b.WriteString(prompt + inputStyled)
	b.WriteString("\n\n")

	// Examples
	b.WriteString(DimStyle.Render("  " + strings.Repeat("─", innerWidth-4)))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Examples:"))
	b.WriteString("\n")
	examples := []string{
		"notes about machine learning",
		"meeting notes from last week",
		"ideas related to the project roadmap",
		"anything mentioning deployment",
	}
	for _, ex := range examples {
		b.WriteString(DimStyle.Render("    " + IconSearchChar + " " + ex))
		b.WriteString("\n")
	}

	// Note count
	b.WriteString("\n")
	noteCount := len(nls.noteIndex)
	b.WriteString(DimStyle.Render(fmt.Sprintf("  %d notes indexed", noteCount)))
	b.WriteString("\n\n")

	// Hints
	b.WriteString(DimStyle.Render("  Enter: search  Esc: close"))

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

// viewSearching renders the spinner phase while waiting for AI.
func (nls NLSearch) viewSearching() string {
	width := nls.overlayWidth()
	innerWidth := nls.overlayInnerWidth()

	var b strings.Builder

	// Title
	titleIcon := lipgloss.NewStyle().Foreground(mauve).Render(IconSearchChar)
	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  " + titleIcon + " Natural Language Search")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", innerWidth)))
	b.WriteString("\n\n")

	// Query echo
	b.WriteString(lipgloss.NewStyle().Foreground(subtext0).Italic(true).Render(
		"  \"" + nls.query + "\""))
	b.WriteString("\n\n")

	// Spinner
	spinFrames := []string{"|", "/", "-", "\\"}
	frame := spinFrames[nls.spinner%len(spinFrames)]
	spinnerStyled := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(frame)

	thinkingLabel := "Searching with local analysis..."
	connectLabel := ""
	switch nls.ai.Provider {
	case "ollama":
		thinkingLabel = "Searching with " + nls.ai.Model + "..."
		connectLabel = "Connecting to Ollama at " + nls.ai.OllamaURL
	case "openai":
		thinkingLabel = "Searching with " + nls.ai.Model + "..."
		connectLabel = "Connecting to OpenAI API..."
	}
	b.WriteString("  " + spinnerStyled + " " + lipgloss.NewStyle().Foreground(text).Render(thinkingLabel))
	b.WriteString("\n\n")
	if connectLabel != "" {
		b.WriteString(DimStyle.Render("  " + connectLabel))
		b.WriteString("\n\n")
	}

	// Note count
	b.WriteString(DimStyle.Render(fmt.Sprintf("  Searching across %d notes...", len(nls.noteIndex))))
	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render("  Esc: cancel"))

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

// viewResults renders the search results phase.
func (nls NLSearch) viewResults() string {
	width := nls.overlayWidth()
	innerWidth := nls.overlayInnerWidth()

	var b strings.Builder

	// Title
	titleIcon := lipgloss.NewStyle().Foreground(mauve).Render(IconSearchChar)
	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  " + titleIcon + " Search Results")
	b.WriteString(title)
	b.WriteString("\n")

	// Query echo
	b.WriteString(lipgloss.NewStyle().Foreground(subtext0).Italic(true).Render(
		"  \"" + nls.query + "\""))
	b.WriteString("\n")

	// Provider info
	switch nls.ai.Provider {
	case "ollama":
		providerInfo := lipgloss.NewStyle().Foreground(green).Render(
			"  " + IconBotChar + " Powered by Ollama: " + nls.ai.Model)
		b.WriteString(providerInfo)
		b.WriteString("\n")
	case "openai":
		providerInfo := lipgloss.NewStyle().Foreground(blue).Render(
			"  " + IconBotChar + " Powered by OpenAI: " + nls.ai.Model)
		b.WriteString(providerInfo)
		b.WriteString("\n")
	default:
		providerInfo := DimStyle.Render("  " + IconBotChar + " Local keyword analysis")
		b.WriteString(providerInfo)
		b.WriteString("\n")
	}

	b.WriteString(DimStyle.Render(strings.Repeat("─", innerWidth)))
	b.WriteString("\n")

	// Result count
	resultCountStr := fmt.Sprintf("  %d result", len(nls.results))
	if len(nls.results) != 1 {
		resultCountStr += "s"
	}
	resultCountStr += " found"
	b.WriteString(DimStyle.Render(resultCountStr))
	b.WriteString("\n\n")

	if len(nls.results) == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(overlay0).Render("  No matching notes found"))
		b.WriteString("\n")
	} else {
		maxVisible := nls.maxVisibleResults()
		start := nls.scroll
		end := start + maxVisible
		if end > len(nls.results) {
			end = len(nls.results)
		}

		// Scroll indicator (top)
		if start > 0 {
			b.WriteString(DimStyle.Render(fmt.Sprintf("  ... %d more above ...", start)))
			b.WriteString("\n")
		}

		for idx := start; idx < end; idx++ {
			result := nls.results[idx]
			isCurrent := idx == nls.cursor

			// Result number
			numStr := fmt.Sprintf("%d", idx+1)

			// Title line
			fileIcon := lipgloss.NewStyle().Foreground(blue).Render(IconFileChar)
			titleText := strings.TrimSuffix(filepath.Base(result.Path), ".md")
			if result.Title != "" {
				titleText = result.Title
			}

			if isCurrent {
				// Highlighted current selection
				pointer := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("  > ")
				numStyled := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(numStr + ". ")
				titleStyled := lipgloss.NewStyle().
					Foreground(peach).
					Bold(true).
					Render(titleText)

				titleLine := pointer + numStyled + fileIcon + " " + titleStyled
				b.WriteString(lipgloss.NewStyle().
					Background(surface0).
					Width(innerWidth).
					Render(titleLine))
				b.WriteString("\n")

				// Relevance on highlighted
				if result.Relevance != "" {
					relText := TruncateDisplay(result.Relevance, innerWidth-8)
					relevanceLine := lipgloss.NewStyle().
						Background(surface0).
						Foreground(lavender).
						Width(innerWidth).
						Render("      " + relText)
					b.WriteString(relevanceLine)
					b.WriteString("\n")
				}

				// Snippet on highlighted
				if result.Snippet != "" {
					snipText := TruncateDisplay(result.Snippet, innerWidth-8)
					snippetLine := lipgloss.NewStyle().
						Background(surface0).
						Foreground(overlay0).
						Italic(true).
						Width(innerWidth).
						Render("      " + snipText)
					b.WriteString(snippetLine)
					b.WriteString("\n")
				}

				// Path on highlighted
				pathText := TruncateDisplay(result.Path, innerWidth-8)
				pathLine := lipgloss.NewStyle().
					Background(surface0).
					Foreground(surface1).
					Width(innerWidth).
					Render("      " + pathText)
				b.WriteString(pathLine)
			} else {
				// Non-highlighted result
				numStyled := DimStyle.Render("    " + numStr + ". ")
				titleStyled := lipgloss.NewStyle().Foreground(text).Render(titleText)
				b.WriteString(numStyled + fileIcon + " " + titleStyled)
				b.WriteString("\n")

				// Relevance
				if result.Relevance != "" {
					relText := TruncateDisplay(result.Relevance, innerWidth-8)
					b.WriteString(lipgloss.NewStyle().Foreground(teal).Render("      " + relText))
					b.WriteString("\n")
				}

				// Snippet
				if result.Snippet != "" {
					snipText := TruncateDisplay(result.Snippet, innerWidth-8)
					b.WriteString(DimStyle.Render("      " + snipText))
				}
			}

			if idx < end-1 {
				b.WriteString("\n")
				b.WriteString(DimStyle.Render("  " + strings.Repeat("·", innerWidth-4)))
				b.WriteString("\n")
			}
		}

		// Scroll indicator (bottom)
		if end < len(nls.results) {
			b.WriteString("\n")
			b.WriteString(DimStyle.Render(fmt.Sprintf("  ... %d more below ...", len(nls.results)-end)))
		}
	}

	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("─", innerWidth-4)))
	b.WriteString("\n")

	// Navigation and position indicator
	posStr := ""
	if len(nls.results) > 0 {
		posStr = fmt.Sprintf(" [%d/%d]", nls.cursor+1, len(nls.results))
	}
	b.WriteString(DimStyle.Render("  j/k: navigate  Enter: open  Esc: back  q: close" + posStr))

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}
