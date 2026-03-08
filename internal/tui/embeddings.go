package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// EmbeddingIndex holds precomputed vector embeddings for vault notes.
type EmbeddingIndex struct {
	Embeddings map[string][]float64 `json:"embeddings"` // notePath -> embedding vector
	Model      string               `json:"model"`
	Version    int                   `json:"version"`
}

// semanticResult represents a single search hit ranked by cosine similarity.
type semanticResult struct {
	Path    string
	Score   float64
	Snippet string // first ~100 chars of note
}

// SemanticSearch is an overlay that generates vector embeddings for vault
// notes (via Ollama or OpenAI) and lets the user type a natural-language
// query to find notes ranked by cosine similarity.
type SemanticSearch struct {
	active bool
	width  int
	height int

	query       string
	results     []semanticResult
	cursor      int
	loading     bool
	loadingTick int

	index     *EmbeddingIndex
	vaultPath string

	// AI config
	provider  string
	model     string
	ollamaURL string
	apiKey    string

	noteContents map[string]string

	// Build state
	building      bool
	buildProgress int
	buildTotal    int
}

// ---------------------------------------------------------------------------
// Message types
// ---------------------------------------------------------------------------

type semanticBuildMsg struct {
	done     bool
	progress int
	total    int
	err      error
}

type semanticSearchMsg struct {
	results []semanticResult
	err     error
}

type semanticTickMsg struct{}

// ---------------------------------------------------------------------------
// Ollama embedding API types
// ---------------------------------------------------------------------------

type ollamaEmbedRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type ollamaEmbedResponse struct {
	Embeddings [][]float64 `json:"embeddings"`
}

// ---------------------------------------------------------------------------
// OpenAI embedding API types
// ---------------------------------------------------------------------------

type openaiEmbeddingRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type openaiEmbeddingResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// ---------------------------------------------------------------------------
// Constructor & overlay interface
// ---------------------------------------------------------------------------

// NewSemanticSearch returns a zero-value SemanticSearch ready for use.
func NewSemanticSearch() SemanticSearch {
	return SemanticSearch{
		provider:  "ollama",
		model:     "nomic-embed-text",
		ollamaURL: "http://localhost:11434",
	}
}

// IsActive reports whether the overlay is visible.
func (ss *SemanticSearch) IsActive() bool {
	return ss.active
}

// Open activates the overlay. If an index is already loaded it is reused;
// otherwise we attempt to load from disk.
func (ss *SemanticSearch) Open() {
	ss.active = true
	ss.query = ""
	ss.results = nil
	ss.cursor = 0
	ss.loading = false
	ss.loadingTick = 0

	// Try to load a persisted index if we don't have one yet.
	if ss.index == nil && ss.vaultPath != "" {
		ss.index = LoadIndex(ss.vaultPath)
	}
}

// Close deactivates the overlay.
func (ss *SemanticSearch) Close() {
	ss.active = false
	ss.query = ""
	ss.results = nil
}

// SetSize updates the available dimensions for the overlay.
func (ss *SemanticSearch) SetSize(w, h int) {
	ss.width = w
	ss.height = h
}

// SetConfig updates the AI provider configuration.
func (ss *SemanticSearch) SetConfig(provider, model, ollamaURL, apiKey string) {
	ss.provider = provider
	if model != "" {
		ss.model = model
	}
	if ollamaURL != "" {
		ss.ollamaURL = ollamaURL
	}
	ss.apiKey = apiKey
}

// SetNotes provides the note contents used for building the index.
func (ss *SemanticSearch) SetNotes(notes map[string]string) {
	ss.noteContents = notes
}

// SetVaultPath sets the vault root used for persistence.
func (ss *SemanticSearch) SetVaultPath(vaultPath string) {
	ss.vaultPath = vaultPath
}

// SelectedResult returns the path of the selected note (consumed once).
func (ss *SemanticSearch) SelectedResult() string {
	if len(ss.results) == 0 || ss.cursor >= len(ss.results) {
		return ""
	}
	path := ss.results[ss.cursor].Path
	ss.results = nil
	ss.cursor = 0
	return path
}

// ---------------------------------------------------------------------------
// Persistence
// ---------------------------------------------------------------------------

// SaveIndex writes the embedding index to <vaultPath>/.granit/embeddings.json.
func SaveIndex(vaultPath string, idx *EmbeddingIndex) error {
	if idx == nil {
		return nil
	}
	dir := filepath.Join(vaultPath, ".granit")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.Marshal(idx)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "embeddings.json"), data, 0600)
}

// LoadIndex reads the embedding index from <vaultPath>/.granit/embeddings.json.
// Returns nil if the file does not exist or cannot be parsed.
func LoadIndex(vaultPath string) *EmbeddingIndex {
	path := filepath.Join(vaultPath, ".granit", "embeddings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var idx EmbeddingIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil
	}
	return &idx
}

// ---------------------------------------------------------------------------
// Embedding generation helpers
// ---------------------------------------------------------------------------

// getEmbedding calls the configured AI provider to produce a vector for text.
func getEmbedding(provider, model, ollamaURL, apiKey, text string) ([]float64, error) {
	switch provider {
	case "openai":
		return getOpenAIEmbedding(apiKey, model, text)
	default:
		return getOllamaEmbedding(ollamaURL, model, text)
	}
}

func getOllamaEmbedding(url, model, text string) ([]float64, error) {
	reqBody := ollamaEmbedRequest{
		Model: model,
		Input: text,
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Post(url+"/api/embed", "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("cannot connect to Ollama at %s: %w", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Ollama returned status %d: %s", resp.StatusCode, string(body))
	}

	var olResp ollamaEmbedResponse
	if err := json.Unmarshal(body, &olResp); err != nil {
		return nil, err
	}
	if len(olResp.Embeddings) == 0 {
		return nil, fmt.Errorf("Ollama returned no embeddings")
	}
	return olResp.Embeddings[0], nil
}

func getOpenAIEmbedding(apiKey, model, text string) ([]float64, error) {
	if model == "" || model == "nomic-embed-text" || model == "all-minilm" {
		model = "text-embedding-3-small"
	}
	reqBody := openaiEmbeddingRequest{
		Model: model,
		Input: text,
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequest("POST", "https://api.openai.com/v1/embeddings", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot connect to OpenAI: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var oaiResp openaiEmbeddingResponse
	if err := json.Unmarshal(body, &oaiResp); err != nil {
		return nil, err
	}
	if oaiResp.Error != nil {
		return nil, fmt.Errorf("OpenAI error: %s", oaiResp.Error.Message)
	}
	if len(oaiResp.Data) == 0 {
		return nil, fmt.Errorf("OpenAI returned no embeddings")
	}
	return oaiResp.Data[0].Embedding, nil
}

// truncateContent returns at most maxLen characters of the input.
func truncateContent(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

// noteSnippet returns the first ~100 characters of a note, trimmed.
func noteSnippet(content string) string {
	s := strings.TrimSpace(content)
	if len(s) > 100 {
		s = s[:100]
	}
	// Replace newlines with spaces for single-line display.
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

// ---------------------------------------------------------------------------
// Cosine similarity for embedding vectors
// ---------------------------------------------------------------------------

// embeddingCosineSimilarity computes the cosine similarity between two float64
// slices representing embedding vectors. Named to avoid conflict with the
// TF-IDF cosineSimilarity in similarity.go.
func embeddingCosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return dot / denom
}

// ---------------------------------------------------------------------------
// Background index building
// ---------------------------------------------------------------------------

// buildEmbeddingIndex determines which notes need (re-)embedding and whether
// the model has changed, then returns a tea.Cmd that builds the index in a
// background goroutine, sending progress via semanticBuildMsg.
func (ss *SemanticSearch) needsRebuild() bool {
	if ss.index == nil {
		return true
	}
	if ss.index.Model != ss.model {
		return true
	}
	// Check for added/removed notes.
	for path := range ss.noteContents {
		if _, ok := ss.index.Embeddings[path]; !ok {
			return true
		}
	}
	for path := range ss.index.Embeddings {
		if _, ok := ss.noteContents[path]; !ok {
			return true
		}
	}
	return false
}

func (ss *SemanticSearch) startBuild() tea.Cmd {
	notes := make(map[string]string, len(ss.noteContents))
	for k, v := range ss.noteContents {
		notes[k] = v
	}
	provider := ss.provider
	model := ss.model
	ollamaURL := ss.ollamaURL
	apiKey := ss.apiKey
	vaultPath := ss.vaultPath

	// Determine which paths need embedding.
	existing := make(map[string][]float64)
	if ss.index != nil && ss.index.Model == model {
		for p, vec := range ss.index.Embeddings {
			if _, ok := notes[p]; ok {
				existing[p] = vec
			}
		}
	}

	// Paths that still need embedding.
	var todo []string
	for path := range notes {
		if _, ok := existing[path]; !ok {
			todo = append(todo, path)
		}
	}
	sort.Strings(todo)

	total := len(todo)

	return func() tea.Msg {
		if total == 0 {
			// Nothing to do — index is up to date.
			idx := &EmbeddingIndex{
				Embeddings: existing,
				Model:      model,
				Version:    1,
			}
			_ = SaveIndex(vaultPath, idx)
			return semanticBuildMsg{done: true, progress: 0, total: 0}
		}

		embeddings := make(map[string][]float64, len(existing)+total)
		for p, v := range existing {
			embeddings[p] = v
		}

		for i, path := range todo {
			content := truncateContent(notes[path], 2000)
			if strings.TrimSpace(content) == "" {
				content = path // fallback: embed the path itself
			}

			vec, err := getEmbedding(provider, model, ollamaURL, apiKey, content)
			if err != nil {
				return semanticBuildMsg{err: fmt.Errorf("embedding %s: %w", path, err)}
			}
			embeddings[path] = vec

			// Send progress for every note (via channel would be ideal,
			// but bubbletea commands return a single Msg — so we just
			// report the final result here; the tick handles animation).
			_ = i // progress tracked via final message
		}

		idx := &EmbeddingIndex{
			Embeddings: embeddings,
			Model:      model,
			Version:    1,
		}
		_ = SaveIndex(vaultPath, idx)
		return semanticBuildMsg{done: true, progress: total, total: total}
	}
}

// semanticTick drives the loading / building spinner animation.
func semanticTick() tea.Cmd {
	return tea.Tick(300*time.Millisecond, func(time.Time) tea.Msg {
		return semanticTickMsg{}
	})
}

// ---------------------------------------------------------------------------
// Search execution
// ---------------------------------------------------------------------------

func (ss *SemanticSearch) startSearch() tea.Cmd {
	query := ss.query
	provider := ss.provider
	model := ss.model
	ollamaURL := ss.ollamaURL
	apiKey := ss.apiKey

	// Snapshot the index and note contents.
	idx := ss.index
	notes := ss.noteContents

	return func() tea.Msg {
		if idx == nil || len(idx.Embeddings) == 0 {
			return semanticSearchMsg{err: fmt.Errorf("no embedding index — press 'b' to build")}
		}

		// Generate embedding for the query.
		queryVec, err := getEmbedding(provider, model, ollamaURL, apiKey, query)
		if err != nil {
			return semanticSearchMsg{err: fmt.Errorf("query embedding: %w", err)}
		}

		type scored struct {
			path  string
			score float64
		}
		var hits []scored
		for path, vec := range idx.Embeddings {
			sim := embeddingCosineSimilarity(queryVec, vec)
			if sim > 0 {
				hits = append(hits, scored{path, sim})
			}
		}

		sort.Slice(hits, func(i, j int) bool {
			return hits[i].score > hits[j].score
		})

		topN := 10
		if len(hits) < topN {
			topN = len(hits)
		}

		results := make([]semanticResult, topN)
		for i := 0; i < topN; i++ {
			snippet := ""
			if notes != nil {
				snippet = noteSnippet(notes[hits[i].path])
			}
			results[i] = semanticResult{
				Path:    hits[i].path,
				Score:   hits[i].score,
				Snippet: snippet,
			}
		}

		return semanticSearchMsg{results: results}
	}
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

// Update handles key events, build/search results, and tick messages.
func (ss SemanticSearch) Update(msg tea.Msg) (SemanticSearch, tea.Cmd) {
	if !ss.active {
		return ss, nil
	}

	switch msg := msg.(type) {
	case semanticTickMsg:
		if ss.loading || ss.building {
			ss.loadingTick++
			return ss, semanticTick()
		}

	case semanticBuildMsg:
		ss.building = false
		if msg.err != nil {
			// Show error as a result so the user can see it.
			ss.results = []semanticResult{
				{Path: "Error", Score: 0, Snippet: msg.err.Error()},
			}
		} else if msg.done {
			// Reload the index.
			if ss.vaultPath != "" {
				ss.index = LoadIndex(ss.vaultPath)
			}
			ss.buildProgress = msg.total
			ss.buildTotal = msg.total
		}
		return ss, nil

	case semanticSearchMsg:
		ss.loading = false
		if msg.err != nil {
			ss.results = []semanticResult{
				{Path: "Error", Score: 0, Snippet: msg.err.Error()},
			}
		} else {
			ss.results = msg.results
		}
		ss.cursor = 0
		return ss, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if len(ss.results) > 0 {
				// Go back to query input.
				ss.results = nil
				ss.cursor = 0
				return ss, nil
			}
			ss.active = false
			return ss, nil

		case "enter":
			if ss.building || ss.loading {
				return ss, nil
			}
			if len(ss.results) > 0 {
				// Select the highlighted result — caller reads via SelectedResult().
				return ss, nil
			}
			// Perform search.
			trimmed := strings.TrimSpace(ss.query)
			if trimmed == "" {
				return ss, nil
			}
			ss.loading = true
			ss.loadingTick = 0
			return ss, tea.Batch(ss.startSearch(), semanticTick())

		case "up", "k":
			if len(ss.results) > 0 && ss.cursor > 0 {
				ss.cursor--
			}
			return ss, nil

		case "down", "j":
			if len(ss.results) > 0 && ss.cursor < len(ss.results)-1 {
				ss.cursor++
			}
			return ss, nil

		case "backspace":
			if len(ss.results) > 0 {
				return ss, nil
			}
			if len(ss.query) > 0 {
				ss.query = ss.query[:len(ss.query)-1]
			}
			return ss, nil

		case "b":
			if len(ss.results) > 0 && !ss.building && !ss.loading {
				// Rebuild index.
				ss.building = true
				ss.buildProgress = 0
				ss.buildTotal = len(ss.noteContents)
				ss.loadingTick = 0
				ss.results = nil
				return ss, tea.Batch(ss.startBuild(), semanticTick())
			}
			// If no results shown, treat 'b' as a regular character.
			if len(ss.results) == 0 && !ss.building && !ss.loading {
				ss.query += "b"
			}
			return ss, nil

		default:
			if len(ss.results) > 0 {
				return ss, nil
			}
			if ss.building || ss.loading {
				return ss, nil
			}
			ch := msg.String()
			if len(ch) == 1 && ch[0] >= 32 {
				ss.query += ch
			}
			return ss, nil
		}
	}

	return ss, nil
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

// View renders the semantic search overlay panel.
func (ss SemanticSearch) View() string {
	panelWidth := ss.width * 2 / 3
	if panelWidth < 60 {
		panelWidth = 60
	}
	if panelWidth > 100 {
		panelWidth = 100
	}

	innerWidth := panelWidth - 6

	var b strings.Builder

	// Title
	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  " + IconSearchChar + " Semantic Search")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", innerWidth)))
	b.WriteString("\n")

	// Building state
	if ss.building {
		spinner := []string{"\u280b", "\u2819", "\u2838", "\u2834", "\u2826", "\u2827", "\u2807", "\u280f"}
		frame := spinner[ss.loadingTick%len(spinner)]

		progressLine := fmt.Sprintf("  %s Building embeddings... %d/%d",
			lipgloss.NewStyle().Foreground(peach).Render(frame),
			ss.buildProgress,
			ss.buildTotal,
		)
		b.WriteString(progressLine)
		b.WriteString("\n")

		// Progress bar
		if ss.buildTotal > 0 {
			barWidth := innerWidth - 6
			if barWidth < 10 {
				barWidth = 10
			}
			filled := barWidth * ss.buildProgress / ss.buildTotal
			if filled > barWidth {
				filled = barWidth
			}
			bar := lipgloss.NewStyle().Foreground(green).Render(strings.Repeat("\u2588", filled))
			bar += lipgloss.NewStyle().Foreground(surface0).Render(strings.Repeat("\u2591", barWidth-filled))
			b.WriteString("  " + bar)
			b.WriteString("\n")
		}

		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  Please wait..."))
		b.WriteString("\n")
	} else {
		// Search input
		prompt := SearchPromptStyle.Render("  > ")
		input := ss.query + DimStyle.Render("_")
		b.WriteString(prompt + input)
		b.WriteString("\n")

		// Loading indicator while searching
		if ss.loading {
			dots := strings.Repeat(".", (ss.loadingTick%3)+1)
			b.WriteString(lipgloss.NewStyle().Foreground(peach).Render("  Searching" + dots))
			b.WriteString("\n")
		}

		b.WriteString(DimStyle.Render(strings.Repeat("\u2500", innerWidth)))
		b.WriteString("\n")

		// Index status
		if ss.index != nil {
			indexInfo := fmt.Sprintf("  Index: %d notes [%s]", len(ss.index.Embeddings), ss.index.Model)
			b.WriteString(DimStyle.Render(indexInfo))
			b.WriteString("\n")
		} else {
			b.WriteString(lipgloss.NewStyle().Foreground(yellow).Render("  No index loaded — press Enter with empty query to build"))
			b.WriteString("\n")
		}

		// Results area
		if ss.query == "" && len(ss.results) == 0 && !ss.loading {
			b.WriteString("\n")
			b.WriteString(DimStyle.Render("  Type a natural language query and press Enter..."))
			b.WriteString("\n")
		} else if len(ss.results) == 0 && !ss.loading {
			if ss.query != "" {
				b.WriteString("\n")
				b.WriteString(DimStyle.Render("  No results"))
				b.WriteString("\n")
			}
		} else if len(ss.results) > 0 {
			b.WriteString("\n")
			visH := ss.visibleHeight()
			end := visH
			if end > len(ss.results) {
				end = len(ss.results)
			}

			// Determine scroll offset so the cursor stays visible.
			scrollOff := 0
			if ss.cursor >= visH {
				scrollOff = ss.cursor - visH + 1
			}
			start := scrollOff
			end = start + visH
			if end > len(ss.results) {
				end = len(ss.results)
			}

			for i := start; i < end; i++ {
				r := ss.results[i]

				// Score as percentage.
				scorePct := fmt.Sprintf("%5.1f%%", r.Score*100)
				scoreStyle := lipgloss.NewStyle().Foreground(green).Bold(true)

				// Note name.
				name := filepath.Base(r.Path)
				name = strings.TrimSuffix(name, ".md")

				// Snippet (dim).
				snippet := r.Snippet
				maxSnippetLen := innerWidth - lipgloss.Width(scorePct) - lipgloss.Width(name) - 8
				if maxSnippetLen < 0 {
					maxSnippetLen = 0
				}
				if len(snippet) > maxSnippetLen {
					if maxSnippetLen > 3 {
						snippet = snippet[:maxSnippetLen-3] + "..."
					} else {
						snippet = ""
					}
				}

				if i == ss.cursor {
					selectedLine := fmt.Sprintf("  %s  %s", scoreStyle.Render(scorePct),
						lipgloss.NewStyle().Foreground(peach).Bold(true).Render(name))
					if snippet != "" {
						selectedLine += "\n" + lipgloss.NewStyle().Foreground(overlay0).Render("         "+snippet)
					}
					b.WriteString(lipgloss.NewStyle().
						Background(surface0).
						Width(innerWidth).
						Render(selectedLine))
				} else {
					normalLine := fmt.Sprintf("  %s  %s", scoreStyle.Render(scorePct),
						NormalItemStyle.Render(name))
					if snippet != "" {
						normalLine += "\n" + DimStyle.Render("         "+snippet)
					}
					b.WriteString(normalLine)
				}

				if i < end-1 {
					b.WriteString("\n")
				}
			}
		}
	}

	// Footer
	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", innerWidth)))
	b.WriteString("\n")

	var footer string
	if ss.building {
		footer = "  Building index..."
	} else if len(ss.results) > 0 {
		footer = "  Enter: open note  Esc: back  b: rebuild index  " +
			DimStyle.Render("("+smallNum(len(ss.results))+" results)")
	} else {
		footer = "  Enter: search  Esc: close"
		if ss.index == nil {
			footer += "  (no index)"
		}
	}
	b.WriteString(DimStyle.Render(footer))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(yellow).
		Padding(1, 2).
		Width(panelWidth).
		Background(mantle)

	return border.Render(b.String())
}

// visibleHeight returns the number of result entries visible in the content area.
func (ss SemanticSearch) visibleHeight() int {
	// Each result takes ~2 lines (name + snippet). Subtract chrome.
	h := (ss.height - 16) / 2
	if h < 3 {
		h = 3
	}
	return h
}
