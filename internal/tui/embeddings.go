package tui

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// EmbeddingEntry holds a cached embedding vector alongside its content hash
// so that stale entries can be detected without re-calling the API.
type EmbeddingEntry struct {
	Vector      []float64 `json:"vector"`
	ContentHash string    `json:"content_hash"` // SHA-256 of note content at embed time
}

// EmbeddingIndex holds precomputed vector embeddings for vault notes.
type EmbeddingIndex struct {
	Embeddings map[string][]float64 `json:"embeddings,omitempty"` // legacy: notePath -> embedding vector
	Model      string               `json:"model"`
	Version    int                   `json:"version"`

	// V2 cache with content hashes for incremental updates.
	Entries map[string]EmbeddingEntry `json:"entries,omitempty"` // notePath -> entry
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
	ai AIConfig

	noteContents map[string]string

	// Build state
	building      bool
	buildProgress int
	buildTotal    int

	// Background indexing state
	bgIndexing bool   // background build in progress (not user-triggered)
	bgMu       sync.Mutex
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

// semanticBgIndexMsg is sent when background embedding indexing completes or
// reports progress (used when SemanticSearchEnabled triggers indexing on startup
// or after a note is saved).
type semanticBgIndexMsg struct {
	done     bool
	progress int
	total    int
	err      error
	statusText string // human-readable status for the status bar
}

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
func NewSemanticSearch() *SemanticSearch {
	return &SemanticSearch{
		ai: AIConfig{
			Provider:  "ollama",
			Model:     "nomic-embed-text",
			OllamaURL: "http://localhost:11434",
		},
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
func (ss *SemanticSearch) SetConfig(cfg AIConfig) {
	ss.ai = cfg
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
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(idx)
	if err != nil {
		return err
	}
	return atomicWriteState(filepath.Join(dir, "embeddings.json"), data)
}

// LoadIndex reads the embedding index from <vaultPath>/.granit/embeddings.json.
// Returns nil if the file does not exist or cannot be parsed. A v1 index
// (Embeddings map without per-entry hashes) is migrated to v2 in-place
// before returning so callers never have to think about the on-disk shape.
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
	if idx.Version < 2 || len(idx.Embeddings) > 0 {
		migrateIndex(&idx)
	}
	return &idx
}

// contentHash returns the hex-encoded SHA-256 hash of s.
func contentHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// migrateIndex upgrades a legacy v1 index (flat Embeddings map, no hashes)
// to the v2 format with EmbeddingEntry structs. After migration the legacy
// Embeddings map is cleared.
func migrateIndex(idx *EmbeddingIndex) {
	if idx == nil {
		return
	}
	if idx.Entries == nil {
		idx.Entries = make(map[string]EmbeddingEntry, len(idx.Embeddings))
	}
	// Migrate any legacy entries that are not yet in Entries.
	for path, vec := range idx.Embeddings {
		if _, ok := idx.Entries[path]; !ok {
			idx.Entries[path] = EmbeddingEntry{
				Vector:      vec,
				ContentHash: "", // unknown — will be re-embedded on next build
			}
		}
	}
	idx.Embeddings = nil
	idx.Version = 2
}

// copyEmbeddingIndex returns a deep copy of idx (or nil if idx is nil) that
// the caller can mutate without racing the original. The vector slices
// inside each entry are NOT copied — they are immutable once written, so
// sharing them is safe and avoids a large allocation per build.
func copyEmbeddingIndex(idx *EmbeddingIndex) *EmbeddingIndex {
	if idx == nil {
		return nil
	}
	dup := &EmbeddingIndex{
		Model:   idx.Model,
		Version: idx.Version,
	}
	if idx.Entries != nil {
		dup.Entries = make(map[string]EmbeddingEntry, len(idx.Entries))
		for k, v := range idx.Entries {
			dup.Entries[k] = v
		}
	}
	if idx.Embeddings != nil {
		dup.Embeddings = make(map[string][]float64, len(idx.Embeddings))
		for k, v := range idx.Embeddings {
			dup.Embeddings[k] = v
		}
	}
	return dup
}

// indexAllVecs returns all path->vector pairs from the index, preferring v2
// Entries over legacy Embeddings.
func indexAllVecs(idx *EmbeddingIndex) map[string][]float64 {
	if idx == nil {
		return nil
	}
	result := make(map[string][]float64, len(idx.Entries)+len(idx.Embeddings))
	for path, vec := range idx.Embeddings {
		result[path] = vec
	}
	for path, entry := range idx.Entries {
		result[path] = entry.Vector
	}
	return result
}

// MarkNoteStale marks a note's cached embedding as stale so that it will be
// re-embedded on the next background index pass.
func (ss *SemanticSearch) MarkNoteStale(notePath string) {
	ss.bgMu.Lock()
	defer ss.bgMu.Unlock()
	if ss.index == nil {
		return
	}
	if ss.index.Entries != nil {
		if entry, ok := ss.index.Entries[notePath]; ok {
			entry.ContentHash = "" // empty hash forces re-embed
			ss.index.Entries[notePath] = entry
		}
	}
}

// ---------------------------------------------------------------------------
// Embedding generation helpers
// ---------------------------------------------------------------------------

// Shared HTTP clients for embedding requests. Both endpoints are infrequent
// (one call per stale note during a build, then quiet) but creating a fresh
// http.Client per request defeats connection pooling and leaks file
// descriptors when many notes are indexed in a row. The Ollama timeout is
// generous because local CPUs can take 30+ seconds per call on cold start;
// the OpenAI timeout is shorter because the network is the only variable.
var (
	embeddingOllamaClient = &http.Client{Timeout: 120 * time.Second}
	embeddingOpenAIClient = &http.Client{Timeout: 60 * time.Second}
)

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

	resp, err := embeddingOllamaClient.Post(url+"/api/embed", "application/json", bytes.NewReader(data))
	if err != nil {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
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

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/embeddings", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := embeddingOpenAIClient.Do(req)
	if err != nil {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
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

// truncateContent returns at most maxLen characters (runes) of the input.
func truncateContent(s string, maxLen int) string {
	r := []rune(s)
	if len(r) <= maxLen {
		return s
	}
	return string(r[:maxLen])
}

// noteSnippet returns the first ~100 characters of a note, trimmed.
func noteSnippet(content string) string {
	s := strings.TrimSpace(content)
	if r := []rune(s); len(r) > 100 {
		s = string(r[:100])
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

// needsRebuild reports whether the index needs re-building because notes
// were added, removed, changed, or the model was switched.
func (ss *SemanticSearch) needsRebuild() bool {
	if ss.index == nil {
		return true
	}
	if ss.index.Model != ss.ai.Model {
		return true
	}
	allVecs := indexAllVecs(ss.index)
	// Check for added/removed notes.
	for path := range ss.noteContents {
		if _, ok := allVecs[path]; !ok {
			return true
		}
	}
	for path := range allVecs {
		if _, ok := ss.noteContents[path]; !ok {
			return true
		}
	}
	// Check for content changes via hash.
	for path, content := range ss.noteContents {
		if ss.index.Entries != nil {
			if entry, ok := ss.index.Entries[path]; ok {
				if entry.ContentHash == "" || entry.ContentHash != contentHash(content) {
					return true
				}
			}
		}
	}
	return false
}

func (ss *SemanticSearch) startBuild() tea.Cmd {
	notes := make(map[string]string, len(ss.noteContents))
	for k, v := range ss.noteContents {
		notes[k] = v
	}
	provider := ss.ai.Provider
	model := ss.ai.Model
	ollamaURL := ss.ai.OllamaURL
	apiKey := ss.ai.APIKey
	vaultPath := ss.vaultPath

	// Snapshot existing entries that are still valid (same model, path exists,
	// content hash matches).
	existingEntries := make(map[string]EmbeddingEntry)
	if ss.index != nil && ss.index.Model == model {
		// Migrate legacy index if needed.
		migrateIndex(ss.index)
		for p, entry := range ss.index.Entries {
			content, noteExists := notes[p]
			if !noteExists {
				continue // note was deleted
			}
			// Keep the cached embedding only if the content hash matches.
			if entry.ContentHash != "" && entry.ContentHash == contentHash(content) {
				existingEntries[p] = entry
			}
		}
	}

	// Paths that still need embedding.
	var todo []string
	for path := range notes {
		if _, ok := existingEntries[path]; !ok {
			todo = append(todo, path)
		}
	}
	sort.Strings(todo)

	total := len(todo)

	return func() tea.Msg {
		if total == 0 {
			// Nothing to do — index is up to date.
			idx := &EmbeddingIndex{
				Entries: existingEntries,
				Model:   model,
				Version: 2,
			}
			if err := SaveIndex(vaultPath, idx); err != nil {
				return semanticBuildMsg{err: fmt.Errorf("saving embedding index: %w", err)}
			}
			return semanticBuildMsg{done: true, progress: 0, total: 0}
		}

		entries := make(map[string]EmbeddingEntry, len(existingEntries)+total)
		for p, e := range existingEntries {
			entries[p] = e
		}

		for i, path := range todo {
			content := truncateContent(notes[path], 2000)
			if strings.TrimSpace(content) == "" {
				content = path // fallback: embed the path itself
			}

			vec, err := getEmbedding(provider, model, ollamaURL, apiKey, content)
			if err != nil {
				// Save what we have so far so progress is not lost.
				partial := &EmbeddingIndex{
					Entries: entries,
					Model:   model,
					Version: 2,
				}
				saveErr := SaveIndex(vaultPath, partial)
				embedErr := fmt.Errorf("embedding %s: %w", path, err)
				if saveErr != nil {
					return semanticBuildMsg{err: fmt.Errorf("%w (also failed to save partial index: %v)", embedErr, saveErr)}
				}
				return semanticBuildMsg{err: embedErr}
			}
			entries[path] = EmbeddingEntry{
				Vector:      vec,
				ContentHash: contentHash(notes[path]),
			}

			_ = i // progress tracked via final message
		}

		idx := &EmbeddingIndex{
			Entries: entries,
			Model:   model,
			Version: 2,
		}
		if err := SaveIndex(vaultPath, idx); err != nil {
			return semanticBuildMsg{err: fmt.Errorf("saving embedding index: %w", err)}
		}
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
// Background indexing (non-overlay, triggered on startup / note save)
// ---------------------------------------------------------------------------

// StartBackgroundIndex returns a tea.Cmd that builds or updates the embedding
// index in the background. Unlike startBuild (which is tied to the overlay),
// this runs silently and reports progress via semanticBgIndexMsg so the status
// bar can show "Building search index: 42/500 notes...".
func (ss *SemanticSearch) StartBackgroundIndex(notes map[string]string) tea.Cmd {
	// Single critical section: acquire the bgMu once, check the flag,
	// snapshot every field we need, and DEEP-COPY the index entries.
	//
	// The previous version captured ss.index by pointer, so the goroutine
	// and a concurrent MarkNoteStale (which also touches ss.index.Entries
	// under bgMu) shared the same map. The goroutine releases bgMu before
	// migrateIndex/the iteration loop runs, so MarkNoteStale could mutate
	// the entries map while migrateIndex was iterating it — a concurrent
	// map read+write that Go panics on.
	//
	// Copying the entries here means the goroutine never touches the
	// shared ss.index after this point; it builds a fresh EmbeddingIndex
	// and SaveIndex writes that out. MarkNoteStale stays correct because
	// it operates on the live ss.index that the next build will load
	// from disk via LoadIndex.
	ss.bgMu.Lock()
	if ss.bgIndexing {
		ss.bgMu.Unlock()
		return nil // already running
	}
	ss.bgIndexing = true
	provider := ss.ai.Provider
	model := ss.ai.Model
	ollamaURL := ss.ai.OllamaURL
	apiKey := ss.ai.APIKey
	vaultPath := ss.vaultPath
	idx := copyEmbeddingIndex(ss.index)
	ss.bgMu.Unlock()

	// Load or create index. Copy the on-disk version too so the goroutine
	// never touches anything reachable from the main loop.
	if idx == nil && vaultPath != "" {
		idx = LoadIndex(vaultPath)
	}

	notesCopy := make(map[string]string, len(notes))
	for k, v := range notes {
		notesCopy[k] = v
	}

	return func() tea.Msg {
		defer func() {
			ss.bgMu.Lock()
			ss.bgIndexing = false
			ss.bgMu.Unlock()
		}()

		// Migrate legacy index.
		if idx != nil {
			migrateIndex(idx)
		}

		// Determine which notes need (re-)embedding.
		existingEntries := make(map[string]EmbeddingEntry)
		if idx != nil && idx.Model == model {
			for p, entry := range idx.Entries {
				content, noteExists := notesCopy[p]
				if !noteExists {
					continue
				}
				if entry.ContentHash != "" && entry.ContentHash == contentHash(content) {
					existingEntries[p] = entry
				}
			}
		}

		var todo []string
		for path := range notesCopy {
			if _, ok := existingEntries[path]; !ok {
				todo = append(todo, path)
			}
		}
		sort.Strings(todo)

		total := len(todo)
		if total == 0 {
			// Index is fully up to date.
			newIdx := &EmbeddingIndex{
				Entries: existingEntries,
				Model:   model,
				Version: 2,
			}
			if err := SaveIndex(vaultPath, newIdx); err != nil {
				return semanticBgIndexMsg{
					err:        fmt.Errorf("saving embedding index: %w", err),
					statusText: "Failed to save embedding index",
				}
			}
			return semanticBgIndexMsg{done: true, progress: 0, total: 0, statusText: "Embedding index up to date"}
		}

		entries := make(map[string]EmbeddingEntry, len(existingEntries)+total)
		for p, e := range existingEntries {
			entries[p] = e
		}

		for i, path := range todo {
			content := truncateContent(notesCopy[path], 2000)
			if strings.TrimSpace(content) == "" {
				content = path
			}

			vec, err := getEmbedding(provider, model, ollamaURL, apiKey, content)
			if err != nil {
				// Save partial progress.
				partial := &EmbeddingIndex{
					Entries: entries,
					Model:   model,
					Version: 2,
				}
				saveErr := SaveIndex(vaultPath, partial)
				embedErr := fmt.Errorf("embedding %s: %w", path, err)
				statusText := fmt.Sprintf("Embedding index error at %d/%d", i, total)
				if saveErr != nil {
					embedErr = fmt.Errorf("%w (also failed to save partial index: %v)", embedErr, saveErr)
					statusText += " (save failed)"
				}
				return semanticBgIndexMsg{
					err:        embedErr,
					progress:   i,
					total:      total,
					statusText: statusText,
				}
			}
			entries[path] = EmbeddingEntry{
				Vector:      vec,
				ContentHash: contentHash(notesCopy[path]),
			}

			// Save every 10 notes to avoid losing all progress on crash.
			if (i+1)%10 == 0 {
				checkpoint := &EmbeddingIndex{
					Entries: entries,
					Model:   model,
					Version: 2,
				}
				if err := SaveIndex(vaultPath, checkpoint); err != nil {
					return semanticBgIndexMsg{
						err:        fmt.Errorf("saving embedding checkpoint at %d/%d: %w", i+1, total, err),
						progress:   i + 1,
						total:      total,
						statusText: fmt.Sprintf("Failed to save embedding checkpoint at %d/%d", i+1, total),
					}
				}
			}
		}

		finalIdx := &EmbeddingIndex{
			Entries: entries,
			Model:   model,
			Version: 2,
		}
		if err := SaveIndex(vaultPath, finalIdx); err != nil {
			return semanticBgIndexMsg{
				err:        fmt.Errorf("saving final embedding index: %w", err),
				progress:   total,
				total:      total,
				statusText: "Failed to save embedding index",
			}
		}

		return semanticBgIndexMsg{
			done:       true,
			progress:   total,
			total:      total,
			statusText: fmt.Sprintf("Embedding index built: %d notes", len(entries)),
		}
	}
}

// IsBgIndexing reports whether background indexing is in progress.
func (ss *SemanticSearch) IsBgIndexing() bool {
	ss.bgMu.Lock()
	defer ss.bgMu.Unlock()
	return ss.bgIndexing
}

// ---------------------------------------------------------------------------
// Search execution
// ---------------------------------------------------------------------------

func (ss *SemanticSearch) startSearch() tea.Cmd {
	query := ss.query
	provider := ss.ai.Provider
	model := ss.ai.Model
	ollamaURL := ss.ai.OllamaURL
	apiKey := ss.ai.APIKey

	// Snapshot the index and note contents.
	idx := ss.index
	notes := ss.noteContents

	// Collect all vectors from both legacy and v2 formats.
	allVecs := indexAllVecs(idx)

	return func() tea.Msg {
		if len(allVecs) == 0 {
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
		for path, vec := range allVecs {
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
func (ss *SemanticSearch) Update(msg tea.Msg) (*SemanticSearch, tea.Cmd) {
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
				ss.query = TrimLastRune(ss.query)
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
func (ss *SemanticSearch) View() string {
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
			indexCount := len(indexAllVecs(ss.index))
			indexInfo := fmt.Sprintf("  Index: %d notes [%s]", indexCount, ss.index.Model)
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

			// Determine scroll offset so the cursor stays visible.
			scrollOff := 0
			if ss.cursor >= visH {
				scrollOff = ss.cursor - visH + 1
			}
			start := scrollOff
			end := start + visH
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
				snippet = TruncateDisplay(snippet, maxSnippetLen)

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
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(panelWidth).
		Background(mantle)

	return border.Render(b.String())
}

// visibleHeight returns the number of result entries visible in the content area.
func (ss *SemanticSearch) visibleHeight() int {
	// Each result takes ~2 lines (name + snippet). Subtract chrome.
	h := (ss.height - 16) / 2
	if h < 3 {
		h = 3
	}
	return h
}
